package cli

import (
	"os"
	"strings"
	"time"

	"github.com/brentkeller/waid/internal/gh"
	"github.com/brentkeller/waid/internal/ids"
)

// Environment seams that pin the sources of non-determinism a spawned waid would otherwise carry.
// They exist so the differential harness can diff two processes byte for byte; a normal run leaves
// them unset and takes the system clock, cryptographic randomness and the network.
const (
	// EnvNow pins the clock to an RFC 3339 instant.
	EnvNow = "WAID_NOW"
	// EnvIds pins the id generator to a comma-separated sequence, consumed in order.
	EnvIds = "WAID_IDS"
	// EnvGhFixture points at a recorded GitHub response served in place of a live query.
	EnvGhFixture = "WAID_GH_FIXTURE"
)

// Now returns the instant the run should treat as the present: the value pinned in EnvNow when it
// holds a parseable timestamp, otherwise the system clock.
func Now() time.Time {
	pinned := strings.TrimSpace(os.Getenv(EnvNow))
	if pinned == "" {
		return time.Now()
	}
	parsed, err := time.Parse(time.RFC3339, pinned)
	if err != nil {
		return time.Now()
	}
	return parsed
}

// IdGenerator returns the generator new items draw from: the sequence pinned in EnvIds when it is
// set, otherwise ids.New.
func IdGenerator() ids.Generator {
	pinned := []string{}
	for _, id := range strings.Split(os.Getenv(EnvIds), ",") {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			pinned = append(pinned, trimmed)
		}
	}
	if len(pinned) == 0 {
		return ids.New
	}
	return ids.Sequence(pinned...)
}

// GhClient returns the GitHub client the run should query: one served from the recorded response
// EnvGhFixture names when it is set, otherwise nil, which leaves detection to reach for the real gh.
func GhClient() gh.Client {
	path := strings.TrimSpace(os.Getenv(EnvGhFixture))
	if path == "" {
		return nil
	}
	return gh.Fixture(path)
}
