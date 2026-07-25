MILL_REVIEW_BEGIN
# Review: webster: rewrite for flat card list — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-25
```

## Findings

### [NIT] `findingsEnvelope`'s `"card"` finding key is never asserted by a test
**Location:** `internal/webstercli/validate.go:35-47`, `internal/webstercli/cli_test.go:220-256`
**Issue:** Card 42 required a test asserting the pinned `validate` finding-entry JSON contract (`card` key from `f.Card`, replacing the old `batch`/`f.Batch`). `TestValidateCmd_ValidPlan` only exercises the zero-findings ok-envelope (`"cards":1`); `TestValidateCmd_MissingPlan` exercises a `ParsePlan` failure, not a `Validate`-findings failure. No test in `internal/webstercli` drives a syntactically-valid-but-check-failing plan through `validateCmd` to assert the `findings` array's `check`/`card`/`detail` shape, so `findingsEnvelope`'s own code path (correct on inspection) is untested.
**Fix:** Add a `webstercli` test that seeds a plan tripping one `planparser.Validate` check (e.g. a missing field) and asserts the emitted `findings` entries carry `"card"` (not `"batch"`).

### [NIT] `bisect`'s empty-`shas` case returns a hard error, not the "graceful degrade" the plan describes
**Location:** `internal/websterengine/integration.go:142-148`
**Issue:** Card 37 says "Guard the bisect so an empty `shas` slice or a single-SHA plan degrades gracefully (report the sole/HEAD card)." The single-SHA case degrades gracefully (returns index 0, no error); the empty-slice case instead returns `fmt.Errorf("webster: bisect: no card SHAs recorded to search")`. In the current wiring this path is effectively unreachable (`Run` already refuses a zero-batch plan up front, and every terminal batch's `CardSHAs` is populated by `RecordBatch`/`RecoverAwait` before it can reach `accumulatedCardSHAs`), so this is low-risk, but it is a literal deviation from the card's stated behavior for that one input shape.
**Fix:** Either update the card-37 prose to reflect the hard-error choice for the genuinely-empty case, or make `bisect` degrade the same way (report index 0 / "unknown") for consistency with the single-SHA case.

## Verdict

APPROVE
Thorough, correct, plan-aligned implementation across all 10 batches; only two low-severity test/prose gaps found.
MILL_REVIEW_END
