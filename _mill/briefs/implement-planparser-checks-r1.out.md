{"status":"success","commit_sha":"038187a053cf4684439258439852e341b244d51b","session_id":"32c024e6-2fcb-4c07-ac81-de017933010e","cards_done":[6,7,8,9]}

Summary: All 4 cards in batch `02-planparser-checks` (Card 6, 7, 8, 9) were committed this turn, honestly verified against the batch's `## Cards` section — 4 of 4 cards complete.

- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/planparser/validate.go` (new): `ValidationError`, `Validate(plan, worktreeRoot)`, and all 14 plan-format-v3 checks, keyed by flat card id (`N-<slug>`) instead of v2's batch key.
- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/planparser/validate_test.go` (new): table-driven coverage for all 14 checks (triggering + clean case each), including a golden-fixture zero-findings happy path and three hermetic `t.TempDir()`-backed existence checks.

Commits (webster-rewrite branch, pushed to origin):
- `49faf0bd` feat(planparser): validation types and format/structure checks (Card 6)
- `bf12104e` feat(planparser): card-path and Moves grammar checks (Card 7)
- `59b7fa90` feat(planparser): existence-dependent checks and depends-on-order (Card 8)
- `038187a0` test(planparser): table-driven coverage for all 14 validation checks (Card 9)

Verify (`go test ./internal/planparser/...`) passes. One notable design decision made while implementing, since it wasn't explicit in the plan: `index-file-mismatch` owns the Card-Index numbering-gap/duplicate check (mirroring v2's original scope), while `card-numbering` owns only the heading-vs-index cross-check — re-reading each card file directly via `plan.Dir` (reusing parse.go's unexported `cardFileName`/`cardHeadingRe`, same package, no edit to parse.go needed) since `ParsePlan` deliberately discards the heading's own captured number. This avoids double-reporting the same gap defect under two different Check names while still exercising both spec bullets.
