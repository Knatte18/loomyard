MILL_REVIEW_BEGIN
# Review: Ensure weft branches are orphan branches — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-24
```

## Findings

### [NIT] Step 6b `err` return from RunGit is not checked
**Location:** `internal/worktree/add.go:135-138`
**Issue:** The `err` value from `git.RunGit(["rev-parse", "--abbrev-ref", "HEAD"], ...)` is assigned but never tested; a process-spawn failure (`err != nil`, `exitCode = -1`) triggers the `exitCode != 0` branch and returns the misleading "detached HEAD" message instead of the real execution error.
**Fix:** Add `if err != nil { return AddResult{}, fmt.Errorf("rev-parse abbrev-ref HEAD: %w", err) }` before the `exitCode`/stdout check; the detached-HEAD guard then becomes the `else` branch.

### [NIT] Unborn-branch signal path is untested
**Location:** `internal/worktree/add_test.go:115-124` (`DetachedHEAD` case)
**Issue:** The guard fires on two distinct signals — detached HEAD (exit 0, stdout `"HEAD"`) and unborn branch (non-zero exit) — but only the detached-HEAD signal is exercised by the test; the `exitCode != 0` branch relies on reasoning, not a concrete test case.
**Fix:** Add a second row to the `TestAdd` table that puts the host repo on an unborn branch (e.g., `git checkout --orphan unborn-branch` without committing) and asserts the same error substring; this directly validates the second guard path.

## Verdict

APPROVE
Implementation is complete and correct; two nits, neither blocking.
MILL_REVIEW_END