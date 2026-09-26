MILL_REVIEW_BEGIN
# Review: Loom persists done only after post-run friction reflection

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-26
```

## Findings

### [NIT:consistency] Recipe header's escalate-set ignores new row
**Section:** Technical context (recipe bullet) / Stale row-count and last-row text
**Issue:** `loom-recipe.yaml`'s header lists the rows with no `on_stuck` and counts them into an "eight rows in total escalate" set; `Friction-Reflect` also has no `on_stuck` but never returns `Stuck`, and the discussion names no disposition for it in that set (the grep sweep targets counts and `Finalize`-as-last phrasing, not this set).
**Fix:** State that the header gains a sentence placing `Friction-Reflect` outside the escalate set because it always returns `Done`, so the "eight" stays true.

### [NIT:scope] Lock-wait test has no named home
**Section:** Testing, "Row waits on a held reflection lock"
**Issue:** The test needs loomcli's real `reflectFriction` (the lock lives there) and also asserts `state: done` persistence (a shed), but unlike the other bullets it names neither package nor fixture, so a planner could shrink it to a lock-only `reflectFriction` test that never checks persistence.
**Fix:** Name `internal/loomcli` and a minimal `shedengine.Shed` over the new loomshed producer wrapping the receiver's closure, with deps that make `frictionengine.Reflect` fail validation fast after the lock is released.

## Verdict

APPROVE
Engine, lock, arming-verb and envelope claims check out against source; only two non-blocking clarifications remain.
MILL_REVIEW_END
