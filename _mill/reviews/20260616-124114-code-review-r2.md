MILL_REVIEW_BEGIN
# Review: Rename mhgo to Loomyard (lyx) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-16
```

## Findings

### [BLOCKING] Residual `mhgo` in docs/benchmarks/board-performance.md

**Location:** `docs/benchmarks/board-performance.md:125-126`
**Issue:** Two `mhgo` prose references survive the batch 2 sweep: "Every `mhgo` invocation is a fresh process. Measured startup on this machine (50× a no-op `mhgo`, by launcher):" — Card 9 explicitly lists this file and the batch 2 reviewer check (`grep -rI mhgo .`) must return nothing outside `_mill/`; these lines fail it.
**Fix:** Replace both occurrences with `lyx` in code font: "Every `lyx` invocation …" and "50× a no-op `lyx`, by launcher".

### [NIT] Stale brand string in integration-gated `.go` files

**Location:** `internal/board/boardtest/integration_test.go:42` and `internal/board/boardtest/bench_git_test.go:39`
**Issue:** Git `user.name` literals read `"MHGo Integration Test"` and `"MHGo Bench"` — mixed-case old brand; not caught by the batch 1 `grep -rI mhgo` check (case-sensitive) but inconsistent with the prose-voice decision.
**Fix:** Change to `"Loomyard Integration Test"` and `"Loomyard Bench"` respectively.

## Verdict

REQUEST_CHANGES
One blocking gap (2 residual `mhgo` lines in board-performance.md) plus one minor brand-string NIT in integration-gated test files.
MILL_REVIEW_END
