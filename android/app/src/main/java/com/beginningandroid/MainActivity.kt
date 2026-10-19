package com.beginningandroid

import android.Manifest
import android.content.*
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.IBinder
import android.speech.RecognitionListener
import android.speech.RecognizerIntent
import android.speech.SpeechRecognizer
import android.view.View
import android.widget.*
import androidx.appcompat.app.AppCompatActivity
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import kotlinx.coroutines.*
import java.io.File
import java.io.FileOutputStream
import java.util.*

class MainActivity : AppCompatActivity() {

    private val wordList = listOf(
        "amber", "breeze", "cedar", "drift", "echo", "frost",
        "glade", "haven", "iris", "jasper", "kinetic", "lunar",
        "meadow", "nexus", "orbit", "prism", "quartz", "river",
        "solar", "timber", "ultra", "vivid", "willow", "zenith"
    )

    private var syncService: SyncService? = null
    private var isBound = false

    private lateinit var screenPairing: View
    private lateinit var screenMain: View
    private lateinit var badgeStatus: TextView
    private lateinit var etPairCode: EditText
    private lateinit var etServerUrl: EditText
    private lateinit var btnAutoDetect: Button
    private lateinit var tvDiscoveryStatus: TextView
    private lateinit var btnGenerateCode: Button
    private lateinit var btnConnect: Button

    private lateinit var tvConnectedPeer: TextView
    private lateinit var tvActivePairCode: TextView
    private lateinit var tvLastClipboard: TextView
    private lateinit var btnCopyClipboard: Button
    private lateinit var tvVoiceTranscription: TextView
    private lateinit var btnVoiceTyping: Button
    private lateinit var btnDisconnect: Button

    private val discoveryManager by lazy { DiscoveryManager(this) }
    private val coroutineScope = CoroutineScope(Dispatchers.Main + SupervisorJob())

    private var speechRecognizer: SpeechRecognizer? = null
    private var isListening = false

