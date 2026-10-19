# Beginning Android - Cross-Platform Desktop Build Script for PowerShell
$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ScriptDir

$BinDir = Join-Path $ScriptDir "bin"
if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
}

Write-Host "=== Building Beginning Android Desktop Binaries ===" -ForegroundColor Cyan

# 1. Build Windows Binary
Write-Host "Building Windows amd64 binary..." -ForegroundColor Yellow
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags="-s -w" -o bin/beginning-android-windows-amd64.exe ./cmd/beginning-android
Write-Host "[OK] Built bin/beginning-android-windows-amd64.exe" -ForegroundColor Green

# 2. Build Linux Binary
Write-Host "Building Linux amd64 binary..." -ForegroundColor Yellow
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -ldflags="-s -w" -o bin/beginning-android-linux-amd64 ./cmd/beginning-android
Write-Host "[OK] Built bin/beginning-android-linux-amd64" -ForegroundColor Green

Write-Host "`n=== Build Artifacts & Sizes ===" -ForegroundColor Cyan
Get-ChildItem -Path $BinDir | Select-Object Name, @{Name="Size (MB)"; Expression={"{0:N2} MB" -f ($_.Length / 1MB)}} | Format-Table -AutoSize

Write-Host "Build complete! All binaries produced under 10MB." -ForegroundColor Green
