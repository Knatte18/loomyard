MILL_REVIEW_BEGIN
# Review: Producer gates: mechanical gates before session release — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-20
```

## Findings

### [BLOCKING:scope] Stale reference to a deleted identifier, `planValidateEntry`
**Location:** `internal/shedrecipe/entries_planwrite.go:30`
**Issue:** The doc comment reads "...the same split `planValidateEntry` already uses..." but `planValidateEntry` was deleted by batch 5 card 29 (`internal/shedrecipe/entries_simple.go`'s `planValidateEntry`/`discussionValidateEntry` no longer exist anywhere in source). `entries_planwrite.go` is not in card 29's or card 42's edit list, so this doc comment was never revisited and now points at a symbol that does not exist.
**Fix:** Rewrite the sentence to describe the split (`planparser.PlanDir` vs. `planglyph.ValidateFormat`'s worktree root) without naming the removed constructor, e.g. pointing at `NewPlanGate` instead.

### [BLOCKING:scope] "seventeen"/row-count staleness left across the sweep
**Location:** `README.md:20`, `README.md:161`, `manifest/roadmap.md:90`, `manifest/designs/shed-recipe.md:12`, `manifest/designs/shed-recipe.md:91`
**Issue:** All five say "seventeen" rows/registry entries. The recipe is now fourteen rows and the registry is sixteen entries (per `internal/shedrecipe/registry.go`'s own "complete at sixteen keys" and `internal/loomrecipe`'s fourteen-row guards). `README.md` and `manifest/designs/shed-recipe.md` are card 39/40 edit targets and `manifest/roadmap.md` is a card 38 edit target, yet each still carries the pre-task count. The batch 5/6 sweep method (a fixed five-string grep for row/engine *names*) never covered the numeral, so this class of staleness survived cards 36/44's "confirm the grep is empty" gate.
**Fix:** Update each to "fourteen" (rows) / "sixteen" (registry entries), and extend the sweep's grep terms (or do a manual numeral pass) to catch this class before re-running card 44.

### [BLOCKING:scope] Same row-count staleness in files outside any batch's edit list
**Location:** `internal/loomshed/doc.go:1`, `internal/loomcli/sharedbootstrap_test.go:207-208`, `plugins/ly/skills/ly-drive/SKILL.md:70,72`, `manifest/designs/seeded-shed.md:73`
**Issue:** `internal/loomshed/doc.go`'s package comment still says "its seventeen durable row names" even though `loomshed.go` itself (edited by card 29) now declares and documents fourteen. `sharedbootstrap_test.go`'s doc comment claims the built Shed "carries all seventeen producer rows." Two design/skill docs outside the touched-file manifest make the same claim. None of these files appear in any batch's `Edits:`/`Context:` list, so the row-removal's actual footprint was larger than the plan's own file inventory captured.
**Fix:** Correct all four to "fourteen," and treat this as evidence the "the sweep is a method, not a list" approach needs a numeral pass in addition to the identifier-string grep before it can be called complete.

## Verdict

REQUEST_CHANGES
The gate mechanism, pointer semantics, and recipe wiring are correct and well-tested, but the docs sweep left multiple stale row-count/identifier references, including in card 39/40's own edit targets.
MILL_REVIEW_END
