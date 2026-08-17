package command

import (
	"os"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/tui"
)

// UiResult is what the app leaves for dispatch to present: nothing. The app owns the terminal while
// it runs and replays its own receipts to the restored one, so there is no result to render.
type UiResult struct{}

// The guard and the app sit behind seams, so `ui` is exercised from a test binary that has no
// terminal to attach to and could not be allowed to start the program if it did.
var (
	stdoutIsTerminal = func() bool { return isTerminal(os.Stdout) }
	startApp         = func(ctx *cli.Ctx) error {
		return tui.Run(tui.Options{Cfg: ctx.Cfg, Now: ctx.Now, Ids: ctx.Ids})
	}
)

// runUi starts the terminal app, refusing to run when stdout is not a terminal. The app is never a
// fallback for a pipe: a redirected run would write control sequences and an alt-screen switch to
// whatever is reading, and there would be no keyboard to drive it with.
func runUi(ctx *cli.Ctx) (UiResult, error) {
	if !stdoutIsTerminal() {
		return UiResult{}, errs.Userf("waid ui requires an interactive terminal")
	}
	return UiResult{}, startApp(ctx)
}

func renderUi(UiResult, *cli.Ctx) string { return "" }

// isTerminal reports whether the file is a character device, which a console handle and a pty both
// are, and which a pipe, a socket and a regular file are not.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
