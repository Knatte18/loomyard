MILL_REVIEW_BEGIN
# Review: reed: watchdog daemon

```yaml
duration_s: 138.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:design] Poll fallback's box observation is unspecified
**Section:** `hook-availability-decides-poll-fallback` vs `self-apply-does-not-retrigger-and-is-guarded-anyway`
**Issue:** The guard decision states "the box the guard compares is the one `ReapplyLayout` **returns**, not one the watcher queries for itself", but poll mode is defined as the watcher observing the box itself each cycle and comparing to the last applied one — and it never says what the poll's own query is (`liveBoxLocked` documents "assumes the op lock is already held" and the watcher holds no lock between re-applies), nor what a degraded query means there, where the `cfg.Width/Height` fallback yields exactly the spurious-re-apply-forever loop `BoxIsLive` exists to prevent on the signal path.
**Fix:** State the poll cycle's box-query seam (which method, under what lock) and that a non-live box in poll mode is likewise not an observation — no apply, no last-applied update.

### [BLOCKING:design] `watchdog: off` does not stop a running watcher
**Section:** `watchdog-single-toggle-no-tunables`
**Issue:** All three consumers are process-start or op-time reads — `reedcli`'s `PersistentPreRunE` resolves config once per process — so flipping to `off` leaves the already-running header-pane loop running; unsetting the hook silences signal mode but a poll-mode watcher keeps re-applying indefinitely, which defeats the stated purpose ("an operator who hits a watchdog bug needs a way to turn it off").
**Fix:** State the take-effect boundary explicitly (as `mouse`/`debug_log` do in the template comments) — e.g. `off` applies on the next header-pane rebuild (`down` + `up`) — or decide that the loop re-reads the key, and cover it in the config-key test.

### [NIT:decision] Loop constants left as ranges or unstated
**Section:** `debounce-in-the-watcher`, `watch-loop-failures-are-never-fatal`, poll fallback
**Issue:** The debounce quiet period is "150–250ms", the retry cap is "N, on the order of 3", and the signal-tick and poll-cycle intervals carry no magnitude at all — yet the tier-1 synthetic-clock tests assert against exactly these values.
**Fix:** Fix each constant to a single number (or state that the plan picks them and the tests read the constants rather than literals).

### [NIT:design] `ReapplyResult` on the guard-skip paths
**Section:** `reapply-layout-is-a-new-public-engine-op`
**Issue:** `applyLayoutLocked` returns before computing any box when `len(live) < 2` or `!anyPlacedStrand(...)`, so `ReapplyResult{Applied, Box, BoxIsLive}` has no defined value on the two guard paths the discussion most wants inherited; separately, propagating the box out requires a signature change on `applyLayoutLocked`, not only the named `liveBoxLocked` sibling.
**Fix:** State that a guard skip yields `Applied=false, BoxIsLive=false` (no last-applied update) and name the apply-side value propagation alongside the `liveBoxLocked` change.

## Verdict

REQUEST_CHANGES
Poll-mode box semantics and the kill-switch's take-effect boundary are unspecified.
MILL_REVIEW_END
