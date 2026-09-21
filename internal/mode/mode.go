// Package mode: umschaltbare Betriebsmodi zur Drosselung.
//
//	performance – schneller: Heartbeat 10s, Poll 5s, 8x Punch, ~1000 UDP-Pakete/s
//	normal      – Standard: Heartbeat 30s, Poll 15s, 5x Punch, 50 Pakete/s
//	eco         – sparsam: Heartbeat 120s, Poll 20s, 2x Punch, 5 Pakete/s
//
// Plus Meshpoint-Schalter (config MeshDisabled): deaktiviert den UDP-Socket
// komplett (nur noch Signaling). Mesh-Umschalten braucht einen Daemon-Neustart,
// Moduswechsel wirkt live.
package mode

import (
	"fmt"
	"time"
)

// Mode ist einer der drei Modi ("" = normal).
type Mode string

const (
	Performance Mode = "performance"
	Normal      Mode = "normal"
	Eco         Mode = "eco"
)

// Params fasst alle Drossel-Stellschrauben eines Modus zusammen.
type Params struct {
	Heartbeat time.Duration `json:"heartbeat"`
	PollWait  time.Duration `json:"poll_wait"`
	Punch     int           `json:"punch"`
	MaxPPS    int           `json:"max_pps"` // 0 = unbegrenzt
}

// Table ist die Quelle der Wahrheit (Admin kann hier ablesen, was jeder Modus kostet).
func Table() map[Mode]Params {
	return map[Mode]Params{
		Performance: {Heartbeat: 10 * time.Second, PollWait: 5 * time.Second, Punch: 8, MaxPPS: 1000},
		Normal:      {Heartbeat: 30 * time.Second, PollWait: 15 * time.Second, Punch: 5, MaxPPS: 50},
		Eco:         {Heartbeat: 120 * time.Second, PollWait: 20 * time.Second, Punch: 2, MaxPPS: 5},
	}
}

// Parse validiert ("", "normal" -> normal).
func Parse(s string) (Mode, error) {
	switch Mode(s) {
	case "", Normal:
		return Normal, nil
	case Performance:
		return Performance, nil
	case Eco:
		return Eco, nil
	default:
		return Normal, fmt.Errorf("unbekannter mode %q (performance|normal|eco)", s)
	}
}

// Of normalisiert (unbekannt/leer -> normal-Params, nie Panik).
func Of(s string) (Mode, Params) {
	m, _ := Parse(s)
	return m, Table()[m]
}
