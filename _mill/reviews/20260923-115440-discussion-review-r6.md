MILL_REVIEW_BEGIN
# Review: Shuttle guarantees a started run is past its startup gates

```yaml
duration_s: 63.1
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewed_file: _mill/discussion.md
date: 2026-09-23
```

## Findings

### [BLOCKING:design] Failed teardown RemoveStrand is an unstated residual
**Section:** Decisions "Not-ready start" (steps 3 and 6), "Loom's llm arm", "Webster's persist-before-block windows" **Issue:** Step 3 makes `reed.RemoveStrand` failure non-fatal (Warn only), yet step 6's `ErrNotStarted` message unconditionally states "that the strand was removed", and the loom/webster decisions claim a not-ready start "no longer leaks" and that mechanism failure is "the one remaining residual" — a failed removal leaves a live, never-ready strand that loom's next `start` resolves as `driverStrandLive` (`resolveDriverStrandAction`, `bootstrap.go`) and skips readiness, and that webster's reclaim cannot see. **Fix:** Specify the error wording when removal fails (e.g. names the removal failure instead of claiming removal), and either list removal failure beside the mechanism-failure residual in loom's `Long` help, the `start.go` comment and webster's two doc comments, or state why it is excluded.

## Verdict

REQUEST_CHANGES
Teardown's non-fatal RemoveStrand failure contradicts the error message and the "one remaining residual" claims.
MILL_REVIEW_END
