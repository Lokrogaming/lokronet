package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Branding: dunkel, dezent – Cyan aktiv, Grün ok, Gelb Warnung, Rot Fehler.
var (
	colSubtle = lipgloss.Color("240")
	colMuted  = lipgloss.Color("243")
	colActive = lipgloss.Color("51")
	colOK     = lipgloss.Color("42")
	colWarn   = lipgloss.Color("214")
	colErr    = lipgloss.Color("196")
	colText   = lipgloss.Color("252")
)

var (
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(colText)
	styleMuted    = lipgloss.NewStyle().Foreground(colMuted)
	styleActive   = lipgloss.NewStyle().Bold(true).Foreground(colActive)
	styleOK       = lipgloss.NewStyle().Bold(true).Foreground(colOK)
	styleWarn     = lipgloss.NewStyle().Bold(true).Foreground(colWarn)
	styleErr      = lipgloss.NewStyle().Bold(true).Foreground(colErr)
	styleBox      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colSubtle).Padding(0, 1)
	styleBoxFocus = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colActive).Padding(0, 1)
	styleSel      = lipgloss.NewStyle().Background(colActive).Foreground(lipgloss.Color("0")).Bold(true)
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(colText).Padding(0, 1)
	styleFooter   = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)
	styleErrMsg   = lipgloss.NewStyle().Foreground(colErr)
)

// dot rendert den Online-Status (niemals Panic, nur Glyphen).
func dot(ok bool) string {
	if ok {
		return styleOK.Render("●")
	}
	return styleMuted.Render("○")
}
