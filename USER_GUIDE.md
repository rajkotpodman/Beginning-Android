# Beginning Android — User Guide

Welcome to **Beginning Android**, an ultra-minimal, high-performance, and end-to-end encrypted utility that seamlessly bridges your Android device and Desktop (Windows & Linux).

---

## 📑 Table of Contents
1. [Prerequisites & Downloads](#-prerequisites--downloads)
2. [One-Time Device Pairing](#-one-time-device-pairing)
3. [Core Features & Usage](#-core-features--usage)
   - [Live Clipboard Sync](#1-live-clipboard-sync)
   - [Automatic Screenshot Transfer](#2-automatic-screenshot-transfer)
   - [Bidirectional File Sharing](#3-bidirectional-file-sharing)
   - [Live Voice Typing](#4-live-voice-typing)
4. [Desktop System Tray Menu](#-desktop-system-tray-menu)
5. [Self-Hosting the Relay Server](#-self-hosting-the-relay-server)
6. [Security & Verification](#-security--verification)
7. [Troubleshooting & FAQs](#-troubleshooting--faqs)

---

## 📦 Prerequisites & Downloads

All release binaries are lightweight with zero external runtime dependencies.

| Platform | Download / Artifact | Size | Description |
| :--- | :--- | :--- | :--- |
| **Android** | `beginning-android-release.apk` | ~2.1 MB | Requires Android 8.0+ (Oreo) or newer. |
| **Windows** | `beginning-android-windows-amd64.exe` | ~5.8 MB | Windows 10/11 (64-bit). Single self-contained `.exe`. |
| **Linux** | `beginning-android-linux-amd64` | ~7.3 MB | Ubuntu, Debian, Fedora, Arch (64-bit). |

### Installation

#### Windows
1. Download `beginning-android-windows-amd64.exe`.
2. Run `powershell -ExecutionPolicy Bypass -File install.ps1` in the `desktop/` folder, or place the executable into `%LOCALAPPDATA%\BeginningAndroid\bin\`.
3. This registers the right-click context menu **"Send to Beginning Android"** in File Explorer.

#### Linux
1. Download `beginning-android-linux-amd64` and make it executable:
   ```bash
   chmod +x beginning-android-linux-amd64
   ```
2. Run `bash install.sh` to register Nautilus, Nemo, and KDE Dolphin context menus.

#### Android
1. Sideload and install `beginning-android-release.apk`.
2. Grant requested permissions for Notifications and Media Store access.

---

## 🔗 One-Time Device Pairing

Beginning Android pairs devices securely using a simple **6-word mnemonic code** (e.g., `amber-breeze-cedar-drift-echo-frost`). No accounts, emails, or passwords are used.

```
       [ Android Device ]                         [ Desktop PC ]
               |                                        |
       1. Tap "Generate Code"                           |
       (e.g., amber-breeze-cedar-...)                   |
               |                                        |
               +--- Enter 6-word code into Desktop ---->|
               |                                        |
       2. Automatic Key Exchange (X25519) <============>+
               |                                        |
               +--- Paired Forever (Auto-Reconnect) --->+
```

1. **On Android**:
   - Open the **Beginning Android** app.
   - Tap **"Generate 6-Word Code"** (or type a custom 6-word phrase).
   - Tap **"Connect & Start Sync"**.
2. **On Desktop**:
   - Launch `beginning-android`.
   - Click the system tray icon $\to$ **Pair Device...**.
   - Enter the exact 6-word code shown on your Android screen.
   - Alternatively via CLI:
     ```bash
     beginning-android pair amber-breeze-cedar-drift-echo-frost
     ```
3. **Status Confirmation**:
   - Android will display: `Connected to: <Your PC Name>`.
   - Desktop tray will display: `Status: Connected`.

---

## 🚀 Core Features & Usage

### 1. Live Clipboard Sync
- **Copy on Android**: Any text copied on Android instantly syncs to your PC clipboard within 300ms.
- **Copy on PC**: Any text copied on your PC (`Ctrl+C`) instantly updates your Android clipboard and displays a silent confirmation toast.
- **Echo Suppression**: Built-in loopback suppression prevents repeated or recursive paste loops.
- **Full Unicode**: Supports code snippets, emojis, international languages, and multi-line text.

### 2. Automatic Screenshot Transfer
- Whenever you take a screenshot on your Android phone (using `Power + Vol Down` or three-finger swipe), Beginning Android automatically detects the newly created image.
- **Zero Taps Needed**: The screenshot streams over the encrypted connection and lands in your PC screenshots directory:
  - **Windows**: `%USERPROFILE%\BeginningAndroid\Screenshots\`
  - **Linux**: `~/BeginningAndroid/Screenshots/`
- Notification displays on PC upon arrival.

### 3. Bidirectional File Sharing

#### A. From Android to PC (Native Share Sheet)
1. In any Android app (Photos, Gallery, Files, Chrome, etc.), select any file or image.
2. Tap the system **Share** button.
3. Select **Beginning Android**.
4. The file is encrypted, streamed in 64KB blocks, and saved to `%USERPROFILE%\BeginningAndroid\Received\` on your PC.

#### B. From PC to Android (Context Menu & CLI)
- **Windows Explorer**: Right-click any file $\to$ click **Send to Beginning Android**.
- **Linux File Manager**: Right-click any file $\to$ **Scripts $\to$ Send to Beginning Android**.
- **CLI Mode**:
  ```bash
  beginning-android send /path/to/my_document.pdf
  ```
- Files land in your Android device's `Downloads/BeginningAndroid/` directory.

### 4. Live Voice Typing
1. Open the Beginning Android app on your phone.
2. Tap **"🎤 Start Voice Typing"**.
3. Speak naturally. Your spoken words are transcribed in real time and automatically typed or placed onto your PC's active clipboard for instant pasting.

---

## 🖥️ Desktop System Tray Menu

Clicking the system tray icon displays the quick control menu:

```
+------------------------------------+
| Status: Connected (Google Pixel 8) |  <-- Dynamic status indicator
|------------------------------------|
| Pair Device...                     |  <-- Set/update 6-word code
| Open Screenshots folder            |  <-- Opens Screenshots directory
| Open Received files folder         |  <-- Opens Received files directory
|------------------------------------|
| Quit                               |  <-- Exit background sync
+------------------------------------+
```

---

## 🛡️ Security & Verification

Beginning Android is engineered under strict zero-knowledge principles:

- **PBKDF2 Master Seed**: 100,000 rounds of `PBKDF2-HMAC-SHA256` ensure brute-force resistance.
- **X25519 Asymmetric Exchange**: Elliptic Curve Diffie-Hellman establishes a unique, ephemeral session secret.
- **HKDF-SHA256 Key Derivation**: High-entropy session key derived from the shared secret and salt.
- **AES-256-GCM**: Constant-time authenticated payload encryption with unique 12-byte random nonces.
- **Replay Protection**: Strict monotonically increasing message counters (`seq`) reject replayed or intercepted envelopes.
- **Blind Relay**: The relay server only forwards raw encrypted JSON blobs; it possesses zero cryptographic keys and cannot read any content.

---

---

## 🌐 Relay Server Deployment & Local Hotspot Setup

Beginning Android can run either over the public Internet or entirely offline on a local private network or mobile hotspot.

### A. Local Wi-Fi Hotspot Setup (Zero-Internet Operation)
You can connect your phone directly to your PC without an active Internet connection (e.g. Android mobile hotspot or local Wi-Fi router).

1. **Start the Local Relay on Windows**:
   - Double-click `run-local-relay.bat` in the repository root, or run:
     ```powershell
     python server/server.py
     ```
   - The relay server binds to `0.0.0.0:8080`.
2. **Determine your PC's Local IP**:
   - When connected via Wi-Fi adapter to your phone's hotspot (e.g. Moto G64), check your IPv4 address (`ipconfig` or `Get-NetIPAddress`).
   - Example local IP: `10.26.96.105`.
3. **Configure Desktop Client**:
   - Launch `beginning-android` and set relay URL to:
     ```text
     ws://localhost:8080/ws
     ```
4. **Configure Android Device**:
   - In the Android app, enter the server URL pointing to your PC:
     ```text
     ws://10.26.96.105:8080/ws
     ```
   - Enter your 6-word pair code on both devices.
   - Sync works entirely within the local Wi-Fi link at high speed with zero mobile data consumption!

---

### B. Self-Hosting on Cloud VPS (Hetzner, Railway, Fly.io)

For full remote operation across disparate networks with automated HTTPS/WSS:
1. Refer to [RELAY_DEPLOY.md](file:///e:/begnnining%20android/RELAY_DEPLOY.md) for step-by-step deployment templates.
2. Deploy the container using Docker:
   ```bash
   docker run -d -p 8080:8080 --name beginning-relay beginning-android-server
   ```
3. Set up free automatic TLS using Caddy:
   ```caddyfile
   relay.yourdomain.com {
       reverse_proxy localhost:8080
   }
   ```
4. Configure clients to use `wss://relay.yourdomain.com/ws`.

---

## ❓ Troubleshooting & FAQs

**Q: My phone and PC show "Disconnected".**
- Ensure both devices are connected to the Internet or local network and that the 6-word pair code matches on both sides.
- Check that the relay server is running and accessible.

**Q: Screenshots are not auto-transferring on Android 14+.**
- Check that the app has permission to access media (`READ_MEDIA_IMAGES`).
- Verify battery optimization is set to "Unrestricted" so the background service remains active.

**Q: Where are my config files stored on PC?**
- Windows: `%USERPROFILE%\.beginningandroid\config.json`
- Linux: `~/.beginningandroid/config.json`
