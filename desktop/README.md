# LokroNet Messenger – Desktop-App (C# + Avalonia)

Cross-platform Desktop-UI für Windows + Ubuntu/Linux. **Kein zweites Chat-Protokoll:**
die App spricht nur mit dem lokalen Go-Daemon über `127.0.0.1:<port> + X-Lokro-Token`
(`~/.lokronet/daemon.token`, Port aus `~/.lokronet/daemon.ipc`, Fallback `37777`).

- Wer hier schreibt (`POST /v1/chat/send`), sieht es sofort in der TUI
  (`lokronet dashboard`) und CLI (`lokronet chat inbox`) – und umgekehrt.
- History liegt im Daemon: `~/.lokronet/chat-history.json` (0600, cap 5000).
- Wire immer E2E (NaCl-box, signierter X25519-Handshake, Session-Code vergleichen).

## Start

```powershell
# Daemon muss laufen (Boot-Beacon geht automatisch an alle Kontakte):
lokronet daemon
# oder: lokronet beacon   # manuell

cd desktop/LokroNet.Desktop
dotnet run
```

## Publish

```powershell
# Windows x64 (Einzeldatei):
dotnet publish -c Release -r win-x64 --self-contained -p:PublishSingleFile=true -o publish/win-x64

# Ubuntu/Linux x64 (auf Linux oder mit RuntimeIdentifier bauen):
dotnet publish -c Release -r linux-x64 --self-contained -p:PublishSingleFile=true -o publish/linux-x64
./publish/linux-x64/LokroNet.Desktop
```

Voraussetzung Linux: `sudo apt install -y libice6 libsm6 libfontconfig1` (Avalonia-X11-Deps,
je nach Distro) + .NET 9 Runtime nur bei framework-dependent Publish nötig
(self-contained bringt sie mit).

## App-Icon (Logo)

Das Logo liegt **nicht** im Repo (Binary), sondern wird an diese Pfade gelegt:

- `desktop/LokroNet.Desktop/Assets/icon.png` – Quelle (dein Logo als PNG)
- `desktop/LokroNet.Desktop/Assets/icon.ico` – Windows-Exe-Icon (wird aktiv,
  sobald die Datei existiert – `ApplicationIcon` in der `.csproj` ist bedingt)

ICO erzeugen (eine der Optionen):

```powershell
# per Pillow (pip install pillow):
python -c "from PIL import Image; Image.open('Assets/icon.png').save('Assets/icon.ico', sizes=[(16,16),(32,32),(48,48),(256,256)])"
# oder: online-Konverter (PNG -> ICO), Datei nach Assets/icon.ico legen
```

Danach neu bauen/publishen – Exe, Taskleiste und Fenster nutzen das Icon.
Linux-Desktop (`.desktop`-Datei + `icon.png`) folgt mit der Linux-App.

## IPC-Referenz (genutzt)

- `GET /v1/status` – eigene ID, Daemon online?
- `GET /v1/chat/sessions` – Sessions + Session-Code
- `GET /v1/chat/inbox` – Verlauf (geteilt mit TUI)
- `POST /v1/chat/send {"id","text"}` – senden (`sent (direct)` / `sent (relay)` / `queued`)
- `GET /v1/presence` – online/offline aus Beacons
- `POST /v1/beacon {"id"}` – Beacon anstoßen (`""` = alle)
