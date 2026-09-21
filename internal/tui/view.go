package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View rendert Header + Body + Statuszeile + Footer (+ Overlays).
func (m Model) View() string {
	if !m.ready {
		return "starte Dashboard …"
	}
	w := m.w
	if w < 20 {
		w = 20
	}
	header := m.viewHeader(w)
	footer := m.viewFooter(w)
	status := m.viewStatusLine(w)
	bodyH := m.h - lipgloss.Height(header) - lipgloss.Height(status) - lipgloss.Height(footer) - 2
	if bodyH < 5 {
		bodyH = 5
	}
	body := m.viewBody(w, bodyH)
	base := lipgloss.JoinVertical(lipgloss.Left, header, body, status, footer)
	if m.dlg != dlgNone {
		return m.overlay(base, m.viewDialog())
	}
	return base
}

func (m Model) viewHeader(w int) string {
	right := dot(m.snap.Online) + " " + map[bool]string{true: "Connected", false: "Offline"}[m.snap.Online]
	left := styleTitle.Render("LokroNet") + styleMuted.Render(" • beta")
	gap := w - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right
	return styleBox.Render(line)
}

func (m Model) viewStatusLine(w int) string {
	text := m.msg
	if text == "" {
		if !m.snap.Online {
			text = m.snap.DaemonErr
		}
	}
	if text == "" {
		return ""
	}
	text = truncate(text, w-4)
	if m.msgErr {
		return styleErrMsg.Render("! " + text)
	}
	return styleMuted.Render("• " + text)
}

func (m Model) viewFooter(w int) string {
	hints := "↑↓ Navigate  Tab Fokus  ←/→ Tab  Enter Select  c Connect  p Ping  r Refresh  ? Hilfe  q Quit"
	hints = truncate(hints, w-4)
	left := styleMuted.Render("LokroNet • beta")
	gap := w - lipgloss.Width(left) - lipgloss.Width(hints) - 4
	if gap < 1 {
		return styleFooter.Render(truncate(hints, w-2))
	}
	return styleFooter.Render(left + strings.Repeat(" ", gap) + hints)
}

func (m Model) compact() bool { return m.w < 90 }

func (m Model) viewBody(w, h int) string {
	if m.compact() {
		// Schmal: Tabs als Kopfzeile, nur Main-Bereich.
		tabs := make([]string, len(tabNames))
		for i, n := range tabNames {
			if Tab(i) == m.tab {
				tabs[i] = styleActive.Render(fmt.Sprintf("[%d %s]", i+1, n))
			} else {
				tabs[i] = styleMuted.Render(fmt.Sprintf("%d %s", i+1, n))
			}
		}
		bar := truncate(strings.Join(tabs, "  "), w-4)
		return lipgloss.JoinVertical(lipgloss.Left, bar, m.viewMain(w-2, h-1))
	}
	sideW := 22
	mainW := w - sideW - 5 // borders + padding
	if mainW < 20 {
		mainW = 20
	}
	side := m.viewSidebar(sideW, h)
	main := m.viewMain(mainW, h)
	box := func(s string, focus bool) string {
		if focus {
			return styleBoxFocus.Render(s)
		}
		return styleBox.Render(s)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		box(side, !m.focusMain),
		box(main, m.focusMain),
	)
}

func (m Model) viewSidebar(w, h int) string {
	var b strings.Builder
	for i, n := range tabNames {
		marker := "  "
		if Tab(i) == m.tab {
			marker = "▸ "
		}
		line := marker + fmt.Sprintf("%d %s", i+1, n)
		if Tab(i) == m.tab {
			b.WriteString(styleSel.Render(padRight(line, w-4)) + "\n")
		} else {
			b.WriteString(styleMuted.Render(padRight(line, w-4)) + "\n")
		}
	}
	return b.String()
}

func padRight(s string, w int) string {
	n := w - len([]rune(s))
	if n <= 0 {
		return truncate(s, w)
	}
	return s + strings.Repeat(" ", n)
}

