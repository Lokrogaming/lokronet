// Package chat: Session-Messenger über den Mesh-Port (nur via Dashboard).
//
// Design (Beta, siehe docs/IDEEN.md #11):
//   - Pro Peer-Paar genau eine Session mit EPHEMEREN X25519-Keys.
//     Handshake läuft über den Signaling-Kanal (msg-hello / msg-hello-ack),
//     Nutzdaten als "lokro-msg:<base64>" über UDP.
//   - Jede Nachricht: NaCl-box (XSalsa20-Poly1305) mit Nonce aus
//     (Sequenz + Sender-Rolle). Beide Seiten zeigen einen Session-Code
//     (aus dem Shared Secret) zum out-of-band Vergleich.
//   - Verlauf nur im RAM (Inbox, gecappt), kein Klartext in Logs.
//   - Replay-Schutz: Sequenz muss strikt steigen, sonst Drop.
//
// Regel: Hooks werden NIE unter Lock gerufen (kein Re-Entrance-Deadlock) –
// Methoden sammeln Aktionen unter Lock und führen sie danach aus.
package chat

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/nacl/box"
)

// MaxTextLen hält Pakete unter der MTU (JSON + Box-Overhead eingerechnet).
const MaxTextLen = 800

// MaxInbox cappt den RAM-Verlauf.
const MaxInbox = 500

// Prefix des UDP-Transports.
const UDPPrefix = "lokro-msg:"

// Message ist eine Chat-Nachricht (Inbox, RAM only).
type Message struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Text     string `json:"text"`
	Ts       int64  `json:"ts"`
	Outgoing bool   `json:"outgoing"`
	Seq      uint64 `json:"seq"`
}

// Session-Info für UI/Status (ohne Secrets).
type Session struct {
	PeerID  string `json:"peer_id"`
	Code    string `json:"code"`
	Ready   bool   `json:"ready"`
	Pending int    `json:"pending"`
	Updated int64  `json:"updated_at"`
}

type session struct {
	peerID    string
	myPriv    [32]byte
	myPub     [32]byte
	peerPub   [32]byte
	hasPeer   bool
	initiator bool
	helloSent bool
	ready     bool
	shared    [32]byte
	code      string
	outSeq    uint64
	inSeq     uint64
	pending   []string
	updated   time.Time
}

// Hooks verbindet den Manager mit Daemon-Netz. Alle Fehler tolerant.
// Werden immer OHNE Manager-Lock gerufen.
type Hooks struct {
	// SendSignal stellt eine Signaling-Nachricht zu (Typ msg-hello/-ack).
	SendSignal func(to, typ, payload string)
	// SendUDP schickt fertige Bytes an den Peer-Endpoint.
	SendUDP func(endpoint string, data []byte) error
	// LookupEndpoint löst eine Peer-ID in "ip:port" auf.
	LookupEndpoint func(peerID string) (string, bool)
}

// Manager hält Sessions + Inbox eines lokalen Peers.
type Manager struct {
	mu       sync.Mutex
	selfID   string
	sessions map[string]*session
	inbox    []Message
	hooks    Hooks
}

// NewManager erzeugt einen Manager für die lokale ID.
func NewManager(selfID string, hooks Hooks) *Manager {
	return &Manager{selfID: selfID, sessions: map[string]*session{}, hooks: hooks}
}

func sessionCode(shared [32]byte) string {
	sum := sha256.Sum256(shared[:])
	return hex.EncodeToString(sum[:])[:16]
}

// nonce baut die 24-Byte-Nonce aus Sequenz + Sender-Rolle.
// Beide Seiten leiten dieselbe Nonce ab (kein Nonce-Transport nötig,
// keine Wiederverwendung: seq steigt, Rolle trennt Richtungen).
func nonceFor(seq uint64, senderIsInitiator bool) [24]byte {
	var n [24]byte
	binary.BigEndian.PutUint64(n[:8], seq)
	if senderIsInitiator {
		n[8] = 1
	}
	return n
}

type wireMsg struct {
	Seq  uint64 `json:"seq"`
	Text string `json:"text"`
	Ts   int64  `json:"ts"`
}

// outSignal / outText sind lock-frei ausführbare Aktionen.
type outSignal struct {
	to, typ, payload string
}

func (m *Manager) emitSignals(sigs []outSignal) {
	if m.hooks.SendSignal == nil {
		return
	}
	for _, s := range sigs {
		m.hooks.SendSignal(s.to, s.typ, s.payload)
	}
}

func encodePub(pub [32]byte) string { return base64.StdEncoding.EncodeToString(pub[:]) }

func trimPrefix(payload []byte) string {
	s := string(payload)
	if len(s) > len(UDPPrefix) && s[:len(UDPPrefix)] == UDPPrefix {
		return s[len(UDPPrefix):]
	}
	return s
}

