# LokroNet Backend (MVP-Skelett)

> Stand: lauffähiges Grundgerüst, stdlib-only (keine `go mod download`-Deps).
> UDP-Ping + Signaling sind bewusst noch **unverschlüsselt und nur für lokale Tests**.
> E2E-Pflicht (Noise/WireGuard) kommt als nächster Schritt, siehe `docs/IDEEN.md` #9.

## Was das Backend heute kann

* `internal/identity` – 12-stellige ID + Ed25519-Keys, Fingerprint, `~/.lokronet/` (0600)
* `internal/config` – Config + localhost-IPC-Token
* `internal/netcore` – genau 1 UDP-Port (Wunsch oder frei aus 49152–65535)
* `internal/signal` – Rendezvous-Server: `register`, `heartbeat`, `lookup`,
  Mailbox + Long-Poll (`GET /v1/signal?to=ID&wait=ms`) als MVP-Realtime-Kanal
* `internal/daemon` – Backend-Kern: UDP-Loop (punch/ping/pong), Heartbeat-Loop,
  Signal-Poll-Loop, localhost-IPC `:37777` mit Token (`status`, `connect`, `ping`, `debug`, `events`)
* `cmd/lokronet` – dünne CLI, ruft den Daemon über IPC (Fallback: lokale Dateien bei `status`/`debug`)

## Voraussetzungen

Go-Toolchain (noch **nicht** auf diesem Rechner installiert – `go version` scheitert).
Installieren, danach hier weiter:

```powershell
winget install -e --id Go.Go
go version
```

## Lokaler Test (3 Terminals)

```powershell
# Terminal 1: Rendezvous (lokal)
go run ./cmd/lokronet rendezvous

# Terminal 2: Setup + Daemon
go run ./cmd/lokronet setup
go run ./cmd/lokronet debug on
go run ./cmd/lokronet daemon

# Terminal 3: Status / Logs
go run ./cmd/lokronet status
go run ./cmd/lokronet logs --tail 20
```

Zwei Rechner/IDs testen (zweite Identität = zweites Windows-Profil oder zweiter PC
mit gleichem Rendezvous): `lokronet connect --id <ID>` (wird `pending`, Fingerprint
im Log vergleichen), dann `lokronet ping --id <ID>`.

## Nächste Schritte (noch nicht gebaut)

1. E2E-Verschlüsselung (Noise + `wireguard-go`, `lokro0`-Interface, Firewall) – Pflicht vor Filesharing
2. `trust approve --id --fp` statt auto-pending
3. Rendezvous-Persistenz (SQLite), Rate-Limit, HTTPS/WSS statt HTTP/Long-Poll
4. `goreleaser` (`.deb`/MSI/Docker) + signiertes APT-Repo
