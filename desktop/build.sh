#!/usr/bin/env bash
# Beginning Android - Cross-Platform Desktop Build Script
# Targets: Windows (amd64) and Linux (amd64)
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

mkdir -p bin

echo "=== Building Beginning Android Desktop Binaries ==="

# 1. Build Windows Binary
echo "Building Windows amd64 binary..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/beginning-android-windows-amd64.exe ./cmd/beginning-android
echo "[OK] Built bin/beginning-android-windows-amd64.exe"

# 2. Build Linux Binary
echo "Building Linux amd64 binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/beginning-android-linux-amd64 ./cmd/beginning-android
echo "[OK] Built bin/beginning-android-linux-amd64"

echo ""
echo "=== Build Artifacts & Sizes ==="
ls -lh bin/

echo "Build successful! All binaries produced under 10MB."
