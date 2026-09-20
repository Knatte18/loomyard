MILL_REVIEW_BEGIN
# Review: Producer gates: mechanical gates before session release — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5, tool-use mode)
reviewed_file: plan/
date: 2026-09-20
```

## Findings

### [BLOCKING:design] Card 29 deletes cancellation_test.go on a false "no other subject" premise
**Location:** batch 5 / card 29
**Issue:** Card 29 says the file "goes with the producers" because it "constructs those two producers, and it has no other subject." Read against `internal/loomshed/cancellation_test.go`, `TestCancellation_RealProducersReturnErrorNotStuck` actually constructs five producers — `NewDiscussionValidate`, `NewPlanValidate`, `NewBatchifier`, `NewWebsterProducer`, and `NewLoomPreflight` — and its own file doc comment says exactly this. Three of those five (`Batchifier`, `Webster`, `LoomPreflight`) are untouched by this task and survive it; deleting the file outright silently drops the only regression test proving their real `Call` wiring returns an error rather than `Stuck`/`Done` under an already-cancelled context (`ctx_test.go` only unit-tests the shared `entryErr`/`cancelErr` helpers in isolation, not through any producer's `Call`).
**Fix:** Retain a reduced version of the test (or move it) asserting cancellation behaviour for `Batchifier`, `Webster`, and `LoomPreflight`, and only drop the two rows/producers actually being removed.

### [BLOCKING:scope] Card 29's moved-helper requirement omits its own source files from Context
**Location:** batch 5 / card 29
**Issue:** The card requires moving `formatDiscussionFindings`, `formatPlanFindings`, and `hasBlockingFinding` into `gates.go`, "each keeping its existing doc comment verbatim." Verified against source: `formatDiscussionFindings` lives in `internal/loomshed/discussionvalidate.go` (line 73) and `formatPlanFindings`/`hasBlockingFinding` live in `internal/loomshed/planvalidate.go`. Neither file is listed in the card's `Context:` or `Edits:` — both appear only under this same card's own `Deletes:`, which the Context-completeness rule does not treat as "implicitly read." The implementer has no told path to the exact verbatim text to preserve.
**Fix:** Add `internal/loomshed/discussionvalidate.go` and `internal/loomshed/planvalidate.go` to card 29's `Context:` list.

## Verdict

REQUEST_CHANGES
Card 29's cancellation-test deletion drops real coverage for three surviving producers on a factually wrong premise.
MILL_REVIEW_END
