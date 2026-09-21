package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeDaemon antwortet wie der echte Daemon (inkl. Token-Check).
func fakeDaemon(t *testing.T, token string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	check := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Lokro-Token") != token {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/v1/status", check(func(w http.ResponseWriter, _ *http.Request) {
		write(w, map[string]any{
			"id": "111111111111", "fingerprint": "abc", "udp_port": 51820,
			"rendezvous": "http://127.0.0.1:8787", "debug": true,
			"mode": "eco", "mesh": false, "peers": map[string]any{},
			"traffic": map[string]any{"total_in_bytes": 10, "total_out_bytes": 20},
		})
	}))
	mux.HandleFunc("/v1/events", check(func(w http.ResponseWriter, _ *http.Request) {
		write(w, []map[string]any{{"time": time.Now().UTC().Format(time.RFC3339), "msg": "hi"}})
	}))
	mux.HandleFunc("/v1/connect", check(func(w http.ResponseWriter, _ *http.Request) {
		write(w, map[string]any{"id": "222222222222", "fingerprint": "fp"})
	}))
	mux.HandleFunc("/v1/ping", check(func(w http.ResponseWriter, _ *http.Request) {
		write(w, map[string]any{"rtt_ms": 24})
	}))
	mux.HandleFunc("/v1/mode", check(func(w http.ResponseWriter, _ *http.Request) {
		write(w, map[string]any{"mode": "eco", "mesh": false})
	}))
	mux.HandleFunc("/v1/debug", check(func(w http.ResponseWriter, _ *http.Request) {
		write(w, map[string]any{"debug": true})
	}))
	return httptest.NewServer(mux)
}

func testClient(t *testing.T) *Client {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("USERPROFILE", dir)
	t.Setenv("HOME", dir)
	// Token vorab erzeugen (Dial lädt/erzeugt es aus ~/.lokronet).
	c0, err := Dial()
	if err != nil {
		t.Fatal(err)
	}
	ts := fakeDaemon(t, c0.token)
	t.Cleanup(ts.Close)
	old := ipcAddr
	ipcAddr = strings.TrimPrefix(ts.URL, "http://")
	t.Cleanup(func() { ipcAddr = old })
	c, err := Dial()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestStatus(t *testing.T) {
	st, err := testClient(t).Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.ID != "111111111111" || st.Mode != "eco" || st.Mesh {
		t.Fatalf("status = %+v", st)
	}
	if st.Traffic.TotalIn != 10 || st.Traffic.TotalOut != 20 {
		t.Fatalf("traffic = %+v", st.Traffic)
	}
}

func TestEvents(t *testing.T) {
	ev, err := testClient(t).Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 1 || ev[0].Msg != "hi" {
		t.Fatalf("events = %+v", ev)
	}
}

func TestConnectPing(t *testing.T) {
	c := testClient(t)
	p, err := c.Connect("222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "222222222222" {
		t.Fatalf("peer = %+v", p)
	}
	rtt, err := c.Ping("222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if rtt != 24 {
		t.Fatalf("rtt = %d", rtt)
	}
}

func TestModeDebug(t *testing.T) {
	c := testClient(t)
	m, err := c.GetMode()
	if err != nil {
		t.Fatal(err)
	}
	if m.Mode != "eco" || m.Mesh {
		t.Fatalf("mode = %+v", m)
	}
	if err := c.SetDebug(true); err != nil {
		t.Fatal(err)
	}
	nm := "normal"
	if _, err := c.SetMode(&nm, nil); err != nil {
		t.Fatal(err)
	}
}

func TestUnauthorized(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("USERPROFILE", dir)
	t.Setenv("HOME", dir)
	ts := fakeDaemon(t, "falsches-token")
	t.Cleanup(ts.Close)
	old := ipcAddr
	ipcAddr = strings.TrimPrefix(ts.URL, "http://")
	t.Cleanup(func() { ipcAddr = old })
	c, err := Dial()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Status(); err == nil {
		t.Fatal("erwarte 401-Fehler")
	}
}
