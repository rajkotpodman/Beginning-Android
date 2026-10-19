# Contributing to Beginning Android

Thank you for your interest in contributing to **Beginning Android**!

## ⚡ Core Principles
1. **Minimalist & Zero-Bloat**: Code must remain as small, fast, and simple as possible. No heavy frameworks or unnecessary third-party dependencies.
2. **Security & Privacy First**: Everything is end-to-end encrypted (E2EE) with zero cloud storage. Never weaken or bypass crypto validation.
3. **Cross-Platform Parity**: Features should work cleanly across Android, Windows, and Linux.

---

## 🛠️ Local Development Setup

### 1. Prerequisites
- **Desktop**: Go 1.22+
- **Android**: Android Studio (or JDK 17+ and Android Command Line Tools)
- **Relay**: Python 3.10+ (for local testing)

### 2. Running Tests
```bash
# Go Desktop Tests
cd desktop
go test -v ./...

# Android Tests
cd android
./gradlew testReleaseUnitTest

# Relay Server Tests
cd server
python test_server.py
```

### 3. Building All Binaries Locally
```bash
# Linux / macOS / Git Bash
./build-all.sh

# Windows (PowerShell)
.\build-all.ps1
```
All release binaries will be placed in the `/dist/` folder.

---

## 📬 Pull Request Guidelines
1. Fork the repo and create your branch from `main`:
   ```bash
   git checkout -b feat/your-feature-name
   ```
2. Keep commits atomic and use conventional commits (`feat:`, `fix:`, `docs:`, `test:`).
3. Ensure all automated tests pass before submitting your PR.
4. Open a Pull Request detailing the changes made and testing steps taken.
