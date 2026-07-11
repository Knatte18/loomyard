All 6 cards' `Commit:` messages are present in the log (plus one plan-scope-extension commit for Card 1's file-list correction). 6 of 6 cards committed. Verify passes and the working tree is clean.

{"status":"success","commit_sha":"1d2ad59","session_id":"3f027585-65f2-478d-b0c1-4789eff43d74"}

Summary of work (6 of 6 cards committed, verify green):

- `internal/hubgeometry/hubgeometry.go`, `internal/hubgeometry/hubgeometry_unit_test.go` — added `PlanDir`, `BuilderDir`, `BuilderReportsDir` helpers (Card 1). Extended the plan file's Card 1 `Edits:` list first (commit `fcb7afe`) because the mirrored `PerchRunsDir` test coverage lives in the non-integration-tagged `hubgeometry_unit_test.go`, not `hubgeometry_test.go` (`//go:build integration`).
- `internal/builderengine/doc.go`, `internal/builderengine/plan.go`, `internal/builderengine/plan_test.go` — `Plan`/`PlanBatch` types and `ParsePlan` (overview frontmatter + Batch Index in Card 2, per-batch file frontmatter/Scope/Cards/verify: in Card 3).
- `internal/builderengine/testdata/{plan-valid,plan-unapproved,plan-broken-chain}/*.md` — hand-written fixtures (Card 4), with a round-trip test over `plan-valid`.
- `internal/builderengine/fingerprint.go`, `internal/builderengine/fingerprint_test.go` — `Fingerprint` (Card 5).
- `internal/builderengine/validate.go`, `internal/builderengine/validate_test.go` — `Validate`/`ValidateCaps`/`ValidationError` implementing all six plan-format checks (Card 6).

`go build ./...` and `go test ./internal/hubgeometry/... ./internal/builderengine/...` are green. Working tree has no uncommitted tracked changes (only the pre-existing untracked `_mill/briefs/implement-plan-model-r1.md` remains, out of scope).
