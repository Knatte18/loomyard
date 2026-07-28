MILL_REVIEW_BEGIN
# Review: PATTERN wiring: conditional constraint-injection into every agent — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-28
```

## Findings

### [BLOCKING] Card 28's inclusion of plan-template.md doesn't fit the batch's own stated criterion
**Location:** Batch 7 scope statement / Card 28
**Issue:** The stated inclusion rule is "(a) writes code, (b) reviews code, or (c) is a context-inheritance root whose in-session forks do," and the same paragraph explicitly excludes `loomengine/discussion-template.md` as failing all three. `loomengine/plan-template.md` is structurally identical to Discussion — a lone autonomous prose producer feeding one `shuttle.Run`, per `plan.go`'s own doc comment which draws that exact parallel ("Like the Discussion producer... the Plan producer is not a Go module") — yet Card 28 wires it in with no stated clause covering it and no explanation of why it is treated oppositely from Discussion.
**Fix:** Add a fourth criterion clause (e.g., "or directly authors the file-op/Requirements instructions a later code-writing agent executes near-verbatim") that justifies Plan's inclusion and Discussion's exclusion, or reconsider whether Discussion should also carry the directive — the plan explains the analogous builder-orchestrator vs. webster-Master asymmetry explicitly elsewhere and should do the same here.

### [BLOCKING] Card 14 widens the pathspec default but doesn't say to fix the now-stale exact-match test
**Location:** Batch 4 / Card 14
**Issue:** `internal/fabricengine/template_test.go`'s existing `TestConfigTemplate_PathspecResolvesToLyx` asserts `pathspec != "_lyx"` (exact string equality against the resolved template default). Card 14 changes that default to `"_lyx _pattern"`, which fails this pre-existing assertion, but the card's Requirements only say to "Extend... to assert the default value parses into two whitespace-separated paths" — never explicitly naming `TestConfigTemplate_PathspecResolvesToLyx` as needing its hardcoded `"_lyx"` comparison rewritten. Every structurally analogous case elsewhere in this plan (Card 8's `UnwireResult` cascade, Card 15's `HostJunctions` subtest, Card 19's sweep) explicitly names the stale pre-existing assertion and says "update it, never delete it" — this card is the one place that pattern lapses, and as written the batch's own `verify:` would fail on this exact test unless the implementer independently infers the fix.
**Fix:** State explicitly that `TestConfigTemplate_PathspecResolvesToLyx`'s exact-equality check must be rewritten to assert two whitespace-split paths, matching the "update, never delete" phrasing used everywhere else in this plan.

### [NIT] Card 29 overclaims that the discussion resolved every Open Question in pattern.md
**Location:** Batch 7 / Card 29, point (6)
**Issue:** `manifest/designs/pattern.md`'s "Detail-submap layout" question (whether `_pattern/<topic>/` has a fixed structure) is about PATTERN's own **content**, which this task explicitly defers to loomyard-init-via-lyx — it is not resolved by anything in these seven batches. Card 29 nonetheless says "Replace the Open Questions section with the decisions this task settled, since the discussion resolved all of them," which would drop this still-open question silently rather than carrying it forward.
**Fix:** Carry the detail-submap-layout question forward as still-open/deferred-to-content-migration when replacing the section, rather than implying it too was settled.

## Verdict

REQUEST_CHANGES
Two BLOCKING gaps: an unjustified plan-vs-discussion template-inclusion asymmetry, and a stale test assertion Card 14 doesn't flag for update.
MILL_REVIEW_END
