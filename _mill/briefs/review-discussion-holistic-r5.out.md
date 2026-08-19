MILL_REVIEW_BEGIN
# Review: loom: phase-machine scaffolding

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:design] "Every row faked to Done" is unreachable under Deps
**Section:** Testing (Tier 1 bullet 3) vs. `explicit-deps-struct`
**Issue:** `explicit-deps-struct` pins exactly two injectable rows (`Preflight`, `Webster`), so rows 3/7/9 are real and can only return `Done` from on-disk state — row 7 requires a fixture passing all fourteen `planparser.Validate` checks (`internal/planparser/validate.go:56-74`, incl. `checkPathMissing`/`checkMoveSourceMissing` against `worktreeRoot`), row 3 requires both discussion files with all seven H2s, row 9 an `_lyx/` or absent-config fallback.
**Fix:** State which resolution the plan writer takes — a shared Tier-1 fixture builder that makes rows 3/7/9 genuinely pass, or additional injection points — since the second contradicts the two-rows-only rule as written.

### [NIT:consistency] `onstuck-routing`'s stated rule does not fit rows 1 and 9
**Section:** `onstuck-routing` vs. the producer table
**Issue:** The rule reads "every gate and validator bounces back to the producer whose artifact it guards", but `Preflight` (gate over git state) and `Batchifier` (gate over `batcher.yaml`) both carry `OnStuck: ""` in the table; a plan writer applying the rule literally would disagree with the table.
**Fix:** Add the missing clause — a gate whose guarded artifact is produced by no row escalates (`""`) — so the rule and the table agree without cross-checking.

### [NIT:decision] `Seed(...)`'s parameter set is never stated
**Section:** `loomshed-owns-seed`
**Issue:** `Seed` is written as `Seed(...)` throughout; the discussion pins what it writes (`current_producer`, `state`, `product` payload) but never says what it takes, in particular where `slug` and `parent` come from and whether it takes `Deps` or bare told paths.
**Fix:** Name the argument set (status path, status-lock path, slug, parent) so the plan writer is not inventing the seam that `loom: session bootstrap` will call.

### [NIT:scope] loom.md's second State-&-contracts bullet is unaddressed
**Section:** Constraints → Documentation Lifecycle
**Issue:** The doc set names only the "current phase, current review stage" bullet (`manifest/designs/loom.md:170`); line 175's "*It also carries a human-readable current-activity narration*" bullet uses the retired `narration` word, even though its now/last/wait example survives verbatim as Shed's `activity`.
**Fix:** Record explicitly whether line 175 is reworded or deliberately left standing, so the enumeration stays reproducible.

## Verdict

REQUEST_CHANGES
The core sequence test's construction contradicts the injection decision; everything else verifies.
MILL_REVIEW_END
