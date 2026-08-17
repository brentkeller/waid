package cli

import (
	"fmt"
	"time"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/gh"
	"github.com/brentkeller/waid/internal/git"
)

// Ctx is everything a command needs to run.
type Ctx struct {
	Cfg   config.Config
	Flags Flags
	// Args are the positionals after the command name, in order.
	Args []string
	Cwd  string
	// Now is the instant the run treats as the present, pinned through EnvNow for a reproducible run.
	Now time.Time
	// Git and Gh are the detection seams. Nil outside tests, where detection reaches for the real
	// clients instead.
	Git git.Client
	Gh  gh.Client
	// Notes are non-fatal degradations, such as an unavailable gh. The CLI prints them itself, so a
	// command only reads them back when it wants them inside its own --json payload.
	Notes []string
}

// Note records a degradation to print alongside the command's own output.
func (c *Ctx) Note(text string) { c.Notes = append(c.Notes, text) }

// Registry maps a command name to the module answering it. cmd/waid supplies the real table.
type Registry map[string]Command

// Command is a command as dispatch sees it, with its result type erased. Only Module implements it,
// so every command is a run/render pair rather than a function that prints.
type Command interface {
	run(ctx *Ctx) (any, error)
	render(data any, ctx *Ctx) string
}

// Module pairs the data a command produces with the text that data renders as. Splitting them is
// what keeps --json and human output from diverging: both are views of the same value.
type Module[D any] struct {
	// Run produces the command's result, which is what --json prints verbatim.
	Run func(ctx *Ctx) (D, error)
	// Render turns that same result into the human output.
	Render func(data D, ctx *Ctx) string
}

func (m Module[D]) run(ctx *Ctx) (any, error) { return m.Run(ctx) }

func (m Module[D]) render(data any, ctx *Ctx) string {
	typed, ok := data.(D)
	if !ok {
		panic(fmt.Sprintf("command produced %T, but renders %T", data, typed))
	}
	return m.Render(typed, ctx)
}
