MILL_REVIEW_BEGIN
# Review: reed: attach's layout computation scales header pane height with terminal height

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:decision] Roadmap watchdog-daemon item has no disposition
**Section:** Constraints ("`manifest/roadmap.md` must not move: this is a bugfix")
**Issue:** `manifest/roadmap.md:32` ("reed: watchdog daemon") explicitly owns "reconciles session geometry after a live terminal resize" and records that a prior task's discussion *rejected* a one-off tmux `set-hook` reaction in favour of a shared daemon paying for hook infrastructure once — this task now ships exactly a `set-hook` resize reaction and never mentions that item.
**Fix:** State a disposition for the roadmap item (narrow it, note the resize half as delivered by this fix, or record why the daemon rationale survives), and reconcile that with the "roadmap must not move" claim.

### [BLOCKING:consistency] "Rebuilds the array" is falsified by the install guards
**Section:** `hook-failure-is-non-fatal-everywhere` ("every one of those routes back through `applyLayoutLocked`, which rebuilds the array")
**Issue:** Per `hook-install-points-are-named-statements`, both of `applyLayoutLocked`'s guards return *before* the install statement (`apply.go:141-147`), so a pane disappearance that leaves `len(live) < 2` or `!anyPlacedStrand` routes through the function without rebuilding; `state.go:151-156` documents a live, reachable state (state file cleared, panes still running) where `anyPlacedStrand` is permanently false, leaving a stale pin array installed indefinitely with no stated removal path.
**Fix:** Either weaken the self-healing rationale to the cases it actually covers and state the disposition for a stale array under the guard-skip states (clear it, leave it, or install a `set-hook -u` on those paths), or move the clear step ahead of the guards as an explicit decision.

### [NIT:scope] Doc-correction target under-specified
**Section:** Scope, `threshold-attribution-is-corrected-in-writing`
**Issue:** `doc.go:312-327`'s chained-attach bullet claims only that the layout "lands verbatim with no rescale", which this discussion's own live measurements confirm as true — so which sentences are wrong is left to the plan writer to guess.
**Fix:** Name the specific claim(s) to correct (the missing round-robin-on-resize note, not the verbatim-landing note).

## Verdict

REQUEST_CHANGES
Roadmap overlap undecided; the fire-time self-healing premise is contradicted by the install guards.
MILL_REVIEW_END
