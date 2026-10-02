package tui

import "testing"

// The window moves only as far as keeping the cursor, and the margin around it, in sight takes.
func TestScrollTopFollowsTheCursor(t *testing.T) {
	cases := []struct {
		name                       string
		top, cursor, total, height int
		want                       int
	}{
		{"a list that fits never scrolls", 3, 4, 5, 10, 0},
		{"a cursor inside the window leaves it alone", 5, 10, 50, 10, 5},
		{"a cursor nearing the bottom pulls the window down", 0, 9, 50, 10, 2},
		{"a cursor nearing the top pulls the window up", 20, 21, 50, 10, 19},
		{"the window stops at the end of the list", 0, 49, 50, 10, 40},
		{"the window stops at the start of the list", 5, 0, 50, 10, 0},
		{"a stale offset past the end is pulled back", 60, 45, 50, 10, 40},
		{"a window of one row sits on the cursor", 0, 7, 50, 1, 7},
	}

	for _, c := range cases {
		if got := scrollTop(c.top, c.cursor, c.total, c.height); got != c.want {
			t.Errorf("%s: scrollTop(%d, %d, %d, %d) = %d, want %d",
				c.name, c.top, c.cursor, c.total, c.height, got, c.want)
		}
	}
}