// viewMain rendert den aktiven Tab (Listen mit Cursor + Scroll-Offset).
func (m Model) viewMain(w, h int) string {
	var lines []string
	sel := func(i int, s string) string {
		if m.focusMain && i == m.cursor {
			return styleSel.Render(padRight("▸ "+s, w))
		}
		prefix := "  "
		if i == m.cursor {
			prefix = "▸ "
		}
		return padRight(prefix+s, w)
	}
	visible := func(all []string) []string {
		if len(all) == 0 {
			return []string{styleMuted.Render("(leer)")}
		}
		end := m.offset + h
		if end > len(all) {
			end = len(all)
		}
		return all[m.offset:end]
	}

	switch m.tab {
	case TabDashboard:
		lines = m.dashboardLines(w)
		return strings.Join(lines, "\n")
	case TabConnections:
		var all []string
		for _, r := range m.snap.Rows {
			name := displayName(r)
			st := dot(r.Online) + " " + map[bool]string{true: "ONLINE", false: "OFFLINE"}[r.Online]
			all = append(all, fmt.Sprintf("%-14s %-12s %s %6s", truncate(name, 14), r.ID, st, rttText(r.RTT)))
		}
		head := styleMuted.Render(padRight(fmt.Sprintf("%-14s %-12s %s %6s", "Alias", "ID", "Status", "RTT"), w))
		out := []string{head}
		for i, l := range visible(all) {
			out = append(out, sel(m.offset+i, l))
		}
		return strings.Join(out, "\n")
	case TabContacts:
		var all []string
		for _, c := range m.snap.Contacts {
			all = append(all, fmt.Sprintf("%-16s %s  fp:%s", c.Name, c.ID, fpShort(c.Fingerprint)))
		}
		if len(all) == 0 {
			return styleMuted.Render("(keine Kontakte – Taste a zum Hinzufügen)")
		}
		var out []string
		for i, l := range visible(all) {
			out = append(out, sel(m.offset+i, l))
		}
		return strings.Join(out, "\n")
	case TabHistory:
		var all []string
		for _, hh := range m.snap.History {
			show := hh.ID
			if a := aliasOf(m.snap, hh.ID); a != "" {
				show = a + " (" + hh.ID + ")"
			}
			all = append(all, fmt.Sprintf("%-24s %3dx  %s", truncate(show, 24), hh.Count, fmtTime(hh.LastSeen)))
		}
		if len(all) == 0 {
			return styleMuted.Render("(keine Verbindungen in den letzten 30 Tagen)")
		}
		var out []string
		for i, l := range visible(all) {
			out = append(out, sel(m.offset+i, l))
		}
		return strings.Join(out, "\n")
	case TabNetwork:
		lines = m.networkLines(w)
		return strings.Join(lines, "\n")
	case TabLogs:
		var all []string
		for _, e := range m.snap.Events {
			all = append(all, e.Time.Format("15:04:05")+"  "+truncate(e.Msg, w-12))
		}
		if len(all) == 0 {
			return styleMuted.Render("(keine Events – `lokronet debug on` einschalten)")
		}
		var out []string
		for i, l := range visible(all) {
			out = append(out, sel(m.offset+i, levelColor(l)))
		}
		return strings.Join(out, "\n")
	case TabSettings:
		rows := settingRows(m.snap)
		var out []string
		for i, r := range rows {
			val := r.value
			if r.planned {
				val += styleMuted.Render("  (planned)")
			}
			out = append(out, sel(i, fmt.Sprintf("%-18s %s", r.name, val)))
		}
		out = append(out, "", styleMuted.Render("Enter: umschalten • m: Modus weiter"))
		return strings.Join(visible(out), "\n")
	case TabIDs:
		lines = m.idsLines()
		return strings.Join(lines, "\n")
	}
	return ""
}

