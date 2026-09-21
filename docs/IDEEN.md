# LokroNet – Ideen-Speicher

Hier halten wir alle Ideen fest + jeweils wie wir sie am besten lösen wollen.
Status-Legende: `MVP` = zuerst bauen · `P1/P2/P3` = später · `Pflicht` = nicht verhandelbar.

Übersicht:

| # | Idee | Status |
|---|------|--------|
| 1 | Mesh-Tunnel mit Portfreigabe + abgeschottetem Bereich | MVP |
| 2 | `setup` mit 12-stelliger ID + Key-Auth | MVP |
| 3 | Debug-/Bestätigungsnachrichten per Command schaltbar | MVP |
| 4 | Datei-/Ordner-Sharing (`filemanager trusted add`) | P2 |
| 5 | Remote-Terminal / Editor über Mesh | P3 |
| 6 | Eigene TUI per `lokronet` (wie OpenCode) | P3 |
| 7 | Verteilung als Paket (`apt`, `winget`, Docker) | P1 |
| 8 | Sicherheitsmechanismen gegen Hacker | Pflicht (ab MVP) |
| 9 | Mesh-Traffic immer encrypted (v.a. Datei-Downloads) | Pflicht (ab MVP, bei Files nochmal verstärkt) |

---

## 1. Mesh-Tunnel (MVP)

**Idee:** Zwei Rechner finden sich über ID und bauen einen direkten Tunnel,
ohne dass Nutzdaten über einen zentralen Server laufen.

**So lösen:**
* Hybrid: kleiner `lokro-rendezvous`-Server nur für Discovery + Signaling
  (`register`, `heartbeat`, `lookup`, WebSocket-Signalkanal).
  Nutzdaten immer direkt Peer-zu-Peer via `wireguard-go` (Userspace, kein Kernel-Treiber nötig).
* Port-Logik: Config-Port (z.B. `51820`) versuchen, sonst freien UDP-Port aus
  `49152-65535` suchen. Dann `UPnP -> NAT-PMP/PCP -> manuell`, Lease erneuern
  und beim Beenden freigeben. Kein wahlloses Scannen öffentlicher Ports.
* Erreichbarkeit: STUN für reflexive Adresse, dann UDP-Hole-Punching
  (beide Seiten senden gleichzeitig). Wenn das an CGNAT/DS-Lite scheitert:
  im MVP ehrliche Fehlermeldung im Debug-Log, in P1 DERP-/TURN-Relay als Fallback.
* Abgeschotteter Bereich: eigenes Interface `lokro0` (`10.77.0.0/16`),
  Default-Deny-Firewall (nur 1x UDP-Port + ESTABLISHED + `lokro0`),
  eigene Routing-Table nur für Mesh, kein Zugriff aufs Host-LAN.
  Linux: NetNS, Windows: WFP-Filter + Restricted Token / Job Object für Tunnel-Worker.

## 2. Setup + 12-stellige ID (MVP)

**Idee:** `lokronet setup` gibt random ID (12 chars `0-9`) zum Connecten.

**So lösen:**
* ID per `crypto/rand`, 12x `0-9`. Kollisionscheck gegen Rendezvous
  (`409 Conflict` -> neu würfeln).
* Wichtig: ID hat nur ~40 Bit Entropie -> **ID ist nur die Adresse, nie das Secret.**
* Beim Setup zusätzlich lokal erzeugen: Ed25519 (Identität) + Curve25519 (WireGuard).
  Private Keys nur in `~/.lokronet/` (`0600` / Windows DPAPI + ACL), nie übertragen.
* Registriert wird `{id, ed25519_pub, wg_pub, fingerprint}`.
* `connect --id` stellt nur Kontakt her. Gegenseite muss mit
  `trust approve --id <ID> --fp <HASH>` Fingerprint bestätigen (TOFU + out-of-band).
  Erst danach wird der WG-Peer konfiguriert. Brute-Force auf die ID bringt ohne
  Key + Freigabe nichts.

## 3. Debug-/Bestätigungsnachrichten (MVP)

**Idee:** Für Debugging soll man Bestätigungsnachrichten im Terminal per Command
ein-/ausschalten können.

**So lösen:**
* `lokronet debug on|off` schreibt `debug: true/false` in `~/.lokronet/config.yaml`.
* Daemon pusht Events auf localhost-Socket (nie ins Netz):
  `[lokro] punch 123… -> 84.x.x.x:51234 OK (42ms)`,
  `[lokro] wg handshake complete fp:ab:cd…`,
  `[lokro] ping ack id=… nonce=…`.
* `lokronet status --watch` / `lokronet logs --tail` zeigen sie an.
  Default `off`. Nie Keys oder Dumps loggen.

