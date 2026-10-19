package com.beginningandroid

import android.app.*
import android.content.*
import android.database.ContentObserver
import android.net.Uri
import android.os.*
import android.provider.MediaStore
import android.util.Log
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.*
import java.io.File

class SyncService : Service(), RelayClient.Listener {

    companion object {
        const val CHANNEL_ID = "beginning_android_sync_channel"
        const val NOTIFICATION_ID = 1001

        const val ACTION_START = "com.beginningandroid.START"
        const val ACTION_STOP = "com.beginningandroid.STOP"
        const val ACTION_SEND_CLIPBOARD = "com.beginningandroid.SEND_CLIPBOARD"
        const val ACTION_SEND_VOICE = "com.beginningandroid.SEND_VOICE"
        const val ACTION_SEND_FILE = "com.beginningandroid.SEND_FILE"

        const val EXTRA_TEXT = "extra_text"
        const val EXTRA_FILE_PATH = "extra_file_path"

        // Broadcast actions for UI
        const val BROADCAST_STATUS = "com.beginningandroid.STATUS_UPDATE"
        const val BROADCAST_CLIPBOARD = "com.beginningandroid.CLIPBOARD_UPDATE"
        const val BROADCAST_PEER = "com.beginningandroid.PEER_UPDATE"
        const val BROADCAST_VOICE = "com.beginningandroid.VOICE_UPDATE"
    }

    private val tag = "SyncService"
    private val binder = LocalBinder()
    private val scope = CoroutineScope(Dispatchers.Main + SupervisorJob())

    private var relayClient: RelayClient? = null
    private var clipboardManager: ClipboardManager? = null
    private var screenshotObserver: ContentObserver? = null

    @Volatile
    private var isInternalClipboardUpdate = false
    private var clipboardDebounceJob: Job? = null
    private var lastSentClipboardText: String? = null
    private var lastUploadedScreenshotPath: String? = null

    inner class LocalBinder : Binder() {
        fun getService(): SyncService = this@SyncService
    }

    override fun onBind(intent: Intent?): IBinder = binder

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        startForeground(NOTIFICATION_ID, buildNotification("Initializing sync..."))

        initClipboardListener()
        initScreenshotObserver()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val action = intent?.action ?: ACTION_START

        when (action) {
            ACTION_START -> {
                val prefs = getSharedPreferences("beginning_android_prefs", Context.MODE_PRIVATE)
                val pairCode = prefs.getString("pair_code", null)
                val relayUrl = prefs.getString("relay_url", getString(R.string.default_relay_url))
                    ?: getString(R.string.default_relay_url)

                if (!pairCode.isNullOrBlank()) {
                    relayClient?.stop()
                    relayClient = RelayClient(relayUrl, pairCode, this).also { it.start() }
                    updateNotification("Beginning Android running - Live Sync Active")
                }
            }
            ACTION_STOP -> {
                relayClient?.stop()
                relayClient = null
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
            }
            ACTION_SEND_CLIPBOARD -> {
                val text = intent?.getStringExtra(EXTRA_TEXT)
                if (!text.isNullOrBlank()) {
                    relayClient?.sendClipboard(text)
                }
            }
            ACTION_SEND_VOICE -> {
                val text = intent?.getStringExtra(EXTRA_TEXT)
                if (!text.isNullOrBlank()) {
                    relayClient?.sendVoiceStream(text)
                }
            }
            ACTION_SEND_FILE -> {
                val path = intent?.getStringExtra(EXTRA_FILE_PATH)
                if (!path.isNullOrBlank()) {
                    val file = File(path)
                    if (file.exists()) {
                        relayClient?.sendFile(file)
                    }
                }
            }
        }

