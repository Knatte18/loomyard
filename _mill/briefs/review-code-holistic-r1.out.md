The board integration tests don't test the detached spawn path either, so the plan's description "as board's tests do" was aspirational. The implementation's approach of not polling is consistent with how the board module handles this — the sync is fire-and-forget and verification is left out. This is acceptable as a pragmatic test implementation choice (not blocking), though it deviates from card 12's explicit requirement.

Now I have all the data I need. Let me summarize findings:

1. **BLOCKING**: `internal/weft/spawn_windows.go` is missing the `//go:build windows` build tag. Both `spawn_windows.go` and `spawn_other.go` will compile on all platforms, producing a duplicate `spawnPush` symbol and compilation failure on non-Windows.

2. **NIT (docs)**: `docs/overview.md` line 64 lists `WeftWorktreePath()`, `WeftLyxDirFor()`, and `HostLyxLink()` without the `slug` parameter — these methods require a `slug string` argument.

3. **NIT (test)**: `weft_integration_test.go` — `TestSyncIntegration_EventuallyPushed` does not poll the bare remote to confirm the commit landed.

4. **NIT (test)**: `weft_test.go:364` — `hostBranch := "prefix-" + slug` constructs a branch name with a hardcoded `"prefix-"`, and the variable is unused.

MILL_REVIEW_BEGIN
# Review: weft engine: paths geometry, paired worktrees, lyx weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-19
```

## Findings

### [BLOCKING] spawn_windows.go missing build tag

**Location:** `internal/weft/spawn_windows.go:1-7`
**Issue:** The file has no `//go:build windows` constraint. `spawn_other.go` has `//go:build !windows`, so on non-Windows both files compile and the `spawnPush` symbol is defined twice, producing a link-time "redeclared in this block" compile error.
**Fix:** Add `//go:build windows` (and a blank line) as the first two lines of `internal/weft/spawn_windows.go`, matching the pattern in `internal/worktree/junction_windows.go`.

### [NIT] docs/overview.md omits slug param from three method signatures

**Location:** `docs/overview.md:64`
**Issue:** The geometry method list writes `WeftWorktreePath()`, `WeftLyxDirFor()`, and `HostLyxLink()` without the `slug` parameter.
**Fix:** Change those three entries to `WeftWorktreePath(slug)`, `WeftLyxDirFor(slug)`, `HostLyxLink(slug)`.

### [NIT] Integration test does not verify commit reached bare remote

**Location:** `internal/weft/weft_integration_test.go:69-94`
**Issue:** `TestSyncIntegration_EventuallyPushed` calls `spawnPush` and verifies only that it does not return an error. Card 12 required polling the bare repo.
**Fix:** Add a bounded retry loop that calls `git rev-list` on the bare repo.

### [NIT] Stale/dead variable in rollback test

**Location:** `internal/worktree/weft_test.go:364-365`
**Issue:** `hostBranch := "prefix-" + slug` constructs a branch name with a literal `"prefix-"` that does not exist; the variable is never used.
**Fix:** Remove the `hostBranch` variable and the unused `RunGit` call at lines 364-365.

## Verdict

REQUEST_CHANGES
One blocking compile error (missing Windows build tag on `spawn_windows.go`) must be fixed before merge.
MILL_REVIEW_END
