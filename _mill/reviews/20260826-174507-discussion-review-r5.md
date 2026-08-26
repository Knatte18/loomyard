MILL_REVIEW_BEGIN
# Review: Custom-typed plan cards skip path-missing checks

```yaml
duration_s: 151.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class model, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Group-owned Pairs has no normalization owner
**Section:** Technical context (normalize.go bullet) + `model-shape-additive`
**Issue:** `normalizeCard` (`normalize.go:50-59`) rewrites only `card.Targets`, `card.Uses`, and `card.Pairs`; group `Pairs` are struct copies (`parse.go:505` appends by value), so under `model-shape-additive` they stay un-normalized — and group-scoped `checkPathMissing` now reads `TargetGroups[*].Pairs.Old`, which under a plan-level `root:` would stat an unprefixed path and false-positive `path-missing`. The stated parity requirement and the "Normalization parity" test scenario both cover `Targets` vs `TargetGroups[*].Refs` only, never `Pairs`/`RenameRaw`.
**Fix:** Extend the normalization rule and its pinned post-condition to `Pairs`/`RenameRaw`: state that `Card.Pairs` must equal the union of `TargetGroups[*].Pairs` after `normalizeCard`, and decide which side is normalized and which is rebuilt.

### [NIT:consistency] Scope list omits sites its own sweep example names
**Section:** Scope (In, final bullet) vs Technical context (sweep predicate)
**Issue:** The enumeration is presented as "this predicate's output as of today" but omits two hits the discussion itself cites as scan results — `internal/planparser/parse.go:9` and `internal/planparser/validate.go:354` — and also misses `parse.go:318,490-493` and `contracts/specs/loom-plan-spec.md:92,218`, all verified present in source.
**Fix:** Either add the named hits to the list or drop the claim that the list is the predicate's output, leaving only the "re-derive, not trust" instruction.

### [NIT:design] "Public API for consumers" rests on an unverified premise
**Section:** `model-shape-additive`, `Card.Type`/`TypeLabelCount` bullet
**Issue:** The retention rationale is that both are public API for consumers outside the package, but no package in the repo reads `CardType`, `Card.Type`, or `TypeLabelCount` outside `internal/planparser` (grep over `*.go`); the only readers are planparser's own checks and tests.
**Fix:** Restate the rationale as compatibility-conservatism for an exported field with no current reader, rather than as an existing consumer surface.

## Verdict

REQUEST_CHANGES
Group-owned Pairs normalization is unspecified and would false-positive path-missing under `root:`.
MILL_REVIEW_END
