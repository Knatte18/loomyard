All counts verified: `render_test.go`=20, `store_test.go`=19, `bench_git_test.go` carries `//go:build integration`. The discussion's verified-counts are accurate.

I have enough to render a verdict. My one substantive concern is the `os.Chdir` parallelisation feasibility gap in `cli_test.go`, which directly contradicts the discussion's "parallelise cli_test.go" claim.

MILL_REVIEW_BEGIN
# Review: Optimise and slim the rest of the test suite

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-22
```

## Findings

### [GAP] cli_test.go uses os.Chdir — not parallelisable
**Section:** `ide-scope` / Technical context (`cli_test.go has no t.Setenv → parallelisable`)
**Issue:** All four `cli_test.go` funcs call `os.Chdir(gitRepo)` with `defer os.Chdir(oldCwd)` (`cli_test.go:56-58,79-81,101-103,123-125`); process-global cwd mutation is incompatible with `t.Parallel()`, so the discussion's "parallelise cli_test.go" plan is infeasible as stated — the binding blocker is `os.Chdir`, which the discussion never mentions (it only cites `t.Setenv` as the serial trigger).
**Fix:** Either keep `cli_test.go` serial, or require the lyxtest migration to drop `os.Chdir` (pass the repo dir into `RunCLI` instead) before claiming parallelism; state this explicitly.

### [NOTE] menu_test git-worktree ops survive the lyxtest migration
**Section:** `ide-scope` (`newTestGitRepoWithWorktrees ≈ CopyPaired`)
**Issue:** `CopyPaired` yields independent sibling repos (hub/bare/weft-prime/weft-bare), not `git worktree`-linked children; `menu_test.go` creates real linked worktrees via `git worktree add`/`remove` in the *test bodies* (`:99-103,156-159,204-216`), so migrating only the `git init` base still leaves per-test git spawns in Tier 2.
**Fix:** Note that the migration removes only the fixture-build spawn; the in-body `git worktree add/remove` spawns remain (acceptable in Tier 2), and confirm `CopyPaired` fit covers only the base repo.

### [NOTE] CopyHostHub is a hub+empty-bare, not a clone-with-history
**Section:** `board-fixtures` (`TestPull/TestCommitPush ... matches CopyHostHub`)
**Issue:** `CopyHostHub` returns a working `Hub` whose origin points at an *empty, never-pushed* bare (`lyxtest.go:108,466-495`); `TestPull` needs an upstream that already has a commit to pull, so `CopyHostHub` does not fit `TestPull` without an extra push (CopyWeft fits better, modulo the master/main caveat already flagged).
**Fix:** When resolving the reuse-vs-new-fixture decider, note `CopyHostHub`'s bare is empty so `TestPull` cannot reuse it directly.

## Verdict

GAPS_FOUND
cli_test.go's os.Chdir blocks the claimed parallelisation; clarify before plan writing.
MILL_REVIEW_END