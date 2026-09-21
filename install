#!/usr/bin/env bash
# LokroNet Installer – wird auf https://net.lokro.dev/install gehostet.
# Quelle der Wahrheit in diesem Repo: deploy/install.sh
#
# Nutzung:
#   curl -fsSL https://net.lokro.dev/install | sudo bash
#   curl -fsSL https://net.lokro.dev/install | sudo bash -s -- --rendezvous
#   curl -fsSL https://net.lokro.dev/install | bash -s -- --user
#
# Optionen:
#   --version X     Version pinnen (Default: 0.1.0, oder LOKRO_VERSION)
#   --base URL      Download-Basis (Default: https://net.lokro.dev/dl, oder LOKRO_BASE)
#   --user          nach ~/.local/bin installieren (kein root, kein systemd-system)
#   --rendezvous    zusätzlich System-Service lokro-rendezvous einrichten (:8787)
#   --daemon        zusätzlich User-Service lokronet-daemon einrichten (systemd --user)
#   --no-systemd    keine Services anfassen (nur Binary)
set -euo pipefail

VERSION="${LOKRO_VERSION:-0.1.0}"
# BASE wird bewusst erst NACH dem Argument-Parsing gesetzt (siehe unten),
# damit --version / LOKRO_VERSION in die Default-URL einfließen.
BASE="${LOKRO_BASE:-}"
USER_MODE=0
WITH_RENDEZVOUS=0
WITH_DAEMON=0

while [ $# -gt 0 ]; do
  case "$1" in
    --version=*) VERSION="${1#--version=}"; shift ;;
    --version) VERSION="${2:?--version braucht einen Wert}"; shift 2 ;;
    --base=*) BASE="${1#--base=}"; shift ;;
    --base) BASE="${2:?--base braucht einen Wert}"; shift 2 ;;
    --user) USER_MODE=1; shift ;;
    --rendezvous) WITH_RENDEZVOUS=1; shift ;;
    --daemon) WITH_DAEMON=1; shift ;;
    --no-systemd) WITH_RENDEZVOUS=0; WITH_DAEMON=0; shift ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "unbekannte Option: $1 (siehe --help)"; exit 2 ;;
  esac
done

# Default: GitHub-Release-Assets (folgt --version, Tags als vX.Y.Z).
# Eigener Mirror (z.B. https://net.lokro.dev/dl) via --base / LOKRO_BASE.
if [ -z "$BASE" ]; then
  BASE="https://github.com/Lokrogaming/lokronet/releases/download/v${VERSION}"
fi

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) echo "fehler: Architektur $ARCH wird nicht unterstützt (nur amd64/arm64)"; exit 1 ;;
esac
[ "$(uname -s)" = "Linux" ] || { echo "fehler: nur Linux wird unterstützt"; exit 1; }

command -v curl >/dev/null 2>&1 || { echo "fehler: curl fehlt (apt install curl)"; exit 1; }
command -v tar >/dev/null 2>&1 || { echo "fehler: tar fehlt"; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo "fehler: sha256sum fehlt (coreutils)"; exit 1; }

if [ "$USER_MODE" = "1" ]; then
  DEST_DIR="$HOME/.local/bin"
else
  [ "$(id -u)" = "0" ] || { echo "fehler: bitte mit sudo/root laufen lassen (oder --user nutzen)"; exit 1; }
  DEST_DIR="/usr/local/bin"
fi
mkdir -p "$DEST_DIR"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

TARBALL="lokronet_${VERSION}_linux_${GOARCH}.tar.gz"
echo "[lokronet] lade $BASE/$TARBALL ..."
curl -fsSL -o "$TMP/$TARBALL" "$BASE/$TARBALL"
echo "[lokronet] lade $BASE/checksums.txt ..."
curl -fsSL -o "$TMP/checksums.txt" "$BASE/checksums.txt"

echo "[lokronet] verifiziere sha256 ..."
(cd "$TMP" && sha256sum -c checksums.txt --ignore-missing --status) \
  || { echo "fehler: Checksumme stimmt NICHT – Abbruch (evtl. manipulierter Download)"; exit 1; }
# Zusatzcheck: exakt diese Datei muss in checksums.txt stehen.
grep -q "$TARBALL" "$TMP/checksums.txt" || { echo "fehler: $TARBALL fehlt in checksums.txt"; exit 1; }

tar -xzf "$TMP/$TARBALL" -C "$TMP"
install -m 0755 "$TMP/lokronet_linux_${GOARCH}" "$DEST_DIR/lokronet"
echo "[lokronet] installiert: $DEST_DIR/lokronet"
"$DEST_DIR/lokronet" version

# --- rendezvous als System-Service (braucht keine Identität, server-seitig ok) ---
if [ "$WITH_RENDEZVOUS" = "1" ]; then
  [ "$USER_MODE" = "0" ] || { echo "fehler: --rendezvous braucht System-Install (ohne --user)"; exit 1; }
  cat > /etc/systemd/system/lokro-rendezvous.service <<'EOF'
[Unit]
Description=LokroNet Rendezvous (Discovery + Signaling, keine Nutzdaten)
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/lokronet rendezvous --addr 0.0.0.0:8787
DynamicUser=yes
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now lokro-rendezvous
  echo "[lokronet] lokro-rendezvous läuft (127.0.0.1:8787 – hinter Reverse-Proxy/TLS legen, siehe docs/INSTALL.md)"
fi

# --- daemon als User-Service (läuft mit DEINER Identität, nicht root) ---
if [ "$WITH_DAEMON" = "1" ]; then
  command -v systemctl >/dev/null 2>&1 || { echo "fehler: systemctl fehlt"; exit 1; }
  UNIT_DIR="$HOME/.config/systemd/user"
  mkdir -p "$UNIT_DIR"
  BIN="$DEST_DIR/lokronet"
  cat > "$UNIT_DIR/lokronet-daemon.service" <<EOF
[Unit]
Description=LokroNet Daemon (Mesh-Backend)
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=$BIN daemon
NoNewPrivileges=yes
PrivateTmp=yes
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF
  systemctl --user daemon-reload 2>/dev/null || true
  echo "[lokronet] User-Service angelegt. Aktivieren mit:"
  echo "  systemctl --user enable --now lokronet-daemon"
  echo "  (ggf. vorher: loginctl enable-linger \$USER für Autostart ohne Login)"
fi

echo ""
echo "[lokronet] fertig. Nächste Schritte (ALS DEIN USER, nicht root):"
echo "  lokronet setup --rendezvous https://net.lokro.dev"
echo "  lokronet debug on"
echo "  lokronet daemon   # oder User-Service, siehe oben"
