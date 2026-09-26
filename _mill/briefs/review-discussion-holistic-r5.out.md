MILL_REVIEW_BEGIN
# Review: Loom persists done only after post-run friction reflection

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [BLOCKING:scope] batten llm-child teardown race left undispositioned
**Section:** Problem / Out / "Reflection only when armed for run" **Issue:** batten can drive a child with `child_driver: llm` (`internal/battencli/arm.go`, `wire.go`'s `ChildDriver`), so that child is step-driven, its row skips, `done` persists, and Worktree-Teardown deletes `.lyx/loom/friction/` and ends the reed session while ly-drive's § Autonomous driver is still listing those notes and writing its stop report — the same done-before-bookkeeping class the Problem attributes to "every batten-driven task", yet the discussion says the notes are "left for ly-drive's operator-gated flow" without noting that batten removes them. **Fix:** State explicitly that batten llm-driven children stay exposed (pre-existing, out of scope, named follow-up such as `shed-llm-driver` #28), or bring them in scope with a mechanism.

### [NIT:design] Blocking friction-lock wait is not cancellable
**Section:** "The row waits for a concurrent reflection instead of skipping it" **Issue:** `lock.AcquireWriteLock` is a bare `flock.Lock()` with no context, and the proposed `ReflectFriction func() string` closure takes no `ctx`, so Ctrl-C or a parent deadline cannot interrupt the row while it waits; the wait is bounded only by the holder's `friction_timeout_min`. **Fix:** Acknowledge the non-cancellable wait as accepted, or pass `ctx` through the closure and use a context-aware acquire.

### [NIT:consistency] ly-drive "near a hundred steps" worst case also moves
**Section:** "Stale row-count and last-row text" **Issue:** Only the 35→36 three-rounds arithmetic is re-derived, but ly-drive's worst-case figure ("near a hundred", which also justifies the autonomous cap of 120) also gains a step. **Fix:** Note that the worst-case text was checked and needs no change, or include it in the sweep.

## Verdict

REQUEST_CHANGES
The batten llm-child path needs a stated disposition before plan writing.
MILL_REVIEW_END
