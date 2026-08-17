package tui

import "github.com/charmbracelet/lipgloss"

// The palette is base-16 ANSI rather than fixed RGB, so every colour resolves through whatever
// theme the terminal is already wearing, and NO_COLOR drops out of it without the app knowing.
const (
	colorAccent  = lipgloss.Color("6") // cyan: the cursor, the active tab, the selected filter
	colorMuted   = lipgloss.Color("8") // bright black: counts, rules, notes, hints
	colorSuccess = lipgloss.Color("2") // green: a write that landed
	colorWork    = lipgloss.Color("3") // yellow: work in flight
)

// Theme is the app's whole palette. Every style any view renders with is a field here, so a colour
// changes in one place and the two tabs sharing the tree cannot drift apart.
type Theme struct {
	// The tab bar and its right-hand slot, the only place slow work is visible (§1).
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	TabRule     lipgloss.Style
	Progress    lipgloss.Style

	// The segmented filter row under the bar and the counts sitting opposite it.
	FilterActive   lipgloss.Style
	FilterInactive lipgloss.Style
	Count          lipgloss.Style

	// The tree: a project heading, its child rows, the row under the cursor, and the secondary
	// columns a row carries — branch, waiting-on, age.
	Heading    lipgloss.Style
	Row        lipgloss.Style
	RowFocused lipgloss.Style
	Meta       lipgloss.Style

	// Everything below the list: the rules, the degradation strip (§7), the receipt left by the
	// last write (§3.1), and the key hints.
	Divider lipgloss.Style
	Dim     lipgloss.Style
	Receipt lipgloss.Style
	Footer  lipgloss.Style
}

// NewTheme builds the palette. The styles carry colour and weight only — no padding, margins or
// borders — because the tree and the tab bar lay themselves out in columns they measure first.
func NewTheme() Theme {
	muted := lipgloss.NewStyle().Foreground(colorMuted)

	return Theme{
		TabActive:   lipgloss.NewStyle().Bold(true).Foreground(colorAccent),
		TabInactive: muted,
		TabRule:     muted,
		Progress:    lipgloss.NewStyle().Foreground(colorWork),

		FilterActive:   lipgloss.NewStyle().Bold(true).Foreground(colorAccent),
		FilterInactive: muted,
		Count:          muted,

		Heading:    lipgloss.NewStyle().Bold(true),
		Row:        lipgloss.NewStyle(),
		RowFocused: lipgloss.NewStyle().Bold(true).Foreground(colorAccent),
		Meta:       muted,

		Divider: muted,
		Dim:     muted,
		Receipt: lipgloss.NewStyle().Foreground(colorSuccess),
		Footer:  muted,
	}
}
