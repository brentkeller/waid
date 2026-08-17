package harness_test

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/harness"
)

// exercise runs one invocation and asserts what has to hold for every command in this package,
// whatever the data it is pointed at: the binary answers rather than failing internally, and a
// --json run leaves a single machine-readable document where a caller looks for it — on stdout when
// the command succeeded, on stderr when it reported something the user can fix.
func exercise(t *testing.T, home string, inv harness.Invocation) harness.Output {
	t.Helper()

	out := harness.Run(t, home, inv)
	line := "waid " + strings.Join(inv.Args, " ")

	if out.Code == cli.ExitInternal {
		t.Fatalf("%s failed internally:\n%s", line, out.Stderr)
	}
	if !asksForJson(inv) {
		return out
	}

	switch out.Code {
	case cli.ExitOK:
		assertJsonDocument(t, line+": stdout", out.Stdout)
	case cli.ExitUser:
		assertJsonDocument(t, line+": stderr", out.Stderr)
	}
	return out
}

func asksForJson(inv harness.Invocation) bool {
	for _, arg := range inv.Args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

// assertJsonDocument checks that a stream holds one JSON value and nothing else, which is the
// contract a caller piping waid into jq depends on: a banner, a stray note or a second document
// would all decode as far as the first value and then fail here.
func assertJsonDocument(t *testing.T, label, stream string) {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stream))
	var document json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		t.Errorf("%s is not a JSON document: %v\n%s", label, err, stream)
		return
	}
	if _, err := decoder.Token(); err != io.EOF {
		t.Errorf("%s carries more than one document:\n%s", label, stream)
	}
}
