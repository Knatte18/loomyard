MILL_REVIEW_BEGIN
# Review: Loom persists done only after post-run friction reflection — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: 2026-09-26
```

## Findings

### [BLOCKING:scope] no-perishable-row-counts sweep misses three stale counts
**Location:** overview `no-perishable-row-counts` Decision; batch 1 card 2; batch 1 card 2 (shedbuild)
**Issue:** The Decision claims an exhaustive sweep "beyond the discussion's floor list" but three verified instances go unedited by any card: `internal/shedrecipe/entries_simple_test.go:1` ("table-driven over the seven value-only entries", becomes 8 after card 2 adds `FrictionReflect` to `simpleEntryCases`), `internal/shedbuild/fixture_test.go:169` ("two of the sixteen engines need it", becomes 17), and `docs/overview.md:353` ("it registers sixteen engine names", becomes 17). All three are perishable tallies of exactly the table the Decision already reworks elsewhere in the same two files' siblings (`entries_simple.go`, `build_engines_test.go`, `registry.go`).
**Fix:** Add these three edits to card 2 (the two Go doc comments) and to card 3 or 5 (the `docs/overview.md` line), rewording each to name the source rather than a count, per the Decision's own rule.

### [NIT:scope] Card 6 omits internal/loomengine/config.go from Context
**Location:** batch 2 card 6
**Issue:** `TestReflectFrictionRow_WaitsOnAHeldReflectionLock`'s Requirements have the test call `loomengine.LoomFrictionLock(c.location)` directly, but `internal/loomengine/config.go` (where it's declared) is not in card 6's Context or Edits. The call is already demonstrated verbatim in `internal/loomcli/run.go`, which IS in card 6's Context, so the practical risk is low.
**Fix:** Add `internal/loomengine/config.go` to card 6's Context for completeness.

### [NIT:consistency] Reworded SKILL.md sentence leaves a dangling possessive
**Location:** batch 1 card 3, `plugins/ly/skills/ly-drive/SKILL.md`
**Issue:** "A recipe with a graph shaped differently from loom's fourteen rows" becomes "A recipe with a graph shaped differently from loom's" per the card's literal instruction — dropping "fourteen rows" entirely rather than "rows" leaves "loom's" a bare possessive with no stated referent.
**Fix:** Reword to "A recipe with a graph shaped differently from loom's own" or "...from loom's own graph".

## Verdict

REQUEST_CHANGES
One BLOCKING: the row-count sweep leaves three stale tallies unedited across two files and one doc.
MILL_REVIEW_END
