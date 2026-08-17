// Package command holds one module per waid command, each pairing the data it produces with the
// text that data renders as.
package command

import "github.com/brentkeller/waid/internal/cli"

// Registry is the dispatch table: every command waid answers to is listed here, and anything absent
// from it is an unknown command.
var Registry = cli.Registry{}
