package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestProxyChangePortFlow(t *testing.T) {
	project := item{title: "/srv/app", dir: "/srv/app", proxy: true}
	var gotDir, gotAction, gotValue string
	m := model{
		list:     list.New([]list.Item{project}, list.NewDefaultDelegate(), 0, 0),
		projects: []list.Item{project},
		onAction: func(dir, action, value string) string {
			gotDir, gotAction, gotValue = dir, action, value
			return "done"
		},
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	for i := 0; i < 3; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.mode != 2 {
		t.Fatalf("change port did not open input; mode=%d", m.mode)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9202")})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if gotDir != "/srv/app" || gotAction != ActionChange || gotValue != "9202" {
		t.Fatalf("callback got %q %q %q", gotDir, gotAction, gotValue)
	}
}

func TestProxyCheckPortAction(t *testing.T) {
	project := item{title: "/srv/app", dir: "/srv/app", proxy: true}
	called := false
	m := model{
		list:     list.New([]list.Item{project}, list.NewDefaultDelegate(), 0, 0),
		projects: []list.Item{project},
		onAction: func(_, action, _ string) string {
			called = action == ActionCheck
			return "upstream is listening"
		},
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	for i := 0; i < 2; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if !called || m.err {
		t.Fatalf("called=%v err=%v result=%q", called, m.err, m.result)
	}
}
