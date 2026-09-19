MILL_REVIEW_BEGIN
# Review: reed: extract Selvage-pane lifecycle — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-19
```

## Findings

### [BLOCKING:consistency] Stale `selvageAlive` references survive the rename, scattered across 4 files
**Location:** `internal/reedengine/reconcile.go:194`, `internal/reedengine/doc.go:183`, `internal/reedengine/reconcile_test.go:106,111,125-126,142,181,191`, `internal/reedengine/spawn_test.go:20`
**Issue:** Card 3 deletes the `selvageAlive` local and replaces it with `policy.authorizesReap()`, but ten comments across two production files and two test files still name `selvageAlive` as if it exists — `reconcile.go`'s own defer comment ("the selvageAlive disjunct above") is now a stale reference to a removed identifier in a file this very card edited, and `doc.go`'s "Load-bearing behavioral assumptions" bullet still states the gate as `anyBoundPresent || selvageAlive` even though the real expression is `anyBoundPresent || policy.authorizesReap()`.
**Fix:** Sweep every `selvageAlive` comment reference (reconcile.go, doc.go, reconcile_test.go, spawn_test.go) to name `policy.authorizesReap()` instead, in the same batch that renamed the identifier it describes.

## Missing context
(not applicable — verdict is REQUEST_CHANGES)

## Verdict

REQUEST_CHANGES
Ten stale `selvageAlive` comment references across 4 files (2 production) must be swept to match the card-3 rename.
MILL_REVIEW_END