// persistAsync speichert die Inbox best-effort (darf nie den Chat blockieren).
func (m *Manager) persistAsync() {
	msgs := m.Inbox()
	go func() { _ = SaveHistory(msgs) }()
}

// OnRelay öffnet eine über Signaling gerelayte Box (gleiche Crypto wie UDP).
func (m *Manager) OnRelay(sealedB64 string) (*Message, bool) {
	raw, err := base64.StdEncoding.DecodeString(sealedB64)
	if err != nil {
		return nil, false
	}
	return m.OnUDP(raw)
}

func decodePub(s string) ([32]byte, error) {
	var pub [32]byte
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(raw) != 32 {
		return pub, fmt.Errorf("ungültiger Pubkey")
	}
	copy(pub[:], raw)
	return pub, nil
}

func (m *Manager) getOrCreateLocked(peerID string) (*session, error) {
	s, ok := m.sessions[peerID]
	if ok {
		return s, nil
	}
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	s = &session{peerID: peerID, myPriv: *priv, myPub: *pub, updated: time.Now()}
	m.sessions[peerID] = s
	return s, nil
}

// adoptPeerLocked übernimmt einen frischen Peer-Pubkey (Rolle deterministisch:
// größerer Pubkey = Initiator, beide Seiten konvergieren).
func adoptPeerLocked(s *session, pub [32]byte) {
	s.peerPub = pub
	s.hasPeer = true
	s.initiator = bytes.Compare(s.myPub[:], pub[:]) > 0
	var shared [32]byte
	box.Precompute(&shared, &s.peerPub, &s.myPriv)
	s.shared = shared
	s.code = sessionCode(shared)
	s.ready = true
	s.updated = time.Now()
}

// sealLocked versiegelt (Seq wird vergeben, Senden passiert außerhalb).
func sealLocked(s *session, text string) (payload []byte, wm wireMsg) {
	s.outSeq++
	wm = wireMsg{Seq: s.outSeq, Text: text, Ts: time.Now().Unix()}
	raw, _ := json.Marshal(wm)
	nonce := nonceFor(s.outSeq, s.initiator)
	sealed := box.SealAfterPrecomputation(nil, raw, &nonce, &s.shared)
	return []byte(UDPPrefix + base64.StdEncoding.EncodeToString(sealed)), wm
}

// Open startet den Handshake (idempotent). "" = noch nicht bereit.
func (m *Manager) Open(peerID string) (string, error) {
	m.mu.Lock()
	s, err := m.getOrCreateLocked(peerID)
	if err != nil {
		m.mu.Unlock()
		return "", err
	}
	var sigs []outSignal
	if !s.helloSent {
		sigs = append(sigs, outSignal{to: peerID, typ: "msg-hello", payload: encodePub(s.myPub)})
		s.helloSent = true
	}
	code, ready := s.code, s.ready
	m.mu.Unlock()
	m.emitSignals(sigs)
	if ready {
		return code, nil
	}
	return "", nil
}

// Send stellt zu (E2E-verschlüsselt) oder queued bis zum Handshake.
// Leerer Text öffnet nur die Session.
// Transport: UDP-Direct zuerst (serverlos), Fallback über Signaling-Relay
// (msg-data, opaque Blob – Rendezvous sieht nur Cipher, kein Klartext).
// Damit funktioniert der Messenger auch hinter NAT/CGNAT ohne Port-Forward.
func (m *Manager) Send(peerID, text string) (string, error) {
	if len([]rune(text)) > MaxTextLen {
		return "", fmt.Errorf("max. %d Zeichen", MaxTextLen)
	}
	m.mu.Lock()
	s, err := m.getOrCreateLocked(peerID)
	if err != nil {
		m.mu.Unlock()
		return "", err
	}
	if text == "" || !s.ready {
		if text != "" && len(s.pending) < 20 {
			s.pending = append(s.pending, text)
		}
		var sigs []outSignal
		if !s.helloSent {
			sigs = append(sigs, outSignal{to: peerID, typ: "msg-hello", payload: encodePub(s.myPub)})
			s.helloSent = true
		}
		ready, code := s.ready, s.code
		m.mu.Unlock()
		m.emitSignals(sigs)
		if ready {
			return "bereit (" + code + ")", nil
		}
		return "queued (Handshake läuft)", nil
	}
	payload, wm := sealLocked(s, text)
	m.mu.Unlock()

	sealedB64 := ""
	if raw, err := base64.StdEncoding.DecodeString(trimPrefix(payload)); err == nil {
		_ = raw
		sealedB64 = trimPrefix(payload)
	}

	// 1) UDP-Direct versuchen (wenn Endpoint bekannt + Mesh/UDP verfügbar).
	if m.hooks.LookupEndpoint != nil && m.hooks.SendUDP != nil {
		if ep, ok := m.hooks.LookupEndpoint(peerID); ok && ep != "" {
			if err := m.hooks.SendUDP(ep, payload); err == nil {
				m.mu.Lock()
				m.appendInboxLocked(Message{From: m.selfID, To: peerID, Text: text, Ts: wm.Ts, Outgoing: true, Seq: wm.Seq})
				s.updated = time.Now()
				m.mu.Unlock()
				m.persistAsync()
				return "sent (direct)", nil
			}
		}
	}
	// 2) Relay über Signaling (funktioniert auch mit Mesh aus / ohne UDP-Port).
	if m.hooks.SendSignal != nil && sealedB64 != "" {
		m.hooks.SendSignal(peerID, "msg-data", sealedB64)
		m.mu.Lock()
		m.appendInboxLocked(Message{From: m.selfID, To: peerID, Text: text, Ts: wm.Ts, Outgoing: true, Seq: wm.Seq})
		s.updated = time.Now()
		m.mu.Unlock()
		m.persistAsync()
		return "sent (relay)", nil
	}
	return "", fmt.Errorf("kein Transport (weder UDP noch Relay)")
}

