package tui

// scrollMargin is the rows kept between the cursor and the edge of a list's window, so the rows
// either side of the selection stay in sight as it nears the edge.
const scrollMargin = 2

// scrollTop is the first line a list of total lines shows through a window of the given height. It
// moves from the previous offset only as far as keeping the cursor in sight takes, so the list holds
// still under a cursor moving within the window rather than recentring on every press.
func scrollTop(top, cursor, total, height int) int {
	if height <= 0 || total <= height {
		return 0
	}

	margin := min(scrollMargin, (height-1)/2)
	if cursor < top+margin {
		top = cursor - margin
	}
	if cursor > top+height-1-margin {
		top = cursor - height + 1 + margin
	}
	return min(max(top, 0), total-height)
}

// scrollLines is the run of lines a window shows, starting at the offset scrollTop chose.
func scrollLines(lines []string, top, height int) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	top = min(max(top, 0), len(lines)-height)
	return lines[top : top+height]
}

// padRows grows a run of lines with blank ones to the rows it was given.
func padRows(lines []string, rows int) []string {
	if grow := rows - len(lines); grow > 0 {
		return append(lines, make([]string, grow)...)
	}
	return lines
}

// follow keeps the live list's offset in step with its cursor. It runs after every update rather
// than in View, which cannot keep what it computed, so the window holds its place from one frame to
// the next.
func (m Model) follow() Model {
	height := m.bodyRows()
	if height <= 0 || m.showKeys {
		return m
	}

	width := m.viewWidth()
	switch m.tab {
	case tabLoops:
		rows := m.loopsListRows(height)
		if m.loops.moving.active() {
			t := m.moveTree(width)
			m.loops.moving.top = scrollTop(m.loops.moving.top, t.CursorLine(), t.LineCount(), rows)
		} else {
			t := m.loopsTree(width)
			m.loops.top = scrollTop(m.loops.top, t.CursorLine(), t.LineCount(), rows)
			// The detail's offset is dropped the moment the cursor leaves the item it was paged on, so
			// coming back to the item opens it at its top like any other.
			if item, selected := t.SelectedItem(); !selected || item.Id != m.loops.detailId {
				m.loops.detailId, m.loops.detailTop = "", 0
			}
		}
	case tabScan:
		t := m.scanTree(width)
		m.scan.top = scrollTop(m.scan.top, t.CursorLine(), t.LineCount(), m.scanListRows(height))
	case tabReview:
		t := m.reviewTree(width)
		m.review.top = scrollTop(m.review.top, t.CursorLine(), t.LineCount(), m.reviewListRows(height))
	}
	return m
}
