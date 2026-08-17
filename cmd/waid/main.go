// Command waid is a local open-loop tracker over an append-only event log.
package main

import (
	"os"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/command"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.DefaultIo, command.Registry))
}
