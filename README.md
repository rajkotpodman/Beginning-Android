# Beginning Android

A ultra-minimal, high-performance, and end-to-end encrypted cross-platform clipboard synchronizer, screenshot auto-transfer utility, and direct file-sharing tool connecting **Android**, **Windows**, and **Linux**.

---

## ⚡ Core Philosophy & Constraints

- **Smallest Possible App Size**: Native Go single binary for PC (<15MB, zero external runtimes); lean Kotlin APK for Android without bloat.
- **Shortest & Cleanest Code**: Minimalist architecture, no unnecessary frameworks, zero fluff.
- **Fastest Performance**: Low-latency WebSocket streaming, instant 500ms debounced live clipboard synchronization, non-blocking I/O.
- **Zero Cloud Storage & Privacy First**: Content is never stored in the cloud. The relay server is a blind forwarder.
- **Military-Grade Security**: End-to-End Encryption (E2EE) using Curve25519 (X25519) key exchange and AES-256-GCM payload encryption.

---

## 🚀 Features

### 1. Live Clipboard Sync
- Type or copy text on Android → instantly reflects in PC clipboard and UI.
- Type or copy text on PC → instantly reflects in Android clipboard and notification.
- Supports all Unicode scripts, emojis, and languages.
- Fast debounced real-time streaming (500ms) with loopback echo suppression.

