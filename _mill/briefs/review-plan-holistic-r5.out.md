MILL_REVIEW_BEGIN
# Review: reed: per-hub daemon reaps orphaned sessions — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-09-19
```

## Findings

### [NIT:consistency] watchdog.go's file header still claims reedengine gains "exactly one" function
**Location:** batch 1, card 2 (and batch 3, cards 9-11, which also edit `internal/reedcli/watchdog.go`) **Issue:** `internal/reedcli/watchdog.go`'s own file-header doc comment (lines 1-10, verified in source) states "internal/reedengine gains exactly one new engine-less function (ListSessions)" — this becomes false once `ReapSession` ships as reedengine's second engine-less function, called from this same file's `dispatchReap`. Card 2 explicitly requires fixing the parallel stale claim in `ListSessions`' own doc comment in `overlay.go` ("that claim becomes false the moment ReapSession ships") but no card updates this mirrored claim in `watchdog.go`'s package header. **Fix:** add one sentence to card 9, 10, or 11 (all of which already edit `watchdog.go`) updating the header to name both `ListSessions` and `ReapSession`, or reword away from "exactly one."

## Verdict
APPROVE
No BLOCKING findings; prior rounds' Told-Geometry, Context-completeness, and rationale-accuracy issues are all fixed in this revision.
MILL_REVIEW_END
