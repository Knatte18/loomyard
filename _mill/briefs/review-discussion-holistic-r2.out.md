MILL_REVIEW_BEGIN
# Review: Centralize glyph ref-shape enumeration

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-family, reported to me as Opus 5); best-effort self-assessment
reviewed_file: _mill/discussion.md
date: 2026-09-09
```

## Findings

### [BLOCKING:consistency] isPathRef retained but all callers migrate away
**Section:** Decision: dead-wrappers vs. Inventory A
**Issue:** `isPathRef`'s only three production callers are `normalize.go:112`, `normalize.go:178`, `validate.go:1040` (verified by grep) — and Inventory A lists exactly those three as policy-lookup migration sites, so after migration `isPathRef` is as dead as `isGlyphRef`, which the same decision deletes.
**Fix:** State the disposition explicitly: either `isPathRef` is deleted too, or the three sites keep the wrapper and are removed from the migration inventory.

### [BLOCKING:consistency] GlyphLanguage signature contradicts source
**Section:** Scope (In) and Decision: planglyph-dedup-seams
**Issue:** Both places give `Plan.GlyphLanguage() (string, bool)`, but `planparser.planLanguage` (`glyphref.go:23`) and `planglyph.resolveLanguage` (`planglyph.go:251`) both return `(glyph.Language, bool)`; a `string` return would break every call site the decision says migrates verbatim.
**Fix:** Correct the signature to `Plan.GlyphLanguage() (glyph.Language, bool)`.

### [BLOCKING:design] "The registry file" is unnamed and classify.go's status is undecided
**Section:** Decision: kind-policy-ledger
**Issue:** The enforcement test's exemption boundary is a single unnamed "registry file", yet `classify.go` itself contains an open-coded `strings.HasPrefix(raw, "plan:")` (line 60, a literal, not `HandlePrefix`) and the retained `isPathRef` wrapper's `classifyRef(raw) == refKindPath` — both trip the two greps unless `classify.go` is that file or is exempted, and the decision claims "no per-function exemption" is needed.
**Fix:** Name the registry file(s) and state whether `classify.go` is inside the boundary or carries a declared exemption, including its `"plan:"` literal.

### [BLOCKING:design] Disposition vocabulary and policy granularity unspecified
**Section:** Decision: kind-policy-ledger
**Issue:** The disposition type's members are never enumerated (only its zero value), and several listed sites are not one kind-gate each: `checkRenamePairShape` (`validate.go:768-822`) has two independent negation gates plus a third arm keyed on self-glyph-old/handle-new that is not a per-kind function at all, `isFileRenamePair` (`validate.go:733`) gates two refs, and `checkProsaSymbolTarget` (`validate.go:1036-1042`) consults shape only on the `!langOK` branch — so "one `map[refKind]disposition` per site" is underdetermined and the completeness meta-test's domain assertion has no defined unit.
**Fix:** Enumerate the disposition values and state whether the ledger key is a site or a single kind-gate, with the multi-gate and language-forked sites named as worked examples.

### [NIT:scope] Status tripwire scoped to planglyph only
**Section:** Decision: status-family-disposition / Inventory C
**Issue:** `quarry.ResolveResult.Status` also has production consumers outside the two packages — `quarrycli/resolve.go:48` and `quarrycli/expand.go:45` — both fail-closed today but outside the proposed grep tripwire's scope, and the discussion never states their disposition.
**Fix:** One sentence recording them as fail-closed today and deliberately outside the tripwire's file scope.

### [NIT:design] Drift guard placement vs. amendment append unstated
**Section:** Decision: batch-coverage-disposition
**Issue:** "erroring `ErrQuarryUnavailable` on a miss" in `drift.go:208-227` returns before the amendment loop (`drift.go:238-254`), whose in-code comment (232-234) insists the audit record lands regardless; the sibling transport error at 209-211 already returns early, so the intent is inferable but not stated.
**Fix:** State that the guard returns at the resolve boundary, matching the existing transport-error path, and that no amendment is appended in that case.

## Verdict

REQUEST_CHANGES
Registry file boundary, disposition vocabulary, wrapper disposition, and one signature need resolving.
MILL_REVIEW_END
