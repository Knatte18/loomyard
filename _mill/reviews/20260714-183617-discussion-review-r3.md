MILL_REVIEW_BEGIN
# Review: Investigate the unexplained lyx mux server crash

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-14
```

## Findings

### [GAP] Shared per-hub server vs per-worktree debug_log/logs
**Section:** Decisions › debug-log-config-key & server-log-under-dotlyx-logs
**Issue:** The socket is per-hub (`socketName(e.layout.Hub)`, `lock.go:56`) so one server serves all worktrees, but `debug_log`, `cmd.Dir`, and the `.lyx/logs/` target are all per-worktree (`DotLyxDir()` = `<Cwd>/.lyx`); only the boot-winning worktree's values take effect — a sibling with `debug_log:0` winning the boot captures nothing, and the log lands in that worktree's `.lyx/logs/`, not where the operator armed it.
**Fix:** State how the operator ensures the boot-winning worktree carries `debug_log>0`, and where forensics should look for a per-hub server's log across a multi-worktree Hub.

### [NOTE] Contract-doc prose left undecided
**Section:** Technical context › Multiplexer contract
**Issue:** Whether `internal/muxengine/doc.go`'s documented subcommand surface needs a sentence about the `-v`/`-vv` server flag is deferred to "plan time" with no default chosen.
**Fix:** Pick a default (e.g. add one sentence since the flag changes the documented spawn invocation) so the plan writer is not left to interpret.

### [NOTE] resume-hint may only fire for persisted-strand scenarios
**Section:** Decisions › resume-hint-in-requireSessionLocked
**Issue:** `cli_integration_test.go:83,112` assert the exact old string; the enriched message only changes when ≥1 strand is persisted, so whether those cases keep the old text or need updating depends on their fixture state.
**Fix:** Note that each old-string assertion must be checked for whether its scenario persists strands, updating only those that do.

## Verdict

GAPS_FOUND
Shared per-hub server undercuts the per-worktree debug-log/log placement; resolve before planning.
MILL_REVIEW_END
