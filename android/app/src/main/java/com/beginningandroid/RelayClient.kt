package com.beginningandroid

import android.os.Build
import android.util.Log
import kotlinx.coroutines.*
import okhttp3.*
import org.json.JSONObject
import java.io.File
import java.io.FileOutputStream
import java.util.Base64
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong

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

    private val pairCodeHash: ByteArray = Crypto.pairCodeHash(pairCode)
    private val pairSeed: ByteArray = Crypto.derivePairSeed(pairCode)
    private val x25519KeyPair: Pair<ByteArray, ByteArray> = Crypto.generateX25519KeyPair()

    @Volatile
    private var sessionKey: ByteArray? = null
    private val outgoingSeq = AtomicLong(1L)
    private var lastIncomingSeq = 0L

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

    private fun getActiveKey(): ByteArray {
        return sessionKey ?: pairSeed
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
            override fun onOpen(webSocket: WebSocket, response: Response) {
                Log.d(tag, "Connected to relay room: $pairCode")
                retryDelayMs = 1000L
                listener.onStatusChanged("Connected")

                // Step 1: Send X25519 Public Key Exchange Announcement
                sendKeyExchange()
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                handleIncomingMessage(text)
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                webSocket.close(1000, null)
                handleDisconnect()
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                Log.w(tag, "WebSocket failure: ${t.message}")
                handleDisconnect()
            }
        })
    }

    private fun handleDisconnect() {
        listener.onStatusChanged("Disconnected")
        sessionKey = null
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

            // 1. Handle Key Exchange (unencrypted envelope carrying public key)
            if (type == "key_exchange") {
                val peerPubBase64 = json.optString("public_key")
                if (peerPubBase64.isNotBlank()) {
                    val peerPub = Base64.getDecoder().decode(peerPubBase64)
                    val sharedSecret = Crypto.computeSharedSecret(x25519KeyPair.first, peerPub)
                    sessionKey = Crypto.deriveSessionKey(sharedSecret, pairCodeHash)
                    val peerName = json.optString("device_name", "Connected PC")
                    Log.i(tag, "E2EE Session Established via X25519 + HKDF-SHA256 with $peerName")
                    listener.onPeerConnected(peerName)
                }
                return
            }

            // 2. Replay Protection: Check sequential message counter
            val seq = json.optLong("seq", 0L)
            if (seq > 0L && seq <= lastIncomingSeq) {
                Log.w(tag, "Rejected replayed message with seq $seq <= $lastIncomingSeq")
                return
            }
            if (seq > lastIncomingSeq) {
                lastIncomingSeq = seq
            }

            val payload = json.optString("payload")
            val nonce = json.optString("nonce")
            val activeKey = getActiveKey()

            when (type) {
                "pair" -> {
                    val decrypted = Crypto.decryptString(activeKey, payload, nonce)
                    val info = JSONObject(decrypted)
                    val peerName = info.optString("device_name", "Connected PC")
                    listener.onPeerConnected(peerName)
                }
                "clipboard" -> {
                    val text = Crypto.decryptString(activeKey, payload, nonce)
                    listener.onClipboardReceived(text)
                }
                "voice_stream" -> {
                    val spokenText = Crypto.decryptString(activeKey, payload, nonce)
                    listener.onVoiceStreamReceived(spokenText)
                }
                "file_start", "screenshot" -> {
                    val decrypted = Crypto.decryptString(activeKey, payload, nonce)
                    val meta = JSONObject(decrypted)
                    val fileName = meta.optString("name", "received_file")
                    setupIncomingFile(fileName)
                }
                "file_chunk" -> {
                    val chunkBytes = Crypto.decrypt(activeKey, payload, nonce)
                    currentFileOutputStream?.write(chunkBytes)
                }
                "file_end" -> {
                    closeCurrentFile()?.let { file ->
                        listener.onFileReceived(file)
                    }
                }
                "ping" -> {
                    sendRaw("pong", 0L, "", "")
                }
            }
        } catch (e: Exception) {
            Log.e(tag, "Error handling incoming message", e)
        }
    }

    private fun sendKeyExchange() {
        val pubBase64 = Base64.getEncoder().encodeToString(x25519KeyPair.second)
        val json = JSONObject().apply {
            put("type", "key_exchange")
            put("public_key", pubBase64)
            put("device_name", "${Build.MANUFACTURER} ${Build.MODEL}")
            put("platform", "android")
        }
        webSocket?.send(json.toString())
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

                // 2. File chunks: 64 KB each, encrypted separately, sequential counter
                val buffer = ByteArray(64 * 1024)
                file.inputStream().use { input ->
                    var bytesRead: Int
                    var chunkIndex = 0
                    while (input.read(buffer).also { bytesRead = it } != -1) {
                        val chunk = if (bytesRead == buffer.size) buffer else buffer.copyOf(bytesRead)
                        sendEncryptedChunk(chunk, chunkIndex++)
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
            val seq = outgoingSeq.getAndIncrement()
            val (payloadBase64, nonceBase64) = Crypto.encrypt(getActiveKey(), dataBytes)
            sendRaw(type, seq, payloadBase64, nonceBase64)
        } catch (e: Exception) {
            Log.e(tag, "Encryption failed for $type", e)
        }
    }

    private fun sendEncryptedChunk(chunkBytes: ByteArray, chunkIndex: Int) {
        try {
            val seq = outgoingSeq.getAndIncrement()
            val (payloadBase64, nonceBase64) = Crypto.encrypt(getActiveKey(), chunkBytes)
            val json = JSONObject().apply {
                put("type", "file_chunk")
                put("seq", seq)
                put("chunk_index", chunkIndex)
                put("payload", payloadBase64)
                put("nonce", nonceBase64)
            }
            webSocket?.send(json.toString())
        } catch (e: Exception) {
            Log.e(tag, "Failed to send chunk $chunkIndex", e)
        }
    }

    private fun sendRaw(type: String, seq: Long, payload: String, nonce: String) {
        val json = JSONObject().apply {
            put("type", type)
            if (seq > 0) {
                put("seq", seq)
            }
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
