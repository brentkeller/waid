package sessions

// Turn is one exchange of a session's main conversation, reduced to the text of it. Sidechain and
// meta records carry no turn: a subagent's prompts and a slash command's expansion are machinery
// rather than conversation, which is the same line the prompt count already draws.
type Turn struct {
	Role string `json:"role"`
	// Ts is the record's timestamp, absent when the record carried none.
	Ts   *string `json:"ts"`
	Text string  `json:"text"`
}

// roleLabels name the two sides of a conversation the way the transcript reads rather than the way
// it is stored.
var roleLabels = map[string]string{"user": "you", "assistant": "claude"}

// RoleLabel is how a turn's side of the conversation is printed. A role the map does not name prints
// as itself, since an unknown role is still worth showing.
func (t Turn) RoleLabel() string {
	if label, named := roleLabels[t.Role]; named {
		return label
	}
	return t.Role
}

// ReadTurns streams a transcript and returns its turns in file order.
//
// A line that is not a usable record is skipped rather than failing the read, matching FeedLine:
// transcripts are third-party data, and a session being appended to right now ends in a half-written
// line. An unreadable file is an error, since that is the caller's own input rather than a record
// inside it.
func ReadTurns(filePath string, read LineReader) ([]Turn, error) {
	turns := []Turn{}
	for line, err := range read(filePath) {
		if err != nil {
			return nil, err
		}
		if turn, ok := turnFromLine(line); ok {
			turns = append(turns, turn)
		}
	}
	return turns, nil
}

// turnFromLine narrows one raw transcript line into a turn, reporting false for anything that is not
// one.
func turnFromLine(rawLine string) (Turn, bool) {
	record, ok := decodeRecord(rawLine)
	if !ok {
		return Turn{}, false
	}

	role := derefOr(str(record["type"]), "")
	if role != "user" && role != "assistant" {
		return Turn{}, false
	}
	if flag(record["isSidechain"]) || flag(record["isMeta"]) {
		return Turn{}, false
	}

	text := messageText(record["message"])
	if text == nil {
		return Turn{}, false
	}

	return Turn{Role: role, Ts: str(record["timestamp"]), Text: *text}, true
}
