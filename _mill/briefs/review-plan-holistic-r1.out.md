MILL_REVIEW_BEGIN
# Review: webster: rewrite for flat card list — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-25
```

## Findings

### [BLOCKING] Stale verb Long help text not updated
**Location:** Batch 9, cards 40/42 (`begin-batch`, `validate`)
**Issue:** The rewrite removes `--restart-chain`, the deferred-verify chain, oversized roles, and the v2 batch model, but no card updates the verb `Long` strings that document them — `begin-batch`'s Long describes `--restart-chain`, "deferred-verify chain", and "oversized: frontmatter"; `validate`'s Long describes "plan-format v2", "Batch Index", "chain-end soundness", and the "oversized-batch context/card-count cap". Card 40 only preserves `Short`. Per the CLI/Cobra Invariant, stale help is a review-blocking defect.
**Fix:** Add to card 40 an explicit instruction to rewrite every affected verb `Long` for the flat card-list model (drop chain/oversized/`--restart-chain`/v2 language); re-audit all eight verbs' Long strings.

### [NIT] validate.go retarget leaves envelope keys unpinned
**Location:** Batch 9, card 40 (`internal/webstercli/validate.go`)
**Issue:** The ok-envelope emits `"batches": len(plan.Batches)` and `findingsEnvelope` emits `"batch": f.Batch`; `planparser.Plan` has `Cards` (no `Batches`) and `planparser.ValidationError` has `Card` (no `Batch`). Card 40 retargets the signature but does not pin the new observable JSON key names/count that card 42's tests must assert.
**Fix:** Have card 40 specify the replacement (e.g. `len(plan.Cards)`, finding key `card`/`f.Card`) so the emitted contract is deterministic.

### [NIT] Cited Shared Decision does not exist
**Location:** Batch 7, cards 27/28
**Issue:** Both cards invoke a `fork-prompt-plan-level-context` decision ("per the `fork-prompt-plan-level-context` decision INJECT the plan-level ## Shared Decisions"), but `## Shared Decisions` in 00-overview.md defines no such entry. The behavior is fully specified inline, but the named decision is dangling.
**Fix:** Add the `### Decision: fork-prompt-plan-level-context` entry to `## Shared Decisions`, or drop the citation from cards 27/28.

### [NIT] Stale cross-batch references in prose
**Location:** Batch 2 scope; Batch 3 scope + card 12
**Issue:** Batch 2 says "batches 9 and 12 consume" `Validate`; Batch 3 says "batches 8 and 9 consume" the batcher; card 12 says "batch 8 adds the config key, batch 12 wires Select". No batch 11/12 exists; the `batcher` config key is added in batch 7 (card 26) and `Select` is wired in batch 9 (card 39); `Validate` is consumed by batches 7 and 9. The authoritative Batch Index DAG is correct — only the prose is stale.
**Fix:** Correct the batch numbers (7/9, not 8/12) in the three prose references.

### [NIT] Card 32 names gitrepo symbol absent from its Context
**Location:** Batch 7, card 32 (`recordbatch.go`)
**Issue:** Requirements name `gitrepo.CurrentSHA` for per-card SHA capture, but `internal/gitrepo/gitrepo.go` is not in card 32's `Context:` (card 25 lists it for the same call). The webster `headSHA` wrapper is available via `gitwrap.go`, which is listed.
**Fix:** Add `internal/gitrepo/gitrepo.go` to card 32 Context, or reference webster `headSHA` (already in-Context via `gitwrap.go`) instead of `gitrepo.CurrentSHA`.

## Verdict

REQUEST_CHANGES
Sound plan; fix stale verb help text and minor cross-reference/spec gaps.
MILL_REVIEW_END
