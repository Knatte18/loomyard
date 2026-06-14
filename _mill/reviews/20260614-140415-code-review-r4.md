MILL_REVIEW_BEGIN
# Review: Extend worktree module: portals and launchers — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-14
```

## Findings

### [NIT] Missing bare-repo rejection test in paths package
**Location:** `internal/paths/worktreelist_test.go`
**Issue:** Card 1 required a bare-repo rejection test case ("exercising single worktree, multiple worktrees with Main only on the first, and the bare-repo rejection") but only `SingleWorktree` and `TwoWorktrees` cases are present; the `parseWorktreePorcelain` bare-rejection branch has no direct coverage in this package.
**Fix:** Add a table entry that `git init --bare`s a temp dir, calls `paths.List`, and asserts the returned error contains "bare".

### [NIT] Dead `os.IsNotExist` branch in removeLaunchers
**Location:** `internal/worktree/launchers.go:92-95`
**Issue:** `os.RemoveAll` documents that it returns nil when the path does not exist, so the `os.IsNotExist(err)` guard is unreachable dead code.
**Fix:** Remove the `os.IsNotExist` branch; the outer `if err != nil` block is sufficient.

### [NIT] `worktree/cli.go` resolves Layout before LoadConfig can gate non-git dirs
**Location:** `internal/worktree/cli.go:50-53`
**Issue:** `paths.Resolve(cwd)` runs (and fails with ErrNotAGitRepo) before `LoadConfig` — this is the intended behaviour but is a change from the pre-migration flow where a missing `_mhgo/` was the first error; any callers expecting a "not initialized" error from outside a git repo now get a different message.
**Fix:** No code change needed (behaviour is by design); add a comment explaining that paths failure precedes config failure intentionally.

## Verdict

APPROVE
All 7 batches are fully implemented and correct; constraint invariant is solid; findings are non-blocking nits only.
MILL_REVIEW_END
