MILL_REVIEW_BEGIN
# Review: webster: stop re-rendering already-inherited context into fork prompts — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic), operating as "Sonnet 5" per system context
reviewed_file: plan/
date: 2026-08-02
```

## Findings

### [NIT] RecoverDeps.Plan becomes fully dead code but is left in place
**Location:** Batch 2, Card 3 (`internal/websterengine/recoverbatch.go`)
**Issue:** `RecoverDeps.Plan`'s only reader in `recoverbatch.go` is the `RenderForkPrompt(deps.Plan, ...)` call at line 216, which Card 3 rewrites to `RenderRecoveryPrompt(batch, ...)` (no plan arg). After this change `deps.Plan` is never read anywhere in `recoverbatch.go`, yet the card only asks to reword the field's doc comment (near lines 54–55), not to remove the field or note why it stays — inconsistent with the same card's own diligence removing `noSharedDecisions`/`batchHasMove`/`renderCard` etc. "no remaining user after the integration change."
**Fix:** Either add a bullet pruning `RecoverDeps.Plan` (and its two populate sites in `internal/webstercli/recoverbatch.go` and `recoverbatch_test.go`, both outside this batch's Edits — would need adding), or have the doc-comment rewrite bullet explicitly say the field is kept only for the `RecoverDeps`/`BeginDeps` shape symmetry despite being otherwise unused in this file.

### [NIT] doc.go bullet's parenthetical misdescribes the RenderForkPrompt signature change
**Location:** Batch 2, Card 3, `internal/websterengine/doc.go` bullet
**Issue:** The bullet says to fix doc.go's "Plan consumption" sentence "since the thinned `RenderForkPrompt` no longer reads them (`plan` is now an unused parameter)" — but the same card's `render.go` bullet explicitly instructs to DROP the `plan *planparser.Plan` parameter entirely, not leave it present-but-unused. The two phrasings conflict.
**Fix:** Reword the doc.go bullet's parenthetical to "(`plan` is no longer a parameter of `RenderForkPrompt` at all)" to match the render.go bullet's actual instruction.

## Verdict

APPROVE
Plan is well-grounded against source (line refs, signatures, invariants all verified); only two non-blocking cleanup nits remain.
MILL_REVIEW_END