### 2. Screenshot Auto-Transfer
- Android background observer detects newly captured screenshots automatically.
- Instantly transfers high-resolution screenshots to the connected PC without manual taps.
- Auto-saved on PC to `~/BeginningAndroid/Screenshots/` (or `%USERPROFILE%\BeginningAndroid\Screenshots\`).

### 3. Bi-Directional File Sharing
- **Android → PC**: Native Android system share sheet ("Share to Beginning Android") delivers files directly to PC.
- **PC → Android**: Windows Explorer / Linux file manager context menu integration ("Send to Beginning Android").
- Files land automatically into the designated downloads folder on the receiving end.

### 4. Auto-Connect & Pairing
- QR-code or one-time code handshake to establish trusted device keys.
- **Pair once, remember forever**: Re-connects seamlessly whenever both devices are online.
- No accounts, no email logins, no passwords.

---

## 🏗️ System Architecture

```
   +-------------------+                     +-------------------+
   |   Android Device  |                     |  Desktop (Win/Lin)|
   |   (Kotlin / UI)   |                     |   (Go App/Tray)   |
   |                   |                     |                   |
   | [ClipboardService]|                     | [ClipboardWatcher]|
   | [ScreenshotWatcher|                     | [File Drop Handler|
   | [Crypto: X25519]  |                     | [Crypto: X25519]  |
   +---------+---------+                     +---------+---------+
             |                                         |
             |  Encrypted Payloads (AES-256-GCM)       |
             |  via TLS WebSocket                      |
             v                                         v
   +-------------------------------------------------------------+
   |                       RELAY SERVER                          |
   |                     (Go / WebSocket)                        |
   |                                                             |
   |  - Ephemeral Session Hub (Channel Routing)                  |
   |  - Blind Packet Forwarding (Zero Knowledge of Content)      |
   |  - Zero Disk Storage, Zero Databases                        |
   +-------------------------------------------------------------+
```

### Protocol Flow & Encryption

```
1. PAIRING PHASE (One-Time)
   Android (Device A)                             Desktop (Device B)
        |                                                 |
   Generates Keypair                               Generates Keypair
   (PrivKey_A, PubKey_A)                           (PrivKey_B, PubKey_B)
        |                                                 |
        +------ Scans QR Code (PubKey_B + SessionID) ---->+
        |                                                 |
        +=========== Shared Secret Established ===========+
                 SharedKey = X25519(PrivKey, RemotePubKey)

2. DATA SYNC PHASE (Live Operation)
   Source Device                                  Target Device
        |                                                 |
   [Content]                                              |
        |                                                 |
   Encrypt with AES-GCM-256                               |
        |                                                 |
        |---- WS Encrypted Envelope ---> [Relay] ----+    |
                                                     |    |
                                                     +--->|
                                                          |
                                               Decrypt with AES-GCM-256
                                                          |
                                                  [Paste / Save File]
```

---

## 📂 Project Structure

```
beginning-android/
├── .github/
│   └── workflows/                # CI/CD workflows for Android, Desktop, and Relay
├── android/                      # Native Android Client (Kotlin)
│   ├── app/
│   │   ├── src/main/
│   │   │   ├── java/com/beginningandroid/app/
│   │   │   │   ├── crypto/       # X25519 & AES-GCM crypto routines
│   │   │   │   ├── network/      # WebSocket client and heartbeat
│   │   │   │   ├── service/      # Clipboard listener & Screenshot observer
│   │   │   │   └── ui/           # Minimal pairing and status screen
│   │   │   └── res/              # UI layouts & resources
│   │   └── build.gradle.kts
│   └── build.gradle.kts
├── desktop/                      # Native Desktop Client for Windows & Linux (Go)
│   ├── cmd/beginning-android/    # Desktop entry point
│   ├── internal/
│   │   ├── clipboard/            # Native OS clipboard hooks
│   │   ├── config/               # Secure key storage & settings
│   │   ├── crypto/               # Go E2EE implementation
│   │   ├── fileshare/            # File transfer protocols
│   │   ├── network/              # WebSocket client & reconnect logic
│   │   ├── platform/             # Windows / Linux OS integration
│   │   └── tray/                 # System tray icon and notifications
│   ├── scripts/                  # Shell & registry context menu installers
│   └── go.mod
├── relay/                        # Ultra-lightweight blind relay server (Go)
│   ├── cmd/relay-server/         # Server entry point
│   ├── internal/
│   │   ├── hub/                  # WebSocket connection routing
│   │   └── session/              # Ephemeral session pairing
│   ├── Dockerfile
│   └── go.mod
├── docs/                         # Specifications & Deep Dives
│   ├── ARCHITECTURE.md
│   ├── PROTOCOL.md
│   └── SECURITY.md
├── .gitignore
├── LICENSE
└── README.md
```

---

## 🛠️ Technology Stack Breakdown

| Component | Technology | Rationale |
| :--- | :--- | :--- |
| **Desktop Client** | **Go (Golang)** | Produces a single, self-contained binary (<15MB) with zero dependencies. Runs identical code across Windows and Linux. Direct OS clipboard and system tray access. |
| **Android Client** | **Kotlin / Jetpack** | Lean native Android architecture. Direct access to `ClipboardManager`, `ContentObserver` for media store/screenshots, and foreground service lifecycle. |
| **Relay Server** | **Go** | Concurrency via lightweight goroutines, ~10MB RAM usage under heavy load, deployable as a scratch Docker container anywhere. |
| **Cryptography** | **X25519 + AES-256-GCM** | Industry standard, constant-time, high speed, and resilient against side-channel attacks. |
| **Transport** | **WebSockets over TLS (WSS)** | Low protocol overhead, bi-directional full duplex, firewall friendly. |

---

## 📦 Setup & Installation Guide

### Prerequisites
- **Desktop (Windows / Linux)**: Go 1.22+ installed.
- **Android**: Android Studio Hedgehog+ / JDK 17+.
- **Relay**: Go or Docker.

### 1. Running the Relay Server
```bash
# Clone the repository
git clone https://github.com/rajkotpodman/Beginning-Android.git
cd Beginning-Android/relay

# Run locally
go run cmd/relay-server/main.go
# Or build Docker image
docker build -t beginning-android-relay .
```

### 2. Building the Desktop Client (Windows / Linux)
```bash
cd desktop

# Build for current OS
go build -o beginning-android cmd/beginning-android/main.go

# Cross-compile for Linux (from Windows):
# $env:GOOS="linux"; $env:GOARCH="amd64"; go build -o bin/beginning-android-linux cmd/beginning-android/main.go

# Install context menu entry:
# Windows: Double-click scripts/install-windows-context-menu.reg
# Linux: bash scripts/install-linux-context-menu.sh
```

### 3. Building the Android Client
```bash
cd android
./gradlew assembleDebug
# The output APK will be in: app/build/outputs/apk/debug/app-debug.apk
```

---

## 🔒 Security Specifications

- **End-to-End Encryption**: Every message (text or file chunk) is sealed using AES-256-GCM with a unique 96-bit nonce before leaving the client.
- **Zero Knowledge**: The relay server cannot read clipboard contents, file names, or screenshot bytes.
- **Replay Protection**: Timestamps and monotonically increasing sequence nonces protect against packet injection and replay.

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
