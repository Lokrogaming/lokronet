package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lokro/lokronet/internal/client"
	"github.com/lokro/lokronet/internal/contacts"
)

// Tabs (Reihenfolge = Sidebar).
type Tab int

const (
	TabDashboard Tab = iota
	TabConnections
	TabContacts
	TabHistory
	TabNetwork
	TabLogs
	TabSettings
	TabIDs
	TabCount
)

var tabNames = []string{"Dashboard", "Connections", "Contacts", "History", "Network", "Logs", "Settings", "IDs"}

// Dialoge (modal, Esc schließt immer).
type dlgKind int

const (
	dlgNone dlgKind = iota
	dlgHelp
	dlgMessage
	dlgAddContact
	dlgTarget // einzeilige Eingabe (connect/ping)
	dlgConfirmRemove
)

// Nachrichten (async, UI bleibt bedienbar).
type snapshotMsg Snapshot
type actionMsg struct {
	text  string
	isErr bool
}
type tickMsg time.Time

// Model ist der gesamte TUI-Zustand.
type Model struct {
	version   string
	w, h      int
	ready     bool
	tab       Tab
	focusMain bool
	cursor    int
	offset    int
	snap      Snapshot
	msg       string
	msgErr    bool
	dlg       dlgKind
	dlgTitle  string
	dlgText   string
	dlgAction string // "connect"|"ping" bei dlgTarget
	pendingID string // für dlgConfirmRemove: Kontaktname
	inputs    []textinput.Model
	tiFocus   int
}

// New erzeugt das Dashboard-Modell.
func New(version string) Model {
	return Model{version: version, snap: FetchSnapshot(version)}
}

