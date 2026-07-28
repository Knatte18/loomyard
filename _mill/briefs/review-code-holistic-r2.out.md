MILL_REVIEW_BEGIN
# Review: PATTERN wiring: conditional constraint-injection into every agent — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-28
```

## Findings

No findings. I read all seven batch files, the overview, and every source file in the manifest (stencil, hubgeometry, fabricengine junction/reconcile/status/drift/remove/weftwiring/weftgit/config/template.yaml, initengine/initcli, loomengine preflight/plan, the new internal/pattern package plus its leaf-enforcement test, and all five prompt-template + engine wiring sites in builder/burler/webster/loom) and cross-checked each against its batch's `Requirements:` text.

Cross-batch contracts hold end to end: `HostJunctions`/`HostJunctionsHere` return `_lyx` then `_pattern` consistently everywhere (hubgeometry.go, weft_test.go, hubgeometry_test.go); the per-junction generalisation in `seedLyxJunction`/`unseedJunctionRecords`/`removeJunctionRecords`/`checkJunctionHealth`/`PairInSync` all loop the same two-entry accessor with junction-named reasons, and `loomengine/preflight.go`'s substring reclassification matches the reworded strings exactly. The pathspec-tolerance filter (`weftPathspecFilter`/`entryMatchesWeft`) lands before the widened `_lyx _pattern` default and both are exercised together in `weftgit_pathspec_integration_test.go`'s `TestCommitWeft_WidenedDefaultPathspec_LyxChangeStillCommitsWithNoPattern`, covering both the wholly-absent and present-but-empty `_pattern` shapes. `internal/pattern` is a genuine stdlib+hubgeometry leaf (verified via its own allowlist test) and its `Directive`/`Role` API is consumed identically at all five injection sites (`builderengine/spawn.go`, `burlerengine/engine.go`+`prompt.go`, `websterengine/render.go` for both fork and Master, `loomengine/plan.go`), each with the correct `Role` (Implementer/ReviewFix/Orchestrator) and each via `stencil.FillOptional` with `pattern_directive` in the optional list. The `RenderForkPrompt`/`RenderMasterPrompt` signature changes are fully propagated — all ten `RenderForkPrompt` test call sites and the sole `RenderMasterPrompt` call site pass a Layout, and the `{{.worktree_root}}`/`l.Cwd`-vs-`l.WorktreeRoot` distinction card 26 warns about is correctly preserved.

Docs land in the same batch that invalidates them throughout: `CONSTRAINTS.md`'s Hub Geometry and Pattern Leaf invariants, `docs/overview.md`'s junction-model list (including the pre-existing `_raddle` junction claim correction), `docs/shared-libs/hubgeometry.md` and `stencil.md`, `internal/fabricengine/doc.go`'s pathspec/junction-upgrade consequences, and `internal/fabriccli/fabric.go`'s reconcile/remove/pairs/checkout `Long` text are all current. `manifest/designs/pattern.md`'s six corrections (rename-mechanic precedent, junction-ownership split, second-not-third-junction correction, weft-persistence note, and the settled-four/open-one rewrite of Open Questions) and `manifest/roadmap.md`'s updated Planned entry (wiring landed, content migration still outstanding, item not moved to Done) both match the plan precisely. No out-of-plan files were found, and no duplicated helper logic across batches — the `unseedJunctionRecords`/`removeJunctionRecords` split (abort-vs-best-effort) is deliberately divergent and documented as such in both files.

## Verdict

APPROVE
Full implementation matches the seven-batch plan precisely; no constraint violations, drift, or gaps found across any batch.
MILL_REVIEW_END
