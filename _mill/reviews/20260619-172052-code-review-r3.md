MILL_REVIEW_BEGIN
# Review: weft engine: paths geometry, paired worktrees, lyx weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-19
```

## Findings

### [BLOCKING] git exit codes silently swallowed in teardown helpers

**Location:** `internal/worktree/weft.go:263-284`, `internal/worktree/add.go:225-246`

**Issue:** `removeWeftWorktree` (all three `git.RunGit` calls) and `rollbackAdd`'s host git teardown steps (lines 225–246) capture `err` but never `exitCode`. Per `git.go:29`, `RunGit` clears `err` on non-zero exit (returning `err=nil, exitCode=N`), so every git failure in these best-effort sections is silently swallowed — violating the `git-via-RunGit-with-cwd` shared decision ("check `exitCode`, not `err`") and the plan's "errors collected, not masked" requirement.

**Fix:** Capture and check the `exitCode` return from each `git.RunGit` call in `removeWeftWorktree` and in `rollbackAdd`'s git steps (lines 225–246); set `firstErr` when `exitCode != 0 && err == nil`.

---

### [BLOCKING] `TestWeftRollbackOnPostHostCreateFailure` does not exercise the rollback path

**Location:** `internal/worktree/weft_test.go:331-385`

**Issue:** The test pre-creates `WeftWorktreePath(slug)` to cause a failure, but `add.go` step 6 pre-checks `os.Stat(weftTarget)` before creating the host worktree and returns early — so `rollbackAdd` is never called. The rollback logic itself has no test coverage for the post-host-create / post-weft-create failure path.

**Fix:** Trigger the failure after host worktree creation and after weft worktree creation to exercise `rollbackAdd`; for example, pre-create a real file at `HostLyxLink(slug)` so `seedLyxJunction` fails after both worktrees are created, then assert both are cleaned up.

---

### [NIT] `os.Getwd()` / `os.Chdir` used directly in `TestRemoveSubpathJunction`

**Location:** `internal/worktree/remove_test.go:244-246`

**Issue:** The test calls `os.Getwd()` + `os.Chdir` manually instead of `t.Chdir(subpathDir)` (Go 1.21+), which auto-restores cwd on test completion.

**Fix:** Replace the `oldCwd`/defer pattern with a single `t.Chdir(subpathDir)` call.

---

### [NIT] `weft_integration_test.go` poll loop uses `rev-list main` on bare, not the commit hash

**Location:** `internal/weft/weft_integration_test.go:97`

**Issue:** The poll assertion runs `git -C <bare> rev-list main` to detect arrival of the spawned push, but exits as soon as ANY commit appears on `main`, not necessarily the specific `"sync-test"` commit.

**Fix:** After `Commit`, record `HEAD` SHA from `weftRepo`; poll until `git -C <bare> cat-file -e <sha>` exits 0.

## Verdict

REQUEST_CHANGES
Two blocking issues: swallowed git exit codes in all teardown paths, and rollback coverage gap.
MILL_REVIEW_END
