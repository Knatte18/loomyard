Now I have confirmed `assertBranch` is defined only in `status_test.go` and never called in any file in the package. Let me produce the review.

MILL_REVIEW_BEGIN
# Review: Speed up internal/warp integration tests — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-26
```

## Findings

### [NIT] Dead-code helper `assertBranch` in status_test.go
**Location:** `C:\Code\loomyard\wts\warp-test-speedup\internal\warp\status_test.go:313-322`
**Issue:** `assertBranch` is defined but never called in any file across the entire `internal/warp` package (confirmed by grep). It was not mentioned in any plan card, suggesting it is a leftover from the test consolidation rather than a deliberately planted helper. It compiles without error (Go permits unused functions) but is dead code.
**Fix:** Delete the `assertBranch` function and, if `gitexec` then becomes unreferenced within `status_test.go`, remove its import as well.

## Verdict

APPROVE
Implementation is complete and correct; all 21 cards realized with no BLOCKING issues.
MILL_REVIEW_END
