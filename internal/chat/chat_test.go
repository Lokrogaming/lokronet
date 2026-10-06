package chat

import (
	"encoding/base64"
	"strings"
	"testing"
)

func decodeB64(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// newPairClean verdrahtet zwei Manager direkt (Signal sofort zugestellt).
// OnHello schickt das Ack selbst (nach Unlock), der Wrapper stellt nur zu.
func newPairClean() (*Manager, *Manager) {
	var a, b *Manager
	mkHook := func(self string, peer **Manager, peerID string) func(to, typ, payload string) {
		return func(to, typ, payload string) {
			if to != peerID || *peer == nil {
				return
			}
			switch typ {
			case "msg-hello":
				_ = (*peer).OnHello(self, payload)
			case "msg-hello-ack":
				_ = (*peer).OnAck(self, payload)
			}
		}
	}
	a = NewManager("A", Hooks{
		SendSignal:     mkHook("A", &b, "B"),
		SendUDP:        func(endpoint string, data []byte) error { return nil },
		LookupEndpoint: func(peerID string) (string, bool) { return "ep", true },
	})
	b = NewManager("B", Hooks{
		SendSignal:     mkHook("B", &a, "A"),
		SendUDP:        func(endpoint string, data []byte) error { return nil },
		LookupEndpoint: func(peerID string) (string, bool) { return "ep", true },
	})
	return a, b
}

func TestHandshakeCodes(t *testing.T) {
	a, b := newPairClean()
	if _, err := a.Send("B", "hallo"); err != nil {
		t.Fatal(err)
	}
	sa := a.Sessions()
	sb := b.Sessions()
	if len(sa) != 1 || len(sb) != 1 || !sa[0].Ready || !sb[0].Ready {
		t.Fatalf("sessions nicht bereit: %+v %+v", sa, sb)
	}
	if sa[0].Code == "" || sa[0].Code != sb[0].Code {
		t.Fatalf("codes ungleich: %q vs %q", sa[0].Code, sb[0].Code)
	}
}

func TestRoundtrip(t *testing.T) {
	a, b := newPairClean()
	var captured []byte
	a.hooks.SendUDP = func(endpoint string, data []byte) error {
		captured = append([]byte(nil), data...)
		return nil
	}
	if _, err := a.Send("B", "ping von A"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(captured), UDPPrefix) {
		t.Fatalf("falsches Framing: %q", captured)
	}
	msg, ok := b.OnUDP(decodeB64(t, string(captured[len(UDPPrefix):])))
	if !ok || msg == nil {
		t.Fatal("B öffnet nicht")
	}
	if msg.Text != "ping von A" || msg.From != "A" || msg.Outgoing {
		t.Fatalf("msg = %+v", msg)
	}
	var back []byte
	b.hooks.SendUDP = func(endpoint string, data []byte) error {
		back = append([]byte(nil), data...)
		return nil
	}
	if _, err := b.Send("A", "pong von B"); err != nil {
		t.Fatal(err)
	}
	msg2, ok := a.OnUDP(decodeB64(t, string(back[len(UDPPrefix):])))
	if !ok || msg2.Text != "pong von B" {
		t.Fatalf("antwort = %+v %v", msg2, ok)
	}
}

func TestReplayDropped(t *testing.T) {
	a, b := newPairClean()
	var captured []byte
	a.hooks.SendUDP = func(endpoint string, data []byte) error {
		captured = append([]byte(nil), data...)
		return nil
	}
	if _, err := a.Send("B", "einmal"); err != nil {
		t.Fatal(err)
	}
	raw := decodeB64(t, string(captured[len(UDPPrefix):]))
	if _, ok := b.OnUDP(raw); !ok {
		t.Fatal("erste Zustellung scheitert")
	}
	n0 := len(b.Inbox())
	if _, ok := b.OnUDP(raw); ok {
		t.Fatal("Replay wurde angenommen")
	}
	if len(b.Inbox()) != n0 {
		t.Fatal("Replay landete in Inbox")
	}
}

func TestIsolation(t *testing.T) {
	a, _ := newPairClean()
	eve := NewManager("E", Hooks{})
	var captured []byte
	a.hooks.SendUDP = func(endpoint string, data []byte) error {
		captured = append([]byte(nil), data...)
		return nil
	}
	if _, err := a.Send("B", "geheim"); err != nil {
		t.Fatal(err)
	}
	if _, ok := eve.OnUDP(decodeB64(t, string(captured[len(UDPPrefix):]))); ok {
		t.Fatal("Dritter öffnet fremde Box")
	}
}

func TestOversizeRejected(t *testing.T) {
	a, _ := newPairClean()
	if _, err := a.Send("B", strings.Repeat("x", MaxTextLen+1)); err == nil {
		t.Fatal("Überlänge akzeptiert")
	}
}

func TestQueuedBeforeAck(t *testing.T) {
	// Manager ohne Gegenstelle: Send queued + Hello geht raus.
	var helloSent bool
	a := NewManager("A", Hooks{
		SendSignal: func(to, typ, payload string) {
			if typ == "msg-hello" {
				helloSent = true
			}
		},
		SendUDP:        func(endpoint string, data []byte) error { return nil },
		LookupEndpoint: func(peerID string) (string, bool) { return "ep", true },
	})
	st, err := a.Send("Niemand", "wartet")
	if err != nil {
		t.Fatal(err)
	}
	if st != "queued (Handshake läuft)" {
		t.Fatalf("status = %q", st)
	}
	if !helloSent {
		t.Fatal("kein Hello verschickt")
	}
	if got := a.Sessions()[0].Pending; got != 1 {
		t.Fatalf("pending = %d", got)
	}
}