        return START_STICKY
    }

    // --- Clipboard Sync Logic ---
    private fun initClipboardListener() {
        clipboardManager = getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager
        clipboardManager?.addPrimaryClipChangedListener {
            if (isInternalClipboardUpdate) {
                // Suppress loopback
                return@addPrimaryClipChangedListener
            }

            clipboardDebounceJob?.cancel()
            clipboardDebounceJob = scope.launch {
                delay(300) // 300ms debounce
                val clipData = clipboardManager?.primaryClip
                if (clipData != null && clipData.itemCount > 0) {
                    val text = clipData.getItemAt(0).coerceToText(this@SyncService).toString()
                    if (text.isNotBlank() && text != lastSentClipboardText) {
                        lastSentClipboardText = text
                        relayClient?.sendClipboard(text)
                        broadcastToUi(BROADCAST_CLIPBOARD, text)
                        Log.d(tag, "Outgoing clipboard synced (debounced 300ms)")
                    }
                }
            }
        }
    }

    fun applyIncomingClipboard(text: String) {
        scope.launch(Dispatchers.Main) {
            try {
                isInternalClipboardUpdate = true
                lastSentClipboardText = text
                val clip = ClipData.newPlainText("BeginningAndroid", text)
                clipboardManager?.setPrimaryClip(clip)
                broadcastToUi(BROADCAST_CLIPBOARD, text)
                Log.d(tag, "Incoming clipboard set successfully")
            } finally {
                // Reset loopback flag after slight delay
                delay(500)
                isInternalClipboardUpdate = false
            }
        }
    }

    // --- Screenshot Auto-Detect Logic ---
    private fun initScreenshotObserver() {
        screenshotObserver = object : ContentObserver(Handler(Looper.getMainLooper())) {
            override fun onChange(selfChange: Boolean, uri: Uri?) {
                super.onChange(selfChange, uri)
                checkLatestScreenshot()
            }
        }

        contentResolver.registerContentObserver(
            MediaStore.Images.Media.EXTERNAL_CONTENT_URI,
            true,
            screenshotObserver!!
        )
    }

    private fun checkLatestScreenshot() {
        scope.launch(Dispatchers.IO) {
            try {
                val projection = arrayOf(
                    MediaStore.Images.Media._ID,
                    MediaStore.Images.Media.DATA,
                    MediaStore.Images.Media.DISPLAY_NAME,
                    MediaStore.Images.Media.DATE_ADDED
                )
                val sortOrder = "${MediaStore.Images.Media.DATE_ADDED} DESC"

                contentResolver.query(
                    MediaStore.Images.Media.EXTERNAL_CONTENT_URI,
                    projection,
                    null,
                    null,
                    sortOrder
                )?.use { cursor ->
                    if (cursor.moveToFirst()) {
                        val pathCol = cursor.getColumnIndex(MediaStore.Images.Media.DATA)
                        val nameCol = cursor.getColumnIndex(MediaStore.Images.Media.DISPLAY_NAME)
                        val dateCol = cursor.getColumnIndex(MediaStore.Images.Media.DATE_ADDED)

                        val path = if (pathCol != -1) cursor.getString(pathCol) else ""
                        val name = if (nameCol != -1) cursor.getString(nameCol) else ""
                        val dateAddedSec = if (dateCol != -1) cursor.getLong(dateCol) else 0L

                        val nowSec = System.currentTimeMillis() / 1000
                        val isRecent = (nowSec - dateAddedSec) <= 15

                        val isScreenshot = path.contains("Screenshots", ignoreCase = true) ||
                                name.contains("screenshot", ignoreCase = true)

                        if (isRecent && isScreenshot && path != lastUploadedScreenshotPath) {
                            lastUploadedScreenshotPath = path
                            val file = File(path)
                            if (file.exists() && file.length() > 0) {
                                Log.i(tag, "Auto-detected new screenshot: $path, uploading to PC...")
                                relayClient?.sendScreenshot(file)
                            }
                        }
                    }
                }
            } catch (e: Exception) {
                Log.e(tag, "Error inspecting screenshots", e)
            }
        }
    }

    // --- RelayClient Callbacks ---
    override fun onStatusChanged(status: String) {
        broadcastToUi(BROADCAST_STATUS, status)
        updateNotification("Beginning Android - $status")
    }

    override fun onPeerConnected(peerName: String) {
        broadcastToUi(BROADCAST_PEER, peerName)
        updateNotification("Beginning Android - Connected to $peerName")
    }

    override fun onClipboardReceived(text: String) {
        applyIncomingClipboard(text)
    }

    override fun onFileReceived(file: File) {
        scope.launch(Dispatchers.Main) {
            updateNotification("File received: ${file.name}")
        }
    }

    override fun onVoiceStreamReceived(text: String) {
        broadcastToUi(BROADCAST_VOICE, text)
    }

    fun sendVoiceStream(text: String) {
        relayClient?.sendVoiceStream(text)
    }

    fun sendFile(file: File) {
        relayClient?.sendFile(file)
    }

    private fun broadcastToUi(action: String, data: String) {
        val intent = Intent(action).apply {
            putExtra(EXTRA_TEXT, data)
            setPackage(packageName)
        }
        sendBroadcast(intent)
    }

    // --- Notification Helper ---
    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                getString(R.string.notification_channel_name),
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "Live sync service notification"
                setShowBadge(false)
            }
            val manager = getSystemService(NotificationManager::class.java)
            manager?.createNotificationChannel(channel)
        }
    }

    private fun buildNotification(text: String): Notification {
        val pendingIntent = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_launcher)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    private fun updateNotification(text: String) {
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as? NotificationManager
        manager?.notify(NOTIFICATION_ID, buildNotification(text))
    }

    override fun onDestroy() {
        super.onDestroy()
        screenshotObserver?.let { contentResolver.unregisterContentObserver(it) }
        relayClient?.stop()
        scope.cancel()
    }
}
