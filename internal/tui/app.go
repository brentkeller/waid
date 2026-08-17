// Package tui is the terminal app `waid ui` runs: a long-running view over the same data the
// commands print, with triage that writes as you go. Nothing outside this package holds a terminal
// concern, and nothing in it is imported by a domain package.
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/ids"
)

// Options are what the app needs from the CLI: the resolved configuration, and the clock and id
// generator dispatch already resolved, so the app draws both from the same places every command does.
type Options struct {
	Cfg config.Config
	Now time.Time
	Ids ids.Generator
}

// Run starts the app on the alternate screen and blocks until it quits. Bubble Tea restores the
// terminal from a deferred call, so a panic mid-render still leaves the alt screen before the stack
// reaches the restored one.
func Run(opts Options) error {
	_, err := tea.NewProgram(New(opts), tea.WithAltScreen()).Run()
	return err
}

// Model is the root model: the tabs, the focus, the global keys and the undo stack all hang off it.
type Model struct {
	opts   Options
	width  int
	height int
}

// New builds the root model.
func New(opts Options) Model { return Model{opts: opts} }

func (m Model) Init() tea.Cmd { return nil }

// Update is pure: every read runs as a tea.Cmd off the update loop and returns a message, so
// Update never waits on the filesystem or the network.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	return placeholder.Render("waid") + "\n\nq quit\n"
}

var placeholder = lipgloss.NewStyle().Bold(true)
