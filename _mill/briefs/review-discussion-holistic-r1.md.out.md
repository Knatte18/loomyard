MILL_REVIEW_BEGIN
# Review: Extract internal/proc (cross-OS windowless + detached spawn)

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-23
```

## Findings

### [NOTE] Detach must not clobber cmd.Env in muxpoc
**Section:** Call-site changes — internal/muxpoc/up.go
**Issue:** `up.go` sets `cmd.Env = clean` before `spawnServer(cmd)`; verified `proc.Detach` only assigns `cmd.SysProcAttr`, so Env is preserved — but the discussion never states this invariant for the inline replacement.
**Fix:** Add one line noting `Detach` touches only `SysProcAttr`, leaving any prior `cmd.Env` intact.

### [NOTE] Doc-comment provenance pointers go stale on file deletion
**Section:** Scope / Call-site changes — internal/vscode/launch_windows.go
**Issue:** `launch_windows.go` comments reference "from git_windows.go/spawn_windows.go" which this task deletes; the discussion does not mention updating those now-dangling comment references.
**Fix:** Note that stale provenance comments pointing at deleted files should be updated to reference `internal/proc`.

## Verdict
APPROVE
Scope, decisions, inventory, and call-sites all verified against source; no missing information blocks plan writing.
MILL_REVIEW_END
