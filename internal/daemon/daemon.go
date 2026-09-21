// Package daemon: der Backend-Kern. Verarbeitet Inputs (CLI via localhost-IPC),
// hält die Mesh-Basis (1x UDP-Socket) und den Realtime-Kanal (Signaling-Poll)
// offen. Hört NIE aufs öffentliche Netz – IPC nur 127.0.0.1 + Token.
package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lokro/lokronet/internal/config"
	"github.com/lokro/lokronet/internal/debug"
	"github.com/lokro/lokronet/internal/identity"
	"github.com/lokro/lokronet/internal/metrics"
	"github.com/lokro/lokronet/internal/netcore"
	"github.com/lokro/lokronet/internal/signal"
	"github.com/lokro/lokronet/pkg/proto"
)

// peerState: "pending" (gefunden, Fingerprint noch nicht bestätigt)
// oder "trusted" (manuell bestätigt; Freigabe-Flow kommt in P1).
type peerState struct {
	Peer   proto.Peer `json:"peer"`
	State  string     `json:"state"`
	LastRTT int64     `json:"last_rtt_ms"`
}

// Daemon bündelt alles Laufende.
type Daemon struct {
	cfg   *config.Config
	ident *identity.Identity
	bus   *debug.Bus
	sig   *signal.Client
	token string
	m     *metrics.Counters

	udp *net.UDPConn

	mu       sync.Mutex
	peers    map[string]*peerState
	pongWait map[string]chan struct{} // nonce -> signal
}

// New lädt Config + Identität (Fehler wenn kein setup lief).
func New() (*Daemon, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	ident, err := identity.Load()
	if err != nil {
		return nil, err
	}
	token, err := config.DaemonToken()
	if err != nil {
		return nil, err
	}
	m := metrics.New()
	sigClient := signal.NewClient(cfg.RendezvousURL)
	sigClient.M = m
	return &Daemon{
		cfg:      cfg,
		ident:    ident,
		bus:      debug.NewBus(200),
		sig:      sigClient,
		token:    token,
		m:        m,
		peers:    make(map[string]*peerState),
		pongWait: make(map[string]chan struct{}),
	}, nil
}

// Bus legt Debug-Events ab (nur wenn debug on).
func (d *Daemon) emit(format string, args ...any) {
	if !d.cfg.Debug {
		return
	}
	d.bus.Add(fmt.Sprintf(format, args...))
}

func (d *Daemon) localEndpoint() string {
	return fmt.Sprintf("127.0.0.1:%d", d.cfg.UDPPort)
}

// Start bindet UDP, startet Loops und blockiert mit dem IPC-Server.
func (d *Daemon) Start() error {
	if d.cfg.UDPPort == 0 {
		port, err := netcore.FindFreeUDPPort(51820)
		if err != nil {
			return err
		}
		d.cfg.UDPPort = port
		if err := config.SaveConfig(d.cfg); err != nil {
			return err
		}
	}
	udp, err := netcore.ListenUDP(d.cfg.UDPPort)
	if err != nil {
		return fmt.Errorf("UDP-Port %d belegt: %w", d.cfg.UDPPort, err)
	}
	d.udp = udp
	d.emit("daemon start id=%s port=%d rendezvous=%s", d.ident.ID, d.cfg.UDPPort, d.cfg.RendezvousURL)

	go d.udpLoop()
	go d.heartbeatLoop()
	go d.signalLoop()

	return d.serveIPC()
}

