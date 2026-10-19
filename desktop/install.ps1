# Beginning Android - Windows Context Menu & Path Installer
$ErrorActionPreference = "Stop"

Write-Host "=== Installing Beginning Android for Windows ===" -ForegroundColor Cyan

$InstallDir = "$env:LOCALAPPDATA\BeginningAndroid\bin"
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

# Locate binary
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$SourceBin = Join-Path $ScriptDir "bin\beginning-android-windows-amd64.exe"
if (-not (Test-Path $SourceBin)) {
    $SourceBin = Join-Path $ScriptDir "bin\beginning-android.exe"
}

if (Test-Path $SourceBin) {
    Copy-Item -Path $SourceBin -Destination "$InstallDir\beginning-android.exe" -Force
    Write-Host "[OK] Binary copied to: $InstallDir\beginning-android.exe" -ForegroundColor Green
} else {
    Write-Host "[WARNING] Pre-built binary not found in bin\. Ensure you build it with build.sh or go build." -ForegroundColor Yellow
}

# 1. Register Context Menu in Current User Registry using reg.exe (Fast & reliable)
reg.exe add "HKCU\Software\Classes\*\shell\BeginningAndroid" /ve /d "Send to Beginning Android" /f | Out-Null
reg.exe add "HKCU\Software\Classes\*\shell\BeginningAndroid" /v "Icon" /d "$InstallDir\beginning-android.exe,0" /f | Out-Null
reg.exe add "HKCU\Software\Classes\*\shell\BeginningAndroid\command" /ve /d "`"$InstallDir\beginning-android.exe`" send `"%1`"" /f | Out-Null

Write-Host "[OK] Windows Context Menu 'Send to Beginning Android' registered successfully!" -ForegroundColor Green

# 2. Add to User PATH if not already present
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$InstallDir;$UserPath", "User")
    Write-Host "[OK] Added $InstallDir to User PATH." -ForegroundColor Green
}

Write-Host "`nInstallation complete! You can now right-click any file in File Explorer and select 'Send to Beginning Android'." -ForegroundColor Cyan
