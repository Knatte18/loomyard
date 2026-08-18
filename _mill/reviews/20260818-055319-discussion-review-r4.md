MILL_REVIEW_BEGIN
# Review: pattern told-geometry

```yaml
duration_s: 204.8
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [NIT:consistency] Webster transposition-coverage claim is false for 237
**Demoted-from:** BLOCKING
**Section:** Constraints › "The new signature loses a compile-time transposition check" (and Testing › `internal/websterengine`)
**Issue:** `patternActiveLayout` is used at exactly ONE call site — `template_test.go:803`, the `RenderRecoveryPrompt` "PATTERN active" sub-test covering `render.go:174`; no test anywhere asserts a non-empty directive for `RenderMasterPrompt` (`render.go:237`), so "covers `render.go:174` and `237` … both assert a **non-empty** directive" is wrong. The real detector for 237 is `TestRenderMasterPrompt_MissingPatternStencilErrors` (`template_test.go:850`), built on the *different* `patternActiveMissingPatternStencilsLayout` fixture, and it detects a transposition by the *absence of an error* (a swapped call would resolve inactive, return `("", nil)`, and the `want non-nil error` assertion would fail), not by an empty directive.
**Fix:** Restate the per-site coverage table with the actual detector and mechanism for each of the four sites, so the "only burler is uncovered" conclusion rests on a verified premise rather than a mis-attributed fixture.

### [NIT:scope] Burler assertion target is a random `round-*` directory
**Section:** Testing › `internal/burlerengine`
**Issue:** `engine.go:116` creates the round dir with `os.MkdirTemp(burlerDir, "round-")`, so no instruction path can be pinned literally; the discussion says "confirm which of the three instruction files carries the marker before pinning a path" without noting the parent is unnameable. (`prompt.go:62`/`:68` show `pattern_directive` lands only in instruction 1, `instruction-1-explore.md`, via `FillOptional`.)
**Fix:** Note that the assertion must discover the round dir (single-entry read of `<AnchorPath>/.lyx/burler`, or the path recorded in the orchestrator prompt) and that instruction 1 is the marker carrier.

### [NIT:scope] Stale test name/comment in `pattern_test.go` not enumerated
**Section:** Scope › `internal/pattern/pattern_test.go`
**Issue:** `TestDirective_RelPathNestedSubdirectory` (299–305) names and describes "a Layout whose RelPath" — `lyxcwd` vocabulary (and not even the real field name, `AnchorRel`) that goes stale when the case passes a plain nested directory string; the Scope list names only `layoutAt` and the two literal-`nil` sites, though it enumerates comparable stale text elsewhere (`pattern.go:84-85`, `doc.go:55/69`, `patternpath_test.go:1`).
**Fix:** Add this test's name and header comment to the pattern_test.go scope line.

## Verdict

APPROVE
One coverage premise contradicts the source; the rest is sound and verified.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
