# Beginning Android - Complete Multi-Platform Release Build Script (PowerShell)
$ErrorActionPreference = "Stop"

$RootDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $RootDir

$DistDir = Join-Path $RootDir "dist"
if (Test-Path $DistDir) {
    Remove-Item -Path $DistDir -Recurse -Force
}
New-Item -ItemType Directory -Path $DistDir -Force | Out-Null

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "    Beginning Android - Full Multi-Platform Build        " -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

# 1. Build Desktop Binaries
Write-Host "`n[1/2] Building Go Desktop Binaries (Windows & Linux)..." -ForegroundColor Yellow
$DesktopDir = Join-Path $RootDir "desktop"
Set-Location $DesktopDir

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags="-s -w" -o "$DistDir\beginning-android-windows-amd64.exe" ./cmd/beginning-android
Write-Host "[OK] Built $DistDir\beginning-android-windows-amd64.exe" -ForegroundColor Green

$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -ldflags="-s -w" -o "$DistDir\beginning-android-linux-amd64" ./cmd/beginning-android
Write-Host "[OK] Built $DistDir\beginning-android-linux-amd64" -ForegroundColor Green

# 2. Build Android APK
Write-Host "`n[2/2] Building Android Release APK..." -ForegroundColor Yellow
$AndroidDir = Join-Path $RootDir "android"
Set-Location $AndroidDir
.\gradlew.bat assembleRelease

$ApkSrc = Join-Path $AndroidDir "app\build\outputs\apk\release\app-release.apk"
if (Test-Path $ApkSrc) {
    Copy-Item -Path $ApkSrc -Destination "$DistDir\beginning-android-release.apk" -Force
    Write-Host "[OK] Copied APK to $DistDir\beginning-android-release.apk" -ForegroundColor Green
} else {
    Write-Host "[WARNING] APK not found at $ApkSrc" -ForegroundColor Red
}

Set-Location $RootDir
Write-Host "`n==========================================================" -ForegroundColor Cyan
Write-Host "    Build Summary - Artifacts in /dist/                   " -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan
Get-ChildItem -Path $DistDir | Select-Object Name, @{Name="Size (MB)"; Expression={"{0:N2} MB" -f ($_.Length / 1MB)}} | Format-Table -AutoSize
Write-Host "[SUCCESS] All platform artifacts built successfully!" -ForegroundColor Green
