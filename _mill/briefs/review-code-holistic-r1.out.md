I have all the evidence needed. Here is my complete review.

MILL_REVIEW_BEGIN
# Review: Build perch - the review gate loop — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-08
```

## Findings

### [BLOCKING] TriagePath is never populated on a round record
**Location:** `internal/perchengine/run.go:23-32` (roundOutcome struct), `run.go:265-279` (runRound's OutcomeAsking branch), `run.go:112` (`TriagePath: outcome.TriagePath`)
**Issue:** `roundOutcome.TriagePath` is declared and consumed at `run.go:112` into `roundRecord.TriagePath`, but `runRound` never sets it in any of its `return roundOutcome{...}` statements — including the done-outcome return after a triage `RETRY` re-attempt. `state.go`'s `roundRecord.TriagePath` field and Card 6/12's plan text ("recording TriagePath") were built specifically to persist this, yet it is always empty even when `runTriage` wrote a real file to `paths.Triage` on disk. `run_test.go`'s "asking with triage RETRY" test never asserts on this field, so the gap passed the suite silently.
**Fix:** Thread the attempt-1 `paths.Triage` value through to the returned `roundOutcome` (e.g. accumulate it across the retry loop) and set `record.TriagePath` from it, so `state.json` records that a triage call actually ran.

### [MEDIUM] Wasteful/inconsistent triage spawn on a second consecutive asking outcome
**Location:** `internal/perchengine/run.go:265-279`
**Issue:** When `attempt == 2` and the outcome is again `OutcomeAsking`, the code still calls `runTriage` (a real shuttle/LLM spawn) before checking `attempt == 2` and erroring out — the round fails regardless of the triage verdict (unless `GIVE_UP`, which then produces a different error message than the generic "failed twice" one a same-situation died/timeout round gets). The discussion's phrasing ("RETRY → retry once, then the second-consecutive rule applies") implies the second consecutive non-done outcome should hit the generic error directly, without a second wasted triage call.
**Fix:** Check `attempt == 2` before invoking `runTriage` on the asking branch (mirroring the died/timeout branch), so a second consecutive asking outcome fails the same generic "failed twice" way without an extra spawn.

## Verdict

REQUEST_CHANGES
TriagePath silently never persists to state.json despite an explicit plan requirement and dedicated schema field.
MILL_REVIEW_END
