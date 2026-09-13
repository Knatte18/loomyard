MILL_REVIEW_BEGIN
# Review: Deploy cited spec/design docs to target repos like stencils — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-09-13
```

## Findings

### [BLOCKING:scope] CommitSeededStencils signature change misses a third caller
**Location:** Batch 3, cards 9 & 11 (`internal/fabricengine/stencilcommit.go`, `stencilcommit_integration_test.go`).
**Issue:** `internal/fabricengine/stencilhistory_integration_test.go`'s `seedStencil` helper (line 34) calls `fabricengine.CommitSeededStencils(hub.Path, []string{relPath}, message, rec)` with the old 4-arg signature. No card edits this file, so once card 9 widens the signature to 6 args, `go test -tags integration ./internal/fabricengine/...` — batch 3's own verify — fails to compile. Card 9's "leave `stencilhistory.go` alone" note covers a different file and doesn't cover this test.
**Fix:** Add `internal/fabricengine/stencilhistory_integration_test.go` to card 11's Edits, updating the one call site to pass `fabricengine.StencilsSubtreeRel(), fabricengine.StencilsDir(hub.Path)`.

### [BLOCKING:scope] PlanSpec/composePlanPrompt signature change breaks plan_test.go's own build
**Location:** Batch 5, card 21 (`internal/loomengine/plan.go`); batch 6, card 32 (`internal/loomengine/plan_test.go`).
**Issue:** `internal/loomengine/plan_test.go` has ~12 calls to `PlanSpec(layout, newTestStencilsDir(t), cfg, reg)` and 3 to `composePlanPrompt(stencilsDir, decisionRecordPath, ...)`, all at the pre-change arity. Card 21's Edits list only `plan.go`, not the test file, so `go test ./internal/loomengine/...` (batch 5's own verify) fails to compile the moment the signatures widen. Card 32 (batch 6, later) only adds *new* assertions to this file — it never says to fix the ~15 pre-existing calls. Worse: once card 25 (batch 6) inserts the real `{{.specs_dir}}` marker into `loom-template-plan.md`, any of those calls "fixed" with a placeholder empty `specsDir` will newly fail at runtime with an unfilled-required-marker error, so the fix needs a genuine non-empty value threaded through, not just an arity patch.
**Fix:** Add `internal/loomengine/plan_test.go` to card 21's Edits (or a dedicated card between 21 and 32) requiring every existing `PlanSpec`/`composePlanPrompt` call to pass a real, non-empty specs directory (e.g. via a shared test helper), landing before batch 5's verify runs.

### [BLOCKING:scope] RenderForkPrompt/RenderRecoveryPrompt signature change breaks template_test.go
**Location:** Batch 5, card 22 (`internal/websterengine/render.go`); `internal/websterengine/template_test.go` (untouched by any card).
**Issue:** `internal/websterengine/template_test.go` (package `websterengine_test`, compiled into the same `go test ./internal/websterengine/...` binary batch 5's verify runs) has ~24 calls to `websterengine.RenderForkPrompt(...)`/`RenderRecoveryPrompt(...)` at the pre-change arity. Card 22's Edits list `render.go`, `beginbatch.go`, `recoverbatch.go` only; this file isn't in card 22's Edits, card 32's Edits (which names `render_test.go`, a different file), or `00-overview.md`'s "All Files Touched". The same later-breaks-again-at-runtime concern from the PlanSpec finding applies here too, once `webster-body-implementer.md` gains `{{.specs_dir}}`.
**Fix:** Add `internal/websterengine/template_test.go` to card 22's Edits, threading a real specs directory through all ~24 call sites.

### [NIT:consistency] sync's second (specs) commit-failure path is unspecified
**Location:** Batch 4, card 14 (`internal/stencilcli/cli.go`, `sync`).
**Issue:** The card names the specs `ForceRefresh`+`CommitSeededStencils` pair and the two new envelope keys, but doesn't say what happens if the specs half fails after the stencils half already committed — whether it mirrors the existing `errWithRecord` early-return shape.
**Fix:** State explicitly that a specs-half failure returns via `errWithRecord` with the rec snapshot accumulated so far, mirroring the stencils half's existing error handling.

### [NIT:scope] New SpecsDir geometry field ships with no direct assertion
**Location:** Batch 2, card 8 (`internal/hubgeom/webstergeom.go`, `internal/standalonegeom/webstergeom.go`).
**Issue:** Neither `internal/hubgeom/webstergeom_test.go`'s `TestWebsterGeometry` nor `standalonegeom`'s equivalent is extended to assert the new `Geometry.SpecsDir` field's value; the batch verify still passes since these are field-by-field, not exhaustive, comparisons, but the new field ships untested at this layer.
**Fix:** Add one assertion per test (`got.SpecsDir == fabricengine.SpecsDir(hub)` / `== SpecsDir(stateDir)`) alongside the existing `StencilsDir` checks.

## Verdict

REQUEST_CHANGES
Three signature changes (CommitSeededStencils, PlanSpec/composePlanPrompt, RenderForkPrompt/RenderRecoveryPrompt) omit existing test call sites, breaking each batch's own verify.
MILL_REVIEW_END
