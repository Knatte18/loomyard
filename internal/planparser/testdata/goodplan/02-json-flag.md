# Card 2 — json-flag

**Edit:**
- `boardcli.newListCmd`
- `list.go`

**Uses:**
- `//internal/output/envelope.go`

**Intent:** Add the `--json` bool flag to `newListCmd` and branch the command's row output between
the existing table writer and the JSON path, reusing `internal/output`'s envelope shape.

**ImpactSummary:** Adds a --json flag to the list command and branches its row-emission path on it.
