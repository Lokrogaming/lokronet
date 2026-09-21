// Package config verwaltet lokale Pfade + Konfiguration (~/.lokronet).
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	// IPCAddr ist NUR localhost – der Daemon hört nie aufs öffentliche Netz.
	IPCAddr = "127.0.0.1:37777"
	// DefaultRendezvous für lokale Tests. Später: https://signal.lokro.net
	DefaultRendezvous = "http://127.0.0.1:8787"
)

// Config liegt in ~/.lokronet/config.json (JSON statt YAML,
// damit wir ohne externe Deps auskommen).
type Config struct {
	Debug         bool   `json:"debug"`
	RendezvousURL string `json:"rendezvous_url"`
	UDPPort       int    `json:"udp_port"`
	// Endpoint ist die öffentlich erreichbare Adresse "ip:port", die
	// Heartbeat + Pairing melden. Leer = lokal raten (127.0.0.1:port,
	// reicht nur für Tests auf einer Maschine). Hinter NAT per
	// `setup --endpoint <public-ip:port>` setzen (STUN-Autoerkennung folgt).
	Endpoint string `json:"endpoint"`
	// Mode: "performance"|"normal"|"eco" ("" = normal). Siehe internal/mode.
	Mode string `json:"mode"`
	// MeshDisabled schaltet den UDP-Meshpoint ab (nur noch Signaling).
	// Umschalten braucht einen Daemon-Neustart.
	MeshDisabled bool `json:"mesh_disabled"`
}

// Dir legt ~/.lokronet an und gibt den Pfad zurück.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".lokronet")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func filePath(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// LoadConfig lädt die Config oder liefert Defaults (ohne zu speichern).
func LoadConfig() (*Config, error) {
	p, err := filePath("config.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{RendezvousURL: DefaultRendezvous}, nil
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.RendezvousURL == "" {
		c.RendezvousURL = DefaultRendezvous
	}
	return &c, nil
}

// SaveConfig speichert mit restriktiven Rechten (0600).
func SaveConfig(c *Config) error {
	p, err := filePath("config.json")
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// DaemonToken lädt oder erzeugt das localhost-IPC-Token.
func DaemonToken() (string, error) {
	p, err := filePath("daemon.token")
	if err != nil {
		return "", err
	}
	if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
		return string(data), nil
	}
	token, err := randomHex(32)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(token), 0o600); err != nil {
		return "", err
	}
	return token, nil
}
