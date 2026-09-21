// Package proto enthält die versionierten Protokoll-Typen (v1).
// Alles was über Rendezvous oder Mesh läuft, nutzt diese Typen,
// damit wir das Protokoll später ohne Break weiterentwickeln können.
package proto

// Version des Protokolls.
const Version = "v1"

// Peer ist ein am Rendezvous registrierter Knoten.
// Endpoint ist "ip:port" des UDP-Sockets (MVP: vom Client gemeldet,
// später via STUN verifiziert). Es werden NIE Private Keys übertragen.
type Peer struct {
	ID          string `json:"id"`          // 12 Ziffern
	EdPubB64    string `json:"ed_pub"`      // Ed25519 public (base64)
	WGPubB64    string `json:"wg_pub"`      // WireGuard public (base64, MVP: Platzhalter)
	Fingerprint string `json:"fingerprint"` // hex(sha256(ed_pub))
	Endpoint    string `json:"endpoint"`    // z.B. "84.1.2.3:51820"
	// Mode wirbt den Drossel-Modus (performance|normal|eco, "" = unbekannt).
	// Basis für Prio-Routing (performance > normal > eco), siehe IDEEN.md.
	Mode     string `json:"mode,omitempty"`
	LastSeen int64  `json:"last_seen"` // unix
}

type RegisterRequest struct {
	Peer Peer `json:"peer"`
}

type HeartbeatRequest struct {
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"`
	Mode     string `json:"mode,omitempty"`
}

// SignalMessage ist der Realtime-Kanal über das Rendezvous
// (MVP: Mailbox + Long-Poll, später WebSocket).
// Type: "pair" | "pair-ack" | "punch" | "ping" | "pong-ack"
type SignalMessage struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Type    string `json:"type"`
	Payload string `json:"payload"` // z.B. Endpoint oder Nonce; nie Secrets
	SentAt  int64  `json:"sent_at"` // unix
}
