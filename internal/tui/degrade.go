package tui

import "strings"

// failureMax is what the tab bar gives a failure before cutting it. A panic carries whatever text it
// was raised with, and the bar has the tab labels to fit alongside it.
const failureMax = 40

// noteStrip is the degradation strip below the list: the notes detection returned, in the CLI's own
// wording and indentation, dimmed and set off by a blank line the way `waid scan` sets them off. A
// degradation is not a failure — the list above it still shows everything that was detected (§7).
func (m Model) noteStrip(notes []string) string {
	if len(notes) == 0 {
		return ""
	}

	lines := make([]string, 0, len(notes)+1)
	lines = append(lines, "")
	for _, note := range notes {
		lines = append(lines, m.theme.Dim.Render("  "+note))
	}
	return strings.Join(lines, "\n")
}

// barSlot is the tab bar's right-hand cell: work in flight while there is any, and otherwise the
// failure the last refresh left, beside the age of the data still on screen. It is computed rather
// than stored because the age keeps counting while the app sits idle (§7).
func (m Model) barSlot() string {
	if m.progress != "" {
		return m.progress
	}
	if m.scan.failure == "" {
		return ""
	}

	slot := "⚠ " + truncate(m.scan.failure, failureMax)
	if age := m.scanAge(); age != "" {
		slot += " · " + age
	}
	return slot
}
