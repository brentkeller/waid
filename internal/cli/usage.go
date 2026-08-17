package cli

// Usage is what `waid --help`, and a bare `waid`, print.
const Usage = `waid — what am I doing

Usage: waid <command> [options]

  waid sync [--full]                  Rebuild the derived session cache
  waid today [--date YYYY-MM-DD]      Sessions + item activity for a day
  waid week [--last]                  Rollup by project for this week (or last)
  waid loops [-p <project>]           Declared open/waiting items, then detected signals
  waid scan [-p <project>]            Detected signals only
  waid list [--status s] [--project p] [--tag t] [--all]
  waid add "<title>" [-p <project>] [--waiting-on <who>] [--tag <t>] [--session <id>]
  waid done <id>                      waid reopen <id>
  waid note <id> "<text>"
  waid show <id>                      Full item with notes and history
  waid promote <key>                  waid dismiss <key>
  waid doctor                         Validate config, log integrity, gh auth, cache freshness

Global flags:
  --json                              Print a single JSON document to stdout
  --no-sync                           Skip the implicit session sync
  --waid-home <path>                  Override $WAID_HOME
  --version                           Print the version
  -h, --help                          Show this help
`
