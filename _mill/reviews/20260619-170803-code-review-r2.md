Confirmed: TestSyncIntegration_EventuallyPushed does not call spawnPush. It calls Push directly. The detached spawn code path has no integration test coverage.

MILL_REVIEW_BEGIN
# Review: weft engine: paths geometry, paired worktrees, lyx weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-19
```

## Findings

### [BLOCKING] Integration test exercises Push, not spawnPush

**Location:** `internal/weft/weft_integration_test.go:71-108`
**Issue:** `TestSyncIntegration_EventuallyPushed` calls `Commit` then `Push` (synchronous) — not `Commit` then `spawnPush`. The detached spawn code path in `spawn_windows.go`/`spawn_other.go` has zero integration coverage; the poll loop is also semantically wrong for a synchronous push. Card 12 explicitly required `Commit + spawnPush` with a bounded poll, and the `WEFT_SKIP_PUSH` guard must not be set in this test.
**Fix:** Replace the `Push(weftRepo)` call with `spawnPush(weftRepo)` and ensure `WEFT_SKIP_PUSH` is not set; the existing poll loop is then semantically correct.

### [BLOCKING] docs/overview.md module dispatch snippet is stale

**Location:** `docs/overview.md:152-165`
**Issue:** The `## Module dispatch` section's Go code block still shows the old switch with only `board`, `ide`, `muxpoc`, and `worktree` cases — `case "weft"` is absent. The actual `main.go` has it, so the doc is wrong.
**Fix:** Add `case "weft": return weft.RunCLI(out, moduleArgs)` to the switch block in the doc.

### [NIT] seedLyxJunction idempotent check calls EvalSymlinks on non-existent target

**Location:** `internal/worktree/weft.go:94-99`
**Issue:** When a link exists but the weft `_lyx` directory does not yet exist, `EvalSymlinks(target)` fails, the idempotent path is not taken, and the code falls through to the mode-bit check, which can return a misleading "predates weft" error.
**Fix:** Check `errTarget != nil` separately and return a clearer error before the mode-bit fallback.

### [NIT] Remove dirty-gate weft check has inverted logic on error

**Location:** `internal/worktree/remove.go:76-87`
**Issue:** The first `if err != nil` branch contains `if exitCode == 0 && strings.TrimSpace(stdout) != ""` which will never execute because `err != nil` when the outer condition is true — the dirty-gate silently passes for an unreachable weft worktree on error.
**Fix:** Remove the dead inner `if` inside the `err != nil` branch; an error querying weft status when `!force` should either skip the check (log a warning) or reject.

## Verdict

REQUEST_CHANGES
Two blocking issues: integration test doesn't cover the detached spawn path; overview.md dispatch snippet is stale.
MILL_REVIEW_END
