package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lokro/lokronet/internal/contacts"
)

func testModel() Model {
	m := New("test")
	m.w, m.h, m.ready = 120, 40, true
	return m
}

func keyPress(m Model, s string) Model {
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return out.(Model)
}

func TestTabNavigation(t *testing.T) {
	m := testModel()
	if m.tab != TabDashboard {
		t.Fatalf("start tab = %v", m.tab)
	}
	m = keyPress(m, "3") // Contacts
	if m.tab != TabContacts {
		t.Fatalf("tab nach '3' = %v", m.tab)
	}
	m = keyPress(m, "1")
	if m.tab != TabDashboard {
		t.Fatalf("tab nach '1' = %v", m.tab)
	}
}

func TestSidebarMoveChangesTab(t *testing.T) {
	m := testModel()
	m.focusMain = false
	m = keyPress(m, "down")
	if m.tab != TabConnections {
		t.Fatalf("tab nach down in Sidebar = %v", m.tab)
	}
}

func TestHelpDialog(t *testing.T) {
	m := testModel()
	m = keyPress(m, "?")
	if m.dlg != dlgHelp {
		t.Fatal("kein Hilfe-Dialog nach '?'")
	}
	m = keyPress(m, "?") // beliebige Taste schließt
	if m.dlg != dlgNone {
		t.Fatal("Dialog schließt nicht")
	}
}

func TestQuitKey(t *testing.T) {
	m := testModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("kein Quit-Command nach 'q'")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("Quit-Command liefert keine Msg")
	} else if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("kein QuitMsg, sondern %T", msg)
	}
}

func buildTestStore() *contacts.Store {
	return &contacts.Store{Contacts: map[string]contacts.Contact{
		"alpha": {Name: "alpha", ID: "111111111111"},
		"beta":  {Name: "beta", ID: "222222222222"},
	}}
}

func TestMergeRowsDedup(t *testing.T) {
	s := Snapshot{}
	rows := mergeRows(buildTestStore(), s.Peers)
	if len(rows) != 2 {
		t.Fatalf("mergeRows = %d Zeilen, want 2", len(rows))
	}
}
