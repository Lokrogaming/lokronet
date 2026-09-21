// lokronet – CLI (dünn) + Backend (daemon, rendezvous).
// Architektur: ALLE Netz-Inputs verarbeitet der Daemon; die CLI spricht
// mit ihm nur über localhost-IPC (127.0.0.1:37777 + Token).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/lokro/lokronet/internal/config"
	"github.com/lokro/lokronet/internal/daemon"
	"github.com/lokro/lokronet/internal/identity"
	"github.com/lokro/lokronet/internal/netcore"
	"github.com/lokro/lokronet/internal/signal"
)

// Version wird beim Release-Build per ldflags gesetzt:
// go build -ldflags "-X main.Version=0.1.0" ./cmd/lokronet
var Version = "0.1.0-dev"

func usage() {
	fmt.Println(`lokronet – Meshnet-CLI (MVP: Mesh + Debug)

  lokronet version
  lokronet setup [--rendezvous URL] [--port N]
  lokronet daemon                       Backend starten (Vordergrund)
  lokronet rendezvous [--addr 127.0.0.1:8787]
  lokronet status
  lokronet connect --id <12 Ziffern>
  lokronet ping --id <12 Ziffern>
  lokronet debug on|off
  lokronet logs [--tail N]`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "version":
		fmt.Println("lokronet", Version)
		return
	case "setup":
		err = cmdSetup(os.Args[2:])
	case "daemon":
		err = cmdDaemon()
	case "rendezvous":
		err = cmdRendezvous(os.Args[2:])
	case "status":
		err = cmdStatus()
	case "connect":
		err = cmdConnect(os.Args[2:])
	case "ping":
		err = cmdPing(os.Args[2:])
	case "debug":
		err = cmdDebug(os.Args[2:])
	case "logs":
		err = cmdLogs(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fehler:", err)
		os.Exit(1)
	}
}

// --- setup ---------------------------------------------------------------
// Erzeugt ID + Keys, wählt UDP-Port, registriert am Rendezvous.
// Bei 409 (ID vergeben) wird bis zu 3x neu gewürfelt.
func cmdSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	rzURL := fs.String("rendezvous", config.DefaultRendezvous, "Rendezvous-Basis-URL")
	port := fs.Int("port", 51820, "Wunsch-UDP-Port (0=auto)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.RendezvousURL = *rzURL
	client := signal.NewClient(cfg.RendezvousURL)

	var ident *identity.Identity
	for tries := 0; tries < 4; tries++ {
		ident, err = identity.Generate()
		if err != nil {
			return err
		}
		if *port == 0 {
			cfg.UDPPort, err = netcore.FindFreeUDPPort(51820)
		} else {
			cfg.UDPPort, err = netcore.FindFreeUDPPort(*port)
		}
		if err != nil {
			return err
		}
		peer, err := ident.ToPeer(fmt.Sprintf("127.0.0.1:%d", cfg.UDPPort))
		if err != nil {
			return err
		}
		if err = client.Register(peer); err == nil {
			break
		}
		if tries == 3 {
			return fmt.Errorf("registrierung fehlgeschlagen: %w", err)
		}
		// Vermutlich 409 -> neu würfeln.
	}
	if err := ident.Save(); err != nil {
		return err
	}
	if err := config.SaveConfig(cfg); err != nil {
		return err
	}
	fp, _ := ident.Fingerprint()
	fmt.Printf("setup ok\n  id:          %s\n  fingerprint: %s\n  udp-port:    %d\n  rendezvous:  %s\n", ident.ID, fp, cfg.UDPPort, cfg.RendezvousURL)
	fmt.Println("nächste Schritte:")
	fmt.Println("  1) lokronet rendezvous   (in eigenem Terminal, lokaler Test-Server)")
	fmt.Println("  2) lokronet daemon       (Backend starten)")
	fmt.Println("  3) lokronet status")
	return nil
}

// --- daemon / rendezvous ---------------------------------------------------

func cmdDaemon() error {
	d, err := daemon.New()
	if err != nil {
		return err
	}
	fmt.Println("daemon läuft (IPC nur 127.0.0.1:37777, STRG+C zum Stoppen) …")
	return d.Start()
}

func cmdRendezvous(args []string) error {
	fs := flag.NewFlagSet("rendezvous", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8787", "Listen-Adresse")
	if err := fs.Parse(args); err != nil {
		return err
	}
	srv := &http.Server{Addr: *addr, Handler: signal.NewServer().Handler()}
	fmt.Printf("rendezvous läuft auf %s …\n", *addr)
	return srv.ListenAndServe()
}

// --- localhost-IPC-Helfer ---------------------------------------------------

func ipcCall(method, path string, body any) ([]byte, error) {
	token, err := config.DaemonToken()
	if err != nil {
		return nil, err
	}
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, "http://"+config.IPCAddr+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Lokro-Token", token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("daemon nicht erreichbar – `lokronet daemon` gestartet? (%w)", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return data, fmt.Errorf("daemon %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

// --- status / connect / ping / debug / logs --------------------------------

func cmdStatus() error {
	data, err := ipcCall("GET", "/v1/status", nil)
	if err == nil {
		var pretty bytes.Buffer
		_ = json.Indent(&pretty, data, "", "  ")
		fmt.Println(pretty.String())
		return nil
	}
	// Fallback ohne Daemon: lokale Dateien.
	ident, ierr := identity.Load()
	if ierr != nil {
		return err // Daemon-Fehler ist relevanter
	}
	cfg, _ := config.LoadConfig()
	fp, _ := ident.Fingerprint()
	fmt.Printf("(daemon offline – lokale Werte)\n  id:          %s\n  fingerprint: %s\n  udp-port:    %d\n  rendezvous:  %s\n", ident.ID, fp, cfg.UDPPort, cfg.RendezvousURL)
	return nil
}

func cmdConnect(args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	id := fs.String("id", "", "Peer-ID (12 Ziffern)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(*id) != 12 {
		return fmt.Errorf("--id mit 12 Ziffern angeben")
	}
	data, err := ipcCall("POST", "/v1/connect", map[string]string{"id": *id})
	if err != nil {
		return err
	}
	fmt.Printf("connect ok (pending – Fingerprint out-of-band prüfen!)\n%s\n", string(data))
	return nil
}

func cmdPing(args []string) error {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	id := fs.String("id", "", "Peer-ID (12 Ziffern)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(*id) != 12 {
		return fmt.Errorf("--id mit 12 Ziffern angeben")
	}
	data, err := ipcCall("POST", "/v1/ping", map[string]string{"id": *id})
	if err != nil {
		return err
	}
	fmt.Printf("ping ok: %s\n", string(data))
	return nil
}

func cmdDebug(args []string) error {
	if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
		return fmt.Errorf("nutze: lokronet debug on|off")
	}
	on := args[0] == "on"
	if data, err := ipcCall("POST", "/v1/debug", map[string]bool{"on": on}); err == nil {
		fmt.Printf("debug %s (Daemon)\n", args[0])
		_ = data
		return nil
	}
	// Fallback: Config direkt schreiben (wirkt beim nächsten Daemon-Start).
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.Debug = on
	if err := config.SaveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("debug %s (gespeichert, Daemon offline)\n", args[0])
	return nil
}

func cmdLogs(args []string) error {
	data, err := ipcCall("GET", "/v1/events", nil)
	if err != nil {
		return err
	}
	var events []struct {
		Time string `json:"time"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(data, &events); err != nil {
		return err
	}
	tail := len(events)
	if len(args) == 2 && args[0] == "--tail" {
		var n int
		_, _ = fmt.Sscanf(args[1], "%d", &n)
		if n > 0 && n < tail {
			events = events[tail-n:]
		}
	}
	if len(events) == 0 {
		fmt.Println("(keine Events – debug mit `lokronet debug on` einschalten)")
		return nil
	}
	for _, e := range events {
		fmt.Printf("[%s] %s\n", e.Time, e.Msg)
	}
	return nil
}
