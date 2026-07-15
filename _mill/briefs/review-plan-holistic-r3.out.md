MILL_REVIEW_BEGIN
# Review: Reconsider whether lyx mux needs anchor:top at all — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-15
```

## Findings

### [NIT] add.go godoc still names --top-band-rows and top vocab
**Location:** Batch 1 / Card 6
**Issue:** The `addCmd` godoc comment (add.go lines 18-23) enumerates the `--top-band-rows` flag and "any value outside top|below-parent|hidden"; card 6 edits add.go but only conditions on the `Long` text (which never names `top`), so this comment survives stale — contradicting the "no dead surface survives" decision.
**Fix:** Card 6 should add: update the `addCmd` godoc comment to drop `--top-band-rows` and the `top` vocabulary.

### [NIT] Smoke test function name retains "TopBands"
**Location:** Batch 1 / Card 7
**Issue:** Card 7 rewrites the `--anchor top` bodies and line-91 comment but not the function name `TestSmokeTopBandsThenStackAddsKeepEverySessionPane`, leaving a stale top-band reference in the rewritten smoke case.
**Fix:** Card 7 should rename the function to reflect the below-parent multi-pane scenario it now exercises.

## Verdict

APPROVE — sound staging and DAG; two minor stale-doc/name sweeps to fold into cards 6 and 7.
MILL_REVIEW_END
