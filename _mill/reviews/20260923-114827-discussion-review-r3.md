MILL_REVIEW_BEGIN
# Review: Shuttle guarantees a started run is past its startup gates

```yaml
duration_s: 66.1
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewed_file: _mill/discussion.md
date: 2026-09-23
```

## Findings

### [BLOCKING:design] Startup step vs run.deadline undecided
**Section:** Decisions / "Run deadline anchoring" **Issue:** The moved `AwaitStarted` loop (`wait.go:381–429`) never checks `run.deadline`, while `Wait` checks it every tick (`wait.go:306`), so a `spec.Timeout` shorter than `startup_timeout_s` (reachable via `lyx shuttle run --timeout`) now ends as a not-ready `OutcomeDied`/`ErrNotStarted` after the startup window, not `OutcomeTimeout` at `spec.Timeout`; the claim that "spec.Timeout keeps covering startup plus work" is therefore only half true. **Fix:** Decide whether the startup step also honours `run.deadline` (and with which outcome, through `classifyDeadlineExpiry`), or state the overrun as accepted, and add a test for either choice.

### [NIT:consistency] RunGated rationale filed under the wrong decision
**Section:** Decisions / "Startup step lives in a tripwire-scanned file" **Issue:** That decision carries two `Rationale` lines, and the second (the burler retry ladder) plus "Rejected: making `RunGated` return the error" belong to "RunGated preserves its OutcomeDied contract", which has neither rationale nor rejected alternatives. **Fix:** Move the burler rationale and its rejected alternative under the RunGated decision.

### [NIT:design] RunGated identity on startup mechanism failure
**Section:** Decisions / "RunGated preserves its OutcomeDied contract" and "Startup mechanism failure" **Issue:** `RunGated` returns `Result{}` on a `StartGated` error (`run.go:394–397`), so a startup mechanism failure loses the `identity()` fields that `Wait`'s own mechanism-failure exits return today; the discussion specifies the private `start`'s return only for the not-ready case. **Fix:** State whether `start` also hands back `identity()` for the mechanism-failure case, so that `RunGated` keeps returning identity fields with that error as today.

## Verdict

REQUEST_CHANGES
The interaction between the startup step and the run deadline is undecided and changes observable outcomes.
MILL_REVIEW_END
