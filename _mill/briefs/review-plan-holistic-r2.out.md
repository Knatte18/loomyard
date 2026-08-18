MILL_REVIEW_BEGIN
# Review: burlerengine + perchengine told-geometry — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-18
```

## Findings

### [BLOCKING:scope] hubgeom.BurlerGeometry/PerchGeometry named outside Context
**Location:** Batch 1 cards 1 and 2; Batch 2 card 6.
**Issue:** Each card's `Requirements:` prescribes a doc-comment sentence naming `hubgeom.BurlerGeometry` (cards 1, 6) or `hubgeom.PerchGeometry` (card 2), but `internal/hubgeom/hubgeom.go` is absent from that card's `Context:`/`Edits:` list (card 1: reedengine/geometry.go, burlerengine/engine.go; card 2: reedengine/geometry.go, perchengine/engine.go; card 6: burlerengine/geometry.go, profile.go, doc.go, pattern/pattern.go).
**Fix:** Add `internal/hubgeom/hubgeom.go` to the `Context:` list of cards 1, 2, and 6.

### [BLOCKING:scope] pattern.File named outside Context in card 13
**Location:** Batch 3, card 13.
**Issue:** Requirements states the new signature "mirrors `planparser.PlanDir(anchorPath string)` and `pattern.File(baseDir string)`," but card 13's `Context:` lists only `internal/lyxdirs/dirs.go` and `internal/planparser/parse.go` — `internal/pattern/pattern.go` is missing.
**Fix:** Add `internal/pattern/pattern.go` to card 13's `Context:` list.

### [NIT:consistency] notransients_test.go row count mismatch
**Location:** Batch 3 `## Batch Scope` narrative vs. card 21.
**Issue:** The batch-scope intro says "the five [rows] in `cmd/lyx/notransients_test.go` go tautological with respect to anchoring," but card 21 itemizes six lines / seven call expressions, and source inspection confirms six lines (two `durableSet` rows, one `ScratchDir` row, two nested `PauseFlagPath` uses, one `mirroredPairs` entry).
**Fix:** Correct the batch-scope prose to "six" to match card 21's precise count.

## Verdict

REQUEST_CHANGES
Two Context-completeness gaps (cards 1/2/6, card 13) and one prose row-count mismatch; design and source-grounding are otherwise sound.
MILL_REVIEW_END
