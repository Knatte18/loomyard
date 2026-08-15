MILL_REVIEW_BEGIN
# Review: Shed: outer phase-FSM skeleton — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-15
```

## Findings

### [NIT:consistency] Vacuous substring assertion for the empty-outcome case
**Location:** `internal/shedengine/run_routing_test.go:348` (`TestRun_UnrecognisedOutcome`)
**Issue:** For the `Outcome("")` sub-case, `strings.Contains(err.Error(), string(tt.outcome))` checks `Contains(s, "")`, which is true for any `s` — the assertion never actually verifies the error names the offending empty value, even though the card requires the message to "name … the offending value."
**Fix:** For the empty-string sub-case, assert on a more specific marker (e.g. the literal `""` produced by `%q` formatting of the empty string) so a regression that dropped the value from the message would still be caught.

## Verdict

APPROVE
Implementation, tests, and docs closely track every batch's cards and all shared decisions; only one non-blocking test-quality nit found.
MILL_REVIEW_END
