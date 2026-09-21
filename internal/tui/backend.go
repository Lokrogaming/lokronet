// Package tui: interaktives Terminal-Dashboard (`lokronet dashboard`).
// Reine UI-Schicht auf Bubble Tea – Business-Logik kommt aus
// internal/client (Daemon), internal/contacts, internal/netinfo.
// Die TUI dupliziert keine Connection-/Contact-/Settings-Logik.
package tui

import (
	"sort"
	"sync"
	"time"

	"github.com/lokro/lokronet/internal/client"
	"github.com/lokro/lokronet/internal/config"
	"github.com/lokro/lokronet/internal/contacts"
	"github.com/lokro/lokronet/internal/identity"
	"github.com/lokro/lokronet/internal/netinfo"
)

// ConnRow ist eine zusammengeführte Verbindungszeile
// (Kontakte + Daemon-Peers + History, dedupliziert nach ID).
type ConnRow struct {
	Alias  string
	ID     string
	State  string // pending|trusted|known
	Mode   string
	RTT    int64 // ms, -1 = unbekannt
	Online bool
	Count  int // History-Verbindungen
}

// Snapshot ist ein fehler-toleranter Datenstand. FetchSnapshot liefert
// IMMER ein Ergebnis (offline -> Online=false + DaemonErr), damit die
// TUI niemals wegen eines Netzwerkfehlers abstürzt.
type Snapshot struct {
	TakenAt     time.Time
	Version     string
	Online      bool
	DaemonErr   string
	ID          string
	Fingerprint string
	UDPPort     int
	Rendezvous  string
	Mode        string
	Mesh        bool
	Debug       bool
	Peers       map[string]client.PeerState
	TotalIn     uint64
	TotalOut    uint64
	AvgIn       float64
	AvgOut      float64
	UptimeS     int64
	Contacts    []contacts.Contact
	History     []contacts.HistoryEntry
	Events      []client.Event
	Rows        []ConnRow
	IfAddrs     []netinfo.IfAddr
	PublicIP    string
}

var publicIPOnce = sync.OnceValue(netinfo.PublicIP)

// FetchSnapshot sammelt alles Best-Effort ein.
func FetchSnapshot(version string) Snapshot {
	s := Snapshot{TakenAt: time.Now(), Version: version}

	if ident, err := identity.Load(); err == nil {
		s.ID = ident.ID
		s.Fingerprint, _ = ident.Fingerprint()
	}
	if cfg, err := config.LoadConfig(); err == nil {
		s.Rendezvous = cfg.RendezvousURL
		if cfg.Mode == "" {
			s.Mode = "normal"
		} else {
			s.Mode = cfg.Mode
		}
		if s.UDPPort == 0 {
			s.UDPPort = cfg.UDPPort
		}
	}

	if c, err := client.Dial(); err == nil {
		if st, err := c.Status(); err == nil {
			s.Online = true
			s.ID = st.ID
			s.Fingerprint = st.Fingerprint
			s.UDPPort = st.UDPPort
			s.Rendezvous = st.Rendezvous
			s.Debug = st.Debug
			s.Mode = nonEmpty(st.Mode, s.Mode)
			s.Mesh = st.Mesh
			s.Peers = st.Peers
			s.TotalIn = st.Traffic.TotalIn
			s.TotalOut = st.Traffic.TotalOut
			s.AvgIn = st.Traffic.AvgBpsIn
			s.AvgOut = st.Traffic.AvgBpsOut
			s.UptimeS = st.Traffic.UptimeS
		} else {
			s.DaemonErr = err.Error()
		}
		if ev, err := c.Events(); err == nil {
			s.Events = ev
		}
	} else {
		s.DaemonErr = "daemon offline – `lokronet daemon` starten"
	}

	if store, err := contacts.Load(); err == nil {
		for _, ct := range store.Contacts {
			s.Contacts = append(s.Contacts, ct)
		}
		sort.Slice(s.Contacts, func(i, j int) bool { return s.Contacts[i].Name < s.Contacts[j].Name })
		s.History = store.List()
		s.Rows = mergeRows(store, s.Peers)
	}

	s.IfAddrs = netinfo.LocalAddrs()
	s.PublicIP = publicIPOnce()
	return s
}

func nonEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// mergeRows vereint Kontakte, Daemon-Peers und History nach ID.
func mergeRows(store *contacts.Store, peers map[string]client.PeerState) []ConnRow {
	byID := map[string]*ConnRow{}
	get := func(id string) *ConnRow {
		r, ok := byID[id]
		if !ok {
			r = &ConnRow{ID: id, State: "known", RTT: -1}
			byID[id] = r
		}
		return r
	}
	for _, c := range store.Contacts {
		r := get(c.ID)
		r.Alias = c.Name
	}
	for _, h := range store.List() {
		r := get(h.ID)
		r.Count = h.Count
		if a := store.AliasFor(h.ID); a != "" {
			r.Alias = a
		}
	}
	for id, p := range peers {
		r := get(id)
		r.Online = true
		r.State = nonEmpty(p.State, "pending")
		r.Mode = p.Peer.Mode
		r.LastRTT(p.LastRTT)
		if a := store.AliasFor(id); a != "" {
			r.Alias = a
		}
	}
	rows := make([]ConnRow, 0, len(byID))
	for _, r := range byID {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Online != rows[j].Online {
			return rows[i].Online
		}
		if rows[i].Alias != rows[j].Alias {
			return rows[i].Alias < rows[j].Alias
		}
		return rows[i].ID < rows[j].ID
	})
	return rows
}

// LastRTT setzt RTT (Daemon liefert 0 bei unbekannt -> -1 anzeigen).
func (r *ConnRow) LastRTT(ms int64) {
	if ms <= 0 {
		r.RTT = -1
		return
	}
	r.RTT = ms
}
