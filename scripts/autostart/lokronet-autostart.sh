#!/usr/bin/env bash
# LokroNet Autostart (Linux, systemd user service ist bevorzugt).
# Diese Datei ist für Systeme OHNE systemd oder als sichtbarer Boot-Beacon.
# Der Daemon schickt beim Start automatisch Beacons an alle Kontakte
# (siehe internal/daemon/presence.go: beaconLoop). Dieses Skript stellt nur
# sicher, dass der Daemon läuft, und stößt einen manuellen Beacon an.
set -euo pipefail

BIN="${LOKRO_BIN:-$(command -v lokronet || echo "$HOME/.local/bin/lokronet")}"
if [ ! -x "$BIN" ]; then
  echo "lokronet nicht gefunden ($BIN). Erst installieren:"
  echo "  curl -fsSL https://net.lokro.dev/install | sudo bash -s -- --ui"
  exit 1
fi

if pgrep -f "lokronet daemon" >/dev/null 2>&1; then
  echo "[lokronet] daemon läuft bereits (PID $(pgrep -f 'lokronet daemon' | head -1))"
else
  echo "[lokronet] starte daemon …"
  if systemctl --user is-active --quiet lokronet-daemon 2>/dev/null; then
    echo "[lokronet] user-service läuft bereits"
  elif command -v systemctl >/dev/null && [ -f "$HOME/.config/systemd/user/lokronet-daemon.service" ]; then
    systemctl --user enable --now lokronet-daemon
  else
    nohup "$BIN" daemon >/dev/null 2>&1 &
    sleep 2
  fi
fi

echo "[lokronet] sende boot-beacon an alle Kontakte …"
"$BIN" beacon || true
echo "[lokronet] presence:"
"$BIN" presence || true
