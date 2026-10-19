package com.beginningandroid

import android.os.Build
import android.util.Log
import kotlinx.coroutines.*
import okhttp3.*
import org.json.JSONObject
import java.io.File
import java.io.FileOutputStream
import java.util.concurrent.TimeUnit
import javax.crypto.spec.SecretKeySpec

class RelayClient(
    private val relayUrl: String,
    private val pairCode: String,
    private val listener: Listener
) {
    interface Listener {
        fun onStatusChanged(status: String)
        fun onPeerConnected(peerName: String)
        fun onClipboardReceived(text: String)
        fun onFileReceived(file: File)
        fun onVoiceStreamReceived(text: String)
    }

    private val tag = "RelayClient"
    private val client = OkHttpClient.Builder()
        .readTimeout(0, TimeUnit.MILLISECONDS)
        .pingInterval(25, TimeUnit.SECONDS)
        .build()

    private val aesKey: SecretKeySpec = Crypto.deriveKey(pairCode)
    private var webSocket: WebSocket? = null
    private var isRunning = false
    private var retryDelayMs = 1000L
    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())

    // Incoming file assembly
    private var currentReceivingFile: File? = null
    private var currentFileOutputStream: FileOutputStream? = null

    fun start() {
        if (isRunning) return
        isRunning = true
        connect()
    }

    fun stop() {
        isRunning = false
        scope.cancel()
        webSocket?.close(1000, "App closed")
        webSocket = null
        closeCurrentFile()
    }

    private fun connect() {
        if (!isRunning) return
        listener.onStatusChanged("Connecting...")

        val cleanUrl = if (relayUrl.endsWith("/")) relayUrl.dropLast(1) else relayUrl
        val roomUrl = "$cleanUrl/$pairCode"

        val request = Request.Builder()
            .url(roomUrl)
            .build()

        webSocket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(ws: WebSocket, response: Response) {
                Log.d(tag, "Connected to relay room: $pairCode")
                retryDelayMs = 1000L
                listener.onStatusChanged("Connected")

                // Send pair announcement with device model
                sendPairAnnouncement()
            }

            override fun onMessage(ws: WebSocket, text: String) {
                handleIncomingMessage(text)
            }

            override fun onClosing(ws: WebSocket, code: Int, reason: String) {
                ws.close(1000, null)
                handleDisconnect()
            }

            override fun onFailure(ws: WebSocket, t: Throwable, response: Response?) {
                Log.w(tag, "WebSocket failure: ${t.message}")
                handleDisconnect()
            }
        })
    }

    private fun handleDisconnect() {
        listener.onStatusChanged("Disconnected")
        if (isRunning) {
            scope.launch {
                delay(retryDelayMs)
                retryDelayMs = (retryDelayMs * 2).coerceAtMost(30000L)
                Log.d(tag, "Attempting reconnect in ${retryDelayMs}ms")
                connect()
            }
        }
    }

    private fun handleIncomingMessage(rawJson: String) {
        try {
            val json = JSONObject(rawJson)
            val type = json.optString("type")
            val payload = json.optString("payload")
            val nonce = json.optString("nonce")

            when (type) {
                "pair" -> {
                    val decrypted = Crypto.decryptString(aesKey, payload, nonce)
                    val info = JSONObject(decrypted)
                    val peerName = info.optString("device_name", "Connected PC")
                    listener.onPeerConnected(peerName)
                }
                "clipboard" -> {
                    val text = Crypto.decryptString(aesKey, payload, nonce)
                    listener.onClipboardReceived(text)
                }
                "voice_stream" -> {
                    val spokenText = Crypto.decryptString(aesKey, payload, nonce)
                    listener.onVoiceStreamReceived(spokenText)
                }
                "file_start" -> {
                    val decrypted = Crypto.decryptString(aesKey, payload, nonce)
                    val meta = JSONObject(decrypted)
                    val fileName = meta.optString("name", "received_file")
                    setupIncomingFile(fileName)
                }
                "file_chunk" -> {
                    val chunkBytes = Crypto.decrypt(aesKey, payload, nonce)
                    currentFileOutputStream?.write(chunkBytes)
                }
                "file_end" -> {
                    closeCurrentFile()?.let { file ->
                        listener.onFileReceived(file)
                    }
                }
                "ping" -> {
                    sendRaw("pong", "", "")
                }
            }
        } catch (e: Exception) {
            Log.e(tag, "Error handling incoming message", e)
        }
    }

    private fun sendPairAnnouncement() {
        val payloadObj = JSONObject().apply {
            put("device_name", "${Build.MANUFACTURER} ${Build.MODEL}")
            put("platform", "android")
        }
        sendEncrypted("pair", payloadObj.toString().toByteArray())
    }

    fun sendClipboard(text: String) {
        sendEncrypted("clipboard", text.toByteArray(Charsets.UTF_8))
    }

    fun sendVoiceStream(text: String) {
        sendEncrypted("voice_stream", text.toByteArray(Charsets.UTF_8))
    }

    fun sendFile(file: File) {
        streamFile(file, isScreenshot = false)
    }

    fun sendScreenshot(file: File) {
        streamFile(file, isScreenshot = true)
    }

    private fun streamFile(file: File, isScreenshot: Boolean) {
        scope.launch {
            try {
                // 1. File start meta
                val msgType = if (isScreenshot) "screenshot" else "file_start"
                val metaObj = JSONObject().apply {
                    put("name", file.name)
                    put("size", file.length())
                    if (isScreenshot) {
                        put("type", "screenshot")
                    }
                }
                sendEncrypted(msgType, metaObj.toString().toByteArray())

                // 2. File chunks (64 KB chunk size for fast streaming)
                val buffer = ByteArray(64 * 1024)
                file.inputStream().use { input ->
                    var bytesRead: Int
                    while (input.read(buffer).also { bytesRead = it } != -1) {
                        val chunk = if (bytesRead == buffer.size) buffer else buffer.copyOf(bytesRead)
                        sendEncrypted("file_chunk", chunk)
                        delay(10) // Prevents socket buffer starvation
                    }
                }

                // 3. File end
                sendEncrypted("file_end", ByteArray(0))
                Log.i(tag, "Finished streaming ${if (isScreenshot) "screenshot" else "file"}: ${file.name}")
            } catch (e: Exception) {
                Log.e(tag, "Failed to stream file", e)
            }
        }
    }

    private fun sendEncrypted(type: String, dataBytes: ByteArray) {
        try {
            val (payloadBase64, nonceBase64) = Crypto.encrypt(aesKey, dataBytes)
            sendRaw(type, payloadBase64, nonceBase64)
        } catch (e: Exception) {
            Log.e(tag, "Encryption failed for $type", e)
        }
    }

    private fun sendRaw(type: String, payload: String, nonce: String) {
        val json = JSONObject().apply {
            put("type", type)
            put("payload", payload)
            put("nonce", nonce)
        }
        webSocket?.send(json.toString())
    }

    private fun setupIncomingFile(name: String) {
        closeCurrentFile()
        try {
            val downloadsDir = File(
                android.os.Environment.getExternalStoragePublicDirectory(android.os.Environment.DIRECTORY_DOWNLOADS),
                "BeginningAndroid"
            )
            if (!downloadsDir.exists()) {
                downloadsDir.mkdirs()
            }
            val targetFile = File(downloadsDir, name)
            currentReceivingFile = targetFile
            currentFileOutputStream = FileOutputStream(targetFile)
        } catch (e: Exception) {
            Log.e(tag, "Failed to initialize incoming file", e)
        }
    }

    private fun closeCurrentFile(): File? {
        val file = currentReceivingFile
        try {
            currentFileOutputStream?.flush()
            currentFileOutputStream?.close()
        } catch (_: Exception) {}
        currentFileOutputStream = null
        currentReceivingFile = null
        return file
    }
}
