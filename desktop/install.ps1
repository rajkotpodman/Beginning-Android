# Beginning Android - Automated One-Click Installer for Windows
$ErrorActionPreference = "Continue"

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "       Beginning Android - Automated Windows Installer    " -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host ""

# 1. Terminate any running instance
Write-Host "[1/8] Checking for running instances..." -ForegroundColor Yellow
Stop-Process -Name "beginning-android*" -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 500

# 2. Setup Target Directory
$InstallDir = "$env:LOCALAPPDATA\BeginningAndroid\bin"
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}
Write-Host "[OK] Installation directory: $InstallDir" -ForegroundColor Green

# 3. Locate & Copy Binaries
Write-Host "[2/8] Installing Beginning Android binary..." -ForegroundColor Yellow
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$PossibleBins = @(
    (Join-Path $ScriptDir "..\dist\beginning-android-windows-amd64.exe"),
    (Join-Path $ScriptDir "bin\beginning-android-windows-amd64.exe"),
    (Join-Path $ScriptDir "beginning-android-windows-amd64.exe"),
    (Join-Path $ScriptDir "beginning-android.exe")
)

$SourceBin = $null
foreach ($bin in $PossibleBins) {
    if (Test-Path $bin) {
        $SourceBin = $bin
        break
    }
}

if ($SourceBin) {
    Copy-Item -Path $SourceBin -Destination "$InstallDir\beginning-android.exe" -Force
    Write-Host "[OK] Copied binary to: $InstallDir\beginning-android.exe" -ForegroundColor Green
} else {
    Write-Host "[ERROR] Binary not found! Please build it first." -ForegroundColor Red
    exit 1
}

# Copy Android APK if present
$PossibleApks = @(
    (Join-Path $ScriptDir "..\dist\beginning-android-release.apk"),
    (Join-Path $ScriptDir "beginning-android-release.apk")
)
foreach ($apk in $PossibleApks) {
    if (Test-Path $apk) {
        Copy-Item -Path $apk -Destination "$InstallDir\beginning-android-release.apk" -Force
        Write-Host "[OK] Mobile APK packaged to: $InstallDir\beginning-android-release.apk" -ForegroundColor Green
        break
    }
}

# 4. Auto-Configure Local Config
Write-Host "[3/8] Auto-configuring pairing settings..." -ForegroundColor Yellow
$ConfigDir = "$env:USERPROFILE\.beginningandroid"
if (-not (Test-Path $ConfigDir)) {
    New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
}
$ConfigFile = Join-Path $ConfigDir "config.json"

$DefaultPairCode = "lunar-willow-drift-echo-drift-quartz"
$CurrentPairCode = $DefaultPairCode

if (Test-Path $ConfigFile) {
    try {
        $existing = Get-Content $ConfigFile -Raw | ConvertFrom-Json
        if ($existing.pair_code -and $existing.pair_code -ne "") {
            $CurrentPairCode = $existing.pair_code
        }
    } catch {}
}

$ConfigData = @{
    pair_code = $CurrentPairCode
    relay_url = "ws://localhost:8080/ws"
    pc_name   = $env:COMPUTERNAME
}
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($ConfigFile, ($ConfigData | ConvertTo-Json -Depth 4), $utf8NoBom)
Write-Host "[OK] Config configured with Pair Code: $CurrentPairCode" -ForegroundColor Green

# 5. Configure Windows Firewall
Write-Host "[4/8] Configuring Windows Firewall for Local Relay & Discovery..." -ForegroundColor Yellow
try {
    netsh advfirewall firewall delete rule name="Beginning Android Relay (TCP 8080)" 2>$null | Out-Null
    netsh advfirewall firewall add rule name="Beginning Android Relay (TCP 8080)" dir=in action=allow protocol=TCP localport=8080 profile=any | Out-Null
    
    netsh advfirewall firewall delete rule name="Beginning Android Discovery (UDP 8082)" 2>$null | Out-Null
    netsh advfirewall firewall add rule name="Beginning Android Discovery (UDP 8082)" dir=in action=allow protocol=UDP localport=8082 profile=any | Out-Null
    Write-Host "[OK] Windows Firewall rules added successfully (TCP 8080, UDP 8082)." -ForegroundColor Green
} catch {
    Write-Host "[NOTE] Run as Administrator if Android cannot reach PC over Wi-Fi." -ForegroundColor Yellow
}