// refreshCmd holt einen Snapshot (blockiert nie die UI).
func refreshCmd(version string) tea.Cmd {
	return func() tea.Msg {
		return snapshotMsg(FetchSnapshot(version))
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(2500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func actionCmd(fn func() (string, bool)) tea.Cmd {
	return func() tea.Msg {
		text, isErr := fn()
		return actionMsg{text: text, isErr: isErr}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.version), tickCmd())
}

// rowCount: navigierbare Zeilen im Main-Bereich je Tab.
func (m Model) rowCount() int {
	switch m.tab {
	case TabConnections:
		return len(m.snap.Rows)
	case TabContacts:
		return len(m.snap.Contacts)
	case TabHistory:
		return len(m.snap.History)
	case TabLogs:
		return len(m.snap.Events)
	case TabSettings:
		return len(settingRows(m.snap))
	default:
		return 0
	}
}

func (m *Model) clampCursor() {
	n := m.rowCount()
	if n == 0 {
		m.cursor, m.offset = 0, 0
		return
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.visibleRows() {
		m.offset = m.cursor - m.visibleRows() + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) visibleRows() int {
	v := m.h - 8 // header(3) + status(1) + footer(1) + box-padding(2) + reserve
	if v < 3 {
		return 3
	}
	return v
}

// Update: Tastatur + async Nachrichten. Fehler landen in der Statuszeile,
// niemals in Panics (alle Backend-Aufrufe sind best-effort).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.ready = true
		m.clampCursor()
		return m, nil
	case snapshotMsg:
		atBottom := true
		if m.tab == TabLogs && m.rowCount() > 0 {
			atBottom = m.cursor >= m.rowCount()-1
		}
		m.snap = Snapshot(msg)
		m.clampCursor()
		if m.tab == TabLogs && atBottom {
			m.cursor = m.rowCount() - 1
			m.clampCursor()
		}
		return m, nil
	case actionMsg:
		m.msg, m.msgErr = msg.text, msg.isErr
		return m, refreshCmd(m.version)
	case tickMsg:
		return m, tea.Batch(refreshCmd(m.version), tickCmd())
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	// Textinputs in Dialogen füttern.
	if m.dlg == dlgAddContact || m.dlg == dlgTarget {
		var cmds []tea.Cmd
		for i := range m.inputs {
			if i == m.tiFocus {
				var cmd tea.Cmd
				m.inputs[i], cmd = m.inputs[i].Update(msg)
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Dialoge haben Vorrang.
	if m.dlg != dlgNone {
		return m.updateDialog(msg)
	}
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.focusMain = false
		return m, nil
	case tea.KeyEnter:
		return m.activate()
	case tea.KeyTab:
		m.tab = (m.tab + 1) % TabCount
		m.cursor, m.offset, m.focusMain = 0, 0, true
		return m, refreshCmd(m.version)
	case tea.KeyShiftTab:
		m.tab = (m.tab + TabCount - 1) % TabCount
		m.cursor, m.offset, m.focusMain = 0, 0, true
		return m, refreshCmd(m.version)
	case tea.KeyLeft:
		m.focusMain = false
		return m, nil
	case tea.KeyRight:
		m.focusMain = true
		return m, nil
	}
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "?":
		m.dlg = dlgHelp
		return m, nil
	case "r":
		m.msg, m.msgErr = "aktualisiert.", false
		return m, refreshCmd(m.version)
	case "up", "k":
		m.move(-1)
		return m, nil
	case "down", "j":
		m.move(1)
		return m, nil
	case "pgup":
		m.move(-m.visibleRows())
		return m, nil
	case "pgdown", " ":
		m.move(m.visibleRows())
		return m, nil
	case "home", "g":
		m.setCursor(0)
		return m, nil
	case "end", "G":
		m.setCursor(m.rowCount() - 1)
		return m, nil
	}
	// Ziffern springen direkt zum Tab.
	if len(msg.String()) == 1 && msg.String() >= "1" && msg.String() <= "8" {
		m.tab = Tab(msg.String()[0] - '1')
		m.cursor, m.offset, m.focusMain = 0, 0, true
		return m, refreshCmd(m.version)
	}
	// Kontext-Aktionen im Main-Bereich.
	if m.focusMain {
		switch msg.String() {
		case "c":
			return m.startTargetDlg("connect")
		case "p":
			return m.startTargetDlg("ping")
		case "a":
			if m.tab == TabContacts || m.tab == TabConnections {
				return m.startAddContact()
			}
		case "x":
			if m.tab == TabContacts {
				return m.startConfirmRemove()
			}
		case "m":
			if m.tab == TabSettings {
				return m, m.cycleMode()
			}
		}
	}
	return m, nil
}

func (m *Model) move(d int) {
	if !m.focusMain {
		// Sidebar-Navigation wechselt den Tab.
		t := int(m.tab) + d
		if t < 0 {
			t = 0
		}
		if t >= int(TabCount) {
			t = int(TabCount) - 1
		}
		if Tab(t) != m.tab {
			m.tab = Tab(t)
			m.cursor, m.offset = 0, 0
		}
		return
	}
	m.setCursor(m.cursor + d)
}

func (m *Model) setCursor(c int) {
	m.cursor = c
	m.clampCursor()
}

// activate: Enter im Main-Bereich (Default-Aktion je Tab).
func (m Model) activate() (tea.Model, tea.Cmd) {
	if !m.focusMain {
		m.focusMain = true
		return m, nil
	}
	switch m.tab {
	case TabConnections:
		if m.cursor < len(m.snap.Rows) {
			r := m.snap.Rows[m.cursor]
			m.dlg, m.dlgTitle = dlgMessage, "Details – "+displayName(r)
			m.dlgText = fmt.Sprintf("Alias:  %s\nID:     %s\nStatus: %s\nMode:   %s\nRTT:    %s\nKnown:  %dx verbunden",
				nonEmpty(r.Alias, "-"), r.ID, connStateText(r), nonEmpty(r.Mode, "-"), rttText(r.RTT), r.Count)
		}
		return m, nil
	case TabContacts:
		if m.cursor < len(m.snap.Contacts) {
			c := m.snap.Contacts[m.cursor]
			m.dlg, m.dlgTitle = dlgMessage, "Kontakt – "+c.Name
			m.dlgText = fmt.Sprintf("Name: %s\nID:   %s\nFP:   %s", c.Name, c.ID, c.Fingerprint)
		}
		return m, nil
	case TabSettings:
		return m, m.applySetting(m.cursor)
	}
	return m, nil
}

func displayName(r ConnRow) string {
	if r.Alias != "" {
		return r.Alias
	}
	return r.ID
}

func connStateText(r ConnRow) string {
	if r.Online {
		return "ONLINE (" + r.State + ")"
	}
	return "OFFLINE"
}

// --- Aktionen (async, Fehler -> Statuszeile) ---

func resolveQuiet(target string) (string, error) {
	store, err := contacts.Load()
	if err != nil {
		return "", err
	}
	id, _, err := store.Resolve(strings.TrimSpace(target))
	return id, err
}

func recordQuiet(id string) {
	store, err := contacts.Load()
	if err != nil {
		return
	}
	_ = store.RecordConnect(id, "")
}

func (m Model) startTargetDlg(action string) (tea.Model, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = "Name oder ID"
	ti.CharLimit = 32
	ti.Width = 30
	ti.Focus()
	m.inputs = []textinput.Model{ti}
	m.tiFocus = 0
	m.dlg, m.dlgAction = dlgTarget, action
	m.dlgTitle = map[string]string{"connect": "Verbinden mit", "ping": "Ping an"}[action]
	return m, nil
}

func (m Model) startAddContact() (tea.Model, tea.Cmd) {
	ni := textinput.New()
	ni.Placeholder = "Name"
	ni.CharLimit = 32
	ni.Width = 30
	ni.Focus()
	ii := textinput.New()
	ii.Placeholder = "ID (12 Ziffern)"
	ii.CharLimit = 12
	ii.Width = 30
	m.inputs = []textinput.Model{ni, ii}
	m.tiFocus = 0
	m.dlg, m.dlgTitle = dlgAddContact, "Kontakt hinzufügen"
	return m, nil
}

func (m Model) startConfirmRemove() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.snap.Contacts) {
		return m, nil
	}
	c := m.snap.Contacts[m.cursor]
	m.pendingID = c.Name
	m.dlg, m.dlgTitle = dlgConfirmRemove, "Kontakt entfernen"
	m.dlgText = fmt.Sprintf("„%s“ (%s) wirklich entfernen?\n(History bleibt erhalten)\n\n[y] Ja   [n/Esc] Nein", c.Name, c.ID)
	return m, nil
}

func (m Model) doConnect(id string) tea.Cmd {
	return actionCmd(func() (string, bool) {
		rid, err := resolveQuiet(id)
		if err != nil {
			return err.Error(), true
		}
		c, err := client.Dial()
		if err != nil {
			return err.Error(), true
		}
		p, err := c.Connect(rid)
		if err != nil {
			return err.Error(), true
		}
		recordQuiet(rid)
		_ = p
		return fmt.Sprintf("connect ok (%s) – Fingerprint out-of-band prüfen!", rid), false
	})
}

func (m Model) doPing(id string) tea.Cmd {
	return actionCmd(func() (string, bool) {
		rid, err := resolveQuiet(id)
		if err != nil {
			return err.Error(), true
		}
		c, err := client.Dial()
		if err != nil {
			return err.Error(), true
		}
		rtt, err := c.Ping(rid)
		if err != nil {
			return err.Error(), true
		}
		recordQuiet(rid)
		return fmt.Sprintf("ping %s → %d ms", rid, rtt), false
	})
}

func (m Model) cycleMode() tea.Cmd {
	order := []string{"normal", "performance", "eco"}
	cur := m.snap.Mode
	if cur == "" {
		cur = "normal"
	}
	next := order[0]
	for i, o := range order {
		if o == cur {
			next = order[(i+1)%len(order)]
		}
	}
	nm := next
	return actionCmd(func() (string, bool) {
		c, err := client.Dial()
		if err != nil {
			return err.Error(), true
		}
		if _, err := c.SetMode(&nm, nil); err != nil {
			return err.Error(), true
		}
		return "mode: " + nm + " (live)", false
	})
}

// applySetting: Enter auf Settings-Zeile (m ashes cycle, mesh/debug toggle).
func (m Model) applySetting(idx int) tea.Cmd {
	rows := settingRows(m.snap)
	if idx < 0 || idx >= len(rows) || rows[idx].planned {
		if idx >= 0 && idx < len(rows) && rows[idx].planned {
			m.msg, m.msgErr = rows[idx].name+": noch nicht implementiert.", true
		}
		return nil
	}
	switch rows[idx].key {
	case "mode":
		return m.cycleMode()
	case "mesh":
		want := !m.snap.Mesh
		return actionCmd(func() (string, bool) {
			c, err := client.Dial()
			if err != nil {
				return err.Error(), true
			}
			res, err := c.SetMode(nil, &want)
			if err != nil {
				return err.Error(), true
			}
			text := fmt.Sprintf("mesh: %s", map[bool]string{true: "on", false: "off"}[res.Mesh])
			if res.RestartNeeded {
				text += " – Daemon-Neustart nötig!"
			}
			return text, false
		})
	case "debug":
		want := !m.snap.Debug
		return actionCmd(func() (string, bool) {
			c, err := client.Dial()
			if err != nil {
				return err.Error(), true
			}
			if err := c.SetDebug(want); err != nil {
				return err.Error(), true
			}
			return fmt.Sprintf("debug: %s", map[bool]string{true: "on", false: "off"}[want]), false
		})
	}
	return nil
}

// settingRow: eine Settings-Zeile (planned = ehrlich als geplant markiert).
type settingRow struct {
	key     string
	name    string
	value   string
	planned bool
}

func settingRows(s Snapshot) []settingRow {
	mesh := "off"
	if s.Mesh {
		mesh = "on"
	}
	dbg := "off"
	if s.Debug {
		dbg = "on"
	}
	return []settingRow{
		{key: "mode", name: "Mode", value: nonEmpty(s.Mode, "normal")},
		{key: "mesh", name: "Meshpoint", value: mesh},
		{key: "debug", name: "Debug-Meldungen", value: dbg},
		{key: "theme", name: "TUI-Theme", value: "lokro-dark", planned: true},
		{key: "notify", name: "Benachrichtigungen", value: "-", planned: true},
	}
}

// updateDialog: Tasten im modalen Dialog.
func (m Model) updateDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.dlg {
	case dlgHelp, dlgMessage:
		return m.closeDlg(), nil // jede Taste schließt
	case dlgConfirmRemove:
		switch msg.String() {
		case "y", "Y":
			name := m.pendingID
			m2 := m.closeDlg()
			return m2, actionCmd(func() (string, bool) {
				store, err := contacts.Load()
				if err != nil {
					return err.Error(), true
				}
				if err := store.Remove(name); err != nil {
					return err.Error(), true
				}
				return fmt.Sprintf("kontakt %q gelöscht", name), false
			})
		default:
			if msg.Type == tea.KeyEsc || msg.String() == "n" {
				return m.closeDlg(), nil
			}
			return m, nil
		}
	case dlgAddContact:
		switch msg.Type {
		case tea.KeyEsc:
			return m.closeDlg(), nil
		case tea.KeyTab, tea.KeyShiftTab:
			m.tiFocus = (m.tiFocus + 1) % len(m.inputs)
			for i := range m.inputs {
				if i == m.tiFocus {
					m.inputs[i].Focus()
				} else {
					m.inputs[i].Blur()
				}
			}
			return m, nil
		case tea.KeyEnter:
			name := strings.TrimSpace(m.inputs[0].Value())
			id := strings.TrimSpace(m.inputs[1].Value())
			m2 := m.closeDlg()
			return m2, actionCmd(func() (string, bool) {
				store, err := contacts.Load()
				if err != nil {
					return err.Error(), true
				}
				if err := store.Add(name, id, ""); err != nil {
					return err.Error(), true
				}
				return fmt.Sprintf("kontakt %q gespeichert", name), false
			})
		}
	case dlgTarget:
		switch msg.Type {
		case tea.KeyEsc:
			return m.closeDlg(), nil
		case tea.KeyEnter:
			target := strings.TrimSpace(m.inputs[0].Value())
			action := m.dlgAction
			m2 := m.closeDlg()
			if target == "" {
				return m2, nil
			}
			if action == "ping" {
				return m2, m.doPing(target)
			}
			return m2, m.doConnect(target)
		}
	}
	// Texteingaben füttern.
	if m.dlg == dlgAddContact || m.dlg == dlgTarget {
		var cmds []tea.Cmd
		for i := range m.inputs {
			if i == m.tiFocus {
				var cmd tea.Cmd
				m.inputs[i], cmd = m.inputs[i].Update(msg)
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m Model) closeDlg() Model {
	m.dlg = dlgNone
	m.dlgText, m.dlgTitle, m.dlgAction, m.pendingID = "", "", "", ""
	m.inputs, m.tiFocus = nil, 0
	return m
}
