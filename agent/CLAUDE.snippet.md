## Tracking work (waid)

When I report a bug, defer work, or get blocked on someone else, record it:
`waid add "<title>" -p <project path> [--waiting-on <who>] [--tag bug] --json`
A path in `-p` records where the work came from; a title fragment files it under that item instead.
When something is resolved, run `waid done <id> --json`. Always pass `--json`.
Don't record routine steps you complete within the session — only work that outlives it.
