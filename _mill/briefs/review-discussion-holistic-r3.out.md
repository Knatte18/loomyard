MILL_REVIEW_BEGIN
# Review: pattern told-geometry

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build unverifiable from inside the sandbox
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] Transposition mitigation false for burler call site
**Section:** Constraints → "The new signature loses a compile-time transposition check" **Issue:** The stated mitigation ("`websterengine`'s `patternActiveLayout` fixture and the new `internal/loomengine` PATTERN case both assert a non-empty directive, so a transposed call site turns them red") does not cover `internal/burlerengine/engine.go:103`: `burlerengine` has no PATTERN fixture at all — `template_test.go:142` injects a literal `pattern_directive` value into the template and never reaches `pattern.Directive`, and no other burler test writes a `PATTERN.md` (verified by grep over `internal/burlerengine`). A transposed burler call site would be caught by nothing. **Fix:** Either state that the burler call site has no behavioural detector and accept it explicitly, or name a concrete detector for it (a burler case exercising `Engine.Run` with an active PATTERN).

### [BLOCKING:scope] Existing active-PATTERN plan_test case not dispositioned
**Section:** Scope (`internal/loomengine/plan_test.go`) and *anchoring-proof-relocates-to-the-call-site* **Issue:** `plan_test.go` already carries `TestPlanSpec_PatternDirectiveOptional` (100) whose "non-empty pattern_directive (PATTERN active)" sub-test (127–153) builds a real `t.TempDir()` worktree, writes `_lyx/PATTERN.md`, and asserts a non-empty directive — i.e. the closest existing host for the relocated proof, differing only in its zero-value `AnchorRel`. The discussion names only `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath` and its rejected-alternatives list (a)–(d) never weighs extending or parameterising this one, so a plan writer may either duplicate the fixture or "helpfully" fold the two together. **Fix:** Name `TestPlanSpec_PatternDirectiveOptional` in Scope with an explicit disposition (untouched, or extended with an `AnchorRel` case) and add it to the rejected-alternatives reasoning.

### [NIT:consistency] LazyRead sub-test distinction is thinner than claimed
**Section:** Testing, second bullet **Issue:** With the guard placed in `Directive` before `isActive` (per *empty-string-guard-placement*), the converted "empty anchorPath, stencilsDir does not exist" sub-test and `TestDirective_EmptyAnchorPath` exit at the *same* early return, so "the former asserts no stat, the latter asserts no stencil read" describes one branch twice rather than two properties. **Fix:** Keep the sub-test for symmetry with its inactive sibling, but state the reason as roster completeness rather than a distinct proved property.

## Verdict

REQUEST_CHANGES
Transposition-detector claim is false for burler; an existing plan_test PATTERN case lacks disposition.
MILL_REVIEW_END
