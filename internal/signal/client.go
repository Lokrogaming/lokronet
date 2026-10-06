// Client für das Rendezvous (wird von CLI + Daemon benutzt).
package signal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lokro/lokronet/internal/metrics"
	"github.com/lokro/lokronet/pkg/proto"
)

// Client spricht mit dem Rendezvous (Default lokal, später HTTPS).
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// M zählt Signaling-Bytes (nil = nicht zählen, z.B. reines CLI-Setup).
	M *metrics.Counters
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
	if c.M != nil {
		c.M.AddSigOut(len(data))
	}
	return c.HTTP.Post(c.BaseURL+path, "application/json", bytes.NewReader(data))
}

// drain liest kleine Antworten vollständig (für Connection-Reuse + Zählung).
// Schließen übernimmt weiterhin das defer am Aufrufer.
func (c *Client) drain(resp *http.Response) {
	n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if c.M != nil {
		c.M.AddSigIn(int(n))
	}
}

// RendezvousError behält den HTTP-Status (für 409-Retry vs. 404-Auto-Register).
type RendezvousError struct {
	Code int
	Msg  string
}

func (e *RendezvousError) Error() string {
	return fmt.Sprintf("rendezvous %d: %s", e.Code, e.Msg)
}

// IsConflict meldet 409 (ID vergeben -> neu würfeln).
func IsConflict(err error) bool {
	var re *RendezvousError
	if ok := errors.As(err, &re); ok {
		return re.Code == 409
	}
	// Fallback für ältere Fehlermeldungen ohne Typ.
	return err != nil && strings.Contains(err.Error(), "rendezvous 409")
}

// IsNotFound meldet 404 (unbekannte ID -> neu registrieren).
func IsNotFound(err error) bool {
	var re *RendezvousError
	if ok := errors.As(err, &re); ok {
		return re.Code == 404
	}
	return err != nil && strings.Contains(err.Error(), "rendezvous 404")
}

func readErr(resp *http.Response) error {
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	// Body ist meist {"error":"..."} – roh übernehmen, kein JSON-Zwang.
	return &RendezvousError{Code: resp.StatusCode, Msg: string(data)}
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
	c.drain(resp)
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
	c.drain(resp)
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return proto.Peer{}, err
	}
	if c.M != nil {
		c.M.AddSigIn(len(data))
	}
	var p proto.Peer
	if err := json.Unmarshal(data, &p); err != nil {
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
	c.drain(resp)
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if c.M != nil {
		c.M.AddSigIn(len(data))
	}
	var msgs []proto.SignalMessage
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}
