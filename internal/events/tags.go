package events

import "strings"

// NormalizeTags is what a tag set looks like once it has been written down: each tag trimmed, the
// blanks dropped, the repeats reduced to their first appearance, and the order otherwise the order
// it was given in. The result is never nil, since the log holds an empty array rather than null.
//
// Every surface that writes tags goes through it, so a set typed into the app and a set assembled
// from -t flags reach the log in the same shape. Case is left alone: it is a difference on the
// command line, and folding it here would silently rename a tag the caller asked for.
func NormalizeTags(tags []string) []string {
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))

	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		normalized = append(normalized, trimmed)
	}
	return normalized
}
