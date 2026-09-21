// Package contacts: Adressbuch (Name -> ID) + Connection-History.
// Alles lokal in ~/.lokronet/contacts.json (0600).
//
//   - Alias: fester Name für eine Peer-ID (+ Fingerprint zur Warnung bei Wechsel)
//   - History: jede erfolgreiche Verbindung (connect/ping) wird mit Zeitstempel
//     gespeichert; `lokronet connections history` zeigt sie. Einträge, die
//     länger als 30 Tage nicht mehr verbunden wurden, werden automatisch
//     gelöscht (Pruning bei jedem Zugriff).
package contacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RetentionDays: History-Einträge ohne Reconnect werden nach 30 Tagen gelöscht.
const RetentionDays = 30

// Contact ist ein gespeicherter Alias.
type Contact struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint"`
	AddedAt     int64  `json:"added_at"`
}

// HistoryEntry ist eine vergangene Verbindung.
type HistoryEntry struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint"`
	FirstSeen   int64  `json:"first_seen"`
	LastSeen    int64  `json:"last_seen"`
	Count       int    `json:"count"`
}

// Store ist das Adressbuch.
type Store struct {
	Contacts map[string]Contact `json:"contacts"` // key: klein geschriebener Name
	History  []HistoryEntry     `json:"history"`
}

func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".lokronet")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "contacts.json"), nil
}

// Load lädt (legt leer an, prunt alte History).
func Load() (*Store, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	s := &Store{Contacts: map[string]Contact{}}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	if s.Contacts == nil {
		s.Contacts = map[string]Contact{}
	}
	if s.prune() {
		_ = s.save()
	}
	return s, nil
}

func (s *Store) save() error {
	p, err := path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// prune löscht History ohne Reconnect seit RetentionDays. true = was gelöscht.
func (s *Store) prune() bool {
	cutoff := time.Now().AddDate(0, 0, -RetentionDays).Unix()
	kept := s.History[:0]
	changed := false
	for _, h := range s.History {
		if h.LastSeen < cutoff {
			changed = true
			continue
		}
		kept = append(kept, h)
	}
	s.History = kept
	return changed
}

func validName(name string) error {
	name = strings.TrimSpace(name)
	if len(name) < 2 || len(name) > 32 {
		return fmt.Errorf("name muss 2-32 Zeichen haben")
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return fmt.Errorf("name erlaubt nur Buchstaben, Ziffern, - und _")
		}
	}
	return nil
}

func validID(id string) error {
	if len(id) != 12 {
		return fmt.Errorf("ID muss 12 Ziffern haben")
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return fmt.Errorf("ID darf nur Ziffern enthalten")
		}
	}
	return nil
}

// Add speichert/überschreibt einen Alias.
func (s *Store) Add(name, id, fp string) error {
	if err := validName(name); err != nil {
		return err
	}
	if err := validID(id); err != nil {
		return err
	}
	key := strings.ToLower(strings.TrimSpace(name))
	s.Contacts[key] = Contact{Name: strings.TrimSpace(name), ID: id, Fingerprint: fp, AddedAt: time.Now().Unix()}
	return s.save()
}

// Remove löscht einen Alias (History bleibt).
func (s *Store) Remove(name string) error {
	key := strings.ToLower(strings.TrimSpace(name))
	if _, ok := s.Contacts[key]; !ok {
		return fmt.Errorf("kontakt %q unbekannt", name)
	}
	delete(s.Contacts, key)
	return s.save()
}

// Resolve löst Name ODER ID auf (gibt ID + Alias-Namen zurück, "" wenn keiner).
func (s *Store) Resolve(nameOrID string) (id, alias string, err error) {
	nameOrID = strings.TrimSpace(nameOrID)
	if validID(nameOrID) == nil {
		for _, c := range s.Contacts {
			if c.ID == nameOrID {
				return nameOrID, c.Name, nil
			}
		}
		return nameOrID, "", nil
	}
	c, ok := s.Contacts[strings.ToLower(nameOrID)]
	if !ok {
		return "", "", fmt.Errorf("%q ist weder Kontakt noch 12-stellige ID", nameOrID)
	}
	return c.ID, c.Name, nil
}

// RecordConnect speichert eine erfolgreiche Verbindung (prunt nebenbei).
func (s *Store) RecordConnect(id, fp string) error {
	if err := validID(id); err != nil {
		return err
	}
	now := time.Now().Unix()
	for i, h := range s.History {
		if h.ID == id {
			s.History[i].LastSeen = now
			s.History[i].Count++
			if fp != "" {
				s.History[i].Fingerprint = fp
			}
			s.prune()
			return s.save()
		}
	}
	s.History = append([]HistoryEntry{{ID: id, Fingerprint: fp, FirstSeen: now, LastSeen: now, Count: 1}}, s.History...)
	if len(s.History) > 100 {
		s.History = s.History[:100]
	}
	s.prune()
	return s.save()
}

// List gibt Einträge absteigend nach LastSeen zurück (RecordConnect stellt
// neueste voran).
func (s *Store) List() []HistoryEntry {
	return s.History
}

// AliasFor findet den Alias zu einer ID ("" wenn keiner).
func (s *Store) AliasFor(id string) string {
	for _, c := range s.Contacts {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}
