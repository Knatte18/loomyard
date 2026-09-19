MILL_REVIEW_BEGIN
# Review: reed: per-hub daemon reaps orphaned sessions

```yaml
duration_s: 114.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Hub-level outage reaps every session at once
**Section:** `gone-predicate-mirrors-validateToldWorktreeRootLive`, `three-consecutive-cycles-before-a-reap`
**Issue:** The predicate guards per-path transients (EACCES/EIO) but not the hub itself vanishing — an unmounted or renamed hub makes `os.Stat(filepath.Join(hub, name))` return `fs.ErrNotExist` for *every* name, while the socket (keyed by `ServerName(hub)`, not by hub liveness) still lists them, so ~15s later the daemon destroys every live agent session on the hub.
**Fix:** State a disposition — either probe the hub directory itself before the reap pass and make a non-live hub a non-affirmative cycle, or record explicitly that a whole-hub disappearance is accepted as reap-worthy and why.

### [NIT:consistency] Empty-`--shell` e2e test has no stated mechanism
**Demoted-from:** BLOCKING
**Section:** Testing, `internal/reedcli` integration/smoke list
**Issue:** "the daemon starts and reaps an orphan with `--shell \"\"`" needs the real `watchdogCmd`/flag path, but the same list mandates "every test here drives `runWatchdogLoop` through the `watchdogTiming` parameter" (which bypasses the flag entirely), and starting a real daemon process would re-exec `os.Executable()` under `go test`, barred by Live-Substrate Spawn Observability.
**Fix:** Name how this assertion is actually driven, or drop it and let `validateWatchdogFlags` plus a direct `runWatchdogLoop`-with-empty-shell case carry the "degrade, never refuse" proof.

### [NIT:scope] Shell path's route to `ReapSession` is unstated
**Demoted-from:** BLOCKING
**Section:** Scope In, `the-daemon-is-told-its-shell`
**Issue:** `ReapSession(tmuxPath, shellPath, socketKey, sessionName)` needs a shell value inside the loop, but Scope lists only a `watchdogTiming` parameter added to `runWatchdogLoop(ctx, hub, tmuxPath)` (verified signature, `watchdog.go:173`) and never says the loop gains a shell parameter or how `watchdogCmd`'s flag reaches the reap goroutine.
**Fix:** State the plumbing explicitly — a `shellPath` parameter on `runWatchdogLoop` or a carrying struct — so the flag-to-reap path is decided rather than improvised.

### [NIT:consistency] Technical Context misattributes two reuse pieces
**Section:** Technical context, "The teardown pieces to reuse, all in `internal/reedengine/lifecycle.go`"
**Issue:** `sessionReapRoots` is in `strand.go:584`, and `descendantClosurePIDs` is in `proctree_linux.go:21`/`proctree_windows.go:24`; only `reapPaneChildren` (`lifecycle.go:1054`) and `paneProcessTreePIDsLocked` live where claimed.
**Fix:** Correct the file attributions so the plan writer opens the right files.

### [NIT:consistency] Cycle ordering omits carried-over in-flight names
**Section:** `ordering-within-a-cycle` vs `the-reap-runs-off-loop-not-inline`
**Issue:** The ordering decision removes only *this cycle's selected* names from the working live list before `planSessionDiff`, while the off-loop decision requires in-flight names from earlier cycles to be excluded from the appeared set too — the two readings differ for a reap still running on the next tick.
**Fix:** Restate the ordering step as "remove selected *and* in-flight names from the working live list".

## Verdict

REQUEST_CHANGES
Whole-hub outage guard, empty-shell test mechanism, and shell plumbing all need a stated decision.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
