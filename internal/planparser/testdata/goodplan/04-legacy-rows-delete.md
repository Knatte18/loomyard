# Card 4 — legacy-rows-delete

**Delete:**
- `//internal/boardengine/legacyrows.go`

**Intent:** Remove the legacy per-row conversion helper now that `boardengine.MapRowJSON` (card 5) supersedes it.

**ImpactSummary:** Deletes the legacy row-conversion file; no remaining callers reference it.
