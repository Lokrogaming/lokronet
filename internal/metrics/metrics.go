// Package metrics zählt Mesh-Traffic (UDP + Signaling), global und pro Peer.
// Antwort auf die Admin-Frage "wie hoch ist der Bandbreitenverbrauch":
// lokronet status -> Feld "traffic".
package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// Snapshot ist ein lesbarer Zählerstand (Bytes + Schnitt seit Daemon-Start).
type Snapshot struct {
	UDPIn     uint64                 `json:"udp_in_bytes"`
	UDPOut    uint64                 `json:"udp_out_bytes"`
	SigIn     uint64                 `json:"sig_in_bytes"`
	SigOut    uint64                 `json:"sig_out_bytes"`
	TotalIn   uint64                 `json:"total_in_bytes"`
	TotalOut  uint64                 `json:"total_out_bytes"`
	UptimeS   int64                  `json:"uptime_s"`
	AvgBpsIn  float64                `json:"avg_bps_in"`
	AvgBpsOut float64                `json:"avg_bps_out"`
	Peers     map[string]PeerTraffic `json:"peers,omitempty"`
}

// PeerTraffic zählt UDP-Bytes je bekannter Peer-ID (Endpoint-Match).
type PeerTraffic struct {
	In  uint64 `json:"in_bytes"`
	Out uint64 `json:"out_bytes"`
}

// Counters sind goroutine-sicher (atomar + Mutex nur für die Peer-Map).
type Counters struct {
	start  time.Time
	udpIn  atomic.Uint64
	udpOut atomic.Uint64
	sigIn  atomic.Uint64
	sigOut atomic.Uint64

	mu    sync.Mutex
	peers map[string]*PeerTraffic
}

// New startet die Uhr (Aufruf beim Daemon-Start).
func New() *Counters {
	return &Counters{start: time.Now(), peers: make(map[string]*PeerTraffic)}
}

func (c *Counters) AddUDPIn(n int)  { c.udpIn.Add(uint64(n)) }
func (c *Counters) AddUDPOut(n int) { c.udpOut.Add(uint64(n)) }
func (c *Counters) AddSigIn(n int)  { c.sigIn.Add(uint64(n)) }
func (c *Counters) AddSigOut(n int) { c.sigOut.Add(uint64(n)) }

// AddPeerUDP ordnet UDP-Bytes einer Peer-ID zu (in=true eingehend).
func (c *Counters) AddPeerUDP(id string, n int, in bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.peers[id]
	if !ok {
		p = &PeerTraffic{}
		c.peers[id] = p
	}
	if in {
		p.In += uint64(n)
	} else {
		p.Out += uint64(n)
	}
}

// Snapshot rechnet Summen + Schnitt (Bytes/s seit Start).
func (c *Counters) Snapshot() Snapshot {
	uptime := time.Since(c.start)
	secs := uptime.Seconds()
	if secs < 1 {
		secs = 1
	}
	udpIn, udpOut := c.udpIn.Load(), c.udpOut.Load()
	sigIn, sigOut := c.sigIn.Load(), c.sigOut.Load()
	totalIn, totalOut := udpIn+sigIn, udpOut+sigOut

	c.mu.Lock()
	peers := make(map[string]PeerTraffic, len(c.peers))
	for id, p := range c.peers {
		peers[id] = *p
	}
	c.mu.Unlock()

	return Snapshot{
		UDPIn: udpIn, UDPOut: udpOut,
		SigIn: sigIn, SigOut: sigOut,
		TotalIn: totalIn, TotalOut: totalOut,
		UptimeS:   int64(uptime.Seconds()),
		AvgBpsIn:  float64(totalIn) / secs,
		AvgBpsOut: float64(totalOut) / secs,
		Peers:     peers,
	}
}
