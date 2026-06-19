MILL_REVIEW_BEGIN
# Review: weft engine: paths geometry, paired worktrees, lyx weft — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-19
```

## Findings

### [NIT] seedGitExclude uses substring match, not line-exact match
**Location:** `internal/worktree/weft.go:178`
**Issue:** `strings.Contains(contentStr, "_lyx")` matches any line containing `_lyx` as a substring (e.g. a comment `# exclude _lyx dirs` would prevent the real `_lyx` entry from being appended); plan specifies "a line equal to `_lyx`".
**Fix:** Split on newline and check for an exact line match: `for _, line := range strings.Split(contentStr, "\n") { if strings.TrimSpace(line) == "_lyx" { return nil } }`.

### [NIT] Integration test short-circuits detached spawnPush
**Location:** `internal/weft/weft_integration_test.go:71-110`
**Issue:** `TestSyncIntegration_EventuallyPushed` calls `Push()` synchronously instead of `spawnPush()` + poll loop.
**Fix:** Acceptable trade-off given `os.Executable()` returns the test binary; the inline comment documents the limitation — no action required beyond noting the gap in detached-spawn coverage.

### [NIT] weft_test.go missing LyxDir vs HostLyxLinkHere divergence assertion
**Location:** `internal/paths/weft_test.go:142-150`
**Issue:** Plan (Card 2) requires "an assertion that `HostLyxLinkHere()` differs from `LyxDir()` when `Cwd != WorktreeRoot`"; the test only verifies the formula, not the explicit inequality.
**Fix:** Add a case where `Cwd` is a true subdirectory, then assert `HostLyxLinkHere() != LyxDir()` in the subdir case.

### [NIT] docs/overview.md geometry list omits three pre-existing methods
**Location:** `docs/overview.md:64`
**Issue:** `MenuLauncherPath()`, `LauncherSpawnRel(slug)`, and `MenuLauncherRel()` are implemented in `paths.go` but absent from the `Layout` method enumeration (pre-existing gap).
**Fix:** Append the three methods to the list (low priority).

## Verdict

APPROVE
All plan cards realised; path invariant, shared decisions, and cross-batch contracts fully satisfied; four minor nits, none blocking.
MILL_REVIEW_END
