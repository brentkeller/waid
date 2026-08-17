package command

import (
	"slices"
	"strconv"
	"strings"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/render"
	"github.com/brentkeller/waid/internal/sessions"
)

// TranscriptResult is one session with the turns its transcript holds, in file order.
type TranscriptResult struct {
	Session sessions.Session `json:"session"`
	Turns   []sessions.Turn  `json:"turns"`
}

// Column widths for a turn row: the clock and the widest role label, leaving the rest of an
// 80-column terminal to the text.
const (
	turnTimeWidth = 5
	turnRoleWidth = 6
	turnTextWidth = 62
)

func runTranscript(ctx *cli.Ctx) (TranscriptResult, error) {
	id, err := requiredSessionId(ctx)
	if err != nil {
		return TranscriptResult{}, err
	}

	session, err := resolveSession(ctx, id)
	if err != nil {
		return TranscriptResult{}, err
	}

	turns, err := sessions.ReadTurns(session.File.Path, sessions.ReadLines)
	if err != nil {
		// The cache records where a transcript was, not where it still is; a file deleted or moved
		// since the last sync is the environment, not a bug.
		return TranscriptResult{}, errs.Userf("cannot read the transcript for %s: %s", session.Id, session.File.Path)
	}

	return TranscriptResult{Session: session.Session, Turns: turns}, nil
}

func requiredSessionId(ctx *cli.Ctx) (string, error) {
	id := ""
	if len(ctx.Args) > 0 {
		id = strings.TrimSpace(ctx.Args[0])
	}
	if id == "" {
		return "", errs.Userf("transcript requires a session id")
	}
	return id, nil
}

// resolveSession takes the id as given, then as a prefix, because a Claude Code session id is a
// 36-character uuid and nobody types one in full.
func resolveSession(ctx *cli.Ctx, id string) (sessions.CachedSession, error) {
	matches := []sessions.CachedSession{}
	for _, session := range ctx.Sessions {
		if session.Id == id {
			return session, nil
		}
		if strings.HasPrefix(session.Id, id) {
			matches = append(matches, session)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return sessions.CachedSession{}, errs.Userf("unknown session id: %s", id)
	default:
		candidates := make([]string, 0, len(matches))
		for _, session := range matches {
			candidates = append(candidates, session.Id+"  "+session.Title)
		}
		slices.Sort(candidates)
		return sessions.CachedSession{}, errs.Ambiguous("ambiguous session id: "+id, candidates)
	}
}

func renderTranscript(data TranscriptResult, _ *cli.Ctx) string {
	session := data.Session

	lines := []string{
		session.Id + "  " + session.Title,
		"",
		field("project", orDash(session.Project)),
		field("branch", orDash(session.Branch)),
		field("started", startedLabel(session.Started)),
		field("prompts", strconv.Itoa(session.Prompts)),
		"",
		"  turns",
	}

	if len(data.Turns) == 0 {
		return strings.Join(append(lines, "    (no turns)"), "\n")
	}
	for _, turn := range data.Turns {
		lines = append(lines, "    "+render.Pad(clock(turn.Ts), turnTimeWidth)+"  "+
			render.Pad(turn.RoleLabel(), turnRoleWidth)+"  "+render.Truncate(oneLine(turn.Text), turnTextWidth))
	}
	return strings.Join(lines, "\n")
}

// startedLabel dates the session in local time, to the minute the turn rows are stamped in.
func startedLabel(ts *string) string {
	if ts == nil {
		return "-"
	}
	return render.LocalYmdISO(*ts) + " " + clock(ts)
}

// clock renders a turn's local time of day, standing in for a timestamp the record never carried or
// that will not parse.
func clock(ts *string) string {
	if ts == nil {
		return "--:--"
	}
	parsed, ok := render.ParseTime(*ts)
	if !ok {
		return "--:--"
	}
	return parsed.Local().Format("15:04")
}

// oneLine collapses a turn onto a single row, since a prompt is often a paragraph and the column
// holds one line.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
