// Package events holds the event log's shapes and the fold that turns its lines into state. It
// never touches the filesystem: callers hand it lines and receive data.
package events

// Statuses lists every status an item can hold, in display order.
var Statuses = []Status{StatusOpen, StatusWaiting, StatusDone}

// Status is one of the three states an item can hold.
type Status string

const (
	StatusOpen    Status = "open"
	StatusWaiting Status = "waiting"
	StatusDone    Status = "done"
)

// IsStatus reports whether value is one of the three statuses.
func IsStatus(value string) bool {
	for _, status := range Statuses {
		if string(status) == value {
			return true
		}
	}
	return false
}

// Note is a timestamped note attached to an item.
type Note struct {
	Ts   string `json:"ts"`
	Text string `json:"text"`
}

// Item is an item as it exists after folding the event log; never stored in this shape.
type Item struct {
	Id     string `json:"id"`
	Title  string `json:"title"`
	Status Status `json:"status"`
	// WaitingOn is free text, only meaningful while Status is StatusWaiting.
	WaitingOn *string `json:"waitingOn"`
	// Project is the absolute path of the project the item belongs to.
	Project *string `json:"project"`
	// Session is the Claude Code session the item was captured from.
	Session *string  `json:"session"`
	Tags    []string `json:"tags"`
	Notes   []Note   `json:"notes"`
	// Created is the ts of the add event.
	Created string `json:"created"`
	// Updated is the ts of the most recent event touching the item.
	Updated string `json:"updated"`
}

// WaidEvent is a line of events.jsonl as waid writes it. Lines read back from disk are untrusted
// and are narrowed by the fold instead — these shapes describe output, never assumptions about
// input. Their fields are declared in the order Node emits them, which is the order the log holds.
type WaidEvent interface {
	event()
	// stamp returns a copy carrying ts, leaving a timestamp the caller already set alone.
	stamp(ts string) WaidEvent
	timestamp() string
}

// AddEvent introduces an item.
type AddEvent struct {
	Ts        string   `json:"ts"`
	Ev        string   `json:"ev"`
	Id        string   `json:"id"`
	Title     string   `json:"title"`
	Status    Status   `json:"status"`
	Project   *string  `json:"project"`
	Session   *string  `json:"session"`
	Tags      []string `json:"tags"`
	WaitingOn *string  `json:"waitingOn"`
}

// UpdateEvent patches the fields it carries, leaving the rest of the item alone. Nothing in this
// build emits one; it describes the lines already in the log.
type UpdateEvent struct {
	Ts        string   `json:"ts"`
	Ev        string   `json:"ev"`
	Id        string   `json:"id"`
	Title     *string  `json:"title,omitempty"`
	Status    Status   `json:"status,omitempty"`
	Project   *string  `json:"project,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	WaitingOn *string  `json:"waitingOn,omitempty"`
}

// ProjectEvent refiles an item, and is an update like any other — the fold reads the same `update`
// line. It is a shape of its own because UpdateEvent omits an absent project, which is what keeps a
// retitle or a waiting-on from moving the item out of the project it is filed under. Refiling has to
// say the opposite: the project is always written, and a null is how an item is moved out of every
// project.
type ProjectEvent struct {
	Ts      string  `json:"ts"`
	Ev      string  `json:"ev"`
	Id      string  `json:"id"`
	Project *string `json:"project"`
}

// NoteEvent appends a note to an item.
type NoteEvent struct {
	Ts   string `json:"ts"`
	Ev   string `json:"ev"`
	Id   string `json:"id"`
	Text string `json:"text"`
}

// CloseEvent marks an item done.
type CloseEvent struct {
	Ts string `json:"ts"`
	Ev string `json:"ev"`
	Id string `json:"id"`
}

// ReopenEvent returns an item to open and clears what it was waiting on.
type ReopenEvent struct {
	Ts string `json:"ts"`
	Ev string `json:"ev"`
	Id string `json:"id"`
}

// DismissEvent hides a detected signal by its key.
type DismissEvent struct {
	Ts  string `json:"ts"`
	Ev  string `json:"ev"`
	Key string `json:"key"`
}

// UndismissEvent restores a dismissed key.
type UndismissEvent struct {
	Ts  string `json:"ts"`
	Ev  string `json:"ev"`
	Key string `json:"key"`
}

func (AddEvent) event()       {}
func (UpdateEvent) event()    {}
func (ProjectEvent) event()   {}
func (NoteEvent) event()      {}
func (CloseEvent) event()     {}
func (ReopenEvent) event()    {}
func (DismissEvent) event()   {}
func (UndismissEvent) event() {}

func (e AddEvent) timestamp() string       { return e.Ts }
func (e UpdateEvent) timestamp() string    { return e.Ts }
func (e ProjectEvent) timestamp() string   { return e.Ts }
func (e NoteEvent) timestamp() string      { return e.Ts }
func (e CloseEvent) timestamp() string     { return e.Ts }
func (e ReopenEvent) timestamp() string    { return e.Ts }
func (e DismissEvent) timestamp() string   { return e.Ts }
func (e UndismissEvent) timestamp() string { return e.Ts }

func (e AddEvent) stamp(ts string) WaidEvent       { e.Ts = firstTs(e.Ts, ts); return e }
func (e UpdateEvent) stamp(ts string) WaidEvent    { e.Ts = firstTs(e.Ts, ts); return e }
func (e ProjectEvent) stamp(ts string) WaidEvent   { e.Ts = firstTs(e.Ts, ts); return e }
func (e NoteEvent) stamp(ts string) WaidEvent      { e.Ts = firstTs(e.Ts, ts); return e }
func (e CloseEvent) stamp(ts string) WaidEvent     { e.Ts = firstTs(e.Ts, ts); return e }
func (e ReopenEvent) stamp(ts string) WaidEvent    { e.Ts = firstTs(e.Ts, ts); return e }
func (e DismissEvent) stamp(ts string) WaidEvent   { e.Ts = firstTs(e.Ts, ts); return e }
func (e UndismissEvent) stamp(ts string) WaidEvent { e.Ts = firstTs(e.Ts, ts); return e }

func firstTs(existing, stamped string) string {
	if existing != "" {
		return existing
	}
	return stamped
}

// ProblemReason is why a log line could not be applied. Reported by `waid doctor`, never fatal.
type ProblemReason string

const (
	ReasonUnparseable     ProblemReason = "unparseable"
	ReasonNotAnObject     ProblemReason = "not-an-object"
	ReasonAddMissingField ProblemReason = "add-missing-fields"
	ReasonDuplicateId     ProblemReason = "duplicate-id"
	ReasonUnknownId       ProblemReason = "unknown-id"
	ReasonNoteMissingText ProblemReason = "note-missing-text"
	ReasonBadStatus       ProblemReason = "bad-status"
	ReasonMissingKey      ProblemReason = "missing-key"
	ReasonUnknownEv       ProblemReason = "unknown-ev"
)

// Problem is a log line the fold could not fully apply, located by its 1-based line number.
type Problem struct {
	Line   int           `json:"line"`
	Reason ProblemReason `json:"reason"`
	Id     *string       `json:"id"`
	Ev     *string       `json:"ev"`
}

// State is the result of folding the whole event log.
type State struct {
	Items []Item `json:"items"`
	// Dismissed holds the detected-signal keys hidden by dismiss.
	Dismissed []string  `json:"dismissed"`
	Problems  []Problem `json:"problems"`
}
