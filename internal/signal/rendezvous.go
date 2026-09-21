// Package signal: Rendezvous-Server (Discovery + Signaling ONLY).
// Hier fließen NIE Nutzdaten/Fileinhalte – nur Peer-Einträge +
// kleine Signaling-Nachrichten für Hole-Punching/Pairing.
package signal

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/lokro/lokronet/pkg/proto"
)

// Server hält Peers + Mailboxen im Speicher (MVP; P1: SQLite/Persistenz).
type Server struct {
	mu       sync.Mutex
	peers    map[string]proto.Peer
	mailbox  map[string][]proto.SignalMessage
}

// NewServer erzeugt einen leeren Server.
func NewServer() *Server {
	return &Server{
		peers:   make(map[string]proto.Peer),
		mailbox: make(map[string][]proto.SignalMessage),
	}
}

// Handler verdrahtet alle /v1/-Routen.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/register", s.handleRegister)
	mux.HandleFunc("/v1/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("/v1/lookup", s.handleLookup)
	mux.HandleFunc("/v1/signal", s.handleSignal)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST erwartet"})
		return
	}
	var req proto.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ungültiges JSON"})
		return
	}
	if len(req.Peer.ID) != 12 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID muss 12 Ziffern haben"})
		return
	}
	if req.Peer.EdPubB64 == "" || req.Peer.Fingerprint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pubkey/fingerprint fehlt"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.peers[req.Peer.ID]; ok && existing.EdPubB64 != req.Peer.EdPubB64 {
		// ID-Kollision mit anderem Key -> Client muss neu würfeln.
		writeJSON(w, http.StatusConflict, map[string]string{"error": "ID vergeben, bitte setup erneut ausführen"})
		return
	}
	req.Peer.LastSeen = time.Now().Unix()
	s.peers[req.Peer.ID] = req.Peer
	writeJSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST erwartet"})
		return
	}
	var req proto.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ungültiges JSON"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.peers[req.ID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unbekannte ID"})
		return
	}
	if req.Endpoint != "" {
		p.Endpoint = req.Endpoint
	}
	p.LastSeen = time.Now().Unix()
	s.peers[req.ID] = p
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if len(id) != 12 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id fehlt/ungültig"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.peers[id]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Peer unbekannt"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// handleSignal: POST legt Nachricht in die Mailbox des Empfängers,
// GET ?to=ID&wait=ms holt ab (Long-Poll = MVP-Realtime-Kanal).
func (s *Server) handleSignal(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var m proto.SignalMessage
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ungültiges JSON"})
			return
		}
		if len(m.To) != 12 || len(m.From) != 12 || m.Type == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from/to/type prüfen"})
			return
		}
		m.SentAt = time.Now().Unix()
		s.mu.Lock()
		s.mailbox[m.To] = append(s.mailbox[m.To], m)
		// Mailbox begrenzen (DoS-Schutz, MVP).
		if len(s.mailbox[m.To]) > 50 {
			s.mailbox[m.To] = s.mailbox[m.To][len(s.mailbox[m.To])-50:]
		}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
	case http.MethodGet:
		to := r.URL.Query().Get("to")
		if len(to) != 12 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to fehlt/ungültig"})
			return
		}
		waitMs := 0
		if v := r.URL.Query().Get("wait"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 20000 {
				waitMs = n
			}
		}
		deadline := time.Now().Add(time.Duration(waitMs) * time.Millisecond)
		for {
			s.mu.Lock()
			msgs := s.mailbox[to]
			if len(msgs) > 0 {
				delete(s.mailbox, to)
				s.mu.Unlock()
				writeJSON(w, http.StatusOK, msgs)
				return
			}
			s.mu.Unlock()
			if time.Now().After(deadline) {
				writeJSON(w, http.StatusOK, []proto.SignalMessage{})
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST/GET erwartet"})
	}
}