    private val serviceConnection = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName?, service: IBinder?) {
            val binder = service as? SyncService.LocalBinder
            syncService = binder?.getService()
            isBound = true
        }

        override fun onServiceDisconnected(name: ComponentName?) {
            syncService = null
            isBound = false
        }
    }

    private val updateReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            when (intent?.action) {
                SyncService.BROADCAST_STATUS -> {
                    val status = intent.getStringExtra(SyncService.EXTRA_TEXT) ?: "Disconnected"
                    badgeStatus.text = status
                    if (status.equals("Connected", ignoreCase = true)) {
                        badgeStatus.setBackgroundColor(0xFF0D9488.toInt())
                        badgeStatus.setTextColor(0xFFFFFFFF.toInt())
                    } else {
                        badgeStatus.setBackgroundColor(0xFF334155.toInt())
                        badgeStatus.setTextColor(0xFF94A3B8.toInt())
                    }
                }
                SyncService.BROADCAST_PEER -> {
                    val peer = intent.getStringExtra(SyncService.EXTRA_TEXT) ?: "PC"
                    tvConnectedPeer.text = getString(R.string.connected_to_pc, peer)
                }
                SyncService.BROADCAST_CLIPBOARD -> {
                    val text = intent.getStringExtra(SyncService.EXTRA_TEXT) ?: ""
                    tvLastClipboard.text = text
                }
                SyncService.BROADCAST_VOICE -> {
                    val text = intent.getStringExtra(SyncService.EXTRA_TEXT) ?: ""
                    tvVoiceTranscription.text = text
                }
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        bindViews()
        requestAppPermissions()

        val prefs = getSharedPreferences("beginning_android_prefs", Context.MODE_PRIVATE)
        val savedPairCode = prefs.getString("pair_code", null)

        if (!savedPairCode.isNullOrBlank()) {
            showMainScreen(savedPairCode)
            startSyncService()
        } else {
            showPairingScreen()
        }

        handleShareIntent(intent)
    }

    override fun onNewIntent(intent: Intent?) {
        super.onNewIntent(intent)
        intent?.let { handleShareIntent(it) }
    }

    private fun bindViews() {
        screenPairing = findViewById(R.id.screenPairing)
        screenMain = findViewById(R.id.screenMain)
        badgeStatus = findViewById(R.id.badgeStatus)

        etPairCode = findViewById(R.id.etPairCode)
        etServerUrl = findViewById(R.id.etServerUrl)
        btnAutoDetect = findViewById(R.id.btnAutoDetect)
        tvDiscoveryStatus = findViewById(R.id.tvDiscoveryStatus)
        btnGenerateCode = findViewById(R.id.btnGenerateCode)
        btnConnect = findViewById(R.id.btnConnect)

        tvConnectedPeer = findViewById(R.id.tvConnectedPeer)
        tvActivePairCode = findViewById(R.id.tvActivePairCode)
        tvLastClipboard = findViewById(R.id.tvLastClipboard)
        btnCopyClipboard = findViewById(R.id.btnCopyClipboard)
        tvVoiceTranscription = findViewById(R.id.tvVoiceTranscription)
        btnVoiceTyping = findViewById(R.id.btnVoiceTyping)
        btnDisconnect = findViewById(R.id.btnDisconnect)

        btnAutoDetect.setOnClickListener {
            triggerAutoDiscovery()
        }

        btnGenerateCode.setOnClickListener {
            val code = generateRandomPairCode()
            etPairCode.setText(code)
        }

        btnConnect.setOnClickListener {
            val code = etPairCode.text.toString().trim().lowercase()
            val url = etServerUrl.text.toString().trim()

            if (code.isBlank()) {
                Toast.makeText(this, "Please enter or generate a pair code", Toast.LENGTH_SHORT).show()
                return@setOnClickListener
            }

            getSharedPreferences("beginning_android_prefs", Context.MODE_PRIVATE).edit()
                .putString("pair_code", code)
                .putString("relay_url", url)
                .apply()

            showMainScreen(code)
            startSyncService()
        }

        btnCopyClipboard.setOnClickListener {
            val text = tvLastClipboard.text.toString()
            if (text.isNotBlank()) {
                val clipMgr = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                clipMgr.setPrimaryClip(ClipData.newPlainText("BeginningAndroid", text))
                Toast.makeText(this, R.string.toast_clipboard_copied, Toast.LENGTH_SHORT).show()
            }
        }

        btnVoiceTyping.setOnClickListener {
            toggleVoiceTyping()
        }

        btnDisconnect.setOnClickListener {
            getSharedPreferences("beginning_android_prefs", Context.MODE_PRIVATE).edit()
                .remove("pair_code")
                .apply()

            stopSyncService()
            showPairingScreen()
        }
    }

    private fun showPairingScreen() {
        screenPairing.visibility = View.VISIBLE
        screenMain.visibility = View.GONE
        badgeStatus.text = getString(R.string.status_disconnected)
        badgeStatus.setBackgroundColor(0xFF334155.toInt())
        triggerAutoDiscovery()
    }

    private fun triggerAutoDiscovery() {
        tvDiscoveryStatus.text = "🔍 Searching for PC on mobile hotspot / Wi-Fi..."
        tvDiscoveryStatus.setTextColor(0xFF38BDF8.toInt())
        coroutineScope.launch {
            val pc = discoveryManager.discoverPC(2500)
            if (pc != null) {
                tvDiscoveryStatus.text = "✅ Found PC: ${pc.pcName} (${pc.ip})"
                tvDiscoveryStatus.setTextColor(0xFF10B981.toInt())
                etServerUrl.setText(pc.wsUrl)
                if (pc.pairCode.isNotBlank() && etPairCode.text.isBlank()) {
                    etPairCode.setText(pc.pairCode)
                }
                Toast.makeText(this@MainActivity, "Connected to ${pc.pcName}!", Toast.LENGTH_SHORT).show()
            } else {
                tvDiscoveryStatus.text = "Hotspot connected. Tap Auto-Detect when PC client is open."
                tvDiscoveryStatus.setTextColor(0xFF94A3B8.toInt())
            }
        }
    }

    private fun showMainScreen(pairCode: String) {
        screenPairing.visibility = View.GONE
        screenMain.visibility = View.VISIBLE
        tvActivePairCode.text = "Pair Code: $pairCode"
    }

    private fun generateRandomPairCode(): String {
        val random = Random()
        val words = mutableListOf<String>()
        repeat(6) {
            words.add(wordList[random.nextInt(wordList.size)])
        }
        return words.joinToString("-")
    }

    // --- Live Voice Typing (Prompt 3) ---
    private fun toggleVoiceTyping() {
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.RECORD_AUDIO)
            != PackageManager.PERMISSION_GRANTED
        ) {
            ActivityCompat.requestPermissions(this, arrayOf(Manifest.permission.RECORD_AUDIO), 101)
            return
        }

        if (isListening) {
            stopListening()
        } else {
            startListening()
        }
    }

    private fun startListening() {
        if (!SpeechRecognizer.isRecognitionAvailable(this)) {
            Toast.makeText(this, "Speech recognition unavailable on this device", Toast.LENGTH_SHORT).show()
            return
        }

        speechRecognizer = SpeechRecognizer.createSpeechRecognizer(this)
        val intent = Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH).apply {
            putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM)
            putExtra(RecognizerIntent.EXTRA_PARTIAL_RESULTS, true)
            putExtra(RecognizerIntent.EXTRA_LANGUAGE, Locale.getDefault())
        }

        speechRecognizer?.setRecognitionListener(object : RecognitionListener {
            override fun onReadyForSpeech(params: Bundle?) {
                tvVoiceTranscription.text = getString(R.string.listening_speech)
            }

            override fun onBeginningOfSpeech() {}
            override fun onRmsChanged(rmsdB: Float) {}
            override fun onBufferReceived(buffer: ByteArray?) {}
            override fun onEndOfSpeech() {
                isListening = false
                btnVoiceTyping.text = getString(R.string.btn_voice_typing)
            }

            override fun onError(error: Int) {
                isListening = false
                btnVoiceTyping.text = getString(R.string.btn_voice_typing)
            }

            override fun onResults(results: Bundle?) {
                val matches = results?.getStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION)
                val spoken = matches?.firstOrNull() ?: ""
                if (spoken.isNotBlank()) {
                    tvVoiceTranscription.text = spoken
                    syncService?.sendVoiceStream(spoken)
                }
            }

            override fun onPartialResults(partialResults: Bundle?) {
                val partial = partialResults?.getStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION)
                val spoken = partial?.firstOrNull() ?: ""
                if (spoken.isNotBlank()) {
                    tvVoiceTranscription.text = spoken
                    // Stream partial words live to PC!
                    syncService?.sendVoiceStream(spoken)
                }
            }

            override fun onEvent(eventType: Int, params: Bundle?) {}
        })

        speechRecognizer?.startListening(intent)
        isListening = true
        btnVoiceTyping.text = "⏹️ Stop Voice Typing"
    }

    private fun stopListening() {
        speechRecognizer?.stopListening()
        speechRecognizer?.destroy()
        speechRecognizer = null
        isListening = false
        btnVoiceTyping.text = getString(R.string.btn_voice_typing)
    }

    // --- Native Share Intent Handling ---
    private fun handleShareIntent(intent: Intent) {
        val action = intent.action
        val type = intent.type

        if (Intent.ACTION_SEND == action) {
            val uri = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                intent.getParcelableExtra(Intent.EXTRA_STREAM, Uri::class.java)
            } else {
                @Suppress("DEPRECATION")
                intent.getParcelableExtra(Intent.EXTRA_STREAM)
            }

            if (uri != null) {
                val fileName = getFileNameFromUri(uri)
                Toast.makeText(this, "Sending $fileName to connected PC...", Toast.LENGTH_SHORT).show()
                copyUriToFileAndSend(uri, fileName)
            } else {
                val text = intent.getStringExtra(Intent.EXTRA_TEXT)
                if (!text.isNullOrBlank()) {
                    syncService?.sendClipboard(text)
                    Toast.makeText(this, "Text synced to PC clipboard!", Toast.LENGTH_SHORT).show()
                }
            }
        } else if (Intent.ACTION_SEND_MULTIPLE == action) {
            val uris = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                intent.getParcelableArrayListExtra(Intent.EXTRA_STREAM, Uri::class.java)
            } else {
                @Suppress("DEPRECATION")
                intent.getParcelableArrayListExtra(Intent.EXTRA_STREAM)
            }

            if (!uris.isNullOrEmpty()) {
                Toast.makeText(this, "Sending ${uris.size} files to connected PC...", Toast.LENGTH_SHORT).show()
                for (uri in uris) {
                    val fileName = getFileNameFromUri(uri)
                    copyUriToFileAndSend(uri, fileName)
                }
            }
        }
    }

    private fun getFileNameFromUri(uri: Uri): String {
        var name = "shared_file_${System.currentTimeMillis()}"
        if (uri.scheme == "content") {
            try {
                contentResolver.query(uri, null, null, null, null)?.use { cursor ->
                    val nameIndex = cursor.getColumnIndex(android.provider.OpenableColumns.DISPLAY_NAME)
                    if (nameIndex != -1 && cursor.moveToFirst()) {
                        val found = cursor.getString(nameIndex)
                        if (!found.isNullOrBlank()) {
                            name = found
                        }
                    }
                }
            } catch (e: Exception) {
                e.printStackTrace()
            }
        } else if (uri.scheme == "file") {
            uri.path?.let { p ->
                val fName = File(p).name
                if (fName.isNotBlank()) name = fName
            }
        }
        return name
    }

    private fun copyUriToFileAndSend(uri: Uri, fileName: String) {
        Thread {
            try {
                val tempFile = File(cacheDir, fileName)
                contentResolver.openInputStream(uri)?.use { input ->
                    FileOutputStream(tempFile).use { output ->
                        input.copyTo(output)
                    }
                }
                syncService?.sendFile(tempFile)
            } catch (e: Exception) {
                e.printStackTrace()
            }
        }.start()
    }

    private fun startSyncService() {
        val serviceIntent = Intent(this, SyncService::class.java).apply {
            action = SyncService.ACTION_START
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(serviceIntent)
        } else {
            startService(serviceIntent)
        }
        bindService(serviceIntent, serviceConnection, Context.BIND_AUTO_CREATE)
    }

    private fun stopSyncService() {
        if (isBound) {
            unbindService(serviceConnection)
            isBound = false
        }
        val serviceIntent = Intent(this, SyncService::class.java).apply {
            action = SyncService.ACTION_STOP
        }
        startService(serviceIntent)
    }

    private fun requestAppPermissions() {
        val needed = mutableListOf<String>()

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS)
                != PackageManager.PERMISSION_GRANTED
            ) {
                needed.add(Manifest.permission.POST_NOTIFICATIONS)
            }
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.READ_MEDIA_IMAGES)
                != PackageManager.PERMISSION_GRANTED
            ) {
                needed.add(Manifest.permission.READ_MEDIA_IMAGES)
            }
        } else {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.READ_EXTERNAL_STORAGE)
                != PackageManager.PERMISSION_GRANTED
            ) {
                needed.add(Manifest.permission.READ_EXTERNAL_STORAGE)
            }
        }

        if (needed.isNotEmpty()) {
            ActivityCompat.requestPermissions(this, needed.toTypedArray(), 100)
        }
    }

    override fun onStart() {
        super.onStart()
        val filter = IntentFilter().apply {
            addAction(SyncService.BROADCAST_STATUS)
            addAction(SyncService.BROADCAST_CLIPBOARD)
            addAction(SyncService.BROADCAST_PEER)
            addAction(SyncService.BROADCAST_VOICE)
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            registerReceiver(updateReceiver, filter, Context.RECEIVER_NOT_EXPORTED)
        } else {
            registerReceiver(updateReceiver, filter)
        }

        val prefs = getSharedPreferences("beginning_android_prefs", Context.MODE_PRIVATE)
        if (prefs.getString("pair_code", null) != null) {
            bindService(
                Intent(this, SyncService::class.java),
                serviceConnection,
                Context.BIND_AUTO_CREATE
            )
        }
    }

    override fun onStop() {
        super.onStop()
        try {
            unregisterReceiver(updateReceiver)
        } catch (_: Exception) {}

        if (isBound) {
            unbindService(serviceConnection)
            isBound = false
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        coroutineScope.cancel()
        stopListening()
    }
}
