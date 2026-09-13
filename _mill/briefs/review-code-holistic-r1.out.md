MILL_REVIEW_BEGIN
# Review: Deploy cited spec/design docs to target repos like stencils — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-13
```

## Findings

None. All six batches were read against their plan cards and cross-checked against the source files listed in the manifest.

Verified end-to-end:
- Batch 1: `contracts/specs`/`manifest/designs` embed sites and registry mirror `contracts/stencils`'s shape exactly; `RelPath` family-derivation test added to `internal/stencilstore` with no production change.
- Batch 2: `fabricengine.SpecsDir`/`standalonegeom.SpecsDir` mirror `StencilsDir` precisely, including sibling-non-convergence tests; `websterengine.Geometry` correctly grows to nine fields with both doc comments and both constructors (`hubgeom`, `standalonegeom`) updated in lockstep, and tests assert both the literal join and the one-construction-site property.
- Batch 3: `CommitSeededStencils` generalised over `(subtreeRel, subtreeDir)` with `StencilsSubtreeRel`/`SpecsSubtreeRel` accessors; both production callers (`cmd/lyx/stencilseed.go`, `internal/stencilcli/cli.go`) and the third test call site (`stencilhistory_integration_test.go`) moved together; the new integration test asserts the mutation record's target directory rather than only the pathspec, exactly the partial-regression guard the plan calls for.
- Batch 4: hub pre-run's `seedSubtree` helper and standalone `ResolveStandalone` both seed specs with `sourceDir=""`, matching failure postures (logged-warn vs hard-error) exactly as specified; `lyx stencil list`/`sync` cover both registries with a `kind` tag and combined mutation envelope; integration tests cover idempotence, git-tracked-ness, and the told-`--stencils-dir`-does-not-suppress-specs scenario.
- Batch 5: `shedadapters.ReadRubric` centralises read-strip-fill with the required-marker semantics; both Bouncer rubric sites and `entries_burler.go`'s stencil-route rubric read go through it, while the literal-rubric route is left untouched and separately tested; `SpecsDir` threads through `shedrecipe.Env`, `loomengine.PlanSpec`/`composePlanPrompt`, and `websterengine.RenderForkPrompt`/`RenderRecoveryPrompt`, with every call site in the ~100 affected test cases updated to a real non-empty directory (verified via grep across `plan_test.go`, `template_test.go`, `wiring_test.go`).
- Batch 6: the citation-enforcement test's three-part token rule and the separate `CONSTRAINTS.md` companion assertion are implemented as specified; all eight normative-citation rewrites, the four background-citation path drops, the two `CONSTRAINTS.md` "if present" guards, and the comment-convention rescoping are all present at their stated locations with no stray edits; the rubric marker rule was relaxed to the one-marker allowlist (not deleted) with the new parse-under-render-helper and marker-presence tests added; `docs/code-comment-conventions.md` and `manifest/designs/loom.md` were both repaired to stop claiming a rubric citation/link that no longer exists; `CONSTRAINTS.md`'s Stencil Ownership Invariant and `docs/overview.md`'s stencil module entry were both updated to name the second registry and the per-verb specs coverage; composed-prompt assertions (card 32) were added at all four render sites (`loomengine/plan_test.go`, `websterengine/template_test.go`, `shedadapters/bouncer_judge_test.go`, `shedadapters/bouncer_seed_test.go`) confirming the rendered specs directory appears and no literal marker survives.

No out-of-plan files were found; `burlercli`/`burlerengine`/`webstercli` correctly carry no `SpecsDir` addition, matching the plan's explicit "no change needed" call. No global utility duplication, no cross-batch contract mismatches, no constraint violations (Told-Geometry, Lyxdirs Single-Declarer, Stencil Ownership, Mutation Record, hubforge Fabric-Fixture invariants all respected). Codebase style and error-handling conventions are consistent with the surrounding source throughout.

## Verdict

APPROVE
All six batches faithfully and consistently implement the approved plan with no blocking or nit findings.
MILL_REVIEW_END
