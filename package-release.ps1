# Beginning Android - Package All Release Artifacts (Portable & Source)
$ErrorActionPreference = "Stop"

$RootDir = $PSScriptRoot
if (-not $RootDir) { $RootDir = (Get-Location).Path }
Set-Location $RootDir

$DistDir = Join-Path $RootDir "dist"
if (-not (Test-Path $DistDir)) {
    New-Item -ItemType Directory -Path $DistDir -Force | Out-Null
}

# 1. Prepare Portable Directory
Write-Host "Creating Portable directory..." -ForegroundColor Cyan
$PortableDir = Join-Path $DistDir "BeginningAndroid-Windows-Portable"
if (Test-Path $PortableDir) {
    Remove-Item -Path $PortableDir -Recurse -Force
}
New-Item -ItemType Directory -Path $PortableDir -Force | Out-Null

$WinBin = Join-Path $DistDir "beginning-android-windows-amd64.exe"
if (Test-Path $WinBin) {
    Copy-Item $WinBin -Destination (Join-Path $PortableDir "beginning-android.exe") -Force
}

$ApkBin = Join-Path $DistDir "beginning-android-release.apk"
if (Test-Path $ApkBin) {
    Copy-Item $ApkBin -Destination (Join-Path $PortableDir "beginning-android-release.apk") -Force
}

# Add Helper Scripts
@'
@echo off
title Beginning Android - Portable Runner
echo Starting Beginning Android in System Tray...
start "" "%~dp0beginning-android.exe"
timeout /t 1 >nul
start "" "http://localhost:8080"
'@ | Set-Content -Path (Join-Path $PortableDir "Run-Portable.bat") -Encoding ASCII

@'
@echo off
title Register Beginning Android Startup
reg.exe add "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "BeginningAndroid" /d "\"%~dp0beginning-android.exe\"" /f
echo [OK] Registered in Windows Startup. Beginning Android will now run automatically in the background on boot.
pause
'@ | Set-Content -Path (Join-Path $PortableDir "Register-Startup.bat") -Encoding ASCII

@'
@echo off
title Register Explorer Context Menu
reg.exe add "HKCU\Software\Classes\*\shell\BeginningAndroid" /ve /d "Send to Beginning Android" /f
reg.exe add "HKCU\Software\Classes\*\shell\BeginningAndroid" /v "Icon" /d "\"%~dp0beginning-android.exe\",0" /f
reg.exe add "HKCU\Software\Classes\*\shell\BeginningAndroid\command" /ve /d "\"%~dp0beginning-android.exe\" send \"%%1\"" /f
echo [OK] Registered Explorer right-click menu. Right-click any file to send it to your phone.
pause
'@ | Set-Content -Path (Join-Path $PortableDir "Register-ContextMenu.bat") -Encoding ASCII

@'
========================================================================
   BEGINNING ANDROID - PORTABLE EDITION
========================================================================

QUICK START:
1. Double-click 'Run-Portable.bat' to start the app in your system tray.
2. The Web Pairing Dashboard will open at http://localhost:8080.
3. Open Beginning Android on your Android phone (connected to Mobile Hotspot or Wi-Fi).
4. Tap 'Auto-Detect PC on Hotspot/Wi-Fi' on the Android app, or enter the Pair Code shown on the dashboard.
5. Devices connect once and stay connected in the background live!

FEATURES:
- Live 300ms Clipboard Sync between PC and Android.
- Instant Screenshot Auto-Transfer to PC (Saved in ~/BeginningAndroid/Screenshots).
- Direct Bi-Directional File Sharing (Right-click files on PC or Share from Android).
- Zero external dependencies; embedded WebSocket relay server and LAN auto-discovery.

OPTIONAL HELPERS:
- 'Register-Startup.bat': Run automatically in background when Windows starts.
- 'Register-ContextMenu.bat': Add 'Send to Beginning Android' to Windows Explorer right-click menu.
========================================================================
'@ | Set-Content -Path (Join-Path $PortableDir "README-PORTABLE.txt") -Encoding ASCII

# Create Portable ZIP
Write-Host "Creating Portable ZIP archive..." -ForegroundColor Cyan
$PortableZip = Join-Path $DistDir "BeginningAndroid-Windows-Portable.zip"
if (Test-Path $PortableZip) { Remove-Item $PortableZip -Force }
Compress-Archive -Path "$PortableDir\*" -DestinationPath $PortableZip -Force
Write-Host "[OK] Created: $PortableZip" -ForegroundColor Green

# 2. Create Complete Source ZIP Archive
Write-Host "Creating Complete Source ZIP archive..." -ForegroundColor Cyan
$SourceZip = Join-Path $DistDir "BeginningAndroid-Complete-Source.zip"
if (Test-Path $SourceZip) { Remove-Item $SourceZip -Force }

git archive --format=zip --output=$SourceZip HEAD
Write-Host "[OK] Created: $SourceZip" -ForegroundColor Green

Write-Host "`nAll release packages created successfully!" -ForegroundColor Green
Get-ChildItem -Path $DistDir | Select-Object Name, @{Name="Size"; Expression={"{0:N2} MB" -f ($_.Length / 1MB)}} | Format-Table -AutoSize
