MILL_REVIEW_BEGIN
# Review: Move <hub>/.lyx into <hub>/_board — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-14
```

## Findings

### [BLOCKING:consistency] repairPairWiring's doc comment goes stale after card 14
**Location:** batch 2 / card 14 (`internal/fabricengine/reconcile.go`)
**Issue:** Card 14 deletes the `wireBoardLink(rec, warpLayout, slug)` call and its error branch from `repairPairWiring`, and separately fixes the two comments that name `wireBoardLink` by identifier (`reconcileWarpBinding`'s simile and `appendPrDetail`'s caller list) — but `repairPairWiring`'s own doc comment is never added to that list. That comment's summary sentence currently reads "it re-wires whatever checkJunctionHealth reports broken, always re-wires the operator-convenience _board link, and applies declarative stale-removal," and a full paragraph explains "The _board re-wire is unconditional with respect to junction health... this convenience link must never be able to downgrade a reconcile verdict." After card 14's edit this describes code that no longer exists in the function it documents — the exact "contradicts itself / carries a superseded statement" shape, on the very function whose behavior is being changed in this card.
**Fix:** Add `repairPairWiring`'s own doc comment to card 14's requirements: drop the "_board link" clause from the summary sentence and delete (or rewrite to past-tense/historical framing) the "_board re-wire is unconditional..." paragraph, alongside the two comments the card already names.

## Verdict

REQUEST_CHANGES
Card 14 leaves `repairPairWiring`'s own doc comment describing deleted `_board` re-wire behavior as current.
MILL_REVIEW_END