## 4. Datei-/Ordner-Sharing (P2, später)

**Idee:** `lokronet filemanager trusted add -id [ID] -p [path\to\file-or-folder]`
gibt etwas frei, die andere Person kann laden / bearbeiten / hochladen wie im
Terminal/Editor.

**So lösen (vorgemerkt, noch nicht bauen):**
* Share-Jail: nur freigegebene Pfade sichtbar, Canonicalisierung +
  Symlink-/Junction-Check, keine `..`-Escapes, Quota + Read/Write-Rechte pro ID.
* Protokoll: `ls/get/put/stat/watch` über den bestehenden WG-Tunnel (mTLS),
  Chunk-Hashes + Resume, Audit-Log wer was wann geändert hat.
* Stufen: `trusted` (lesen+schreiben) vs. später `readonly`, `revoke` pro ID.

## 5. Remote-Terminal / Editor (P3, später)

**Idee:** Auf freigegebenen Shares arbeiten wie in normalem Terminal/Editor.

**So lösen (vorgemerkt):**
* Kein offenes SSH, sondern eigene Shell über Mesh mit Mutual-Auth,
  pro Command Whitelist/Policy, Session-Timeout, komplettes Audit-Log.
* Editor zuerst als `get/edit/put`-Flow, erst danach echtes interaktives Remote-Terminal.

## 6. Eigene UI wie OpenCode-TUI (P3, später)

**Idee:** `lokronet` ohne Args öffnet eine Terminal-UI (Peers, Shares, Logs).

**So lösen (vorgemerkt):**
* In Go mit `bubbletea` (passt zu CLI, keine Electron-Abhängigkeit).
  Nutzt dieselbe localhost-Socket-API wie die CLI, also kein neues Sicherheitsloch.
* Ansichten: Peers, Tunnel-Status, Shares, Debug-Log, Setup-Wizard für neue IDs.

## 7. Paketverteilung (P1)

**Idee:** Paket auf Server / PC laden und per `apt`/`pkg` installierbar machen.

**So lösen:**
* Build mit `goreleaser`: `.deb` (apt), `.rpm`, `.tar.gz`, Windows `.msi/.zip`, Docker.
* `apt`: eigenes signiertes APT-Repo auf VPS (`aptly`/`reprepro` + Nginx + GPG):
  `apt update; apt install lokronet`. Paketsignatur per Cosign/Sigstore prüfen.
* `pkg` (FreeBSD) / AUR erst nach `.deb` + MSI. Auto-Update-Check mit Hash-Verifikation.
* Kein Admin nötig außer beim ersten `setup` fürs WG-Interface.

## 8. Sicherheit gegen Hacker (Pflicht ab MVP)

* mTLS 1.3 + WireGuard mit PFS, Nonce-/Replay-Schutz, Key-Rotation (7 Tage / bei `revoke`).
* Rendezvous: Rate-Limit (`register`/`lookup`), Fail2Ban nach falschen Fingerprints,
  keine Nutzdaten speichern, nur `{id, pubkeys, endpoint, last-seen}`.
* Daemon: least-privilege, nur 1 offener UDP-Port, CLI spricht nur via
  localhost-Socket mit Auth-Token mit dem Daemon.
* `lokronet reset --revoke` für Key-Verlust. Alles versioniert (`/v1/`), damit wir
  Protokoll-Bugs ohne Break fixen können.

## 9. PFLICHT: Mesh-Traffic immer encrypted (v.a. Datei-Downloads)

> **Nicht verhandelbar:** Jeglicher Mesh-Traffic – Handshake, Ping, und später
> Datei-Downloads/Uploads und Remote-Shell – muss Ende-zu-Ende verschlüsselt sein,
> damit nichts mitgeschnitten / abgegriffen werden kann.

**So lösen:**
* Ab MVP: Transport nur über WireGuard (`Curve25519` + `ChaCha20-Poly1305`),
  kein Plaintext-Fallback, kein unverschlüsseltes Relay.
* Für P2 (Files, vorgemerkt): zusätzlich pro Chunk Hash + Signatur,
  Resume nur nach Verifikation, optional doppelte Schicht (z.B. `age`/Noise
  auf Datei-Ebene), damit selbst ein kompromittierter Relay-Server nichts lesen kann.
* Debug-Logs enthalten nie Klartext-Inhalte, nur Metadaten (ID, Bytes, Dauer).

---

## Offene Punkte (noch zu entscheiden)

* [ ] Rendezvous in diesem Repo (`cmd/rendezvous`) oder separates Repo für VPS?
* [ ] Eigene STUN-/DERP-Server oder vorerst öffentliche + später eigene?
* [ ] Config-Format final: `~/.lokronet/config.yaml` ok?
