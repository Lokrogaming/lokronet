// Package client: typisierte localhost-IPC zum Daemon.
// Einzige Stelle, die das Daemon-Protokoll (/v1/...) spricht –
// CLI (cmd/lokronet) und TUI (internal/tui) nutzen beide diese Schicht,
// damit keine zweite Connection-Implementierung entsteht.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lokro/lokronet/internal/config"
	"github.com/lokro/lokronet/internal/metrics"
	"github.com/lokro/lokronet/pkg/proto"
)

// PeerState spiegelt daemon.peerState (Felder absichtlich dupliziert,
// damit kein Import-Zyklus daemon -> client entsteht).
type PeerState struct {
	Peer    proto.Peer `json:"peer"`
	State   string     `json:"state"`
	LastRTT int64      `json:"last_rtt_ms"`
}

// Status ist GET /v1/status typisiert.
type Status struct {
	ID          string               `json:"id"`
	Fingerprint string               `json:"fingerprint"`
	UDPPort     int                  `json:"udp_port"`
	Rendezvous  string               `json:"rendezvous"`
	Debug       bool                 `json:"debug"`
	Mode        string               `json:"mode"`
	Mesh        bool                 `json:"mesh"`
	Peers       map[string]PeerState `json:"peers"`
	Traffic     metrics.Snapshot     `json:"traffic"`
}

// Event ist ein Debug-/Bestätigungs-Eintrag (GET /v1/events).
type Event struct {
	Time time.Time `json:"time"`
	Msg  string    `json:"msg"`
}

// ModeInfo ist GET/POST /v1/mode typisiert.
type ModeInfo struct {
	Mode          string `json:"mode"`
	Mesh          bool   `json:"mesh"`
	RestartNeeded bool   `json:"restart_needed,omitempty"`
}

// Client spricht mit genau einem lokalen Daemon (127.0.0.1 + Token).
type Client struct {
	token string
	http  *http.Client
}

// ipcAddr ist als Variable ausgelegt, damit Tests einen Fake-Daemon
// einhängen können ("" = auto: daemon.ipc-Datei, sonst 37777).
var ipcAddr = ""

func resolveIPCAddr() string {
	if ipcAddr != "" {
		return ipcAddr
	}
	return fmt.Sprintf("127.0.0.1:%d", config.ReadIPCPort())
}

// Dial lädt das IPC-Token (Fehler, wenn nie ein Daemon lief).
func Dial() (*Client, error) {
	token, err := config.DaemonToken()
	if err != nil {
		return nil, err
	}
	return &Client{token: token, http: &http.Client{Timeout: 10 * time.Second}}, nil
}

// Call ist der rohe Aufruf (Statuscodes >= 300 -> Fehler mit Body).
func (c *Client) Call(method, path string, body any) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, "http://"+resolveIPCAddr()+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Lokro-Token", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("daemon nicht erreichbar – `lokronet daemon` gestartet? (%w)", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return data, fmt.Errorf("daemon %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

// Status holt den Daemon-Zustand (Fehler, wenn Daemon offline).
func (c *Client) Status() (Status, error) {
	data, err := c.Call("GET", "/v1/status", nil)
	if err != nil {
		return Status{}, err
	}
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return Status{}, err
	}
	return s, nil
}

