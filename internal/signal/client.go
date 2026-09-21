// Client für das Rendezvous (wird von CLI + Daemon benutzt).
package signal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lokro/lokronet/pkg/proto"
)

// Client spricht mit dem Rendezvous (Default lokal, später HTTPS).
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient erzeugt einen Client mit 10s-Timeout.
func NewClient(baseURL string) *Client {
	return &Client{BaseURL: baseURL, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) post(path string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.HTTP.Post(c.BaseURL+path, "application/json", bytes.NewReader(data))
}

func readErr(resp *http.Response) error {
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("rendezvous %d: %s", resp.StatusCode, string(data))
}

// Register meldet den Peer an. 409 = ID vergeben -> neu würfeln.
func (c *Client) Register(p proto.Peer) error {
	resp, err := c.post("/v1/register", proto.RegisterRequest{Peer: p})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return readErr(resp)
	}
	return nil
}

// Heartbeat aktualisiert Endpoint/LastSeen.
func (c *Client) Heartbeat(id, endpoint string) error {
	resp, err := c.post("/v1/heartbeat", proto.HeartbeatRequest{ID: id, Endpoint: endpoint})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return readErr(resp)
	}
	return nil
}

// Lookup löst eine ID in Peer-Daten auf.
func (c *Client) Lookup(id string) (proto.Peer, error) {
	resp, err := c.HTTP.Get(c.BaseURL + "/v1/lookup?id=" + id)
	if err != nil {
		return proto.Peer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return proto.Peer{}, readErr(resp)
	}
	var p proto.Peer
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return proto.Peer{}, err
	}
	return p, nil
}

// Send legt eine Signaling-Nachricht in die Mailbox des Empfängers.
func (c *Client) Send(m proto.SignalMessage) error {
	resp, err := c.post("/v1/signal", m)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return readErr(resp)
	}
	return nil
}

// Poll holt Nachrichten ab (Long-Poll, waitMs 0-20000). Realtime-Kanal im MVP.
func (c *Client) Poll(to string, waitMs int) ([]proto.SignalMessage, error) {
	resp, err := c.HTTP.Get(fmt.Sprintf("%s/v1/signal?to=%s&wait=%d", c.BaseURL, to, waitMs))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, readErr(resp)
	}
	var msgs []proto.SignalMessage
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}
