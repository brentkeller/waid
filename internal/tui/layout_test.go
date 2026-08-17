package tui

import "testing"

// The split rule is one measurement shared by every tab (§5): at or above the threshold the detail
// sits beside the list, and below it the detail is a full-width overlay instead.
func TestSplitAtTheBoundaryWidths(t *testing.T) {
	cases := []struct {
		width      int
		sideBySide bool
	}{
		{0, false},
		{80, false},
		{119, false},
		{120, true},
		{140, true},
		{240, true},
	}

	for _, c := range cases {
		if got := Split(c.width).SideBySide; got != c.sideBySide {
			t.Errorf("Split(%d).SideBySide = %v, want %v", c.width, got, c.sideBySide)
		}
	}
}

// The panes and the gutter between them have to account for every column the terminal has: a pane a
// column too wide wraps the row it draws and pushes the whole list out of alignment.
func TestSplitPaneWidthsSumToTheTerminalWidth(t *testing.T) {
	for width := 0; width <= 240; width++ {
		panes := Split(width)

		if got := panes.List + panes.Divider + panes.Detail; panes.SideBySide && got != width {
			t.Errorf("Split(%d) panes span %d columns (%d + %d + %d), want %d",
				width, got, panes.List, panes.Divider, panes.Detail, width)
		}
	}
}

// Below the threshold there is no gutter and no second column: the detail covers the list, so both
// panes measure the full terminal and whichever is showing fills it.
func TestSplitBelowTheThresholdIsAFullWidthOverlay(t *testing.T) {
	for _, width := range []int{0, 40, 80, 119} {
		panes := Split(width)

		if panes.Divider != 0 {
			t.Errorf("Split(%d).Divider = %d, want 0 — an overlay has no gutter", width, panes.Divider)
		}
		if panes.List != width || panes.Detail != width {
			t.Errorf("Split(%d) = list %d, detail %d, want both %d",
				width, panes.List, panes.Detail, width)
		}
	}
}

// The threshold exists because a narrow detail pane is unreadable for prose, so the split it allows
// has to clear that bar at the narrowest width that reaches it, and leave the list the wider share
// since the list lays out in columns.
func TestSplitLeavesBothPanesUsable(t *testing.T) {
	for _, width := range []int{120, 140, 200} {
		panes := Split(width)

		if panes.Detail < 40 {
			t.Errorf("Split(%d).Detail = %d columns, too narrow for prose", width, panes.Detail)
		}
		if panes.List < panes.Detail {
			t.Errorf("Split(%d) gives the list %d columns and the detail %d, want the list the wider share",
				width, panes.List, panes.Detail)
		}
	}
}

// A negative width is what a terminal reports before the first resize message arrives; it must not
// come back as a negative pane some renderer then slices a string with.
func TestSplitNeverReturnsNegativePanes(t *testing.T) {
	for _, width := range []int{-1, -80} {
		panes := Split(width)

		if panes.List < 0 || panes.Divider < 0 || panes.Detail < 0 {
			t.Errorf("Split(%d) = %+v, want no negative pane", width, panes)
		}
	}
}
