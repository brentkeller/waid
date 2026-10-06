package cli

// Usage is what `waid --help`, and a bare `waid`, print.
const Usage = `waid — what am I doing

Usage: waid <command> [options]

  waid sync [--full]                  Rebuild the derived session cache
  waid today [--date YYYY-MM-DD]      Sessions + item activity for a day
  waid week [--last]                  Rollup by project for this week (or last)
  waid loops [-p <parent>] [--origin <frag>] [--tag t] [--headings]
                                      Open items as a tree, then detected signals
  waid scan [-p <project>]            Detected signals only
  waid list [--status s] [--origin <frag>] [--tag t] [--all]
    [--headings]                      Show empty headings alongside the matches
  waid add "<title>" [-p <parent>|--parent <id>] [--waiting-on <who>] [--tag <t>]
    [--session <id>] [--heading]      --heading creates the item as a heading
  waid done <id>...                   Close one or more; waid reopen <id>
  waid move <id> [-p <parent>|--parent <id>|--top]
                                      Refile an item under a parent, or to the top level
  waid heading <id> [--off]           Mark an item a heading, or unmark it
  waid tag <id> [-t <t>] [--remove <t>]
    [--off]                           Add or remove tags; --off clears them all
  waid note <id> "<text>"
  waid show <id>                      Full item with notes and history
  waid transcript <id>                A session's turns, oldest first
  waid promote <key>                  waid dismiss <key>
  waid undismiss <key>                Restore a dismissed signal
  waid doctor                         Validate config, log integrity, gh auth, cache freshness
  waid ui                             The terminal app: loops, repos and agents in one screen

-p names a parent: any substring, ignoring case, matching exactly one item's title. An absolute
path, or . for the current directory, records where the work came from instead and leaves the item
at the top level. On add and move, --parent names the parent by its id instead, which no other item
can share. --origin filters list and loops by that recorded path. On scan, -p still names a
repo, since detection is keyed by path.

Global flags:
  --json                              Print a single JSON document to stdout
  --no-sync                           Skip the implicit session sync
  --waid-home <path>                  Override $WAID_HOME
  --version                           Print the version
  -h, --help                          Show this help
`