# 6. Register Windows Explorer Context Menu
Write-Host "[5/8] Registering File Explorer 'Send to Beginning Android'..." -ForegroundColor Yellow
reg.exe add "HKCU\Software\Classes\*\shell\BeginningAndroid" /ve /d "Send to Beginning Android" /f | Out-Null
reg.exe add "HKCU\Software\Classes\*\shell\BeginningAndroid" /v "Icon" /d "$InstallDir\beginning-android.exe,0" /f | Out-Null
reg.exe add "HKCU\Software\Classes\*\shell\BeginningAndroid\command" /ve /d "`"$InstallDir\beginning-android.exe`" send `"%1`"" /f | Out-Null
Write-Host "[OK] Right-click context menu registered." -ForegroundColor Green

# 7. Register Windows Startup (Background Live Connection)
Write-Host "[6/8] Registering Windows Auto-Start on Boot..." -ForegroundColor Yellow
reg.exe add "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "BeginningAndroid" /d "`"$InstallDir\beginning-android.exe`"" /f | Out-Null
Write-Host "[OK] Auto-Start enabled (runs in background on login)." -ForegroundColor Green

# 8. Create Desktop & Start Menu Shortcuts
Write-Host "[7/8] Creating Desktop & Start Menu Shortcuts..." -ForegroundColor Yellow
$WshShell = New-Object -ComObject WScript.Shell

$DesktopShortcutPath = [System.IO.Path]::Combine([Environment]::GetFolderPath("Desktop"), "Beginning Android.lnk")
$Shortcut = $WshShell.CreateShortcut($DesktopShortcutPath)
$Shortcut.TargetPath = "$InstallDir\beginning-android.exe"
$Shortcut.WorkingDirectory = $InstallDir
$Shortcut.Description = "Beginning Android - Live Clipboard & File Sync"
$Shortcut.Save()

$ProgramsDir = [Environment]::GetFolderPath("Programs")
$StartMenuShortcutPath = [System.IO.Path]::Combine($ProgramsDir, "Beginning Android.lnk")
$Shortcut2 = $WshShell.CreateShortcut($StartMenuShortcutPath)
$Shortcut2.TargetPath = "$InstallDir\beginning-android.exe"
$Shortcut2.WorkingDirectory = $InstallDir
$Shortcut2.Description = "Beginning Android - Live Clipboard & File Sync"
$Shortcut2.Save()
Write-Host "[OK] Shortcuts created on Desktop and Start Menu." -ForegroundColor Green

# Add to PATH
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$InstallDir;$UserPath", "User")
    Write-Host "[OK] Added to User PATH." -ForegroundColor Green
}

# 9. Launch Desktop Client
Write-Host "[8/8] Starting Beginning Android background client..." -ForegroundColor Yellow
Start-Process "cmd.exe" -ArgumentList "/c start `"`" `"$InstallDir\beginning-android.exe`"" -WindowStyle Hidden

Start-Sleep -Seconds 1

Write-Host ""
Write-Host "==========================================================" -ForegroundColor Green
Write-Host "       INSTALLATION & CONFIGURATION COMPLETE!            " -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Green
Write-Host ""
Write-Host "  Pair Code:      $CurrentPairCode" -ForegroundColor Cyan
Write-Host "  Local Relay:    http://localhost:8080" -ForegroundColor Cyan
Write-Host "  System Tray:    Beginning Android icon is active" -ForegroundColor Cyan
Write-Host ""
Write-Host "Opening Pairing Hub in default browser..." -ForegroundColor Yellow
Start-Process "http://localhost:8080"
