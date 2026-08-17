// Command waid is a local open-loop tracker over an append-only event log.
package main

import (
	"os"

	"github.com/brentkeller/waid/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.DefaultIo))
}
