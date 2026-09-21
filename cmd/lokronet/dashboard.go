package main

import (
	"flag"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lokro/lokronet/internal/tui"
)

// cmdDashboard startet das interaktive Terminal-Dashboard (Alias: dash).
// --dump gibt stattdessen eine Text-Momentaufnahme aus (ohne TTY,
// für Scripts/Tests).
func cmdDashboard(args []string) error {
	fs := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	dump := fs.Bool("dump", false, "Text-Snapshot statt TUI (headless)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dump {
		printDump()
		return nil
	}
	p := tea.NewProgram(tui.New(Version), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("dashboard braucht ein interaktives Terminal (%v) – Tipp: `lokronet dashboard --dump`", err)
	}
	return nil
}

// printDump: dieselben Daten wie die TUI, als Text (kein TTY nötig).
func printDump() {
	s := tui.FetchSnapshot(Version)
	fmt.Printf("lokronet %s-beta  online=%v  mode=%s  mesh=%v\n", s.Version, s.Online, s.Mode, s.Mesh)
	fmt.Printf("id=%s  contacts=%d  connections=%d  traffic_in=%d  traffic_out=%d\n",
		s.ID, len(s.Contacts), len(s.Rows), s.TotalIn, s.TotalOut)
	for _, r := range s.Rows {
		name := r.ID
		if r.Alias != "" {
			name = r.Alias + " (" + r.ID + ")"
		}
		rtt := "-"
		if r.RTT >= 0 {
			rtt = fmt.Sprintf("%d ms", r.RTT)
		}
		fmt.Printf("  %-28s online=%v rtt=%s\n", name, r.Online, rtt)
	}
	if !s.Online && s.DaemonErr != "" {
		fmt.Printf("hinweis: %s\n", s.DaemonErr)
	}
}
