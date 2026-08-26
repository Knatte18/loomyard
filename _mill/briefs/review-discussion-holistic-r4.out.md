MILL_REVIEW_BEGIN
# Review: Custom-typed plan cards skip path-missing checks

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Sweep predicate still misses bare "type label" phrasings
**Section:** Technical context — "Repo-wide sweep predicate"
**Issue:** The six singularity greps are phrase-anchored and do not match live sites that carry the rule in other wordings: `internal/planparser/parse.go:9` ("a card's own type label" — "own" defeats `a card's type label`), `internal/planparser/validate.go:354`'s user-visible detail string `card %d's type label carries no targets`, and `contracts/stencils/loom/loom-rubric-webster-review.md:54` / `manifest/designs/loom.md:261` ("the card's Type-specific mechanical check", keyed on the card's *type*, not on the label word at all) — so the predicate reproduces round 3's failure mode in a new spelling.
**Fix:** Replace the phrase list with a bare case-insensitive `type label` scan plus a second family keyed on a card's *type* (`the card's Type`, `Type-specific`, per-type table rows), and state that a single-hit-per-phrase list is not the method.

### [BLOCKING:design] Per-type table columns have no multi-label composition rule
**Section:** Decisions — group-scoped-checks / documentation-and-fixture-updates
**Issue:** The card-types table in `contracts/specs/loom-plan-spec.md:100-108` and `manifest/designs/plan-card-format.md:26-34` is keyed one row per card type across four columns, and only `ImpactSummary` gets a stated composition rule ("required when any group is Edit or Delete"); `Mechanical check` and `Batchable?` are left undecided for a card whose groups disagree (an `Edit`+`Delete` card is simultaneously "No" and "Yes — independent targets only", and owes both blast-radius and assert-no-callers), while the plan only promises the table "gains a note".
**Fix:** State the composition rule for every column of that table — e.g. mechanical checks union across groups, batchability takes the least permissive row — and carry it into the Webster-review rubric bullet that today says "the card's Type-specific mechanical check".

### [NIT:consistency] Two `**Custom:**` labels: sole-label rule vs repeat rule
**Section:** Decisions — legal-label-combinations / card-type-missing-relaxed-plus-new-check
**Issue:** "`Custom` must be a card's **sole** type label" and "a label appearing more than once on a card is legal" give opposite answers for a card carrying two `**Custom:**` groups, and the check is defined as "`Custom` alongside any other type label", which would not fire.
**Fix:** Say outright that `card-custom-not-alone` fires only on a `Custom` group beside a group of a *different* type, so repeated `Custom` stays legal.

## Verdict

REQUEST_CHANGES
Enumeration method and per-type table composition both still unresolved.
MILL_REVIEW_END
