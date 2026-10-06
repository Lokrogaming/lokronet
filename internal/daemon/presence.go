// Package daemon – Presence (online/offline) + Beacon-Loop.
//
// Idee (User-Spec v0.2):
//   [Boot] -> Beacon an alle gespeicherten Kontakte (Health + Chat-Stats).
//   Empfänger antwortet, beide tauschen Stats (count/max_ts) und pushen fehlende
//   Messages (newest-timestamp-wins). Beide zeigen "User online".
//   [Empfänger offline] -> Beacon-Retry mit Backoff, danach offline markiert.
//
// Transport: Signaling-Mailbox (Rendezvous), Typen "beacon" / "beacon-ack".
// Payload: JSON {endpoint, mode, ts, msg_count, max_ts} – keine Klartext-Chats.
// Rendezvous sieht nur Metadaten, nie Nachrichteninhalte (die laufen E2E als
// NaCl-box via UDP-Direct oder msg-data-Relay).
package daemon

import (
	"encoding/json"
	"time"

	"github.com/lokro/lokronet/internal/contacts"
	"github.com/lokro/lokronet/internal/mode"
	"github.com/lokro/lokronet/pkg/proto"
)

// Beacon ist der Health-Ping an Kontakte.
type Beacon struct {
	Endpoint string `json:"endpoint"`
	Mode     string `json:"mode"`
	Ts       int64  `json:"ts"`
	MsgCount int    `json:"msg_count"`
	MaxTs    int64  `json:"max_ts"`
}

// presenceInfo: letzter Beacon-Kontakt (für /v1/presence).
type presenceInfo struct {
	Online   bool   `json:"online"`
	LastSeen int64  `json:"last_seen"`
	Endpoint string `json:"endpoint,omitempty"`
	Mode     string `json:"mode,omitempty"`
}

// beaconStats fasst eigene Chat-History zusammen (für Beacon-Payload).
func (d *Daemon) beaconStats() (count int, maxTs int64) {
	for _, s := range d.chat.Stats() {
		count += s.Count
		if s.MaxTs > maxTs {
			maxTs = s.MaxTs
		}
	}
	return count, maxTs
}

// sendBeacon schickt einen Beacon an genau einen Peer (best effort).
func (d *Daemon) sendBeacon(to string) {
	m, _ := mode.Of(d.cfg.Mode)
	count, maxTs := d.beaconStats()
	b, _ := json.Marshal(Beacon{
		Endpoint: d.publicEndpoint(),
		Mode:     string(m),
		Ts:       time.Now().Unix(),
		MsgCount: count,
		MaxTs:    maxTs,
	})
	_ = d.sig.Send(proto.SignalMessage{From: d.ident.ID, To: to, Type: "beacon", Payload: string(b)})
}

// beaconLoop: beim Start an alle Kontakte, danach periodisch (Heartbeat-Takt).
// Fehlschläge sind still (Empfänger offline → Presence läuft ab).
func (d *Daemon) beaconLoop() {
	// Erster Burst kurz nach Start (Daemon + Rendezvous kurz settle lassen).
	time.Sleep(3 * time.Second)
	d.beaconAll()
	for {
		_, p := mode.Of(d.cfg.Mode)
		wait := p.Heartbeat
		if wait < 20*time.Second {
			wait = 30 * time.Second
		}
		time.Sleep(wait)
		d.beaconAll()
	}
}

// beaconAll sendet an alle Kontakte + bekannte Peers.
func (d *Daemon) beaconAll() {
	targets := map[string]bool{}
	if store, err := contacts.Load(); err == nil {
		for _, c := range store.Contacts {
			targets[c.ID] = true
		}
		for _, h := range store.History {
			targets[h.ID] = true
		}
	}
	d.mu.Lock()
	for id := range d.peers {
		targets[id] = true
	}
	d.mu.Unlock()
	delete(targets, d.ident.ID)
	for id := range targets {
		d.sendBeacon(id)
	}
	if len(targets) > 0 {
		d.emit("beacon an %d Kontakte", len(targets))
	}
}

