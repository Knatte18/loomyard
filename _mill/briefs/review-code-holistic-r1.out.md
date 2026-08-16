MILL_REVIEW_BEGIN
# Review: PATTERN directives: move from Go constants to stencil files — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-16
```

## Findings

No findings. Verified end-to-end across all three batches:

- `stencils/pattern/*.md` bodies are marker-free, weft/warp/host-clean, LF-pinned in `.gitattributes` immediately after the webster-body-implementer line and before `internal/boardengine/template.yaml`, and registered in `stencils/stencils.go` as a trailing `pattern` family block in the required order.
- `internal/pattern.Directive` now has signature `(l *lyxcwd.Location, stencilsDir string, role Role) (string, error)`, preserves the exact guard order (nil layout / inactive / unknown role → `("", nil)`, no read), reads via `stencilstore.Read` and strips via `stencil.StripLeadingComment` only on the post-switch path, and wraps a read failure as `pattern: directive stencil: %w` without duplicating the stencil name.
- All four call sites (`loomengine/plan.go`, `burlerengine/engine.go`, `websterengine/render.go` ×2) were updated to the two-value form with each package's own house-style error wrap (`loom: PlanSpec: %w`, `burler: %w`, `webster: <what>: %w`), and both webster hoists collapse `fabricengine.StencilsDir(l.HubPath)` into one local shared by the `Directive` call and the template read.
- The Pattern Leaf Invariant's two new entries (`internal/stencilstore`, `internal/stencil`) are consistent across all four statement sites — `leaf_enforcement_test.go`'s header comment, `allowedImports` map, and failure-message parenthetical, plus `CONSTRAINTS.md` — each carrying its own distinct justification (stencil = zero-import leaf; stencilstore = shared infrastructure, not a leaf, verified-acyclic).
- Test coverage is thorough and matches the plan's stated shape: the ten migrated `pattern_test.go` tests keep their original substantive assertions verbatim, the four new tests (lazy read, missing-stencil error, banner strip, stripped-body parity) are present and correctly scoped, and the stamped-fixture helper (`newTestStencilsDir` using `ApplyStamp`) is correctly distinguished from the three consumer packages' raw-byte helpers, with the rationale stated in both. Webster's two missing-stencil error-path tests exist at both hoisted call sites. `loomengine/plan_test.go` was correctly left untouched per the card's explicit instruction, and its PATTERN-active ordering assertion still holds against the new code path.
- No out-of-plan files, no duplicated helpers across batches, no dangling references to the deleted `implementerDirective`/`reviewFixDirective`/`orchestratorDirective` constants outside historical docs (discussion.md, plan files, the design doc's own "Why" narrative, which correctly retains them as before-state description).
- Docs batch: `doc.go`'s "Why the pointer stays relative" section and new "The stencil read path" subsection are accurate and scoped correctly; the design doc's four corrections (fail-loud posture, two-plumbing-free/two-hoisted call sites, banner-strip step, `stencil.md` Related bullet, Test-migration wording) all landed and the file was kept per the recorded discussion decision; the roadmap entry moved to Done with the scheduling sentence dropped and the pointer retargeted to package documentation; the sandbox suite's stencil count was updated to eighteen with no other stale count found in the file.

## Verdict

APPROVE
All three batches align with the plan, shared decisions, and CONSTRAINTS.md; no blocking or nit issues found.
MILL_REVIEW_END
