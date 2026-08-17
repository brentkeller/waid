package command

import (
	"fmt"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/sessions"
)

// SyncResult is what a sync moved, so a run is legible without diffing the cache file. Fields are
// declared in the order Node emits them, which is the order --json renders.
type SyncResult struct {
	// Scanned is the transcripts found on disk.
	Scanned int `json:"scanned"`
	// Parsed is the transcripts re-read because they were new or had changed.
	Parsed int `json:"parsed"`
	// Reused is the transcripts skipped because their stats matched the cache.
	Reused int `json:"reused"`
	// Removed is the cached sessions dropped because their transcript is gone.
	Removed int `json:"removed"`
	// Sessions is how many sessions the cache holds afterwards.
	Sessions int    `json:"sessions"`
	SyncedAt string `json:"syncedAt"`
}

func runSync(ctx *cli.Ctx) (SyncResult, error) {
	result, err := sessions.Sync(ctx.Cfg, sessions.SyncOptions{Full: ctx.Flags.Bool("full"), Now: ctx.Now})
	if err != nil {
		return SyncResult{}, err
	}

	return SyncResult{
		Scanned:  result.Stats.Scanned,
		Parsed:   result.Stats.Parsed,
		Reused:   result.Stats.Reused,
		Removed:  result.Stats.Removed,
		Sessions: len(result.Sessions),
		SyncedAt: result.SyncedAt,
	}, nil
}

func renderSync(data SyncResult, _ *cli.Ctx) string {
	return fmt.Sprintf(
		"synced %s  parsed %d, reused %d, removed %d",
		plural(data.Sessions, "session"), data.Parsed, data.Reused, data.Removed,
	)
}
