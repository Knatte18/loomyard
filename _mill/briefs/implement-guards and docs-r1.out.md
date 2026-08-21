{"status":"success","commit_sha":"1867aa7f7d869d135f21e564fa56637c2a12348d","session_id":"b06ffffb-7b27-496c-8d0e-b799fd70e729","cards_done":[19,20,21,22,23,24]}

All 6 of 6 cards in this batch are committed (Cards 19-24). Summary of work:

- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/shedrecipe/coverage_guard_test.go` (new) — Card 19: coverage guard building a real `loomshed.New` and cross-checking against `loomRowEngines`, plus `TestRegistry_ShipsTwelveEntries` and the Publish/Finalize identity pin.
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/shedrecipe/seam_enforcement_test.go` (new) — Card 20: production-import allowlist guard (`shedrecipeAllowedImports`, ten packages) plus the explicit `internal/lyxcwd` exclusion assertion.
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/CONSTRAINTS.md` — Card 21: new `## Shed Recipe Registry Invariant` section after `## Shed Producer-Seam Invariant`; added `internal/shedrecipe` to the Told-Geometry Invariant's Machine-enforced bullet and corrected its stale count from "seven" to "ten".
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/docs/overview.md` — Card 22: new tree-listing row for `internal/shedrecipe`, plus a sentence in the `shed` prose bullet.
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/manifest/designs/shed-recipe.md` — Card 23: narrowed title/banner to piece-1-shipped, pieces-2-4-DRAFT; marked piece 1 built in the Pieces list; added the live-seams/Env paragraph.
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/manifest/roadmap.md` — Card 24: removed the engine-registry item from Planned, fixed the two stale dependency references, added the Done entry.

`go test ./internal/shedrecipe/... ./internal/lyxcwd/...` (the batch's `verify:`) passes, and `go vet ./...` (the plan's module-wide verify) is clean. No uncommitted tracked changes remain (`git status --porcelain --untracked-files=no` is empty). The `_mill/briefs/implement-guards and docs-r1.md` file is untracked and out of scope, left as-is.
