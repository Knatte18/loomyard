MILL_REVIEW_BEGIN
# Review: Custom-typed plan cards skip path-missing checks

```yaml
duration_s: 166.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class; env reports model id claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [NIT:scope] File inventory misses count/rule-bearing prose
**Demoted-from:** BLOCKING
**Section:** Scope / Technical context ("Other files carrying `format: 4` plan text")
**Issue:** The enumeration method was "files carrying format-4 plan text", which does not reach the two things this change actually invalidates repo-wide — the "sixteen checks / fifteen upstream" count and the "exactly one bold type label" rule: `internal/planparser/doc.go:78`, `internal/planparser/validate.go:3-4,56,63,173`, `internal/planparser/plan.go:86-88` (`TypeLabelCount` godoc calling a two-label card a defect), `internal/planparser/validate_test.go:1-10,28,81`, `contracts/recipes/loom-recipe.yaml:171`, `contracts/stencils/loom/loom-rubric-plan-review.md:17,20,31`, `manifest/designs/loom.md:161`, `manifest/designs/plan-card-format.md:91`, `manifest/designs/scout-plan-symbol-fields.md:64`; and `internal/loomengine/plan_test.go:217-227` asserts the stencil's literal `"exactly one bold type label from ..."` string, so the claim "None should break" is false.
**Fix:** Replace the file-list method with a stated sweep predicate (every site stating a check count, the entry-point split, or the one-label rule — Go godoc, tests, recipe, stencils, designs included) and re-derive the Scope list from it.

### [NIT:consistency] webstercli declared out of scope but pins "16 checks"
**Demoted-from:** BLOCKING
**Section:** Scope → Out ("`internal/webstercli`: no change")
**Issue:** `internal/webstercli/validate.go:47` states in the command's `Long` that it runs "the 16 checks contracts/specs/loom-plan-spec.md's 'Validation checks' section pins", which this task takes to 17; the CLI/Cobra Invariant makes help accuracy a review obligation whenever observable behaviour changes.
**Fix:** Move `internal/webstercli`'s help text into In-scope (help wording only), keeping its consumption of the flat `Targets` unchanged as stated.

### [BLOCKING:design] Duplicate-label rationale is false for Rename
**Section:** Decisions → legal-label-combinations / group-scoped-checks
**Issue:** The rationale for adding no duplicate-label check is "two `Edit` groups validate identically to one merged group", but `Card.Pairs`/`Card.RenameRaw` are flat card-level accumulators (`parse.go:501-511`) with no group ownership, so a card with two `Rename` groups makes the group-scoped `checkPathMissing` iterate all of `c.Pairs` once per `Rename` group and report every missing `Pairs.Old` twice.
**Fix:** State the disposition explicitly — either per-card dedup of `path-missing` findings, per-group `Pairs` ownership, or a rule that `Rename` may appear at most once per card.

### [NIT:design] Rename group's own Refs content unstated
**Section:** Decisions → model-shape-additive / rename-in-multi-label-cards
**Issue:** Testing requires `Targets` to equal the union of `TargetGroups[*].Refs`, while group-scoped `path-missing` says a `Rename` group's "refs" are skipped — whether a `Rename` group's `Refs` holds the projected `Old`/`New` endpoints or is empty is left to inference.
**Fix:** Say outright that a `Rename` group's `Refs` carries both endpoints in `Old`-then-`New` order and is skipped by `path-missing`.

### [NIT:design] Card.Type left with no production reader
**Section:** Decisions → model-shape-additive
**Issue:** All six sites reading `Card.Type` become group-scoped, and no package outside `internal/planparser` reads it, so the retained "first label" field ends up production-unread and free to drift from `TargetGroups`.
**Fix:** State whether `Type` is retained as public API for consumers only, and that its godoc says it is not what validation keys on.

## Verdict

REQUEST_CHANGES
Doc/test sweep method incomplete, webstercli disposition contradicts source, Rename duplicate-group behaviour unspecified.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
