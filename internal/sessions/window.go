package sessions

import (
	"time"

	"github.com/brentkeller/waid/internal/render"
)

// StartedAt is when a session began. A session carrying only one of its two timestamps stands the
// other one in, so a truncated transcript still places itself on a calendar.
func (s Session) StartedAt() (time.Time, bool) { return instant(s.Started, s.Ended) }

// EndedAt is when a session last recorded anything, standing Started in when Ended is missing.
func (s Session) EndedAt() (time.Time, bool) { return instant(s.Ended, s.Started) }

// Overlaps reports whether any part of the session's span falls inside the half-open window. A
// session with neither timestamp parseable belongs to no window at all.
func (s Session) Overlaps(start, end time.Time) bool {
	started, ok := s.StartedAt()
	ended, endOk := s.EndedAt()
	if !ok || !endOk {
		return false
	}
	return started.Before(end) && !ended.Before(start)
}

// ByStart orders sessions oldest first, treating an unparseable timestamp as equal to everything so
// an unsortable session keeps its place rather than moving one.
func ByStart(a, b Session) int {
	left, leftOk := a.StartedAt()
	right, rightOk := b.StartedAt()
	if !leftOk || !rightOk {
		return 0
	}
	return left.Compare(right)
}

// instant parses the first of the two timestamps that is present.
func instant(preferred, fallback *string) (time.Time, bool) {
	for _, candidate := range []*string{preferred, fallback} {
		if candidate == nil {
			continue
		}
		return render.ParseTime(*candidate)
	}
	return time.Time{}, false
}
