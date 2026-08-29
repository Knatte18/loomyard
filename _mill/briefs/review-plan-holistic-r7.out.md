MILL_REVIEW_BEGIN
# Review: reed: pane reap isn't applied consistently across up/add's mutating paths — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: plan/
date: 2026-08-29
```

## Findings

### [BLOCKING:consistency] Card 3 contradicts itself on the reap log's id values
**Location:** batch 1 / card 3
**Issue:** The card first requires "two further keys carrying `plan.deadPanesToKill` and `plan.untrackedPanesToKill`" — the *scheduled* lists — then requires accumulating "the killed ids into two locals as the loops progress" and emitting from a `defer`; on the partial-kill path (`return killed, fmt.Errorf("kill pane %s: %w", ...)` from inside the loop, reconcile.go:178) those two sources diverge, and the scheduled lists would name panes that were never destroyed.
**Fix:** State unambiguously that the two keys carry the accumulated actually-killed locals, not `plan.*ToKill`, since the card's own third test case ("naming the pane it did destroy") passes under either reading and so cannot discriminate.

### [NIT:scope] reconcile.go's file-header comment is unassigned
**Location:** batch 3 / cards 10 and 12
**Issue:** `internal/reedengine/reconcile.go:1-5` says `planReconcile` "decides which strand pane bindings to clear and which dead panes to kill" — card 1 splits that return into a four-field struct with untracked kills as a first-class category and card 2 makes the untracked reap routine, yet no card names this comment and neither of card 12's grep terms (`adopt`, `untracked reap|bound present pane|reap.*does not fire`) reaches it.
**Fix:** Name it in card 10 alongside the other reconcile.go comment, or record it as a deliberate out in card 12's enumeration.

## Verdict

REQUEST_CHANGES
One card-internal contradiction about the reap log's values; everything else verified against source.
MILL_REVIEW_END
