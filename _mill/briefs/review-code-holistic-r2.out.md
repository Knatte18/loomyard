MILL_REVIEW_BEGIN
# Review: Replace reed's header pane with a status-line and Selvage — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-19
```

## Findings

### [BLOCKING:scope] card 41's watchdog daemon integration suite is missing several mandated assertions
**Location:** `internal/reedcli/watchdog_integration_test.go`
**Issue:** Card 41 enumerates specific live-behaviour assertions this file must carry. Four are absent: (1) resizing one worktree's window re-applies only that worktree's layout, (2) the daemon writes its diagnostics into `fabricengine.HubLogsDir(hub)` (the only observable proof the durable sink was pointed before stderr was discarded), (3) a `down` immediately followed by an `up` does not kill the daemon, and (4) re-entry re-reads config — `down` a worktree, flip its `watchdog:` key, `up` it again, and confirm the new value takes effect without restarting the daemon. `grep`-confirmed: no test in the file references `HubLogsDir`, no test re-ups a down'd worktree, and no test asserts per-worktree resize isolation.
**Fix:** Add the four missing assertions to `internal/reedcli/watchdog_integration_test.go` (or state explicitly why each is out of scope, with the plan updated to match).

### [NIT:consistency] roadmap's positional claim is now false after the item moved to Done
**Location:** `manifest/roadmap.md:32`
**Issue:** The `reed: cross-worktree columns` Someday entry's parenthetical reads "...and now Selvage — claimed by the now-Done header-replacement item above". The referenced item is in `## Done` (line 111), which is textually *below* `## Someday` (line 32) in the file, not above it — card 46 explicitly required fixing the stale positional claim ("the item is no longer Planned and is no longer above it"), and the fix only swapped "Planned" for "now-Done" while leaving the now-incorrect "above".
**Fix:** Drop the positional word, e.g. "...claimed by the shipped header-replacement work".

## Verdict

REQUEST_CHANGES
Otherwise faithful, thorough implementation across all seven batches; one real test-coverage gap in card 41's daemon integration suite.
MILL_REVIEW_END
