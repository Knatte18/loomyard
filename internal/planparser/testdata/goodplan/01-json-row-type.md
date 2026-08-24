# Card 1 — json-row-type

**Create:**
- `boardcli.RowJSON`

**Intent:** Define the `RowJSON` struct carrying the list command's existing table columns as
JSON-taggable fields — the shape `--json` emission marshals through `output.Ok`.

**Commit:** `1: json-row-type`
**Verify:** go build ./...
