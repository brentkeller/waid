package cli

import (
	"fmt"
	"time"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/gh"
	"github.com/brentkeller/waid/internal/git"
	"github.com/brentkeller/waid/internal/ids"
	"github.com/brentkeller/waid/internal/sessions"
)

// SyncStaleMinutes is how stale cache/sessions.json may be before a command needing sessions
// refreshes it.
const SyncStaleMinutes = 5

// attachSessions fills ctx.Sessions, refreshing the cache first when it is missing or stale. A sync
// that fails is not worth failing the command over: the cached sessions are still useful, so the
// command runs on them and the reason is noted.
func attachSessions(ctx *Ctx) {
	if !ctx.Flags.Bool("no-sync") {
		age := sessions.CacheAgeMinutes(ctx.Cfg, ctx.Now)
		if age == nil || *age >= SyncStaleMinutes {
			result, err := sessions.Sync(ctx.Cfg, sessions.SyncOptions{Now: ctx.Now})
			if err == nil {
				ctx.Sessions = result.Sessions
				return
			}
			ctx.Note(fmt.Sprintf("session sync failed (%s); using the cached sessions", err))
		}
	}

	ctx.Sessions = sessions.Load(ctx.Cfg).Sessions
}

// Ctx is everything a command needs to run.
type Ctx struct {
	Cfg   config.Config
	Flags Flags
	// Args are the positionals after the command name, in order.
	Args []string
	Cwd  string
	// Now is the instant the run treats as the present, pinned through EnvNow for a reproducible run.
	Now time.Time
	// Ids is the generator new items draw from, pinned through EnvIds for a reproducible run.
	Ids ids.Generator
	// Sessions are the harvested sessions, attached by dispatch for a command declaring
	// NeedsSessions.
	Sessions []sessions.CachedSession
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
	wantsSessions() bool
}

// Module pairs the data a command produces with the text that data renders as. Splitting them is
// what keeps --json and human output from diverging: both are views of the same value.
type Module[D any] struct {
	// Run produces the command's result, which is what --json prints verbatim.
	Run func(ctx *Ctx) (D, error)
	// Render turns that same result into the human output.
	Render func(data D, ctx *Ctx) string
	// NeedsSessions asks dispatch to fill Ctx.Sessions before the command runs.
	NeedsSessions bool
}

func (m Module[D]) run(ctx *Ctx) (any, error) { return m.Run(ctx) }

func (m Module[D]) wantsSessions() bool { return m.NeedsSessions }

func (m Module[D]) render(data any, ctx *Ctx) string {
	typed, ok := data.(D)
	if !ok {
		panic(fmt.Sprintf("command produced %T, but renders %T", data, typed))
	}
	return m.Render(typed, ctx)
}
