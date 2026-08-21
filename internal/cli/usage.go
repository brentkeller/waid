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
  waid move <id> [-p <parent>|--top]  Refile an item under a parent, or to the top level
  waid note <id> "<text>"
  waid show <id>                      Full item with notes and history
  waid transcript <id>                A session's turns, oldest first
  waid promote <key>                  waid dismiss <key>
  waid undismiss <key>                Restore a dismissed signal
  waid doctor                         Validate config, log integrity, gh auth, cache freshness
  waid ui                             The terminal app: loops, repos and agents in one screen

Global flags:
  --json                              Print a single JSON document to stdout
  --no-sync                           Skip the implicit session sync
  --waid-home <path>                  Override $WAID_HOME
  --version                           Print the version
  -h, --help                          Show this help
`
