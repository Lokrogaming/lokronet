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
// einhängen können (Produktion: config.IPCAddr).
var ipcAddr = config.IPCAddr

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
	req, err := http.NewRequest(method, "http://"+ipcAddr+path, rdr)
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