func aliasOf(s Snapshot, id string) string {
	for _, c := range s.Contacts {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}

func (m Model) dashboardLines(w int) []string {
	s := m.snap
	mode := nonEmpty(s.Mode, "normal")
	onlineN := 0
	for _, r := range s.Rows {
		if r.Online {
			onlineN++
		}
	}
	iface, ip := "N/A", "N/A"
	if len(s.IfAddrs) > 0 {
		iface, ip = s.IfAddrs[0].Iface, s.IfAddrs[0].IP
	}
	last := "-"
	if len(s.History) > 0 {
		h := s.History[0]
		last = fmtTime(h.LastSeen) + " " + h.ID
		if a := aliasOf(s, h.ID); a != "" {
			last = fmtTime(h.LastSeen) + " " + a
		}
	}
	mesh := "off"
	if s.Mesh {
		mesh = "on"
	}
	lines := []string{
		styleTitle.Render("LokroNet") + styleMuted.Render("  v"+s.Version+"-beta"),
		"",
		fmt.Sprintf("Status       %s %s", dot(s.Online), map[bool]string{true: "ONLINE", false: "OFFLINE"}[s.Online]),
		fmt.Sprintf("Mode         %s", mode),
		fmt.Sprintf("Mesh         %s", mesh),
		fmt.Sprintf("Connections  %d (%d online)", len(s.Rows), onlineN),
		fmt.Sprintf("Contacts     %d", len(s.Contacts)),
		fmt.Sprintf("Interface    %s", iface),
		fmt.Sprintf("Local IP     %s", ip),
		fmt.Sprintf("Traffic      ↓ %s  ↑ %s", humanBytes(s.TotalIn), humanBytes(s.TotalOut)),
	}
	if !s.Online && s.DaemonErr != "" {
		lines = append(lines, styleWarn.Render("Hinweis: "+truncate(s.DaemonErr, w-10)))
	}
	lines = append(lines, "", styleMuted.Render("── Recent Activity ──"))
	n := 0
	for i := len(s.Events) - 1; i >= 0 && n < 6; i-- {
		e := s.Events[i]
		lines = append(lines, truncate(e.Time.Format("15:04")+"  "+e.Msg, w-2))
		n++
	}
	if n == 0 {
		lines = append(lines, styleMuted.Render("noch keine Aktivität – verbinde dich per [c]"))
	}
	_ = last
	return lines
}

func (m Model) networkLines(w int) []string {
	s := m.snap
	lines := []string{
		fmt.Sprintf("Rendezvous   %s", nonEmpty(s.Rendezvous, "N/A")),
		fmt.Sprintf("UDP-Port     %d", s.UDPPort),
		fmt.Sprintf("Mesh         %s", map[bool]string{true: "on", false: "off"}[s.Mesh]),
		fmt.Sprintf("Public IP    %s", nonEmpty(s.PublicIP, "N/A")),
		"",
		styleMuted.Render("── Interfaces ──"),
	}
	if len(s.IfAddrs) == 0 {
		lines = append(lines, styleMuted.Render("N/A (keine Interfaces ermittelbar)"))
	}
	for _, a := range s.IfAddrs {
		lines = append(lines, fmt.Sprintf("%-12s %s", truncate(a.Iface, 12), a.IP))
	}
	_ = w
	return lines
}

func (m Model) idsLines() []string {
	s := m.snap
	return []string{
		fmt.Sprintf("ID           %s", nonEmpty(s.ID, "N/A (setup nötig)")),
		fmt.Sprintf("Fingerprint  %s", nonEmpty(s.Fingerprint, "N/A")),
		fmt.Sprintf("Public IP    %s", nonEmpty(s.PublicIP, "N/A")),
		"",
		styleMuted.Render("IPv6 beim setup in eckige Klammern: --endpoint \"[ip]:port\""),
	}
}

// --- Overlays ---

func (m Model) overlay(base, dlg string) string {
	// Zentriert das Dialog-Overlay über dem abgedunkelten Basis-Layout.
	bw, bh := lipgloss.Width(base), lipgloss.Height(base)
	dw, dh := lipgloss.Width(dlg), lipgloss.Height(dlg)
	ox := (bw - dw) / 2
	if ox < 0 {
		ox = 0
	}
	oy := (bh - dh) / 2
	if oy < 0 {
		oy = 0
	}
	lines := strings.Split(base, "\n")
	dlines := strings.Split(dlg, "\n")
	for i, dl := range dlines {
		y := oy + i
		if y < 0 || y >= len(lines) {
			continue
		}
		row := []rune(lines[y])
		for len(row) < bw {
			row = append(row, ' ')
		}
		dr := []rune(dl)
		for x, r := range dr {
			if ox+x < len(row) {
				row[ox+x] = r
			}
		}
		lines[y] = string(row)
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewDialog() string {
	switch m.dlg {
	case dlgHelp:
		return styleBox.Render(
			styleTitle.Render("Keyboard Shortcuts") + "\n\n" +
				"↑/↓ oder j/k   Navigieren\n" +
				"←/→ oder Tab   Bereich / Tab wechseln\n" +
				"1-8            Direkt zum Tab\n" +
				"Enter          Auswählen / Details\n" +
				"Esc            Zurück / Schließen\n" +
				"c / p          Connect / Ping (Auswahl oder Eingabe)\n" +
				"a / x          Kontakt hinzu / entfernen\n" +
				"m              Modus weiter (Settings)\n" +
				"r              Refresh\n" +
				"?              Diese Hilfe\n" +
				"q / Strg+C     Beenden\n\n" +
				styleMuted.Render("Taste zum Schließen"))
	case dlgMessage:
		return styleBox.Render(styleTitle.Render(m.dlgTitle) + "\n\n" + m.dlgText + "\n\n" + styleMuted.Render("Taste zum Schließen"))
	case dlgConfirmRemove:
		return styleBox.Render(styleTitle.Render(m.dlgTitle) + "\n\n" + m.dlgText)
	case dlgTarget:
		return styleBox.Render(styleTitle.Render(m.dlgTitle) + "\n\nName oder ID:\n" + m.inputs[0].View() + "\n\n" + styleMuted.Render("Enter OK • Esc Abbrechen"))
	case dlgAddContact:
		return styleBox.Render(styleTitle.Render(m.dlgTitle) + "\n\nName:\n" + m.inputs[0].View() + "\nID (12 Ziffern):\n" + m.inputs[1].View() + "\n\n" + styleMuted.Render("Tab Feldwechsel • Enter Speichern • Esc Abbrechen"))
	}
	return ""
}
