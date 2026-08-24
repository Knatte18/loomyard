# Card 5 — rowmapper-rename

**Rename:**
- `boardengine.MapRow` -> `boardengine.MapRowJSON`
- `//internal/boardengine/rows.go` -> `//internal/boardengine/rowsjson.go`

**Intent:** Rename the row mapper and its file to make the JSON-oriented behavior explicit ahead of a later extraction.
