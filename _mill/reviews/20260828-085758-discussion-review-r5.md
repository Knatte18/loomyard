MILL_REVIEW_BEGIN
# Review: reed: watchdog daemon

```yaml
duration_s: 197.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (self-assessed)
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Loop's package home contradicts its seam access
**Section:** `watchdog-lives-in-the-header-pane-process` vs **Testing**
**Issue:** The loop is placed in `internal/reedcli/header.go`, but everything it must reach is unexported in `reedengine`: `e.tmux` and `TmuxCmd.execHook` (`overlay.go:37`), `stateDir()` (`lifecycle.go:33`), and the resolved config (`reedcli/cli.go:89` keeps `cfg` as a local, only `c.eng` survives); the tier-1 cases are specified as "drivable through `TmuxCmd.execHook`" and the tier-2 test reuses the pty harness inside package `reedengine` — none of which a `reedcli`-resident loop can do.
**Fix:** Decide where the loop body lives (an exported `reedengine` entry point that `header.go` calls, vs. `reedcli`) and enumerate the full exported seam set that follows; today only `ReapplyLayout` is named.

### [BLOCKING:design] Hook-availability probe has no op or lock discipline
**Section:** `hook-availability-decides-poll-fallback`
**Issue:** `reapply-layout-is-a-new-public-engine-op` commits to one new public method, and the design is emphatic that the watcher performs no unlocked tmux query (because `liveBoxLocked` assumes the lock is held), yet the `show-hooks`-class round trip runs at startup and again on every poll cycle with no stated owner, no stated lock (blocking / try-lock / none), and no stated relationship to `withOpLock`'s told-geometry validations and post-op compromise check.
**Fix:** Name the probe as its own engine op, state its lock discipline, and state what mode is selected when the lock is held rather than only when the round trip errors.

### [NIT:consistency] "No subprocess in steady state" is false in poll mode
**Demoted-from:** BLOCKING
**Section:** **Constraints** ("the watch loop spawns no OS process in steady state") and `hook-touches-a-signal-file` ("**zero** subprocesses")
**Issue:** Poll mode calls `ReapplyLayout` plus a hook re-probe once per 2s cycle, and every `TmuxCmd` call is a real `exec.Command` (`overlay.go:54`, `overlay.go:70`) — a permanent multi-spawn-per-cycle cost, i.e. more than the ~23ms `display-message` poll the design rejected on exactly that cost ground, and forever on the one platform (psmux/Windows) that lands in poll mode.
**Fix:** Scope the zero-subprocess claim to signal mode and state poll mode's accepted steady-state spawn cost explicitly, including how it squares with the geometry-poll rejection.

### [NIT:consistency] `watchdogOption` shape and `mouse.go` line cite
**Section:** `watchdog-single-toggle-no-tunables` / **Technical context**
**Issue:** The validator is specified as `watchdogOption(raw) (bool, error)` "mirroring `mouseOption` exactly", but `mouseOption` is `(string, error)` and lives at `mouse.go:15`, not `:14`.
**Fix:** State the intended return type deliberately (bool is fine) rather than claiming exact mirroring, and correct the citation.

## Verdict

REQUEST_CHANGES
Package home, probe-op locking, and the zero-spawn claim need settling before planning.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