// --- Mesh-Basis: UDP-Loop -----------------------------------------------
// MVP-Protokoll (DEBUG, unverschlüsselt – nur für lokale Tests!):
//
//	lokro-punch            -> NAT-Loch öffnen, Antwort lokro-punch-ack
//	lokro-ping:<nonce>     -> Antwort lokro-pong:<nonce>
//	P1 ersetzt das durch Noise/WireGuard (E2E-Pflicht, siehe IDEEN.md #9).
func (d *Daemon) udpLoop() {
	buf := make([]byte, 2048)
	for {
		n, addr, err := d.udp.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		d.m.AddUDPIn(n)
		if id := d.peerIDByEndpoint(addr.String()); id != "" {
			d.m.AddPeerUDP(id, n, true)
		}
		msg := string(buf[:n])
		switch {
		case msg == "lokro-punch":
			out := []byte("lokro-punch-ack")
			_, _ = d.udp.WriteToUDP(out, addr)
			d.m.AddUDPOut(len(out))
			d.emit("punch eingehend von %s -> ack", addr.String())
		case strings.HasPrefix(msg, "lokro-ping:"):
			nonce := strings.TrimPrefix(msg, "lokro-ping:")
			out := []byte("lokro-pong:" + nonce)
			_, _ = d.udp.WriteToUDP(out, addr)
			d.m.AddUDPOut(len(out))
			if id := d.peerIDByEndpoint(addr.String()); id != "" {
				d.m.AddPeerUDP(id, len(out), false)
			}
			d.emit("ping eingehend von %s nonce=%s -> pong", addr.String(), nonce)
		case strings.HasPrefix(msg, "lokro-pong:"):
			nonce := strings.TrimPrefix(msg, "lokro-pong:")
			d.mu.Lock()
			if ch, ok := d.pongWait[nonce]; ok {
				close(ch)
				delete(d.pongWait, nonce)
			}
			d.mu.Unlock()
			d.emit("pong von %s nonce=%s", addr.String(), nonce)
		}
	}
}

// --- Realtime-Kanal: Heartbeat + Signaling-Poll --------------------------

func (d *Daemon) heartbeatLoop() {
	_ = d.sig.Heartbeat(d.ident.ID, d.localEndpoint())
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		if err := d.sig.Heartbeat(d.ident.ID, d.localEndpoint()); err != nil {
			d.emit("heartbeat fehlgeschlagen: %v", err)
		}
	}
}

func (d *Daemon) signalLoop() {
	for {
		msgs, err := d.sig.Poll(d.ident.ID, 15000)
		if err != nil {
			d.emit("signal-poll Fehler: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, m := range msgs {
			d.onSignal(m)
		}
	}
}

func (d *Daemon) onSignal(m proto.SignalMessage) {
	d.emit("signal %s von %s payload=%s", m.Type, m.From, m.Payload)
	switch m.Type {
	case "pair":
		peer, err := d.sig.Lookup(m.From)
		if err != nil {
			d.emit("pair-lookup %s fehlgeschlagen: %v", m.From, err)
			return
		}
		d.mu.Lock()
		d.peers[m.From] = &peerState{Peer: peer, State: "pending"}
		d.mu.Unlock()
		// Bestätigung zurück (Gegenstelle sieht uns als pending).
		_ = d.sig.Send(proto.SignalMessage{From: d.ident.ID, To: m.From, Type: "pair-ack", Payload: d.localEndpoint()})
	case "punch":
		// Gegenstelle will NAT öffnen -> ein paar Punch-Pakete zurück.
		if peer, err := d.sig.Lookup(m.From); err == nil && peer.Endpoint != "" {
			go d.punch(peer.Endpoint, 3)
		}
	}
}

// peerIDByEndpoint ordnet eine Absenderadresse einer bekannten Peer-ID zu
// (für die Traffic-Abrechnung; "" wenn unbekannt).
func (d *Daemon) peerIDByEndpoint(endpoint string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, p := range d.peers {
		if p.Peer.Endpoint == endpoint {
			return id
		}
	}
	return ""
}

// punch sendet n leere Pakete an endpoint (UDP Hole Punching, MVP).
func (d *Daemon) punch(endpoint string, n int) {
	addr, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		d.emit("punch: ungültiger Endpoint %s", endpoint)
		return
	}
	for i := 0; i < n; i++ {
		_, _ = d.udp.WriteToUDP([]byte("lokro-punch"), addr)
		time.Sleep(100 * time.Millisecond)
	}
	d.emit("punch -> %s (%dx)", endpoint, n)
}

