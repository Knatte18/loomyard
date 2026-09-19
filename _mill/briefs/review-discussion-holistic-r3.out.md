MILL_REVIEW_BEGIN
# Review: reed: per-hub daemon reaps orphaned sessions

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Required --shell can kill the daemon outright
**Section:** `### the-daemon-is-told-its-shell`
**Issue:** `reedengine.Config` has no per-key defaulting (`config.go` unmarshals straight into the struct) and nothing anywhere validates `cfg.Shell`, so a `reed.yaml` that exists but omits `shell:` yields `Shell == ""`; `ensureWatchdogSpawned` would then spawn `--shell ""`, the new mandatory pre-flight rejects it, and the hub loses the whole watchdog daemon (resize self-heal included) silently, since the spawn is best-effort and the child's error lands only in the hub durable log.
**Fix:** state the disposition for an empty `cfg.Shell` at spawn time — spawn-without-`--shell`, validate/default `Shell` in `LoadConfig`, or make the pre-flight non-fatal — and name it as a decision rather than leaving it to the implementer.

### [BLOCKING:design] In-flight set's synchronization is unnamed
**Section:** `### the-reap-runs-off-loop-not-inline`
**Issue:** the in-flight set is written by the loop at dispatch and cleared "until its goroutine returns" — i.e. from the reap goroutine — but no synchronization mechanism is chosen (mutex-guarded map vs a completion channel the loop drains at the top of each tick); a plain map write from the goroutine is a data race the integration tests specified here would trip under `-race`, and the two options differ observably in removal timing (immediate vs tick granularity).
**Fix:** name the mechanism and the removal point in the decision, as `reapSessionTmux`'s signature and `watchdogTiming`'s shape already are.

### [NIT:scope] Idle/errored cycles vs the consecutive-cycle rule
**Section:** `### three-consecutive-cycles-before-a-reap` / Gotchas
**Issue:** a cycle where `ListSessions` errors or returns empty `continue`s before the reap pass, so a name's gone-counter is neither advanced, reset, nor pruned; "three **consecutive** cycles" therefore spans a tmux outage rather than requiring contiguity, and counter-map pruning is skipped on those cycles.
**Fix:** state explicitly that a non-affirmative listing leaves counters untouched (and why that is acceptable) rather than leaving contiguity to inference.

### [NIT:scope] Reap only runs when some daemon is running
**Section:** `## Problem` / `## Scope`
**Issue:** the daemon exists only after an `up`/`resume`/`attach` spawns it (`ensureWatchdogSpawned`); on a hub whose worktrees are all gone, or after the daemon itself is killed, orphan sessions persist until an operator runs an lyx command on that hub — an unstated boundary of the "never-entered orphan" coverage the design leans on.
**Fix:** record the precondition (a daemon must be running or re-spawned) as an accepted limitation in Scope Out or the Problem section.

### [NIT:consistency] Dropping the design doc's forward-reference empties a section
**Section:** `## Documentation`
**Issue:** `manifest/designs/reed-header-selvage.md:90` is one of two bullets under `## Related`; the instruction to "drop the trailing Related line" is fine, but the discussion does not say whether the remaining bullet (line 89) stays, so the section's fate reads as ambiguous.
**Fix:** say the other `Related` bullet is retained and only line 90 goes.

## Verdict

REQUEST_CHANGES
Two unresolved decisions: empty `cfg.Shell` at spawn, and the in-flight set's synchronization.
MILL_REVIEW_END
