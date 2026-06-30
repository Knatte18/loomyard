MILL_REVIEW_BEGIN
# Review: Fix lyx CLI defects + host-commit gap from the sandbox run

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\lyx-sandbox-fixes\_mill\discussion.md
date: 2026-06-30
```

## Findings

### [NOTE] hubgeometry line 93 err-path leak left unaddressed
**Section:** Decisions → "Raw git stderr leak fixed at the source"
**Issue:** The fix targets `hubgeometry.go:96` (`%w: %s` stderr, exitCode!=0 path), but line 93 `fmt.Errorf("%w: %v", ErrNotAGitRepo, err)` (the err!=nil exec-failure path) also interpolates a raw error and is silent in the discussion; the TDD `"fatal:"` substring assertion would not exercise it.
**Fix:** State whether line 93 is in or out of scope, and what string replaces the dropped stderr at line 96 (bare `ErrNotAGitRepo` = "not a git repository", verified self-describing).

### [NOTE] Existing status/prune/reconcile tests mask the ToSlash change
**Section:** Testing
**Issue:** status_test/reconcile_test/prune_test compare `filepath.Clean(jsonField) == filepath.Clean(expected)`; on Windows `Clean` re-normalizes forward slashes to backslash, so these tests neither break nor verify the #37 fix.
**Fix:** Confirm the new no-backslash assertions are added against the raw JSON field (not a Clean-wrapped value), so the separator guarantee is actually tested.

## Verdict

APPROVE
Scope, decisions, and call sites verified accurate against source; only two non-blocking clarifications.
MILL_REVIEW_END