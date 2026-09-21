# LokroNet auf Linux installieren

## Für Nutzer (auf dem Linux-Server/PC)

Voraussetzungen: 64-Bit-Linux (amd64 oder arm64), `curl`. Kein Go nötig –
das Script lädt das fertige Binary aus den GitHub-Releases und prüft sha256.

```bash
# Standard (Binary nach /usr/local/bin, braucht sudo)
curl -fsSL https://net.lokro.dev/install | sudo bash

# Version pinnen (folgt automatisch dem Release-Tag vX.Y.Z)
curl -fsSL https://net.lokro.dev/install | sudo bash -s -- --version=0.1.0

# Ohne root, nach ~/.local/bin
curl -fsSL https://net.lokro.dev/install | bash -s -- --user

# Rendezvous gleich als System-Service mitnehmen (nur auf Servern)
curl -fsSL https://net.lokro.dev/install | sudo bash -s -- --rendezvous

# Daemon als Autostart (läuft mit deiner Identität, nicht root)
curl -fsSL https://net.lokro.dev/install | bash -s -- --daemon
```

Bis `net.lokro.dev` per DNS live ist, geht ersatzweise direkt:

```bash
curl -fsSL https://raw.githubusercontent.com/Lokrogaming/lokronet/main/deploy/install.sh | sudo bash
```

Danach (als dein User, **nicht** root):

```bash
lokronet setup --rendezvous http://DEIN-RENDEZVOUS-HOST:8787
lokronet debug on
lokronet daemon
```

Sicherheit: Downloads werden per `sha256sum` gegen `checksums.txt` verifiziert,
Abbruch bei Mismatch.

## Hosting: net.lokro.dev (GitHub Pages, statisch)

Pages liefert nur statische Dateien – das reicht für `/install` und die
Landingpage. Binaries kommen aus den GitHub-Releases.

* Quelle: `docs/` auf `main` (`.nojekyll`, `CNAME`, `index.html`,
  `install` = Kopie von `deploy/install.sh`, sync per `make pages`).
* Repo → Settings → Pages → Deploy from branch → `main` + `/docs`
  (alternativ schon per API aktiviert, siehe unten).
* DNS beim Provider: CNAME `net` → `Lokrogaming.github.io`, dann in den
  Pages-Settings „Enforce HTTPS“ aktivieren (Let's Encrypt, automatisch).
* Test: `curl -fsSL https://net.lokro.dev/install | bash -s -- --help`

## Rendezvous auf dem VPS (z.B. DiscordBot)

Das Rendezvous braucht einen echten Server (Pages kann kein Signaling).
Auf dem Server:

```bash
curl -fsSL https://net.lokro.dev/install | sudo bash -s -- --rendezvous
systemctl status lokro-rendezvous --no-pager
curl http://127.0.0.1:8787/healthz   # muss "ok" sagen
```

Firewall öffnen (sonst kommt kein Peer durch):

```bash
ufw allow 8787/tcp       # Signaling
ufw allow 51820/udp      # Mesh-UDP (Default-Port, je Peer ggf. mehr)
```

Clients nutzen dann `--rendezvous http://SERVER-IP-O-DOMAIN:8787`.
MVP-Hinweis: dieses HTTP ist unverschlüsselt – nur für Tests im vertrauten
Netz. TLS fürs Rendezvous (eigene Subdomain + Reverse-Proxy) kommt mit P1.
