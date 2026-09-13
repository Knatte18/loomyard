MILL_REVIEW_BEGIN
# Review: Deploy cited spec/design docs to target repos like stencils

```yaml
duration_s: 168.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [NIT:scope] Hub seeding's weft-commit half is unscoped
**Demoted-from:** BLOCKING
**Section:** Scope "In" / `specs-dir-mirrors-stencils-dir` / Technical context "Existing call sites to extend for seeding"
**Issue:** The hub pre-run does not only reconcile — `cmd/lyx/stencilseed.go:129` then commits what it wrote via `fabricengine.CommitSeededStencils`, whose pathspec is hardcoded to the stencils subtree (`internal/fabricengine/stencilcommit.go:56`: `stencilsRel := path.Join(lyxdirs.LyxDirName, stencilsDirName)`), so a seeded `_lyx/specs/` tree is written but never staged; the decision asserts "same weft-commit story" without naming the commit path, Scope lists only "seeding/refreshing", and the Fabric Git Invariant ("every weft-commit caller passes a positive-only file list via `ScopedPathspec`") is absent from the Constraints section.
**Fix:** Decide and state the weft-commit mechanism for the specs subtree (generalise `CommitSeededStencils` over a subtree-relative prefix, or add a sibling verb), add it to Scope, and list the Fabric Git Invariant among the binding constraints.

### [BLOCKING:design] Rubric-fill helper conflates stencil rubrics with literal ones
**Section:** `rubric-marker-allowlist`, "Collateral this decision creates"
**Issue:** Two of the four named sites do not read a stencil: `internal/burlercli/run.go:63` takes `Rubric` from a user-authored profile YAML `rubric:` key (`profileYAML.Rubric`, run.go:33), and `internal/shedrecipe/entries_burler.go` accepts a literal `rubric` key as the documented alternative to `rubric_stencil` (lines 165, 192-197) — routing those through `stencil.Fill` makes any `{{` an author writes in prose a `parse template` error (`internal/stencil/stencil.go:29-31`) and imposes specs-dir semantics on text that never had them, a behaviour change the discussion neither names nor rejects.
**Fix:** State whether the helper applies to stencil-sourced rubrics only (leaving literal `rubric:` values untouched) or to all rubric values, and if the latter, decide the disposition of a literal rubric containing template syntax.

### [NIT:consistency] Strip-before-Fill rationale rests on a false premise
**Section:** `rubric-marker-allowlist`; Testing "The rubric-render helper"
**Issue:** `FillOptional` already calls `stripLeadingComment` on its template before parsing (`internal/stencil/stencil.go:27`), so the stated test scenario "the stamp banner is stripped *before* filling, so a banner containing a marker-like string cannot reach `Fill`" describes a guarantee `Fill` provides on its own; the helper's explicit strip is for the *value* semantics `entries_burler.go:207-211` documents, not for Fill's protection.
**Fix:** Restate the strip's purpose as keeping the banner out of the interpolated value, and drop the "cannot reach Fill" justification.

### [NIT:scope] Conditional gotcha is already determinate
**Section:** `code-comment-conventions-is-misscoped` Gotcha; Testing "rubric_test.go — extend, and check for a break"
**Issue:** `contracts/stencils/rubric_test.go:123` does pin `"code-comment-conventions.md"`, and `docs/code-comment-conventions.md:5` keeps the doc out of the deletion class precisely because "live producer rubrics still cite" it — the rewrite falsifies that retention rationale, not just that one line.
**Fix:** Drop the "check whether" hedge, and state the doc's post-change Documentation-Lifecycle disposition rather than only "update that doc line".

### [NIT:scope] Shed Recipe Registry Invariant unlisted
**Section:** Constraints
**Issue:** Reaching the two burler rubric sites means threading a told specs dir through `shedbuild`→`shedrecipe` (`burlerRoundProfile(cfg, stencilsDir)`, `entries_burler.go:156`) and through `shedadapters.BouncerConfig`, which today carries `StencilsDir` and no specs sibling; the governing "no direct import of `lyxcwd`; every path is told" invariant is not in the list.
**Fix:** Add the Shed Recipe Registry Invariant to the Constraints section and note that the specs dir arrives as a new told parameter, never derived.

## Verdict

REQUEST_CHANGES
Two decisions missing: specs weft-commit path, and helper scope over literal rubrics.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
