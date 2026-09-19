MILL_REVIEW_BEGIN
# Review: reed: born-as-strand for loom start's operator attach — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-19
```

## Findings

### [NIT:consistency] start_watchdog_test.go doesn't drive the real call site
**Location:** `internal/loomcli/start_watchdog_test.go:42-45`
**Issue:** The test substitutes a stub into `c.spawnWatchdog` but never invokes `startCmd()`'s `RunE`; it instead re-issues the identical call expression `c.spawnWatchdog(c.location.HubPath, c.reed.TmuxPath(), c.suppressWatchdogSpawn)` by hand, so a future edit to `start.go`'s actual call site (wrong field, dropped call) would not be caught here.
**Fix:** None required now — the gap is honestly disclosed in the file's own doc comment and the argument-plumbing is exercised end-to-end by the real subprocess in `smoke_operatorstrand_test.go`'s case 5 (`TestSmokeWatchdog_NoAttachStillSpawnsTheDaemon`) and by `TestSmokeOperatorStrand_*`; no action needed unless a future round wants a stronger offline seam.

## Verdict

APPROVE
Every card in both batches is faithfully realized, cross-batch contracts hold, and constraints (Told-Geometry, CLI/Cobra, Test Tier Purity) are respected.
MILL_REVIEW_END
