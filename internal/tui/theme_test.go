package tui

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// styleKey describes a style by the attributes the palette sets, so two styles can be compared
// without rendering them — rendered output depends on the colour profile of whatever the test
// binary is attached to, and the palette does not.
func styleKey(s lipgloss.Style) string {
	return fmt.Sprintf("fg=%v bg=%v bold=%v faint=%v italic=%v underline=%v reverse=%v",
		s.GetForeground(), s.GetBackground(), s.GetBold(), s.GetFaint(),
		s.GetItalic(), s.GetUnderline(), s.GetReverse())
}

// styles reads the palette as name → style, which is what lets the tests below assert over every
// field without listing them one by one.
func styles(t *testing.T, theme Theme) map[string]lipgloss.Style {
	t.Helper()

	value := reflect.ValueOf(theme)
	found := make(map[string]lipgloss.Style, value.NumField())
	for i := range value.NumField() {
		field := value.Type().Field(i)
		style, ok := value.Field(i).Interface().(lipgloss.Style)
		if !ok {
			t.Fatalf("Theme.%s is %s, want lipgloss.Style", field.Name, field.Type)
		}
		found[field.Name] = style
	}
	return found
}

// Every surface the app draws resolves its style from the palette, so the set has to cover all of
// them: a missing name is a view that would have to build a style of its own.
func TestThemeCoversEverySurfaceTheAppDraws(t *testing.T) {
	found := styles(t, NewTheme())

	for _, name := range []string{
		"TabActive", "TabInactive", "TabRule", "Progress",
		"FilterActive", "FilterInactive", "Count",
		"Heading", "Row", "RowFocused", "Meta",
		"Divider", "Dim", "Receipt", "Footer",
	} {
		if _, ok := found[name]; !ok {
			t.Errorf("Theme has no %s style", name)
		}
	}
}

// The palette colours text and nothing else. Padding, margins or borders on a style would shift the
// columns the tree and the tab bar measure themselves against.
func TestThemeStylesLeavePrintedWidthAlone(t *testing.T) {
	const sample = "review  #6886  Add SharedDashboards role"

	for name, style := range styles(t, NewTheme()) {
		if got, want := lipgloss.Width(style.Render(sample)), lipgloss.Width(sample); got != want {
			t.Errorf("Theme.%s renders %d columns wide, want %d", name, got, want)
		}
	}
}

// The row under the cursor has to be distinguishable from the rest, and the same holds for the two
// selected-vs-not pairs in the chrome.
func TestThemeDistinguishesSelectionFromTheRest(t *testing.T) {
	theme := NewTheme()

	cases := []struct {
		name             string
		selected, normal lipgloss.Style
	}{
		{"row", theme.RowFocused, theme.Row},
		{"tab", theme.TabActive, theme.TabInactive},
		{"filter", theme.FilterActive, theme.FilterInactive},
	}

	for _, c := range cases {
		if styleKey(c.selected) == styleKey(c.normal) {
			t.Errorf("the selected and unselected %s styles are both %s", c.name, styleKey(c.normal))
		}
	}
}

// The dim strip carries degradation notes and the footer carries key hints; neither may read as
// loudly as the content above them.
func TestThemeMutesTheStripAndTheFooter(t *testing.T) {
	theme := NewTheme()

	for name, style := range map[string]lipgloss.Style{"Dim": theme.Dim, "Footer": theme.Footer, "Meta": theme.Meta} {
		if style.GetForeground() != colorMuted {
			t.Errorf("Theme.%s has foreground %v, want the muted colour %v", name, style.GetForeground(), colorMuted)
		}
	}
}
