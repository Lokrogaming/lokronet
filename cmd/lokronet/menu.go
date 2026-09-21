// Menü-UI für bares `lokronet` (ohne Args).
// Bewusst nur Stdlib (bufio/fmt): läuft in cmd, PowerShell und Linux-Terminals
// ohne externe TUI-Deps. Späterer Ausbau per Bubbletea ist in docs/IDEEN.md vermerkt.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var menuIn = bufio.NewReader(os.Stdin)

// ask liest eine Zeile ("" bei EOF/Fehler -> Aufrufer beendet).
func ask(prompt string) (string, bool) {
	fmt.Print(prompt)
	line, err := menuIn.ReadString('\n')
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(line), true
}

// choose zeigt Optionen und gibt die gewählte Nummer zurück (-1 = raus).
func choose(title string, options []string) int {
	fmt.Printf("\n=== %s ===\n", title)
	for i, o := range options {
		fmt.Printf("  %d) %s\n", i+1, o)
	}
	fmt.Println("  0) Zurück / Beenden")
	for {
		s, ok := ask("> ")
		if !ok {
			return -1
		}
		var n int
		if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
			fmt.Println("bitte Zahl eingeben")
			continue
		}
		if n < 0 || n > len(options) {
			fmt.Println("außerhalb – nochmal")
			continue
		}
		return n - 1 // -1 = 0 gewählt
	}
}

// runMenu ist die Haupt-Schleife für `lokronet` ohne Argumente.
func runMenu() {
	fmt.Println("LokroNet – Menü (beta). Tipp: Namen aus Kontakten statt IDs nutzen.")
	for {
		switch choose("Hauptmenü", []string{
			"Status anzeigen",
			"Verbinden (Name oder ID)",
			"Ping (Name oder ID)",
			"Kontakte",
			"Verbindungs-History (30 Tage)",
			"Einstellungen",
			"Logs anzeigen",
			"Eigene ID / IPs",
		}) {
		case -1:
			fmt.Println("tschüss.")
			return
		case 0:
			_ = cmdStatus()
		case 1:
			if t, ok := ask("Name oder ID: "); ok && t != "" {
				_ = cmdConnect([]string{"--id", t})
			}
		case 2:
			if t, ok := ask("Name oder ID: "); ok && t != "" {
				_ = cmdPing([]string{"--id", t})
			}
		case 3:
			menuContacts()
		case 4:
			_ = cmdConnections([]string{"history"})
		case 5:
			menuSettings()
		case 6:
			_ = cmdLogs([]string{"--tail", "20"})
		case 7:
			_ = cmdStatus()
			_ = cmdIPs()
		}
	}
}

func menuContacts() {
	for {
		switch choose("Kontakte", []string{"Liste", "Hinzufügen", "Entfernen"}) {
		case -1:
			return
		case 0:
			_ = cmdContacts([]string{"list"})
		case 1:
			name, ok := ask("Name: ")
			if !ok || name == "" {
				continue
			}
			id, ok := ask("ID (12 Ziffern): ")
			if !ok || id == "" {
				continue
			}
			_ = cmdContacts([]string{"add", "--name", name, "--id", id})
		case 2:
			name, ok := ask("Name: ")
			if !ok || name == "" {
				continue
			}
			_ = cmdContacts([]string{"remove", name})
		}
	}
}

func menuSettings() {
	for {
		switch choose("Einstellungen", []string{
			"Modus anzeigen/setzen (performance|normal|eco)",
			"Mesh an/aus",
			"Debug an/aus",
		}) {
		case -1:
			return
		case 0:
			_ = cmdMode(nil)
			if m, ok := ask("Neuer Modus (leer = lassen): "); ok && m != "" {
				_ = cmdMode([]string{m})
			}
		case 1:
			_ = cmdMesh(nil)
			if m, ok := ask("Mesh on/off (leer = lassen): "); ok && (m == "on" || m == "off") {
				_ = cmdMesh([]string{m})
			}
		case 2:
			if m, ok := ask("Debug on/off: "); ok && (m == "on" || m == "off") {
				_ = cmdDebug([]string{m})
			}
		}
	}
}
