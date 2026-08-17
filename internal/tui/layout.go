package tui

const (
	// splitMinWidth is the one measurement behind the responsive split (§5). Below it a side-by-side
	// layout leaves the detail pane a gutter too narrow to read prose in, so the detail becomes a
	// full-width overlay instead.
	splitMinWidth = 120

	// dividerWidth is the gutter between the panes: a vertical rule with a space either side.
	dividerWidth = 3

	// detailShare is the fraction of the usable width the detail takes when the panes sit side by
	// side. The list keeps the larger share because it lays out in columns; the detail wraps prose.
	detailShare = 2.0 / 5.0
)

// Panes is the geometry of a tab's master/detail split: the columns each side gets and whether the
// two are showing at once. Every tab measures through it, so the tabs cannot disagree about where
// the split falls or how wide the detail is.
type Panes struct {
	// SideBySide reports whether the detail sits beside the list. When it does not, the detail is
	// an overlay drawn over the list rather than next to it, and only one of the two is visible.
	SideBySide bool

	// List, Divider and Detail are column counts. Side by side they span the terminal exactly;
	// as an overlay the divider is zero and both panes measure the full width.
	List    int
	Divider int
	Detail  int
}

// Split measures the panes for a terminal of the given width. Widths a terminal reports before its
// first resize — zero, or negative — come back as empty panes rather than negative ones.
func Split(width int) Panes {
	if width < 0 {
		width = 0
	}
	if width < splitMinWidth {
		return Panes{List: width, Detail: width}
	}

	usable := width - dividerWidth
	detail := int(float64(usable) * detailShare)
	return Panes{SideBySide: true, List: usable - detail, Divider: dividerWidth, Detail: detail}
}
