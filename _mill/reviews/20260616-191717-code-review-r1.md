I have all the information I need. Let me now compose the review.

MILL_REVIEW_BEGIN
# Review: Extract internal/fsx and build internal/state — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-16
```

## Findings

### [BLOCKING] integration_test.go calls deleted board.AtomicWrite
**Location:** `C:\Code\loomyard\wts\extract-internal-fsx\internal\board\boardtest\integration_test.go:65`
**Issue:** Batch 2 (card 3) deleted `board.AtomicWrite` from `internal/board/git.go`, but `boardtest/integration_test.go` still calls `board.AtomicWrite(repoPath, filename, content)`. This file is not in any batch's `Context:`, `Edits:`, or `Creates:` lists and was not updated. The `//go:build integration` tag shields it from the `go test ./internal/board/...` verify step, but the `integration` build will fail with an undefined symbol.
**Fix:** Add `internal/board/boardtest/integration_test.go` to batch 2's scope; replace `board.AtomicWrite(repoPath, filename, content)` with `fsx.AtomicWrite(repoPath, filename, content)` (adding the `fsx` import).

### [NIT] state_test.go imports encoding/json beyond plan's stated stdlib subset
**Location:** `C:\Code\loomyard\wts\extract-internal-fsx\internal\state\state_test.go:4`
**Issue:** Card 9 specifies "Use stdlib `testing`/`os`/`path/filepath` only", but the test file also imports `encoding/json` for `TestJSONFormatting`. The test itself is useful and correct, but it was added outside the stated scope.
**Fix:** Either update card 9's requirements to permit `encoding/json`, or remove `TestJSONFormatting` if the constraint is intentional.

## Verdict

REQUEST_CHANGES
One blocking issue: deleted `board.AtomicWrite` is still called in the unmodified integration test.
MILL_REVIEW_END
