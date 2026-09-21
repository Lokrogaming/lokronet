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
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lokro/lokronet/internal/config"
	"github.com/lokro/lokronet/internal/contacts"
	"github.com/lokro/lokronet/internal/daemon"
	"github.com/lokro/lokronet/internal/identity"
	"github.com/lokro/lokronet/internal/mode"
	"github.com/lokro/lokronet/internal/netcore"
	"github.com/lokro/lokronet/internal/signal"
)

// Version wird beim Release-Build per ldflags gesetzt:
// go build -ldflags "-X main.Version=0.1.0" ./cmd/lokronet
var Version = "0.1.0-dev"

func usage() {
	fmt.Println(`lokronet – Meshnet-CLI (MVP: Mesh + Debug)

  lokronet version
  lokronet setup [--rendezvous URL] [--port N] [--endpoint ip:port]
  lokronet daemon                       Backend starten (Vordergrund)
  lokronet rendezvous [--addr 127.0.0.1:8787]
  lokronet status
  lokronet connect --id <12 Ziffern>
  lokronet ping --id <12 Ziffern>
  lokronet debug on|off
  lokronet mode [performance|normal|eco]
  lokronet mesh [on|off]
  lokronet contacts add --name NAME --id ID
  lokronet contacts list
  lokronet contacts remove NAME
  lokronet connections history
  lokronet ips
  lokronet logs [--tail N]`)
}

