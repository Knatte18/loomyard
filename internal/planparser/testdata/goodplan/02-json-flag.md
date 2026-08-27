# Card 2 — json-flag

**Edit:**
- `boardcli.newListCmd`
- `list.go`

**Create:**
- `list_json_test.go`

**Uses:**
- `//internal/output/envelope.go`

**Intent:** Add the `--json` bool flag to `newListCmd` and branch its row output between the table writer and the JSON path.

**ImpactSummary:** Adds a --json flag to the list command and branches its row-emission path on it.
