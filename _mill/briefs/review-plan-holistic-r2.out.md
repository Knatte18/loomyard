MILL_REVIEW_BEGIN
# Review: format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [BLOCKING:consistency] Card 10 inserts into shed.md beyond the batch's own "token-only" scope claim
**Location:** batch 2 / card 10 **Issue:** Batch 2's "rename crosses task-ownership boundaries deliberately" decision states the crossing into `shed.md` (task E's file) is "only the literal token is replaced; no surrounding prose is rewritten," but card 10 also *inserts* `Discussion-Validate` into `:13` and `:41` — new content, not a token rename. `shed-followups.md:423–424` names exactly this insertion ("Both must gain `Discussion-Review-Gate` once task C inserts it") as task E's own `Part one` action item, so card 10 does E's assigned work under a decision that claims it isn't doing that. **Fix:** either narrow the batch-local decision's wording to admit the insertion explicitly (with rationale for taking it from E), or drop the insertion from card 10 and leave `:13`/`:41` for task E to reconcile against the finished table, consistent with `shed-followups.md`'s own ownership split.

### [NIT:scope] Card 9 edits loom.md:75, outside task C's stated "rows 2-7" grant
**Location:** batch 2 / card 9 **Issue:** `shed-followups.md`'s task C Scope section grants "loom.md's producer table rows 2–7 only... Task E owns everything else in loom.md," and line 75 (Open questions prose) is outside the table. Card 9 repairs it anyway, without a batch-local-decision entry acknowledging the crossing the way the Batchifier-cell and `## The gate`-section exceptions are acknowledged. **Fix:** add a one-line batch-local decision naming this as a deliberate, self-consistency-driven exception (Card 8's own insertion is what falsifies the line), mirroring the other two documented exceptions.

### [NIT:scope] Card 2's discussion-format.md "Plan producer" site list omits line 56
**Location:** batch 1 / card 2 **Issue:** Card 2 enumerates "the Plan producer" at `:7, :10, :14, :15, :31, :54, :83`, but the pre-edit file also carries the phrase at `:56` ("The Plan producer explores the codebase itself...") inside the same compaction-rules list as `:54`. The card's own acceptance grep and Batch Tests item 1 would catch the miss, so it's self-correcting but the enumeration is not exhaustive as claimed. **Fix:** add `:56` to card 2's enumerated site list.

### [NIT:consistency] Card 12 misattributes the "Override recorded" convention to shed-followups.md:289
**Location:** batch 2 / card 12 **Issue:** Card 12(c) cites `:289` as prior use of the `**Override recorded ...**` lead-in convention, but `:289` actually reads `**Instruction repaired 2026-08-09 (task A, as landed).**` — a related but distinct convention used elsewhere in the same file (also at `:92`). The other five citations (`:296`, `:441`, `:449`, `:462`, `:470`) are correct. **Fix:** drop `:289` from the precedent list, or cite `:296` twice/cite `:92` instead as an "Instruction repaired" example if that distinction matters.

## Verdict

REQUEST_CHANGES
Card 10's undocumented scope crossing into task E's shed.md work item is the blocking issue; the rest are minor.
MILL_REVIEW_END
