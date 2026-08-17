package command_test

import (
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/command"
)

// Every command the binary answers to has a row in the usage table, so one added without a line in
// it is caught here rather than by a reader who never learns it exists.
func TestUsageListsEveryCommand(t *testing.T) {
	for name := range command.Registry {
		if !strings.Contains(cli.Usage, "waid "+name+" ") {
			t.Errorf("usage has no row for %q", name)
		}
	}
}
