All 4 cards in this batch (8, 9, 10, 11) are committed and match their declared `Commit:` messages in the log — 4 of 4 cards complete. The batch's `verify: go test ./internal/planparser/...` passes cleanly.

{"status":"success","commit_sha":"fee4e6543d7bfdd41c11deb729e795a600270a9d","session_id":"98beb5de-4a41-4ae2-bcba-e68ae16d3189","cards_done":[8,9,10,11]}

Summary of work:
- **Card 8** — Rebuilt `internal/planparser/testdata/goodplan/` as a seven-card format-4 fixture (`00-overview.md` plus `01-json-row-type.md` through `07-json-docs.md`), one card per type (Create/Edit/Custom/Delete/Rename/Move/Prosa), deleting the four old format-3 card files.
- **Card 9** — Rewrote `internal/planparser/parse_test.go` for the format-4 grammar: updated `minimalOverview`/`minimalCardFile`, deleted two format-3-only tests, renamed the Moves test to `TestParsePlan_Card_RenameGrammar`, added type-label/Uses-empty/multiline-ImpactSummary/retired-label coverage, and rewrote `TestParsePlan_GoldenFixture` as a full seven-card round trip.
- **Card 10** — Rewrote `internal/planparser/normalize_test.go` (Pairs-based endpoint test, classifier-gate test, Pairs/Targets-agreement test) and `internal/planparser/sections_test.go` (updated Rename mechanic body text, moved the plan-level-sections fixture to format 4).
- **Card 11** — Rewrote `internal/planparser/validate_test.go` with one focused test per each of the sixteen checks, a rebuilt `validCard` baseline, a Custom-card generic-checks-still-apply test, and a rewritten zero-findings golden test over the new seven-card fixture.

All 4 cards are committed; `go test ./internal/planparser/...` passes. No uncommitted tracked changes remain.