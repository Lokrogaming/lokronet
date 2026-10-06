// Package chat – lokale History-Persistenz.
//
// Design für v0.2 (Messenger-Fokus):
//   - Der Daemon ist Single Source of Truth. Er lädt ~/.lokronet/chat-history.json
//     (0600) beim Start in den Manager und speichert nach jeder ein-/ausgehenden
//     Nachricht (debounced nicht nötig – Datei ist klein, cap 5000).
//   - TUI, CLI (`lokronet chat inbox`) und Desktop-App (C# Avalonia) lesen ALLE
//     über dieselbe Daemon-IPC (/v1/chat/inbox, /v1/chat/sessions). Wer in der
//     Desktop-App schreibt (POST /v1/chat/send), sieht es sofort auch in der TUI.
//   - Dedupe: (from,to,seq,ts,text) – Seq ist pro Session/Ephemeral-Key, nach
//     Restart beginnt sie neu, daher zusätzlich ts+text als Fallback-Key.
//   - Kein Klartext in Logs (nur Metadaten). Datei lokal 0600, Wire immer
//     NaCl-box (E2E). At-rest-Verschlüsselung (age) ist als Opt-in geplant.
package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// MaxStored cappt die persistierte History (RAM-Cap MaxInbox bleibt 500 für UI-Snappy).
const MaxStored = 5000

func historyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".lokronet")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "chat-history.json"), nil
}

// LoadHistory lädt persistierte Nachrichten (leere Liste bei Fehlen/Korrupt → nicht fatal).
func LoadHistory() []Message {
	p, err := historyPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var msgs []Message
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil
	}
	return msgs
}

// SaveHistory speichert (0600, gecappt, sortiert nach Ts).
func SaveHistory(msgs []Message) error {
	p, err := historyPath()
	if err != nil {
		return err
	}
	if len(msgs) > MaxStored {
		msgs = msgs[len(msgs)-MaxStored:]
	}
	sort.Slice(msgs, func(i, j int) bool {
		if msgs[i].Ts == msgs[j].Ts {
			return msgs[i].Seq < msgs[j].Seq
		}
		return msgs[i].Ts < msgs[j].Ts
	})
	data, err := json.MarshalIndent(msgs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// ImportHistory übernimmt persistierte Nachrichten in den Manager (dedupliziert).
func (m *Manager) ImportHistory(msgs []Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]bool, len(m.inbox)+len(msgs))
	for _, x := range m.inbox {
		seen[historyKey(x)] = true
	}
	for _, x := range msgs {
		k := historyKey(x)
		if seen[k] {
			continue
		}
		seen[k] = true
		m.inbox = append(m.inbox, x)
	}
	sort.Slice(m.inbox, func(i, j int) bool {
		if m.inbox[i].Ts == m.inbox[j].Ts {
			return m.inbox[i].Seq < m.inbox[j].Seq
		}
		return m.inbox[i].Ts < m.inbox[j].Ts
	})
	if len(m.inbox) > MaxInbox {
		m.inbox = m.inbox[len(m.inbox)-MaxInbox:]
	}
}

func historyKey(x Message) string {
	return fmt.Sprintf("%s|%s|%d|%d|%s", x.From, x.To, x.Seq, x.Ts, x.Text)
}

// ExportHistory gibt eine Kopie für Persistenz/Sync zurück.
func (m *Manager) ExportHistory() []Message {
	return m.Inbox()
}

// Stats fasst pro Peer den Sync-Stand zusammen (für Beacon + newest-wins).
type PeerStats struct {
	PeerID string `json:"peer_id"`
	Count  int    `json:"count"`
	MaxTs  int64  `json:"max_ts"`
}

// Stats baut pro Peer Count + MaxTs (Inbox + Sessions-Peers, auch leere).
func (m *Manager) Stats() []PeerStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	byPeer := map[string]*PeerStats{}
	for _, s := range m.sessions {
		if _, ok := byPeer[s.peerID]; !ok {
			byPeer[s.peerID] = &PeerStats{PeerID: s.peerID}
		}
	}
	for _, x := range m.inbox {
		other := x.From
		if x.Outgoing {
			other = x.To
		}
		// Self-ID des Managers ist nicht direkt verfügbar ohne Lock-Umweg;
		// wir nehmen Gegenüber: From bei incoming, To bei outgoing.
		// Falls selfID selbst drinsteht (Loopback-Test), normalisieren:
		st := byPeer[other]
		if st == nil {
			st = &PeerStats{PeerID: other}
			byPeer[other] = st
		}
		st.Count++
		if x.Ts > st.MaxTs {
			st.MaxTs = x.Ts
		}
	}
	out := make([]PeerStats, 0, len(byPeer))
	for _, v := range byPeer {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PeerID < out[j].PeerID })
	return out
}

// MessagesSince liefert Nachrichten an/für peer mit Ts > since (aufsteigend, für Sync-Push).
func (m *Manager) MessagesSince(peerID string, since int64, limit int) []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Message
	for _, x := range m.inbox {
		if x.Ts <= since {
			continue
		}
		if x.From == peerID || x.To == peerID {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ts == out[j].Ts {
			return out[i].Seq < out[j].Seq
		}
		return out[i].Ts < out[j].Ts
	})
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// MergeImported übernimmt fremde (eigene, vom anderen Gerät) Nachrichten newest-wins.
// Gibt Anzahl übernommener Nachrichten zurück.
func (m *Manager) MergeImported(msgs []Message) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]bool, len(m.inbox))
	for _, x := range m.inbox {
		seen[historyKey(x)] = true
	}
	n := 0
	for _, x := range msgs {
		// Nur Nachrichten, die uns betreffen (selfID als From oder To).
		if x.From != m.selfID && x.To != m.selfID {
			continue
		}
		if x.Ts <= 0 {
			x.Ts = time.Now().Unix()
		}
		k := historyKey(x)
		if seen[k] {
			continue
		}
		seen[k] = true
		m.inbox = append(m.inbox, x)
		n++
	}
	sort.Slice(m.inbox, func(i, j int) bool {
		if m.inbox[i].Ts == m.inbox[j].Ts {
			return m.inbox[i].Seq < m.inbox[j].Seq
		}
		return m.inbox[i].Ts < m.inbox[j].Ts
	})
	if len(m.inbox) > MaxInbox {
		m.inbox = m.inbox[len(m.inbox)-MaxInbox:]
	}
	return n
}
