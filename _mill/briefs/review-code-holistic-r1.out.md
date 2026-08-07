MILL_REVIEW_BEGIN
# Review: fabric: close the weft-visibility leak (slice 8) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-07
```

## Findings

### [BLOCKING] Batch 08 card 29 doc reword left builder-contract.md pervasively un-reworded
**Location:** `docs/reference/builder-contract.md:23,25,64,67,81,93,99,150,160,162,164,166,177,186,193`
**Issue:** Card 29 states the design goal "builder's contract must not say weft exists" and named lines 22, 24, 167 for reword — those three are fixed — but a dozen+ sibling mentions in the SAME verb table and body remain: line 23 "Weft-commits `state.json` on success", line 25 "never weft-commits", the section heading "## The three weft-commit points" (line 162), "weft-BLIND"/"weftengine.Commit" (line 164), "durable weft history"/"weft pull" (line 166), "weft commits" (line 177), "host repo" (line 93), "downed reed hosts no live strand" is fine but line 186's "hosts" context and line 93/99's "host repo"/"host HEAD" are the exact fabric-sense phrases the invariant polices. This is not a historical-record doc (unlike test-suite-timing.md) — it is the current, as-built contract this task explicitly targeted.
**Fix:** Complete the reword pass over the whole file per doc-vocabulary-split's consumer-behaviour side, consistent with the three lines the card did fix.

### [BLOCKING] status-schema.md and plan-format.md carry the same "host HEAD"/"host repo" leak the plan fixed one line above
**Location:** `docs/reference/status-schema.md:52,67`; `docs/reference/plan-format.md:320`
**Issue:** Card 29 fixed `status-schema.md:11,33` but left `:52` (`"host HEAD stamped when Builder begins"`) and `:67` (`"the host \`HEAD\` stamped when Builder begins"`) — the exact `host HEAD` phrase the Fabric Vocabulary Invariant's phrase list names. `plan-format.md:320` ("rolls the host repo back to the chain-start SHA") mirrors the same unaddressed pattern as `builder-contract.md:93`. Both docs are consumer-behaviour docs per doc-vocabulary-split, not fabric-mechanism docs, so these are in-scope leaks the card's own stated rule should have caught.
**Fix:** Reword `status-schema.md:52,67`'s "host HEAD" to a fabric-neutral phrase (e.g. "the repo HEAD"/"HEAD") and `plan-format.md:320`'s "host repo" to "the repo", matching the sibling fixes already applied at :212/:11/:33 in the same files.

## Verdict

REQUEST_CHANGES
Code changes (batches 1-7) are solid and plan-conformant; batch 8's doc reword is materially incomplete.
MILL_REVIEW_END
