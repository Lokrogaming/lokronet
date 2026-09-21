// Package debug ist der schaltbare Event-Bus für Bestätigungsmeldungen
// (lokronet debug on|off). Enthält NIE Keys oder Inhalte, nur Metadaten.
package debug

import (
	"sync"
	"time"
)

// Event ist eine einzelne Debug-/Bestätigungsmeldung.
type Event struct {
	Time time.Time `json:"time"`
	Msg  string    `json:"msg"`
}

// Bus hält einen Ringpuffer der letzten Events (Standard: 200).
type Bus struct {
	mu     sync.Mutex
	events []Event
	max    int
}

// NewBus erzeugt einen Bus mit Puffergröße max (<=0 -> 200).
func NewBus(max int) *Bus {
	if max <= 0 {
		max = 200
	}
	return &Bus{max: max}
}

// Add hängt eine Meldung an (nur Metadaten, keine Secrets loggen!).
func (b *Bus) Add(msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, Event{Time: time.Now(), Msg: msg})
	if len(b.events) > b.max {
		b.events = b.events[len(b.events)-b.max:]
	}
}

// Last gibt die neuesten n Events zurück (n<=0 -> alle).
func (b *Bus) Last(n int) []Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n <= 0 || n > len(b.events) {
		n = len(b.events)
	}
	out := make([]Event, n)
	copy(out, b.events[len(b.events)-n:])
	return out
}
