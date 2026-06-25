Confirmed. Card 12 in batch 4 edits the integration-tagged `configcli_integration_test.go`, but batch 4's verify (`go test -tags integration ./internal/warp/ ./internal/initcli/`) does not include `./internal/configcli/`, so that edit's compilation/correctness is unverified until batch 9. That's a BLOCKING reachability gap.

MILL_REVIEW_BEGIN
# Review: Introduce warp: the host↔weft-coordinated git module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-25
```

## Findings

### [BLOCKING] Batch-4 verify can't reach card-12's configcli edit
**Location:** Batch 4 (verify) / Card 12
**Issue:** Card 12 edits the `//go:build integration`-tagged `internal/configcli/configcli_integration_test.go` (rewiring `warp.Add` to explicit `WireJunctions` now that Add is dormant), but batch 4's `verify: go test -tags integration ./internal/warp/ ./internal/initcli/` omits `./internal/configcli/`; `go build ./...` never compiles test files, so a broken edit there goes undetected until batch 9.
**Fix:** Add `./internal/configcli/` to batch 4's `verify` test list.

### [NIT] Card 20 under-specifies the weft junction-only tests
**Location:** Batch 6 / Card 20
**Issue:** `weft/status_test.go` contains a table-driven test and a second test whose *only* assertions are `junction_ok`/`junction_reason` and whose calls pass the to-be-removed `hostLink`/`weftLyxDir` params; "drop the junction assertions" alone would leave empty tests and stale `Status(weftRepo, hostLink, weftLyxDir, …)` call signatures that won't compile after the param removal.
**Fix:** State that the junction-dedicated tests are removed (not just their assertions) and the remaining content tests' `Status(...)` calls are updated to the trimmed signature.

## Verdict

REQUEST_CHANGES
One reachability gap (batch-4 verify misses card-12's configcli edit) plus a minor specificity nit.
MILL_REVIEW_END
