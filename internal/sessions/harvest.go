// Package sessions turns Claude Code transcripts into the sessions waid reports on.
package sessions

import (
	"encoding/json"
	"regexp"
	"strings"
)

// titleLength is how many characters of a user message are kept as a fallback title.
const titleLength = 80

// Session is one Claude Code session, distilled from its transcript file. Its fields are declared
// in the order Node emits them, which is the order the session cache holds.
type Session struct {
	// Id is the Claude Code session id; the transcript file is named after it.
	Id    string `json:"id"`
	Title string `json:"title"`
	// Project is the absolute path the session ran in.
	Project *string `json:"project"`
	Branch  *string `json:"branch"`
	// Started is the ISO timestamp of the first record, including sidechains.
	Started *string `json:"started"`
	// Ended is the ISO timestamp of the last record, including sidechains.
	Ended *string `json:"ended"`
	// Prompts counts user messages that were neither sidechain nor meta.
	Prompts int `json:"prompts"`
}

// Accumulator is the state built up one transcript line at a time. Only the handful of fields the
// session layer needs are tracked; the rest of each record is ignored.
type Accumulator struct {
	Id           *string
	AiTitle      *string
	FirstMessage *string
	Project      *string
	Branch       *string
	Started      *string
	Ended        *string
	Prompts      int
}

// NewAccumulator returns an accumulator holding nothing.
func NewAccumulator() *Accumulator { return &Accumulator{} }

// FeedLine applies one raw transcript line to the accumulator.
//
// Streaming and pure: no filesystem, no whole-file buffering, so a 2.5 MB transcript costs one line
// of memory. Transcript records are third-party data, so a line that is not a usable record is
// skipped silently rather than treated as an error.
func (acc *Accumulator) FeedLine(rawLine string) {
	record, ok := decodeRecord(rawLine)
	if !ok {
		return
	}

	if sessionId := str(record["sessionId"]); acc.Id == nil && sessionId != nil {
		acc.Id = sessionId
	}

	if timestamp := str(record["timestamp"]); timestamp != nil {
		// Every record type contributes to the span, so sidechain work keeps the session's real length.
		if acc.Started == nil {
			acc.Started = timestamp
		}
		acc.Ended = timestamp
	}

	recordType := derefOr(str(record["type"]), "")

	if recordType == "ai-title" {
		// Emitted repeatedly as the title is refined; the last one wins.
		if aiTitle := str(record["aiTitle"]); aiTitle != nil {
			acc.AiTitle = aiTitle
		}
		return
	}

	if recordType != "user" {
		return
	}

	sidechain := flag(record["isSidechain"])
	meta := flag(record["isMeta"])

	if !sidechain && acc.Project == nil {
		acc.Project = str(record["cwd"])
	}

	if branch := str(record["gitBranch"]); branch != nil && *branch != "" {
		acc.Branch = branch
	}

	if !meta && acc.FirstMessage == nil {
		if text := messageText(record["message"]); text != nil {
			acc.FirstMessage = text
		}
	}

	if !sidechain && !meta {
		acc.Prompts++
	}
}

// Finalize distils the accumulated state into a Session.
//
// fallbackProject is the caller's best guess from the transcript's directory name — lossy, and used
// only when no user record carried a cwd.
func (acc *Accumulator) Finalize(fallbackProject *string) Session {
	project := acc.Project
	if project == nil {
		project = fallbackProject
	}

	return Session{
		Id:      derefOr(acc.Id, ""),
		Title:   acc.title(),
		Project: project,
		Branch:  acc.Branch,
		Started: acc.Started,
		Ended:   acc.Ended,
		Prompts: acc.Prompts,
	}
}

// title prefers the model's title, falls back to the opening prompt, and gives up with a placeholder.
func (acc *Accumulator) title() string {
	if acc.AiTitle != nil {
		return *acc.AiTitle
	}
	if acc.FirstMessage != nil {
		return truncate(*acc.FirstMessage, titleLength)
	}
	return "(untitled)"
}

// windowsSlug matches a project directory name encoding a Windows path.
var windowsSlug = regexp.MustCompile(`^([A-Za-z])--(.*)$`)

// DecodeProjectSlug is a best-effort reversal of a Claude Code project directory name back into a
// path.
//
// Explicitly lossy: the slug flattens both separators and literal dashes, so `C--dev-dr-devresults`
// could be `C:\dev\dr\devresults` or `C:\dev\dr-devresults`. Only a last resort — a transcript's cwd
// is always preferred.
func DecodeProjectSlug(dirName string) string {
	if windows := windowsSlug.FindStringSubmatch(dirName); windows != nil {
		return strings.ToUpper(windows[1]) + `:\` + strings.ReplaceAll(windows[2], "-", `\`)
	}
	if strings.HasPrefix(dirName, "-") {
		return strings.ReplaceAll(dirName, "-", "/")
	}
	return dirName
}

// decodeRecord narrows a raw transcript line to the record it holds, reporting false for anything
// that is not a JSON object — a blank line, or the half-written final line of a session being
// appended to right now.
func decodeRecord(rawLine string) (map[string]any, bool) {
	line := strings.TrimSpace(rawLine)
	if line == "" {
		return nil, false
	}

	var parsed any
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		return nil, false
	}
	record, isRecord := parsed.(map[string]any)
	return record, isRecord
}

// messageText pulls the text out of a message, whose content is either a string or content blocks.
func messageText(message any) *string {
	record, isRecord := message.(map[string]any)
	if !isRecord {
		return nil
	}

	if text, isString := record["content"].(string); isString {
		return nonEmpty(strings.TrimSpace(text))
	}

	blocks, isArray := record["content"].([]any)
	if !isArray {
		return nil
	}
	for _, raw := range blocks {
		block, isRecord := raw.(map[string]any)
		if !isRecord {
			continue
		}
		if text := str(block["text"]); text != nil && strings.TrimSpace(*text) != "" {
			return ptrTo(strings.TrimSpace(*text))
		}
	}
	return nil
}

// truncate keeps the first limit UTF-16 code units of text, matching the character count Node
// counts. A character straddling the limit is dropped whole rather than split.
func truncate(text string, limit int) string {
	units := 0
	for index, char := range text {
		width := 1
		if char > 0xFFFF {
			width = 2
		}
		if units+width > limit {
			return text[:index]
		}
		units += width
	}
	return text
}

// str narrows an untrusted value to a string, reporting anything else as absent.
func str(value any) *string {
	text, isString := value.(string)
	if !isString {
		return nil
	}
	return &text
}

// flag reads an absent isSidechain / isMeta as false, so any falsy value reads as "not set".
func flag(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed != ""
	case float64:
		return typed != 0
	case nil:
		return false
	default:
		return true
	}
}

func nonEmpty(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

func derefOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}

func ptrTo[T any](value T) *T { return &value }
