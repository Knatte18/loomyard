MILL_REVIEW_BEGIN
# Review: shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions

```yaml
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [NIT:decision] `loom.md:70–72` listed as an edit site, no disposition
**Demoted-from:** BLOCKING
**Section:** Technical context → Exact edit sites (`loom.md`), and Scope → In
**Issue:** `:70–72` appears in the inventory as "`loom.md`'s own copy of the two-part contract and the pointer rule" but carries no disposition, unlike every other entry (`:29` verify-only, `:82` delete, `:76–77` no edit), and Scope's `loom.md` list omits it entirely; verified in tree, `:70` reads "A producer's contract is exactly two parts — **Input** (a *pointer* to the format-contract file … ) and **Output**", which the new thin-Input carve-out contradicts (Discussion-Write has no Input), while `:71` restates the pointer rule in full — the duplication `shed-md-is-authoritative-loom-md-points` and the new CONSTRAINTS invariant both forbid.
**Fix:** State a disposition for `:70–72` — qualify/point at `shed.md`'s carve-out, or declare verify-only with the reason the restatement survives.

### [NIT:decision] `shed.md:8`'s own disposition left implicit
**Section:** Technical context → Exact edit sites (`shed.md`)
**Issue:** `:8` is described only as "the unqualified atomicity claim the carve-out scopes", never as edited, and Scope's `shed.md` residue list names `:7`, `:18`, `:19`, `:63` but not `:8` — while its `loom.md` mirror at `:44` is explicitly told to gain a pointer.
**Fix:** Say whether `:8` is qualified in place or left to be scoped by the section 14 lines below it.

### [NIT:consistency] `finalize.md` line citation disagrees between sections
**Section:** Q&A log (Finalize simple-or-bespoke) vs. `finalize-is-bespoke`
**Issue:** The Q&A entry cites "`finalize.md:38` requires be atomic end to end"; the decision cites `:37`, and the tree confirms `:37` carries "never released and re-acquired partway through" while `:38` is the `raddle.md` pointer.
**Fix:** Use `:37` in both places.

## Verdict

APPROVE
One inventoried `loom.md` site carries no disposition and contradicts the new carve-out.
MILL_REVIEW_END
