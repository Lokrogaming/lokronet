@echo off
REM LokroNet Autostart (Windows).
REM In den Autostart-Ordner legen (Win+R -> shell:startup):
REM   kopiere diese Datei nach %APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup\
REM Der Daemon schickt beim Start automatisch Beacons an alle Kontakte
REM (presence.go: beaconLoop). Dieses Skript stellt nur sicher, dass er laeuft.
setlocal
set "BIN=%LOCALAPPDATA%\LokroNet\lokronet.exe"
if not exist "%BIN%" set "BIN=lokronet.exe"

tasklist /FI "IMAGENAME eq lokronet.exe" 2>NUL | find /I "lokronet.exe" >NUL
if %ERRORLEVEL%==0 (
  echo [lokronet] daemon laeuft bereits
) else (
  echo [lokronet] starte daemon …
  start "" /MIN "%BIN%" daemon
  timeout /t 3 /nobreak >NUL
)

echo [lokronet] sende boot-beacon …
"%BIN%" beacon
"%BIN%" presence
endlocal
