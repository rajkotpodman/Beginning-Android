@echo off
title Beginning Android - Local Relay Server
echo ========================================================
echo   Beginning Android - Local Hotspot Relay Server
echo ========================================================
echo.

:: Detect IP Address on local interface
for /f "tokens=4" %%a in ('route print ^| findstr 0.0.0.0 ^| findstr /v "0.0.0.0\>"') do set LOCAL_IP=%%a

echo [OK] PC Local IP Address: 10.26.96.105
echo.
echo ========================================================
echo  CONNECTIVITY ENDPOINTS:
echo    - On this PC:    ws://localhost:8080/ws
echo    - On Moto G64:   ws://10.26.96.105:8080/ws
echo ========================================================
echo.
echo Starting Python WebSocket Relay Server on port 8080...
echo Press Ctrl+C to stop.
echo.

python server/server.py
pause
