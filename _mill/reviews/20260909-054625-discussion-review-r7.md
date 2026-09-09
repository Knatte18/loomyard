MILL_REVIEW_BEGIN
# Review: Centralize glyph ref-shape enumeration

```yaml
duration_s: 124.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic) — Opus-class model, opus-5 per environment banner
reviewed_file: _mill/discussion.md
date: 2026-09-09
```

## Findings

### [NIT:consistency] Enum-sync test stated two ways: count vs set
**Demoted-from:** BLOCKING
**Section:** Testing → "Meta-tests to add" (l.293) and Q&A l.312, vs Decision: kind-policy-ledger (l.83)
**Issue:** The decision mandates set equality "not just cardinality" (r6), but the Testing checklist and the Q&A entry both still say "const-block declaration count == len(allRefKinds)" with no supersession note — and the Testing section is what a plan writer lifts, so the weaker cardinality test can ship and reopen the exact duplicate-plus-omission hole r6 closed.
**Fix:** Restate both l.293 and l.312 as set equality over the declared kinds, and mark the Q&A entry superseded the way l.300/304/308/313 already are.

### [NIT:design] Enum↔slice comparison bridge left unstated
**Section:** Decision: kind-policy-ledger (l.83)
**Issue:** The test is described as comparing `classify.go`'s AST-parsed *identifier* set against `allRefKinds`' members "via their `refKindName` renderings" — but `refKindName` yields prose ("a file path"), never `refKindPath`, so the two sides are different vocabularies unless the test maps identifier→value by iota position first.
**Fix:** Name the bridge in one clause (identifiers are valued by their `iota` position, then rendered) so the plan writer does not invent a third hand-maintained name table.

### [NIT:consistency] `isHandleRef` described as having callers to migrate
**Section:** Decision: dead-wrappers (l.195)
**Issue:** "`isHandleRef` is superseded by the exported `IsHandleRef` (its in-package callers migrate)" contradicts Technical context l.209 and the tree — `isHandleRef` has zero callers outside `classify_test.go`; nothing migrates.
**Fix:** Drop the parenthetical; `isHandleRef` is deleted as dead, identically to `isGlyphRef`.

## Verdict

APPROVE
One stale cardinality restatement can reintroduce the silent kind-list lag r6 closed.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
