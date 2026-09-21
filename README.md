# LokroNet

Eigenes Meshnet mit Terminal-CLI: Peers per ID verbinden, Tunnel verschlüsselt aufbauen,
später Dateien teilen und per TUI verwalten.

> **Status:** Idee + Planung. Noch kein lauffähiger Code.
> Festgelegt: **Go, Windows + Linux, Hybrid-Modell (Rendezvous + direktes P2P via WireGuard),
> 12-stellige ID als Adresse + Ed25519-Auth, MVP = nur Mesh + Debug-Meldungen.**

## Idee in kurz

1. Paket auf Server / PC laden (`apt install lokronet`, `winget`, Docker – geplant)
2. `lokronet setup` -> bekommt random ID (12 Ziffern `0-9`) + lokales Keypair
3. `lokronet connect --id <ID>` + Gegenstelle bestätigt Fingerprint -> verschlüsselter Tunnel
4. Später: `filemanager`-Shares, Remote-Terminal, TUI per `lokronet` ohne Args

Details und offene Fragen: siehe [`docs/IDEEN.md`](docs/IDEEN.md).

## Geplante CLI (MVP zuerst)

```powershell
lokronet setup              # ID + Keys erzeugen, am Rendezvous registrieren
lokronet status             # ID, Fingerprint, Peers, Tunnel-Status
lokronet connect --id <ID>  # Pairing-Anfrage an Peer
lokronet ping --id <ID>     # E2E-Testnachricht durch den Tunnel
lokronet debug on|off       # Bestätigungs-/Debugmeldungen im Terminal an/aus
lokronet logs --tail 50
```

Später (noch **nicht** im MVP):

```powershell
lokronet filemanager trusted add -id <ID> -p <path\to\file-or-folder>
lokronet                    # öffnet TUI (wie OpenCode Terminal-UI)
```

## Sicherheit in einem Satz

Die 12-stellige ID ist nur die Adresse, **niemals das Secret**.
Auth läuft über lokale Ed25519-/WireGuard-Keys + manuelle Fingerprint-Freigabe,
der gesamte Mesh-Traffic läuft E2E-verschlüsselt (Pflicht, siehe `docs/IDEEN.md`).

## Geplante Projektstruktur (Go, noch nicht gebaut)

```text
cmd/lokronet/          # CLI-Einstieg (cobra)
cmd/rendezvous/        # Signaling-/Discovery-Server für VPS
internal/identity/     # setup, ID-Gen, Keys
internal/daemon/       # Hintergrunddienst, IPC via localhost-Socket
internal/netcore/      # Portwahl, UPnP/NAT-PMP, STUN, Hole-Punching
internal/wg/           # wireguard-go Wrapper, Firewall-Regeln
internal/signal/       # HTTPS/WSS-Client + Server (v1)
internal/debug/        # ping, logs, debug on/off
pkg/proto/             # versionierte Protokoll-Typen (v1)
docs/                  # IDEEN.md, Architektur-Notizen
```

## Installation (Ziel, noch nicht fertig)

* Windows: `winget install lokronet` / MSI (geplant)
* Linux: `apt install lokronet` über eigenes signiertes APT-Repo (geplant)
* Server/Docker-Image für `lokro-rendezvous` (geplant)

## Hinweis zum Ordner

Alter Arbeitsordner `lokroAccounts` wurde durch `lokronet` ersetzt.
Falls `lokroAccounts` bei dir noch als leerer Ordner existiert (war beim
Umbennen gesperrt), kannst du ihn nach Schließen aller Terminals manuell löschen.