// OnHello verarbeitet ein Hello (Antwort-Ack wird nach Unlock verschickt).
func (m *Manager) OnHello(from, helloB64 string) error {
	pub, err := decodePub(helloB64)
	if err != nil {
		return err
	}
	m.mu.Lock()
	s, err := m.getOrCreateLocked(from)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	adoptPeerLocked(s, pub)
	pending := append([]string(nil), s.pending...)
	s.pending = nil
	ack := encodePub(s.myPub)
	m.mu.Unlock()

	m.emitSignals([]outSignal{{to: from, typ: "msg-hello-ack", payload: ack}})
	for _, text := range pending {
		// Best effort: Fehlschlag bleibt ohne Queue (nächstes Hello flusht neu).
		_, _ = m.Send(from, text)
	}
	return nil
}

// OnAck verarbeitet ein Hello-Ack.
func (m *Manager) OnAck(from, ackB64 string) error {
	pub, err := decodePub(ackB64)
	if err != nil {
		return err
	}
	m.mu.Lock()
	s, ok := m.sessions[from]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("keine Session für %s", from)
	}
	adoptPeerLocked(s, pub)
	pending := append([]string(nil), s.pending...)
	s.pending = nil
	m.mu.Unlock()

	for _, text := range pending {
		_, _ = m.Send(from, text)
	}
	return nil
}

// OnUDP öffnet eingehende Box-Bytes (probiert alle Sessions, kein
// Metadaten-Leak im Protokoll). Replay/Duplikate werden verworfen.
// Hook-frei, daher lock-sicher. Persistiert bei Treffer (nach Unlock).
func (m *Manager) OnUDP(sealed []byte) (*Message, bool) {
	m.mu.Lock()
	var found *Message
	for _, s := range m.sessions {
		if !s.ready {
			continue
		}
		if msg, ok := m.tryOpenLocked(s, sealed); ok {
			found = msg
			break
		}
	}
	var toSave []Message
	if found != nil {
		toSave = make([]Message, len(m.inbox))
		copy(toSave, m.inbox)
	}
	m.mu.Unlock()
	if found != nil {
		go func() { _ = SaveHistory(toSave) }()
		return found, true
	}
	return nil, false
}

func (m *Manager) tryOpenLocked(s *session, sealed []byte) (*Message, bool) {
	// Nonce hängt an der (versiegelten) Seq: Fenster der nächsten 32
	// Nonces probieren (Sender = Peer, Rolle invertiert zur eigenen).
	for seq := s.inSeq + 1; seq <= s.inSeq+32; seq++ {
		nonce := nonceFor(seq, !s.initiator)
		raw, ok := box.OpenAfterPrecomputation(nil, sealed, &nonce, &s.shared)
		if !ok {
			continue
		}
		var wm wireMsg
		if err := json.Unmarshal(raw, &wm); err != nil || wm.Seq != seq {
			continue
		}
		if wm.Seq <= s.inSeq {
			return nil, true // Replay/Duplikat: verworfen, aber zugeordnet
		}
		s.inSeq = wm.Seq
		msg := &Message{From: s.peerID, To: m.selfID, Text: wm.Text, Ts: wm.Ts, Seq: wm.Seq}
		m.appendInboxLocked(*msg)
		s.updated = time.Now()
		return msg, true
	}
	return nil, false
}

func (m *Manager) appendInboxLocked(msg Message) {
	m.inbox = append(m.inbox, msg)
	if len(m.inbox) > MaxInbox {
		m.inbox = m.inbox[len(m.inbox)-MaxInbox:]
	}
}

// Sessions listet Sessions (UI-tauglich, ohne Secrets).
func (m *Manager) Sessions() []Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, Session{PeerID: s.peerID, Code: s.code, Ready: s.ready, Pending: len(s.pending), Updated: s.updated.Unix()})
	}
	return out
}

// Inbox gibt den RAM-Verlauf zurück (Kopie).
func (m *Manager) Inbox() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Message, len(m.inbox))
	copy(out, m.inbox)
	return out
}
