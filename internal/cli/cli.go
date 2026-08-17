package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
)

// Process exit codes. Success is 0, anything the user can fix is 1, and everything else is 2.
const (
	ExitOK       = 0
	ExitUser     = 1
	ExitInternal = 2
)

// homeFlag overrides the data directory waid reads and writes.
const homeFlag = "--waid-home"

// Io is where the CLI writes. Tests replace both ends with buffers.
type Io struct {
	Out io.Writer
	Err io.Writer
}

// DefaultIo writes to the process's own streams.
var DefaultIo = Io{Out: os.Stdout, Err: os.Stderr}

// executor runs a parsed command line. It reports whether output was requested as JSON through
// json, which is set as soon as the flags are known so a later failure is reported in kind.
type executor func(argv []string, out Io, json *bool) error

// Run executes argv and returns the process exit code. The optional executor replaces dispatch.
func Run(argv []string, out Io, exec ...executor) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = reportPanic(r, out)
		}
	}()

	run := execute
	if len(exec) > 0 {
		run = exec[0]
	}

	asJson := false
	return report(run(argv, out, &asJson), asJson, out)
}

// execute parses argv and runs the requested command.
func execute(argv []string, out Io, asJson *bool) error {
	for _, arg := range argv {
		if arg == "--json" {
			*asJson = true
		}
	}

	for index := 0; index < len(argv); index++ {
		arg := argv[index]
		if arg == homeFlag {
			// The override carries a path, which is a value rather than the command.
			index++
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			return Userf("unknown command: %s", arg)
		}
	}
	return nil
}

// report writes an error to stderr and returns the exit code it maps to.
func report(err error, asJson bool, out Io) int {
	if err == nil {
		return ExitOK
	}

	var user *UserError
	if errors.As(err, &user) {
		if asJson {
			writeJsonLine(out.Err, userErrorPayload{Error: user.Message, Candidates: user.Candidates})
		} else {
			fmt.Fprint(out.Err, humanUserError(user))
		}
		return ExitUser
	}

	return reportInternal(err.Error(), asJson, out)
}

// reportPanic turns a recovered panic into the internal-error exit path.
func reportPanic(recovered any, out Io) int {
	message, ok := recovered.(string)
	if !ok {
		if err, isError := recovered.(error); isError {
			message = err.Error()
		} else {
			message = fmt.Sprint(recovered)
		}
	}
	return reportInternal(message, false, out)
}

func reportInternal(message string, asJson bool, out Io) int {
	stack := message + "\n" + string(debug.Stack())
	if asJson {
		writeJsonLine(out.Err, internalErrorPayload{Error: message, Stack: stack})
	} else {
		fmt.Fprint(out.Err, ensureNewline(stack))
	}
	return ExitInternal
}

type userErrorPayload struct {
	Error      string   `json:"error"`
	Candidates []string `json:"candidates"`
}

type internalErrorPayload struct {
	Error string `json:"error"`
	Stack string `json:"stack"`
}

// writeJsonLine emits one compact JSON document, leaving `<`, `>`, and `&` unescaped so the output
// matches what the Node CLI writes.
func writeJsonLine(w io.Writer, payload any) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		fmt.Fprintf(w, "%v\n", payload)
		return
	}
	w.Write(buf.Bytes())
}

func humanUserError(err *UserError) string {
	lines := make([]string, 0, len(err.Candidates)+1)
	lines = append(lines, err.Message)
	for _, candidate := range err.Candidates {
		lines = append(lines, "  "+candidate)
	}
	return strings.Join(lines, "\n") + "\n"
}

func ensureNewline(text string) string {
	if strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}
