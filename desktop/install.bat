@echo off
title Beginning Android - Windows Installer
echo ========================================================
echo   Beginning Android - One-Click Installer & Auto-Config
echo ========================================================
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1"
echo.
pause
