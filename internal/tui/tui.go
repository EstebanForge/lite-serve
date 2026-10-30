// Package tui holds the interactive bubbletea menus for `lserve list`
// and `lserve mailpit`. They take data + callbacks so the CLI logic
// stays in cmd/lserve; this package only renders and reports the action
// result back to the user.
package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"
	ActionStatus  = "status"
	ActionCheck   = "check"
	ActionChange  = "change"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type item struct {
	title, desc string
	action      string
	dir         string
	proxy       bool
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

// ProjectItem is one registered project row for the list menu.
type ProjectItem struct {
	Dir   string
	Sites []string
	Proxy bool
}

type model struct {
	list      list.Model
	projects  []list.Item // project rows, restored when esc leaves action mode
	onAction  func(dir, action, value string) string
	onStatus  func() string
	input     textinput.Model
	changeDir string
	status    string // cached status line; View must not shell out per frame
	mailpit   bool
	mode      int // 0: pick project (or mailpit action), 1: pick project action
	result    string
	err       bool
	quitting  bool
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		if msg.String() == "q" && m.mode != 2 {
			m.quitting = true
			return m, tea.Quit
		}
		if m.mode == 2 && msg.String() == "esc" {
			m.mode = 1
			return m, nil
		}
		if m.mode == 1 && msg.String() == "esc" {
			m.mode = 0
			m.list.Title = "lserve: running projects"
			m.list.SetItems(m.projects)
			return m, nil
		}
		if m.mode == 2 && msg.String() == "enter" {
			m.result = m.onAction(m.changeDir, ActionChange, m.input.Value())
			m.err = strings.HasPrefix(m.result, "error: ")
			if m.err {
				m.mode = 1
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit
		}
		if msg.String() == "enter" {
			sel, ok := m.list.SelectedItem().(item)
			if !ok {
				return m, nil
			}
			if m.mailpit || m.mode == 1 {
				if sel.action == ActionChange {
					m.changeDir = sel.dir
					m.input = textinput.New()
					m.input.Prompt = "New port: "
					m.input.Focus()
					m.mode = 2
					return m, nil
				}
				m.result = m.onAction(sel.dir, sel.action, "")
				m.err = strings.HasPrefix(m.result, "error: ")
				if m.onStatus != nil {
					m.status = m.onStatus()
				}
				if sel.action == ActionStop && !m.mailpit {
					// drop the stopped project so esc does not offer a
					// dead entry
					for i, it := range m.projects {
						if p, ok := it.(item); ok && p.dir == sel.dir {
							m.projects = append(m.projects[:i], m.projects[i+1:]...)
							break
						}
					}
					m.mode = 0
					m.list.Title = "lserve: running projects"
					m.list.SetItems(m.projects)
				}
				return m, nil
			}
			// project picked: switch to its action menu
			m.mode = 1
			m.list.Title = sel.title
			actions := []list.Item{
				item{title: "Restart", desc: "re-discover sites and reload the daemon", action: ActionRestart, dir: sel.dir},
				item{title: "Remove", desc: "remove from the registry and stop serving", action: ActionStop, dir: sel.dir},
			}
			if sel.proxy {
				actions = append(actions,
					item{title: "Check port", desc: "probe the proxy upstream", action: ActionCheck, dir: sel.dir},
					item{title: "Change port", desc: "update lserve.json and reload", action: ActionChange, dir: sel.dir})
			}
			m.list.SetItems(actions)
			return m, nil
		}
	}
	if m.mode == 2 {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	var s string
	if m.mode == 2 {
		s = docStyle.Render(m.list.Title + "\n\n" + m.input.View())
	} else {
		s = docStyle.Render(m.list.View())
	}
	if m.result != "" {
		color := lipgloss.Color("10")
		if m.err {
			color = lipgloss.Color("9")
		}
		s += "\n" + lipgloss.NewStyle().Foreground(color).Render(m.result)
	}
	if m.mailpit && m.status != "" {
		s += "\n\n" + lipgloss.NewStyle().Faint(true).Render("current: "+m.status)
	}
	return s + "\n"
}

// RunProjects shows the project picker; enter on a project opens its
// action menu (restart/stop). dirAction receives the project dir and
// the chosen action; its return value is shown as the result line.
func RunProjects(items []ProjectItem, daemonState string, dirAction func(dir, action, value string) string) error {
	li := make([]list.Item, 0, len(items))
	for _, p := range items {
		li = append(li, item{title: p.Dir, desc: strings.Join(p.Sites, ", "), dir: p.Dir, proxy: p.Proxy})
	}
	l := list.New(li, list.NewDefaultDelegate(), 0, 0)
	l.Title = "lserve: running projects (daemon " + daemonState + ")"
	m := model{list: l, projects: li, onAction: dirAction}
	_, err := tea.NewProgram(m).Run()
	return err
}

// RunMailpit shows the mailpit start/stop/restart/status menu. action
// receives the chosen action; status renders the live state line.
func RunMailpit(status func() string, action func(action string) string) error {
	l := list.New([]list.Item{
		item{title: "Start", action: ActionStart},
		item{title: "Stop", action: ActionStop},
		item{title: "Restart", action: ActionRestart},
		item{title: "Status", action: ActionStatus},
	}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "lserve: mailpit"
	m := model{list: l, mailpit: true, onStatus: status, status: status(), onAction: func(_, a, _ string) string { return action(a) }}
	_, err := tea.NewProgram(m).Run()
	return err
}
