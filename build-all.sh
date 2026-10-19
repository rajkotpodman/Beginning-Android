#!/usr/bin/env bash
# Beginning Android - Complete Multi-Platform Release Build Script
# Builds Android APK, Windows AMD64 binary, and Linux AMD64 binary into /dist/
set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${ROOT_DIR}/dist"

echo "=========================================================="
echo "    Beginning Android - Full Multi-Platform Build        "
echo "=========================================================="

rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

# 1. Build Desktop Binaries (Windows & Linux)
echo ""
echo "[1/2] Building Go Desktop Binaries (Windows & Linux)..."
cd "${ROOT_DIR}/desktop"

# Windows binary
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "${DIST_DIR}/beginning-android-windows-amd64.exe" ./cmd/beginning-android
echo "[OK] Built ${DIST_DIR}/beginning-android-windows-amd64.exe"

# Linux binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "${DIST_DIR}/beginning-android-linux-amd64" ./cmd/beginning-android
echo "[OK] Built ${DIST_DIR}/beginning-android-linux-amd64"

# 2. Build Android APK
echo ""
echo "[2/2] Building Android Release APK..."
cd "${ROOT_DIR}/android"
if [ -f "./gradlew" ]; then
    bash ./gradlew assembleRelease --no-daemon
elif command -v gradle &> /dev/null; then
    gradle assembleRelease --no-daemon
fi

APK_SRC="${ROOT_DIR}/android/app/build/outputs/apk/release/app-release.apk"
if [ -f "${APK_SRC}" ]; then
    cp "${APK_SRC}" "${DIST_DIR}/beginning-android-release.apk"
    echo "[OK] Copied APK to ${DIST_DIR}/beginning-android-release.apk"
else
    echo "[WARNING] Android release APK not found at ${APK_SRC}"
fi

echo ""
echo "=========================================================="
echo "    Build Summary - Artifacts in /dist/                   "
echo "=========================================================="
ls -lh "${DIST_DIR}"
echo ""
echo "[SUCCESS] All platform artifacts built successfully!"
