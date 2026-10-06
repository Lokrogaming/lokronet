// Package daemon – signierter Session-Handshake.
//
// Session-Token (User-Spec):
//   Nach Endpoint-Austausch generiert jede Seite einen EPHEMEREN X25519-Key.
//   Der Pubkey wird mit dem LANGFRISTIGEN Ed25519-Identity-Key signiert und
//   über Signaling ausgetauscht (msg-hello / msg-hello-ack v2).
//   Beide leiten per NaCl-box dasselbe Shared Secret ab, zeigen den
//   Session-Code (sha256(shared)[:16]) zum out-of-band Vergleich.
//   Nachrichten danach E2E (XSalsa20-Poly1305), Nonce aus (Seq + Rolle).
//   Rendezvous sieht nur opake Blobs + Metadaten, nie Klartext, keine DB.
package daemon

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"

	"github.com/lokro/lokronet/pkg/proto"
)

// helloV2 ist das signierte Hello (abwärtskompatibel: legacy = bloßer Pubkey).
type helloV2 struct {
	Pub string `json:"pub"`
	Sig string `json:"sig"`
	ID  string `json:"id"`
}

// signHello signiert den ephemeren Pubkey (base64) mit der Identity.
func (d *Daemon) signHello(pubB64 string) string {
	privRaw, err := base64.StdEncoding.DecodeString(d.ident.EdPrivB64)
	if err != nil || len(privRaw) != ed25519.PrivateKeySize {
		return pubB64 // Fallback legacy (kein Signieren möglich)
	}
	sig := ed25519.Sign(privRaw, []byte(pubB64))
	out, err := json.Marshal(helloV2{Pub: pubB64, Sig: base64.StdEncoding.EncodeToString(sig), ID: d.ident.ID})
	if err != nil {
		return pubB64
	}
	return string(out)
}

// verifyHello prüft die Signatur gegen den registrierten Ed-Pub des Senders.
// Gibt (ephemerer Pubkey, signiert?) zurück. Legacy ohne Sig wird akzeptiert
// (TOFU via Session-Code-Vergleich), aber als unsigniert markiert.
func (d *Daemon) verifyHello(from, payload string) (pub string, signed bool, ok bool) {
	var v helloV2
	if err := json.Unmarshal([]byte(payload), &v); err != nil || v.Pub == "" || v.Sig == "" {
		return payload, false, true // legacy
	}
	var peer proto.Peer
	d.mu.Lock()
	if ps, found := d.peers[from]; found {
		peer = ps.Peer
	}
	d.mu.Unlock()
	if peer.EdPubB64 == "" {
		if p, err := d.sig.Lookup(from); err == nil {
			peer = p
		} else {
			return "", false, false
		}
	}
	pubRaw, err := base64.StdEncoding.DecodeString(peer.EdPubB64)
	if err != nil || len(pubRaw) != ed25519.PublicKeySize {
		return "", false, false
	}
	sig, err := base64.StdEncoding.DecodeString(v.Sig)
	if err != nil {
		return "", false, false
	}
	if !ed25519.Verify(pubRaw, []byte(v.Pub), sig) {
		return "", false, false
	}
	return v.Pub, true, true
}
