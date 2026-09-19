MILL_REVIEW_BEGIN
# Review: reed: per-hub daemon reaps orphaned sessions

```yaml
duration_s: 123.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Reap blocks the 5s discovery loop for up to 20s
**Section:** §Decisions `ordering-within-a-cycle`, `reap-kills-the-session-and-its-pane-subtrees`
**Issue:** `reapPaneChildren(pids, reapExitTimeout)` waits `reapExitTimeout` = 15s then `forceKillExitGrace` = 5s (`internal/reedengine/lifecycle.go:1029-1035,1054`), and the stated cycle ordering runs it inline in `runWatchdogLoop`'s body — so one orphan stalls the loop ~20s, several stall it longer, and the discussion never states whether the reap runs inline or off-loop.
**Fix:** Decide inline-vs-goroutine explicitly and state the consequences it accepts: tick coalescing against the 5s ticker, whether the "~15s" three-cycle figure survives, whether `idleCycles` accounting is distorted by a blocked cycle, and how `ctx.Done()` stays responsive during a reap.

### [BLOCKING:design] ReapSession's split point is unnamed, so the untagged test may be unbuildable
**Section:** §Decisions `ReapSession-is-a-second-engine-less-exported-function`; §Testing `internal/reedengine`
**Issue:** The testing plan drives "`ReapSession`'s tmux half" through `TmuxCmd.execHook` with no live server, but `New` (`internal/reedengine/lock.go:42-48`) builds `tmux: NewTmuxCmd(cfg.Tmux, geom.SocketKey)` with no hook, and the pane-reap half (`descendantClosurePIDs`, `reapPaneChildren`) hangs off `*Engine`, not `TmuxCmd` — the discussion says only "the same two-function shape" without saying what the inner function takes or where the tmux half ends and the real-process half begins.
**Issue (2):** If the inner function spans the process reap, the untagged test reaches real `waitProcessExit`/`proc.KillPID` and collides with the Test Tier Purity Invariant it cites.
**Fix:** Name the inner function's parameter (a `TmuxCmd`, or an `*Engine` whose `tmux` a same-package test can stub) and state which of the two halves the untagged test covers and which is integration-tagged.

### [NIT:consistency] `--shell` required contradicts the advertised foreground-diagnosis path
**Section:** §Decisions `the-daemon-is-told-its-shell`; §Documentation
**Issue:** `watchdogCmd`'s own `Long` (`internal/reedcli/watchdog.go:252-266`) advertises running the daemon by hand as "a real diagnosis path" and shows an example with only `--hub-path`/`--tmux`; a hard non-empty `--shell` pre-flight invalidates that example, and the Documentation section lists only `reed-header-selvage.md` and `roadmap.md`.
**Fix:** Add the `Long` help text and its example to the documentation inventory, and say whether the hand-run path is expected to supply a shell path on every GOOS including Linux, where `proctree_linux.go` never reads `cfg.Shell`.

### [NIT:scope] Injectable loop timings are required by Testing but absent from Scope
**Section:** §Scope In; §Testing (integration/smoke)
**Issue:** "the loop's timings must be injectable so no test waits on the production 5s × 3" requires changing `runWatchdogLoop`'s signature or the `watchdogHubDiscoveryCycle`/`watchdogHubIdleCycles` consts (`internal/reedcli/watchdog.go:44,51,173`), which the In-list never names as work.
**Fix:** Add the timing-injection seam to Scope In, and state its shape (extra parameter vs. a struct) so it does not get invented at code time.

### [NIT:design] "Any session on the socket is a lyx session" is slightly stronger than true
**Section:** §Decisions `reap-any-live-session-whose-join-is-gone`
**Issue:** `ServerName` (`internal/reedengine/server.go:55-61`) only makes the socket *named* by lyx; nothing prevents an operator's own `tmux -L lyx-<base>-<hash> new-session -s scratch`, which the rule would reap ~15s later.
**Fix:** Restate the rationale as "every session lyx creates on this socket is worktree-named, and a hand-made session on a lyx-owned socket is accepted collateral", so the premise matches what the code guarantees.

## Verdict

REQUEST_CHANGES
Two blockers: in-cycle reap blocking is undecided, and ReapSession's test seam is underspecified.
MILL_REVIEW_END