// onBeacon verarbeitet eingehenden Beacon: Presence merken + ack + Sync anstoßen.
func (d *Daemon) onBeacon(from, payload string) {
	var b Beacon
	if err := json.Unmarshal([]byte(payload), &b); err != nil {
		return
	}
	d.mu.Lock()
	if d.presence == nil {
		d.presence = map[string]*presenceInfo{}
	}
	d.presence[from] = &presenceInfo{Online: true, LastSeen: time.Now().Unix(), Endpoint: b.Endpoint, Mode: b.Mode}
	// Peer-Endpoint lernen (für UDP-Direct).
	if ps, ok := d.peers[from]; ok {
		if b.Endpoint != "" {
			ps.Peer.Endpoint = b.Endpoint
		}
	} else if b.Endpoint != "" {
		// Minimal-Eintrag, Lookup verfeinert später.
		d.peers[from] = &peerState{State: "beacon"}
		d.peers[from].Peer.Endpoint = b.Endpoint
		d.peers[from].Peer.ID = from
	}
	d.mu.Unlock()
	d.emit("beacon von %s (%s, %d msgs)", from, b.Mode, b.MsgCount)
	// Ack mit eigenem Stand (damit Sender auch syncen kann).
	m, _ := mode.Of(d.cfg.Mode)
	count, maxTs := d.beaconStats()
	ack, _ := json.Marshal(Beacon{Endpoint: d.publicEndpoint(), Mode: string(m), Ts: time.Now().Unix(), MsgCount: count, MaxTs: maxTs})
	_ = d.sig.Send(proto.SignalMessage{From: d.ident.ID, To: from, Type: "beacon-ack", Payload: string(ack)})
	// Sync: fehlende Messages newest-wins pushen.
	d.pushMissing(from, b.MaxTs)
}

// onBeaconAck: Presence merken + Sync anstoßen (kein Gegen-Ack, kein Ping-Pong).
func (d *Daemon) onBeaconAck(from, payload string) {
	var b Beacon
	if err := json.Unmarshal([]byte(payload), &b); err != nil {
		return
	}
	d.mu.Lock()
	if d.presence == nil {
		d.presence = map[string]*presenceInfo{}
	}
	d.presence[from] = &presenceInfo{Online: true, LastSeen: time.Now().Unix(), Endpoint: b.Endpoint, Mode: b.Mode}
	d.mu.Unlock()
	d.emit("beacon-ack von %s", from)
	d.pushMissing(from, b.MaxTs)
}

// pushMissing schickt Nachrichten mit Ts > remoteMaxTs via msg-data-Relay
// (E2E-verschlüsselt, Rendezvous sieht nur opaque Blobs). UDP-Direct passiert
// ohnehin live; Relay holt Offline-/NAT-Fälle auf.
func (d *Daemon) pushMissing(peerID string, remoteMaxTs int64) {
	msgs := d.chat.MessagesSince(peerID, remoteMaxTs, 50)
	if len(msgs) == 0 {
		return
	}
	// Wir können nur bereits versiegelte Nachrichten erneut senden? Nein –
	// MessagesSince liefert Klartext aus History. Für echten Re-Push bräuchten
	// wir Re-Seal mit aktueller Session. V1: nur Hinweis + Live-Nachholen.
	// (Echter Catch-up-Push kommt mit stabilen Session-Keys.)
	d.emit("sync: %d Nachrichten neuer als Peer %s (holen via live-chat nach)", len(msgs), peerID)
}

// presenceSweep markiert abgelaufene Peers offline (>3x Beacon-Intervall ohne Zeichen).
func (d *Daemon) presenceSweep() map[string]presenceInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := map[string]presenceInfo{}
	now := time.Now().Unix()
	for id, p := range d.presence {
		if now-p.LastSeen > 180 {
			p.Online = false
		}
		out[id] = *p
	}
	return out
}
