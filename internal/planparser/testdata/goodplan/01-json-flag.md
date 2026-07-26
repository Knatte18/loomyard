# Card 1 — json-flag

**What:** Add a `--json` bool flag to the list command; define `RowJSON` with the existing
table's columns as fields.
**Context:** none
**Edits:**
- `list.go`
- `//internal/boardengine/rows.go`
**Creates:** none
**Deletes:** none
**Moves:** none
**Depends-on:** none
**Commit:** `1: json-flag`
**verify:** go build ./...
