// Package identity: 12-stellige ID (Adresse, kein Secret) + lokale Keypairs.
// Private Keys verlassen NIE das Gerät (~/.lokronet, 0600).
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lokro/lokronet/pkg/proto"
)

// Identity ist die lokale Identität.
type Identity struct {
	ID        string `json:"id"`
	EdPubB64  string `json:"ed_pub"`
	EdPrivB64 string `json:"ed_priv"`
	WGPubB64  string `json:"wg_pub"`
	WGPrivB64 string `json:"wg_priv"`
	CreatedAt int64  `json:"created_at"`
}

// GenerateID würfelt 12 Ziffern 0-9 per crypto/rand (~40 Bit:
// bewusst nur Adresse, Auth läuft über Keys + Fingerprint-Freigabe).
func GenerateID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	digits := make([]byte, 12)
	for i, v := range b {
		digits[i] = '0' + (v % 10)
	}
	return string(digits), nil
}

// Generate erzeugt ID + Ed25519-Keypair. Das WG-Keypair ist im MVP
// ein Platzhalter (32 Zufallsbytes); echtes Curve25519 kommt mit
// wireguard-go in P1. Format bleibt kompatibel.
func Generate() (*Identity, error) {
	id, err := GenerateID()
	if err != nil {
		return nil, err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	wgPub := make([]byte, 32)
	wgPriv := make([]byte, 32)
	if _, err := rand.Read(wgPub); err != nil {
		return nil, err
	}
	if _, err := rand.Read(wgPriv); err != nil {
		return nil, err
	}
	return &Identity{
		ID:        id,
		EdPubB64:  base64.StdEncoding.EncodeToString(pub),
		EdPrivB64: base64.StdEncoding.EncodeToString(priv),
		WGPubB64:  base64.StdEncoding.EncodeToString(wgPub),
		WGPrivB64: base64.StdEncoding.EncodeToString(wgPriv),
		CreatedAt: time.Now().Unix(),
	}, nil
}

// Fingerprint ist hex(sha256(ed_pub)) – wird beim Pairing out-of-band verglichen.
func (i *Identity) Fingerprint() (string, error) {
	pub, err := base64.StdEncoding.DecodeString(i.EdPubB64)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:]), nil
}

// ToPeer baut die öffentliche Registrierung (ohne Private Keys).
func (i *Identity) ToPeer(endpoint string) (proto.Peer, error) {
	fp, err := i.Fingerprint()
	if err != nil {
		return proto.Peer{}, err
	}
	return proto.Peer{
		ID:          i.ID,
		EdPubB64:    i.EdPubB64,
		WGPubB64:    i.WGPubB64,
		Fingerprint: fp,
		Endpoint:    endpoint,
		LastSeen:    time.Now().Unix(),
	}, nil
}

func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".lokronet")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "identity.json"), nil
}

// Save speichert mit 0600.
func (i *Identity) Save() error {
	p, err := path()
	if err != nil {
		return err
	}
	if len(i.ID) != 12 {
		return fmt.Errorf("ungültige ID-Länge")
	}
	data, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// Load lädt die Identität (Fehler wenn noch kein setup lief).
func Load() (*Identity, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("keine Identität – bitte zuerst `lokronet setup` ausführen")
		}
		return nil, err
	}
	var i Identity
	if err := json.Unmarshal(data, &i); err != nil {
		return nil, err
	}
	return &i, nil
}