// Connect: Peer suchen, NAT anstechen, als pending merken.
// Fingerprint MUSS out-of-band geprüft werden (P1: approve-Flow).
func (d *Daemon) Connect(id string) (proto.Peer, error) {
	if len(id) != 12 {
		return proto.Peer{}, fmt.Errorf("ID muss 12 Ziffern haben")
	}
	peer, err := d.sig.Lookup(id)
	if err != nil {
		return proto.Peer{}, err
	}
	d.mu.Lock()
	d.peers[id] = &peerState{Peer: peer, State: "pending"}
	d.mu.Unlock()
	if peer.Endpoint != "" {
		go d.punch(peer.Endpoint, 5)
	}
	_ = d.sig.Send(proto.SignalMessage{From: d.ident.ID, To: id, Type: "pair", Payload: d.localEndpoint()})
	fp := peer.Fingerprint
	if len(fp) > 16 {
		fp = fp[:16] + "…"
	}
	d.emit("connect %s endpoint=%s fp=%s (pending – Fingerprint prüfen!)", id, peer.Endpoint, fp)
	return peer, nil
}

// Ping: direkter UDP-Ping an den Peer-Endpoint, RTT messen.
func (d *Daemon) Ping(id string) (int64, error) {
	d.mu.Lock()
	ps, ok := d.peers[id]
	d.mu.Unlock()
	if !ok {
		peer, err := d.Connect(id)
		if err != nil {
			return 0, err
		}
		ps = &peerState{Peer: peer}
	}
	if ps.Peer.Endpoint == "" {
		return 0, fmt.Errorf("kein Endpoint für %s bekannt", id)
	}
	addr, err := net.ResolveUDPAddr("udp", ps.Peer.Endpoint)
	if err != nil {
		return 0, err
	}
	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	ch := make(chan struct{})
	d.mu.Lock()
	d.pongWait[nonce] = ch
	d.mu.Unlock()

	start := time.Now()
	out := []byte("lokro-ping:" + nonce)
	if _, err := d.udp.WriteToUDP(out, addr); err != nil {
		return 0, err
	}
	d.m.AddUDPOut(len(out))
	d.m.AddPeerUDP(id, len(out), false)
	select {
	case <-ch:
		rtt := time.Since(start).Milliseconds()
		d.mu.Lock()
		if p, ok := d.peers[id]; ok {
			p.LastRTT = rtt
		}
		d.mu.Unlock()
		d.emit("ping %s -> pong in %dms", id, rtt)
		return rtt, nil
	case <-time.After(3 * time.Second):
		d.mu.Lock()
		delete(d.pongWait, nonce)
		d.mu.Unlock()
		return 0, fmt.Errorf("timeout: kein pong von %s (%s)", id, ps.Peer.Endpoint)
	}
}

// --- localhost-IPC -------------------------------------------------------

func (d *Daemon) checkToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Lokro-Token") != d.token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (d *Daemon) serveIPC() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", d.checkToken(func(w http.ResponseWriter, _ *http.Request) {
		fp, _ := d.ident.Fingerprint()
		d.mu.Lock()
		peers := make(map[string]*peerState, len(d.peers))
		for k, v := range d.peers {
			cp := *v
			peers[k] = &cp
		}
		d.mu.Unlock()
		writeJSON(w, map[string]any{
			"id":          d.ident.ID,
			"fingerprint": fp,
			"udp_port":    d.cfg.UDPPort,
			"rendezvous":  d.cfg.RendezvousURL,
			"debug":       d.cfg.Debug,
			"peers":       peers,
			"traffic":     d.m.Snapshot(),
		})
	}))
	mux.HandleFunc("/v1/events", d.checkToken(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, d.bus.Last(0))
	}))
	mux.HandleFunc("/v1/debug", d.checkToken(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			On bool `json:"on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		d.cfg.Debug = body.On
		_ = config.SaveConfig(d.cfg)
		// Bestätigung auch bei debug=off einmal direkt (danach ist Ruhe).
		d.bus.Add(fmt.Sprintf("debug %s", map[bool]string{true: "on", false: "off"}[body.On]))
		writeJSON(w, map[string]any{"debug": d.cfg.Debug})
	}))
	mux.HandleFunc("/v1/connect", d.checkToken(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		peer, err := d.Connect(body.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, peer)
	}))
	mux.HandleFunc("/v1/ping", d.checkToken(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		rtt, err := d.Ping(body.ID)
		if err != nil {
			w.WriteHeader(http.StatusGatewayTimeout)
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"rtt_ms": rtt})
	}))
	return http.ListenAndServe(config.IPCAddr, mux)
}
