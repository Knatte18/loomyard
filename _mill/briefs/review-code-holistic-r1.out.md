MILL_REVIEW_BEGIN
# Review: Shuttle guarantees a started run is past its startup gates — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-23
```

## Findings

### [NIT:consistency] Stale doc comment: mechanism failure no longer "reaches Wait"
**Location:** `internal/shuttleengine/../shuttlecli/cli_test.go:371-373` (`preparingEngine` doc comment)
**Issue:** The comment says a `Runner` built over `preparingEngine`/`statusFailingReed` "reaches Wait — where statusFailingReed's error drives the mechanism-failure path." Verified against `internal/shuttleengine/run.go`'s `RunGated` (`run, result, err := r.start(spec, gate); ...; if err != nil { return result, err }; return run.Wait()`) and `wait.go`'s `awaitStartup`/`checkLivenessTick`: `statusFailingReed.Status()` fails on every call, so the mechanism failure is now raised inside `awaitStartup` (called from `start()`, before any handle exists) and `RunGated` returns at the `err != nil` branch — `run.Wait()` is never invoked in this scenario. The test's assertions still pass (the identity-carrying error shape is preserved), but the comment's causal claim is now false.
**Fix:** Reword the comment to say the mechanism failure now surfaces from the startup step inside `start()`, not from `Wait()`. (This file is Context-only for card 4 per the plan, so no functional edit was required — the plan's own batch-tests note already acknowledges the mechanism moved; only the comment text lags.)

## Verdict

APPROVE
Implementation matches the plan and discussion precisely across both batches; only a stale test comment in an unedited context file.
MILL_REVIEW_END
