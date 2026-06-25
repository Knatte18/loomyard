So `gitclone_test.go` is untagged (runs in default `go test`), while `clone_integration_test.go` is `//go:build integration`. The plan moves both: card 5 moves `gitclone_test.go → clone_test.go` (untagged) and `clone_integration_test.go → clone_integration_test.go` (tagged, preserved).

So batch 2's `go test ./internal/warp/` runs the untagged unit tests but NOT the integration test. This is consistent with the existing repo split — the integration tests were never part of the default `verify` loop in this repo. The criterion is "BLOCKING if integration tests added but `verify:` doesn't run them." However, the repo's established convention is that integration tests run via a separate tier (`testtiming -full` / `-tags integration`), never in per-batch verify. The plan follows the existing repo convention.

This is a judgment call. The criterion exists to prevent dead integration tests. But here the repo has a deliberate two-tier model where integration tests are intentionally out of the default loop. New integration tests (cards 17, 22, 26, 30) carry load-bearing validation criteria (e.g., the card-30 hook-divergence assertion, card-17 rollback assertion). If never run in `verify`, those criteria aren't exercised by the batch gate. The plan does not add a `-tags integration` verify step anywhere, nor mention the integration tier will be run. I'll flag this as the primary finding — it's at least a significant gap worth raising, since several key behaviors (rollback, hook-on-drift) are ONLY covered by integration-tagged tests that no `verify:` runs.

I have enough to write the review.

MILL_REVIEW_BEGIN
# Review: Introduce warp: the host↔weft-coordinated git module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-25
```

## Findings

### [BLOCKING] Integration-tagged tests never run by any verify
**Location:** Batches 2,5,6,7,8 (cards 5,17,22,26,30)
**Issue:** Load-bearing scenarios are integration-tagged (`//go:build integration`, mirroring `clone_integration_test.go`) — host-rollback (card 17), hook-warns-on-drift (card 30), reconcile/prune/cleanup real-git paths (cards 22,26) — but every batch `verify:` is plain `go test ./internal/warp/`, which excludes tagged files; those criteria are never exercised by the batch gate.
**Fix:** Add a `-tags integration` invocation to the affected batches' `verify:` (e.g. `go test -tags integration ./internal/warp/`) so the rollback/hook/reconcile assertions actually run.

### [NIT] Card 20 edits Status signature but omits weft/cli.go from Edits
**Location:** Batch 6, card 20
**Issue:** Dropping `hostLink`/`weftLyxDir` from `weft.Status` breaks its sole production caller `internal/weft/cli.go:109`, which is listed only in `Context:`, not `Edits:`; the package won't compile until cli.go is changed.
**Fix:** Promote `internal/weft/cli.go` to card 20's `Edits:` (the card text already says "adjust the caller").

### [NIT] Card 13 stale note about drift_test.go
**Location:** Batch 4, card 13
**Issue:** Card states `drift_test.go` "is not in the overview All Files Touched list," but it is present in `00-overview.md` (`internal/warp/drift_test.go`).
**Fix:** Drop the inaccurate parenthetical.

## Verdict

REQUEST_CHANGES — integration-tagged behavior coverage is unreachable from any batch verify.
MILL_REVIEW_END
