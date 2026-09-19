MILL_REVIEW_BEGIN
# Review: reed: per-hub daemon reaps orphaned sessions

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (self-assessed; brief says "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Descendant closure runs after kill-session
**Section:** `ReapSession-is-a-second-engine-less-exported-function` (the named split point)
**Issue:** The split puts `kill-session` inside `reapSessionTmux` and `descendantClosurePIDs(roots)` after it, but `Engine.Down` — cited as "the canonical ordering to copy" — calls `paneProcessTreePIDsLocked()` (`lifecycle.go:1025`, doc comment: "Must run before kill-session while panes exist"), which is `descendantClosurePIDs(sessionReapRootsLocked())`, i.e. the closure is computed *before* the kill; so the claim that "capture-roots-and-kill is pure tmux round trips" is false and the reap would expand the tree after tmux has already SIGHUP'd/reparented pane children, losing exactly the detached agent descendants the task exists to kill.
**Fix:** Move the split so the process-tree closure is captured before `kill-session` (the tmux half returns roots *and* the engine expands them pre-kill), or state why post-kill expansion is sufficient on both `/proc` and Win32_Process.

### [BLOCKING:design] Untagged tests listed have no seam to run against
**Section:** Testing — `internal/reedcli`, pure untagged
**Issue:** Three listed untagged cases (completion-channel drain "leaves the set on the next tick", in-flight name excluded from the appeared set while tmux still lists it, the hub-probe cycle over "a listing that names several live sessions") are properties of `runWatchdogLoop`'s own tick body, not of the single `planSessionReap`-shaped pure seam Scope names; the discussion itself argues (in the `validateWatchdogFlags` bullet) that an untagged test cannot drive `runWatchdogLoop` because its first tick reaches `ListSessions` → `exec.Command`, and `internal/reedcli/watchdog_test.go` today has no lister injection.
**Fix:** Either widen the named pure seam to cover one whole cycle's bookkeeping (live names, hub-live bool, in-flight set, counters in → reaps/appeared/departed out), or move those three cases to the tagged tier.

### [NIT:consistency] watchdogDefaultTiming sources a constant that does not exist
**Section:** Scope In (timing-injection seam) vs `three-consecutive-cycles-before-a-reap`
**Issue:** Scope says `watchdogDefaultTiming()` sources "the existing package constants and nothing else", but `OrphanGoneCycles` maps to `watchdogOrphanGoneCycles`, which is not declared anywhere in `internal/reedcli` today; Scope In never lists it as a new constant.
**Fix:** Name `watchdogOrphanGoneCycles = 3` as a new constant in Scope In and reword the "existing constants" phrasing.

## Verdict

REQUEST_CHANGES
Reap ordering copies Down incorrectly, and three untagged tests have no seam.
MILL_REVIEW_END
