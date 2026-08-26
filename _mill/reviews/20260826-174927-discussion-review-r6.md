MILL_REVIEW_BEGIN
# Review: Custom-typed plan cards skip path-missing checks

```yaml
duration_s: 187.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Sweep predicate misses the "a <Type> card" family
**Section:** Technical context, "Repo-wide sweep predicate"
**Issue:** The three scans key on the count words, `type label`, and `type-specific|the card's type|per-type`, but a fourth family states card-scoped type semantics by naming the type alone and matches none of them: `validate.go:439` ("a Prosa card's target list must hold only file(s)"), `:455` (user-visible detail `"card %d is a Prosa card but targets the symbol %q"`), `:292` ("a plan with at least one Rename card"), `:319` ("a card of type Edit or Delete"), `:516` ("every CardTypeCreate card's path-shaped Targets entries"), `plan.go:93-95` ("For a Rename card, both endpoints..."), and spec rows 8/13/15 plus `:225` at `contracts/specs/loom-plan-spec.md`. Every one of these becomes false under group scoping, and this is the third successive round in which the stated method under-enumerates.
**Fix:** Add a fourth over-broad scan keyed on the seven type names in card-scoped prose (e.g. `grep -rni "prosa card\|rename card\|create card\|custom card\|edit card\|delete card\|move card\|type Edit\|CardType[A-Z][a-z]* card"`), or restate the predicate as "any site asserting a card has one file-operation type, in any phrasing", and note the spec's per-row wording (rows 8, 13, 15, 225) is in scope, not just row count/IDs/ordering.

### [NIT:consistency] Create's mechanical-check cell is misquoted as "none"
**Demoted-from:** BLOCKING
**Section:** Decisions § `per-type-table-composition`, Mechanical check bullet
**Issue:** The bullet asserts "A `Create` or `Prosa` group contributes nothing to the union (its cell is 'none')", but `contracts/specs/loom-plan-spec.md:102`'s Create cell reads "none — check nothing equivalent exists first", which is a real obligation; the motivating `Edit`+`Create` card would therefore owe that check under the union rule the same bullet states. Prosa's cell alone is a bare "none".
**Fix:** Decide explicitly whether a `Create` group's "check nothing equivalent exists first" joins the union (and say so for the golden card 2), or amend the table cell in the same edit so the "contributes nothing" claim becomes true.

### [NIT:design] `card-custom-not-alone` finding cardinality unstated
**Section:** Decisions § `card-type-missing-relaxed-plus-new-check`; Testing item 2
**Issue:** The predicate is defined but not the finding count: a card with two `Custom` groups plus one `Edit` group could yield one finding per card or one per offending group, and no test scenario pins it.
**Fix:** State one finding per card (the card-level rule shape) and add that fixture to the Testing list.

### [NIT:consistency] Stencil-drift test asserts two strings, not one
**Section:** Technical context, `internal/loomengine/plan_test.go:217-227`; Testing § Stencil/prompt drift guard
**Issue:** `TestPlanSpec_PromptStatesTypeLabelGrammar` asserts both `"exactly one bold type label from ..."` and `"sub-bullets are the card's targets"`; the proposed stencil wording ("each with its own target sub-bullets") breaks the second assertion too, while the discussion refers to a single assertion.
**Fix:** Say both asserted substrings are updated, or keep the phrase "sub-bullets are the card's targets" verbatim in the new stencil line.

## Verdict

REQUEST_CHANGES
Enumeration method still misses a phrasing family; one table cell is misquoted.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
