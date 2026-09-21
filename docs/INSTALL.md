# LokroNet auf Linux installieren (via net.lokro.dev)

## Für Nutzer (auf dem Linux-Server/PC)

Voraussetzungen: 64-Bit-Linux (amd64 oder arm64), `curl`, Honduras keine.
Frische Go-Installation ist **nicht** nötig – das Script lädt das fertige Binary.

```bash
# Nur das Binary (nach /usr/local/bin, braucht sudo)
curl -fsSL https://net.lokro.dev/install | sudo bash

# Mit Version pinnen
curl -fsSL https://net.lokro.dev/install | sudo bash -s -- --version=0.1.0

# Ohne root, nach ~/.local/bin
curl -fsSL https://net.lokro.dev/install | bash -s -- --user

# Server-Modus: zusätzlich Rendezvous als System-Service
curl -fsSL https://net.lokro.dev/install | sudo bash -s -- --rendezvous

# Daemon als Autostart (läuft mit deiner Identität, nicht root)
curl -fsSL https://net.lokro.dev/install | bash -s -- --daemon
```

Danach (als dein User, **nicht** root):

```bash
lokronet version
lokronet setup --rendezvous https://net.lokro.dev
lokronet debug on
lokronet daemon
```

Prüfung in zweitem Terminal: `lokronet status`, `lokronet logs --tail 20`.

Sicherheit: das Script verifiziert jeden Download per `sha256sum` gegen
`checksums.txt` und bricht bei Mismatch ab. Prüfsummen zusätzlich out-of-band
vergleichen (Release-Seite), bevor du `| sudo bash` ausführst.

## Für dich (net.lokro.dev einmalig einrichten)

1. Release bauen (hier im Repo, Go-Toolchain reicht):

   ```powershell
   # Windows-Powershell:
   $v = (Get-Content VERSION).Trim()
   $env:GOOS="linux"; $env:GOARCH="amd64"
   go build -trimpath -ldflags "-s -w -X main.Version=$v" -o dist/lokronet_linux_amd64 ./cmd/lokronet
   $env:GOARCH="arm64"
   go build -trimpath -ldflags "-s -w -X main.Version=$v" -o dist/lokronet_linux_arm64 ./cmd/lokronet
   # (oder auf Linux einfach: make build-linux checksums)
   ```

2. Auf dem Webserver ablegen:

   ```bash
   # auf dem net.lokro.dev-Server:
   sudo mkdir -p /srv/lokro-web/dl
   # dist/*.tar.gz + checksums.txt hierher kopieren (scp/rsync)
   sudo cp deploy/install.sh /srv/lokro-web/install
   sudo cp deploy/nginx-net.lokro.dev.conf /etc/nginx/sites-available/net.lokro.dev
   sudo ln -sf /etc/nginx/sites-available/net.lokro.dev /etc/nginx/sites-enabled/
   sudo nginx -t && sudo systemctl reload nginx
   sudo certbot --nginx -d net.lokro.dev   # TLS (Pflicht, sonst MITM beim Install)
   ```

3. Rendezvous auf dem Server starten (eine der Varianten):

   ```bash
   # Variante A: per Installer
   curl -fsSL https://net.lokro.dev/install | sudo bash -s -- --rendezvous
   # Variante B: lokronet läuft schon -> Unit von Hand
   sudo cp deploy/systemd/lokro-rendezvous.service /etc/systemd/system/
   sudo systemctl daemon-reload && sudo systemctl enable --now lokro-rendezvous
   ```

4. Test von einem frischen Linux-Rechner/Container:

   ```bash
   curl -fsSL https://net.lokro.dev/install | sudo bash
   lokronet setup --rendezvous https://net.lokro.dev
   ```

Hinweis: `deploy/install.sh` ist die Quelle der Wahrheit für `/install`;
`deploy/systemd/*.service` für die Units (der Installer enthält Kopien davon inline –
nach Änderungen dort auch die Inline-Kopien in `install.sh` aktualisieren).
