# Card 4 — helptree-rename

**What:** Update the pinned help-tree set with the new `--json` flag help text, and relocate the
row mapper via `git mv` per the Rename mechanic above (no behavior change in this card).
**Context:** none
**Edits:**
- `//cmd/lyx/helptree_test.go`
**Creates:** none
**Deletes:** none
**Moves:**
- `//internal/boardengine/rows.go` -> `//internal/boardengine/rowsjson.go`
**Depends-on:** 1
