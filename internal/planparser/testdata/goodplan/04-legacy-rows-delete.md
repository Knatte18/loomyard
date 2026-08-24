# Card 4 — legacy-rows-delete

**Delete:**
- `//internal/boardengine/legacyrows.go`

**Intent:** Remove the legacy per-row conversion helper `legacyrows.go` now that
`boardengine.MapRowJSON` (card 5) supersedes it — no remaining caller depends on it once card 5
lands.

**ImpactSummary:** Deletes the legacy row-conversion file; no remaining callers reference it.