// Events holt den Debug-Ringpuffer.
func (c *Client) Events() ([]Event, error) {
	data, err := c.Call("GET", "/v1/events", nil)
	if err != nil {
		return nil, err
	}
	var ev []Event
	if err := json.Unmarshal(data, &ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// Connect stößt ein Pairing an (pending bis Fingerprint-Freigabe).
func (c *Client) Connect(id string) (proto.Peer, error) {
	data, err := c.Call("POST", "/v1/connect", map[string]string{"id": id})
	if err != nil {
		return proto.Peer{}, err
	}
	var p proto.Peer
	if err := json.Unmarshal(data, &p); err != nil {
		return proto.Peer{}, err
	}
	return p, nil
}

// Ping misst RTT in Millisekunden (Fehler bei Timeout/Mesh-off).
func (c *Client) Ping(id string) (int64, error) {
	data, err := c.Call("POST", "/v1/ping", map[string]string{"id": id})
	if err != nil {
		return 0, err
	}
	var v struct {
		RTT int64 `json:"rtt_ms"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return 0, err
	}
	return v.RTT, nil
}

// SetDebug schaltet Debug-Meldungen.
func (c *Client) SetDebug(on bool) error {
	_, err := c.Call("POST", "/v1/debug", map[string]bool{"on": on})
	return err
}

// GetMode liest Modus + Mesh-Schalter.
func (c *Client) GetMode() (ModeInfo, error) {
	data, err := c.Call("GET", "/v1/mode", nil)
	if err != nil {
		return ModeInfo{}, err
	}
	var m ModeInfo
	if err := json.Unmarshal(data, &m); err != nil {
		return ModeInfo{}, err
	}
	return m, nil
}

// SetMode setzt Modus und/oder Mesh (nil = unverändert lassen).
func (c *Client) SetMode(setMode *string, setMesh *bool) (ModeInfo, error) {
	body := map[string]any{}
	if setMode != nil {
		body["mode"] = *setMode
	}
	if setMesh != nil {
		body["mesh"] = *setMesh
	}
	data, err := c.Call("POST", "/v1/mode", body)
	if err != nil {
		return ModeInfo{}, err
	}
	var m ModeInfo
	if err := json.Unmarshal(data, &m); err != nil {
		return ModeInfo{}, err
	}
	return m, nil
}

// ChatSession spiegelt chat.Session (ohne Secrets).
type ChatSession struct {
	PeerID  string `json:"peer_id"`
	Code    string `json:"code"`
	Ready   bool   `json:"ready"`
	Pending int    `json:"pending"`
	Updated int64  `json:"updated_at"`
}

// ChatMessage spiegelt chat.Message (RAM-Verlauf des Daemons).
type ChatMessage struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Text     string `json:"text"`
	Ts       int64  `json:"ts"`
	Outgoing bool   `json:"outgoing"`
	Seq      uint64 `json:"seq"`
}

// ChatSessions listet Messenger-Sessions.
func (c *Client) ChatSessions() ([]ChatSession, error) {
	data, err := c.Call("GET", "/v1/chat/sessions", nil)
	if err != nil {
		return nil, err
	}
	var s []ChatSession
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s == nil {
		s = []ChatSession{}
	}
	return s, nil
}

// ChatInbox holt den RAM-Verlauf.
func (c *Client) ChatInbox() ([]ChatMessage, error) {
	data, err := c.Call("GET", "/v1/chat/inbox", nil)
	if err != nil {
		return nil, err
	}
	var m []ChatMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = []ChatMessage{}
	}
	return m, nil
}

// ChatSend öffnet die Session (leerer Text) oder schickt E2E-verschlüsselt.
// Antwort: "sent (direct)" | "sent (relay)" | "queued (Handshake läuft)" | "bereit (...)".
func (c *Client) ChatSend(id, text string) (string, error) {
	data, err := c.Call("POST", "/v1/chat/send", map[string]string{"id": id, "text": text})
	if err != nil {
		return "", err
	}
	var v struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return "", err
	}
	return v.Status, nil
}

// PresenceEntry: online/offline pro Peer (aus Daemon-Beacons).
type PresenceEntry struct {
	Online   bool   `json:"online"`
	LastSeen int64  `json:"last_seen"`
	Endpoint string `json:"endpoint"`
	Mode     string `json:"mode"`
}

// Presence holt GET /v1/presence.
func (c *Client) Presence() (map[string]PresenceEntry, error) {
	data, err := c.Call("GET", "/v1/presence", nil)
	if err != nil {
		return nil, err
	}
	var m map[string]PresenceEntry
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]PresenceEntry{}
	}
	return m, nil
}

// ChatStats holt GET /v1/chat/stats (pro Peer count/max_ts, für Sync-Debug).
func (c *Client) ChatStats() ([]ChatStats, error) {
	data, err := c.Call("GET", "/v1/chat/stats", nil)
	if err != nil {
		return nil, err
	}
	var s []ChatStats
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return s, nil
}

// ChatStats spiegelt chat.PeerStats.
type ChatStats struct {
	PeerID string `json:"peer_id"`
	Count  int    `json:"count"`
	MaxTs  int64  `json:"max_ts"`
}

// Beacon stößt einen Presence-Beacon an ("" = an alle Kontakte).
func (c *Client) Beacon(id string) (string, error) {
	data, err := c.Call("POST", "/v1/beacon", map[string]string{"id": id})
	if err != nil {
		return "", err
	}
	return string(data), nil
}
