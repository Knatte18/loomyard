MILL_REVIEW_BEGIN
# Review: Investigate the unexplained lyx mux server crash — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-14
```

## Findings

### [NIT] Smoke test hardcodes the `.lyx`/`logs` path instead of calling HubLogsDir()
**Location:** `internal/muxcli/smoke_debuglog_test.go:51`
**Issue:** `logsDir := filepath.Join(filepath.Dir(fixture.Hub), ".lyx", "logs")` duplicates the exact path segments `HubLogsDir()` (`internal/hubgeometry/hubgeometry.go:321-323`) already encodes, rather than building a `hubgeometry.Layout{Hub: ...}` and calling the real accessor — precisely the drift risk the Hub Geometry Invariant's own rationale cites ("a config-layout migration once broke a hardcoded test fixture"). Mechanically this is not a guard violation (`.lyx` isn't in `TestEnforcement_GeometryLiterals`'s token list, and `*_test.go` files are excluded from that AST walk regardless), and the plan (card 6) explicitly specified this literal construction, so it is not a fault of execution against the approved plan.
**Fix:** Not required this round; consider routing through `hubgeometry.Layout.HubLogsDir()` in a future pass so the test can't silently drift from the accessor it's meant to mirror.

## Verdict

APPROVE
All four batches faithfully implement the plan; cross-batch contracts, shared decisions, and constraints hold with only one cosmetic NIT.
MILL_REVIEW_END