func main() {
	if len(os.Args) < 2 {
		runMenu() // bares `lokronet` öffnet das Menü
		return
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
	case "mode":
		err = cmdMode(os.Args[2:])
	case "mesh":
		err = cmdMesh(os.Args[2:])
	case "contacts":
		err = cmdContacts(os.Args[2:])
	case "connections":
		err = cmdConnections(os.Args[2:])
	case "logs":
		err = cmdLogs(os.Args[2:])
	case "ips":
		err = cmdIPs()
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
	endpoint := fs.String("endpoint", "", "Öffentlicher Endpoint ip:port (Pflicht hinter NAT, sonst 127.0.0.1)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.RendezvousURL = *rzURL
	cfg.Endpoint = *endpoint
	modeName, _ := mode.Parse(cfg.Mode)
	cfg.Mode = string(modeName)
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
		peer, err := ident.ToPeer(publicEndpoint(cfg), cfg.Mode)
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
	fmt.Printf("setup ok\n  id:          %s\n  fingerprint: %s\n  udp-port:    %d\n  endpoint:    %s\n  rendezvous:  %s\n", ident.ID, fp, cfg.UDPPort, publicEndpoint(cfg), cfg.RendezvousURL)
	fmt.Println("nächste Schritte:")
	fmt.Println("  1) lokronet rendezvous   (in eigenem Terminal, lokaler Test-Server)")
	fmt.Println("  2) lokronet daemon       (Backend starten)")
	fmt.Println("  3) lokronet status")
	return nil
}

// publicEndpoint: gleiche Logik wie im Daemon (Registrierung meldet,
// was Heartbeat später bestätigt).
func publicEndpoint(cfg *config.Config) string {
	if cfg.Endpoint != "" {
		return cfg.Endpoint
	}
	return fmt.Sprintf("127.0.0.1:%d", cfg.UDPPort)
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
	target := fs.String("id", "", "Peer-ID (12 Ziffern) oder Kontaktname")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return fmt.Errorf("--id mit ID oder Kontaktname angeben")
	}
	id, err := resolveTarget(*target)
	if err != nil {
		return err
	}
	data, err := ipcCall("POST", "/v1/connect", map[string]string{"id": id})
	if err != nil {
		return err
	}
	fmt.Printf("connect ok (pending – Fingerprint out-of-band prüfen!)\n%s\n", string(data))
	recordHistory(id)
	return nil
}

func cmdPing(args []string) error {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	target := fs.String("id", "", "Peer-ID (12 Ziffern) oder Kontaktname")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return fmt.Errorf("--id mit ID oder Kontaktname angeben")
	}
	id, err := resolveTarget(*target)
	if err != nil {
		return err
	}
	data, err := ipcCall("POST", "/v1/ping", map[string]string{"id": id})
	if err != nil {
		return err
	}
	fmt.Printf("ping ok: %s\n", string(data))
	recordHistory(id)
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

// --- mode / mesh -------------------------------------------------------------
// Liest/schreibt Modus + Mesh-Schalter. Mit laufendem Daemon live per IPC,
// sonst direkt in der Config (wirkt beim nächsten Daemon-Start).

func modeFromIPC() (string, bool, error) {
	data, err := ipcCall("GET", "/v1/mode", nil)
	if err != nil {
		return "", false, err
	}
	var v struct {
		Mode string `json:"mode"`
		Mesh bool   `json:"mesh"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return "", false, err
	}
	return v.Mode, v.Mesh, nil
}

func setModeIPC(setMode *string, setMesh *bool) (string, error) {
	body := map[string]any{}
	if setMode != nil {
		body["mode"] = *setMode
	}
	if setMesh != nil {
		body["mesh"] = *setMesh
	}
	data, err := ipcCall("POST", "/v1/mode", body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func cmdMode(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("nutze: lokronet mode [performance|normal|eco]")
	}
	if len(args) == 0 {
		if m, _, err := modeFromIPC(); err == nil {
			fmt.Printf("mode: %s (Daemon, live)\n", m)
			return nil
		}
		cfg, err := config.LoadConfig()
		if err != nil {
			return err
		}
		if cfg.Mode == "" {
			cfg.Mode = "normal"
		}
		fmt.Printf("mode: %s (gespeichert, Daemon offline)\n", cfg.Mode)
		return nil
	}
	m := args[0]
	if m != "performance" && m != "normal" && m != "eco" {
		return fmt.Errorf("unbekannter mode %q (performance|normal|eco)", m)
	}
	if out, err := setModeIPC(&m, nil); err == nil {
		fmt.Printf("mode: %s (live) %s\n", m, out)
		return nil
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.Mode = m
	if err := config.SaveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("mode: %s (gespeichert, wirkt beim Daemon-Start)\n", m)
	return nil
}

func cmdMesh(args []string) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "on" && args[0] != "off") {
		return fmt.Errorf("nutze: lokronet mesh [on|off]")
	}
	if len(args) == 0 {
		if _, mesh, err := modeFromIPC(); err == nil {
			fmt.Printf("mesh: %s (Daemon)\n", map[bool]string{true: "on", false: "off"}[mesh])
			return nil
		}
		cfg, err := config.LoadConfig()
		if err != nil {
			return err
		}
		fmt.Printf("mesh: %s (gespeichert, Daemon offline)\n", map[bool]string{true: "on", false: "off"}[!cfg.MeshDisabled])
		return nil
	}
	want := args[0] == "on"
	if out, err := setModeIPC(nil, &want); err == nil {
		fmt.Printf("mesh: %s %s\n", args[0], out)
		fmt.Println("hinweis: Mesh-Umschaltung braucht einen Daemon-Neustart")
		return nil
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.MeshDisabled = !want
	if err := config.SaveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("mesh: %s (gespeichert, wirkt beim Daemon-Start)\n", args[0])
	return nil
}

// --- contacts / connections --------------------------------------------------
// Adressbuch: Name statt ID merken + History der letzten 30 Tage.

func cmdContacts(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("nutze: lokronet contacts [add|list|remove] ...")
	}
	store, err := contacts.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("contacts add", flag.ContinueOnError)
		name := fs.String("name", "", "Anzeigename (2-32 Zeichen, a-z 0-9 - _)")
		id := fs.String("id", "", "Peer-ID (12 Ziffern)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *name == "" || *id == "" {
			return fmt.Errorf("nutze: lokronet contacts add --name NAME --id ID")
		}
		fp := lookupFingerprint(*id) // best effort
		if err := store.Add(*name, *id, fp); err != nil {
			return err
		}
		fmt.Printf("kontakt %q -> %s gespeichert\n", *name, *id)
		return nil
	case "list":
		if len(store.Contacts) == 0 {
			fmt.Println("(keine Kontakte – `lokronet contacts add --name NAME --id ID`)")
			return nil
		}
		for _, c := range store.Contacts {
			fp := c.Fingerprint
			if len(fp) > 16 {
				fp = fp[:16] + "…"
			}
			fmt.Printf("  %-16s %s  fp:%s\n", c.Name, c.ID, fp)
		}
		return nil
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("nutze: lokronet contacts remove NAME")
		}
		if err := store.Remove(args[1]); err != nil {
			return err
		}
		fmt.Printf("kontakt %q gelöscht\n", args[1])
		return nil
	default:
		return fmt.Errorf("unbekannt: %q (add|list|remove)", args[0])
	}
}

func cmdConnections(args []string) error {
	if len(args) != 1 || args[0] != "history" {
		return fmt.Errorf("nutze: lokronet connections history")
	}
	store, err := contacts.Load()
	if err != nil {
		return err
	}
	hist := store.List()
	if len(hist) == 0 {
		fmt.Println("(keine Verbindungen in den letzten 30 Tagen)")
		return nil
	}
	for _, h := range hist {
		alias := store.AliasFor(h.ID)
		show := h.ID
		if alias != "" {
			show = fmt.Sprintf("%s (%s)", alias, h.ID)
		}
		fmt.Printf("  %-24s %dx  zuletzt %s\n", show, h.Count, time.Unix(h.LastSeen, 0).Format("02.01. 15:04"))
	}
	fmt.Println("(Einträge ohne Reconnect werden nach 30 Tagen automatisch gelöscht)")
	return nil
}

// lookupFingerprint holt den Fingerprint best-effort ("" bei Fehler/offline).
func lookupFingerprint(id string) string {
	cfg, err := config.LoadConfig()
	if err != nil {
		return ""
	}
	p, err := signal.NewClient(cfg.RendezvousURL).Lookup(id)
	if err != nil {
		return ""
	}
	return p.Fingerprint
}

// resolveTarget löst Name ODER ID auf (für connect/ping).
func resolveTarget(nameOrID string) (string, error) {
	store, err := contacts.Load()
	if err != nil {
		return "", err
	}
	id, alias, err := store.Resolve(nameOrID)
	if err != nil {
		return "", err
	}
	if alias != "" && alias != nameOrID {
		fmt.Printf("(Kontakt %q -> %s)\n", alias, id)
	}
	return id, nil
}

// recordHistory speichert erfolgreiche Verbindungen (Fehler werden ignoriert,
// History darf nie einen Connect kaputtmachen).
func recordHistory(id string) {
	store, err := contacts.Load()
	if err != nil {
		return
	}
	_ = store.RecordConnect(id, lookupFingerprint(id))
}

// --- ips ---------------------------------------------------------------------
// Zeigt lokale + öffentliche IP (zum Weiterleiten an andere Peers).
// Nur Stdlib: Interfaces lokal auslesen, Public-IP via ipify/ifconfig.me.

func cmdIPs() error {
	fmt.Println("lokale IPs:")
	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	found := false
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			fmt.Printf("  %-10s %s\n", iface.Name, ip.String())
			found = true
		}
	}
	if !found {
		fmt.Println("  (keine gefunden)")
	}
	fmt.Printf("öffentliche IP für Connections: %s\n", publicIP())
	fmt.Println("hinweis: IPv6 beim setup in eckige Klammern: --endpoint \"[ip]:port\"")
	return nil
}

// publicIP fragt externe Dienste (5s-Timeout, tolerant bei Offline).
func publicIP() string {
	client := &http.Client{Timeout: 5 * time.Second}
	for _, url := range []string{"https://api.ipify.org", "https://ifconfig.me"} {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if err != nil {
			continue
		}
		if ip := strings.TrimSpace(string(data)); ip != "" && net.ParseIP(ip) != nil {
			return ip
		}
	}
	return "(unbekannt – offline?)"
}

func cmdLogs(args []string) error {	data, err := ipcCall("GET", "/v1/events", nil)
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
