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

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/errs"
)

// Process exit codes. Success is 0, anything the user can fix is 1, and everything else is 2.
const (
	ExitOK       = 0
	ExitUser     = 1
	ExitInternal = 2
)

// Version is the release the binary reports.
const Version = "0.1.0"

// Io is where the CLI writes. Tests replace both ends with buffers.
type Io struct {
	Out io.Writer
	Err io.Writer
}

// DefaultIo writes to the process's own streams.
var DefaultIo = Io{Out: os.Stdout, Err: os.Stderr}

// Run executes argv against the registry and returns the process exit code.
func Run(argv []string, out Io, registry Registry) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = reportPanic(r, out)
		}
	}()

	// Set as soon as the flags are known, so a failure after that point is reported in kind.
	asJson := false
	return report(execute(argv, out, registry, &asJson), asJson, out)
}

// execute parses argv, prepares the home, and runs the command it names.
func execute(argv []string, out Io, registry Registry, asJson *bool) error {
	parsed, err := ParseArgv(argv)
	if err != nil {
		return err
	}
	*asJson = parsed.Flags.Bool("json")

	if parsed.Flags.Bool("version") {
		fmt.Fprintf(out.Out, "waid %s\n", Version)
		return nil
	}

	override, _ := parsed.Flags.String("waid-home")
	home := config.ResolveHome(override)
	if err := config.EnsureHome(home, nil); err != nil {
		return err
	}
	cfg, err := config.Load(home)
	if err != nil {
		return err
	}

	if parsed.Command == "" || parsed.Flags.Bool("help") {
		fmt.Fprint(out.Out, Usage)
		return nil
	}

	command, known := registry[parsed.Command]
	if !known {
		return errs.Userf("unknown command: %s", parsed.Command)
	}

	ctx := &Ctx{
		Cfg:   cfg,
		Flags: parsed.Flags,
		Args:  parsed.Args,
		Cwd:   workingDir(),
		Now:   Now(),
		Ids:   IdGenerator(),
		Gh:    GhClient(),
	}
	if command.wantsSessions() {
		attachSessions(ctx)
	}

	data, err := command.run(ctx)
	if err != nil {
		return err
	}
	return present(command, data, ctx, *asJson, out)
}

// present writes what the command produced: one JSON document with the notes alongside it on
// stderr, or the rendered text with the notes beneath it.
func present(command Command, data any, ctx *Ctx, asJson bool, out Io) error {
	if asJson {
		if err := writeJsonDocument(out.Out, data); err != nil {
			return err
		}
		// Stdout stays a single JSON document, so degradations are reported next to it.
		for _, note := range ctx.Notes {
			fmt.Fprintf(out.Err, "%s\n", note)
		}
		return nil
	}

	if text := compose(command.render(data, ctx), ctx.Notes); text != "" {
		fmt.Fprint(out.Out, ensureNewline(text))
	}
	return nil
}

// report writes an error to stderr and returns the exit code it maps to.
func report(err error, asJson bool, out Io) int {
	if err == nil {
		return ExitOK
	}

	var user *errs.UserError
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

// writeJsonDocument emits the command's result as one indented JSON document, matching what
// JSON.stringify(data, null, 2) produces. Nothing reaches the stream until the whole document
// encodes, so a failure mid-value cannot leave half a document behind.
func writeJsonDocument(w io.Writer, payload any) error {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return fmt.Errorf("encoding the result: %w", err)
	}
	_, err := w.Write(buf.Bytes())
	return err
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

// compose joins rendered output and the run's notes into one block, dropping the empty parts.
func compose(rendered string, notes []string) string {
	parts := []string{}
	if trimmed := strings.TrimRight(rendered, "\n"); trimmed != "" {
		parts = append(parts, trimmed)
	}
	for _, note := range notes {
		if note != "" {
			parts = append(parts, note)
		}
	}
	return strings.Join(parts, "\n")
}

func humanUserError(err *errs.UserError) string {
	lines := make([]string, 0, len(err.Candidates)+1)
	lines = append(lines, err.Message)
	for _, candidate := range err.Candidates {
		lines = append(lines, "  "+candidate)
	}
	return strings.Join(lines, "\n") + "\n"
}

func workingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

func ensureNewline(text string) string {
	if strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}
