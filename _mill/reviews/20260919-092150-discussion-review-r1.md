# Review: reed: per-hub daemon reaps orphaned sessions

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [NIT:consistency] "three spawn sites" overstates where `--shell` is added
**Section:** Scope (In) / Technical context — "the daemon's own `--shell` flag" bullet and "the spawn sites"
**Issue:** The discussion says `--shell` is "told by `ensureWatchdogSpawned`'s three spawn sites exactly as `--tmux` already is," which reads as three separate command-construction edits. In fact `ensureWatchdogSpawned` has three *callers* (`attach.go`, `resume.go`, `up.go`) but one `exec.Command` construction (`spawnwatchdog.go:46`) that already builds `--tmux` once for all three — confirmed by grep, only one `"--tmux"` string literal exists in `internal/reedcli`. The Technical context section already says the right thing ("This is where `--shell` gets added," singular), so this is a self-resolving wording nit, not a scope gap.
**Suggested fix:** Reword to "told by `ensureWatchdogSpawned`'s single spawn construction, reached from all three call sites, exactly as `--tmux` already is" — or just drop "three spawn sites" and rely on the Technical context section's existing precise statement.

## Verdict

APPROVE
Exceptionally well-grounded discussion — every load-bearing technical claim I spot-checked (`Down()`'s ordering at `lifecycle.go:864`, `validateToldWorktreeRootLive`/`errWorktreeRootGone`, `ListSessions`/`exactSessionTarget`, watchdog loop constants and functions, `descendantClosurePIDs`'s Windows shell usage, `reapExitTimeout`/`forceKillExitGrace`, and that `ReapSession` genuinely does not exist yet) matched the actual source exactly, decisions all carry rationale plus rejected alternatives, and scope/testing/constraints coverage is thorough.
