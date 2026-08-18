MILL_REVIEW_BEGIN
# Review: pattern told-geometry

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:consistency] Q&A entry contradicts the relocation decision
**Section:** Q&A log (3rd entry) vs `### anchoring-proof-relocates-to-the-call-site`
**Issue:** The Q&A answer says relocate the proof "into `internal/loomengine/plan_test.go`'s **existing** subpath-anchored `PlanSpec` case", while the Decisions section and the later `[review r1 gap]` entry both say the existing `TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath` (verified at plan_test.go:284, relative never-created root `home/user/repo`) is **not** modified and a new `t.TempDir()`-rooted sibling is added.
**Fix:** Mark the earlier Q&A entry superseded (or rewrite its answer to "a new sibling test") so a plan writer reading the log top-down cannot implement the retracted option.

### [NIT:scope] Stale-reference inventory misses three in-tree sites
**Section:** Scope / In
**Issue:** Verified stale-after-change text the scope does not name: `pattern.go:84-85` (`Directive`'s own doc comment, "for a nil l"), `pattern.go:120` (`isActive`'s "an absent FileHere(l)"), `doc.go:55` ("an interpolated absolute path built from a `Location` field"), and `patternpath_test.go`'s header line 1 (`File/FileHere constructors`) — the last is named in Testing but not in Scope.
**Fix:** Add these to the Scope list so the doc-in-same-commit rule covers every surviving `FileHere`/`Location` mention in the package.

### [NIT:consistency] `newTestStencilsDir` is not in `plan_test.go`
**Section:** Technical context (loomengine paragraph) and Testing
**Issue:** Both call `newTestStencilsDir(t)` "the existing stencils-seeding helper in this file"; it is declared in `internal/loomengine/prompt_test.go:21` (same package, different file).
**Fix:** Reword to "in the same package" so the plan does not send an implementer looking in the wrong file.

### [NIT:consistency] Relocated tautology comment names only the planparser rows
**Section:** `### anchoring-proof-relocates-to-the-call-site`
**Issue:** The comment being copied onto rows 88/144 reads "The two planparser rows below…" and points at `plan_test.go` **and** `webstercli/verbs_test.go`; copying it verbatim onto a third row makes the count and the pointer list wrong.
**Fix:** State that the existing comment block is amended to cover the `pattern.File` row rather than duplicated verbatim.

### [NIT:design] Type safety lost between two adjacent string params
**Section:** Scope / In — the new `Directive(anchorPath, stencilsDir string, role Role)`
**Issue:** Today `*lyxcwd.Location` vs `string` makes an argument transposition a compile error; after the change both leading params are `string`, so a swapped call site compiles and is caught only by behavioural tests.
**Fix:** Note the residual risk and confirm the named suites (websterengine `patternActiveLayout`, the new loomengine case) are relied on as the detector.

## Verdict

REQUEST_CHANGES
One superseded Q&A answer contradicts the relocation decision; everything else verified accurate.
MILL_REVIEW_END
