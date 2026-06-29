MILL_REVIEW_BEGIN
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-29
```

## Findings

### [NIT] Stale "internal/warp" comments in weftengine/status.go
**Location:** `internal/weftengine/status.go:3,25` and `internal/weftengine/status_test.go:6`
**Issue:** Lines 3 and 25 of status.go reference the deleted `internal/warp` package by name (e.g. "moved to internal/warp", "owned by internal/warp"). status_test.go line 6 similarly cites `internal/warp/status_test.go`. Card 28's comment sweep listed specific files but did not include these two.
**Fix:** Replace `internal/warp` with `internal/warpengine` in all three comment lines.

### [NIT] CLI benchmark in boardengine/boardtest imports boardcli
**Location:** `internal/boardengine/boardtest/bench_test.go:19`
**Issue:** BenchmarkUpsert/Get/List call `boardcli.RunCLI` from within the engine test subpackage. This is a test-file-only import so it creates no compilation cycle and does not affect loom's ability to consume boardengine, but it conceptually mixes CLI-level concerns into the engine's benchmark suite.
**Fix:** Consider moving the CLI benchmarks to `internal/boardcli` in a future pass; no action needed to preserve plan's "behaviour-preserving sweep" intent.

## Verdict

APPROVE
Implementation is complete and correct across all eight batches with two comment-accuracy NITs.
MILL_REVIEW_END