MILL_REVIEW_BEGIN
# Review: Deploy cited spec/design docs to target repos like stencils

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [BLOCKING:decision] `stencil list`/`diff`/`promote` have no disposition
**Section:** Technical context — "Decided, not left to the plan — existing-verb coverage" **Issue:** `internal/stencilcli/cli.go:200` registers five verbs (`list`, `validate`, `sync`, `newDiffCmd`, `newPromoteCmd`), not the two the discussion names; `list`, `diff` and `promote` get no specs disposition, yet `reuse-reconcile-policy-unchanged` leans on an operator being able to see a `StateEdited` deployed spec — which only `list`/`diff` surface. **Fix:** State each of the five verbs' specs disposition explicitly (including whether `sync` gains a second `ForceRefresh` over the specs baseDir, not only the generalised commit).

### [BLOCKING:design] Specs registry has no usable `sourceDir` mapping
**Section:** Technical context — "`stencilstore` needs no modification" **Issue:** `reconcile.go:178` derives the worktree source as `filepath.Join(sourceDir, RelPath(name))`, i.e. `<sourceDir>/loom/loom-plan-spec.md`, and `stencilcli/resolveSourceDir` hardcodes `<worktree>/contracts/stencils`; the two travelling docs live at `contracts/specs/loom-plan-spec.md` and `manifest/designs/plan-card-format.md`, in different directories, with a basename that differs from the registered name (`loom-plan-card-format`). The discussion never says what `sourceDir` the specs `Reconcile` passes. **Fix:** Decide and record it — passing `""` (drift detection and `diff --all`/`promote` port-back silently unavailable for specs) or a per-name source mapping — since "no change to `reconcile.go` at all" only holds under the former.

### [BLOCKING:scope] Audit's "complete set" misses non-citation `internal/` tokens
**Section:** Audit result / `citation-enforcement-test` / Testing **Issue:** The enforcement test is specified to scan for tokens under `contracts/`, `manifest/`, `docs/`, `internal/`, but body text carries such tokens that are not citations and are not in the audit table: `loom-template-plan.md:88` (`internal/boardcli#newListCmd`, `internal/boardcli/list.go`), `:92` (`plan:internal/boardcli/rowjson.go#RowJSON`), `:98-99`, and `webster-body-implementer.md:62` (`internal/planglyph`'s `ScopeGuard`). **Fix:** Either restate the enumeration to cover glyph-example/mechanism tokens (with their intended allowlist disposition), or narrow the scan's token rule — and drop the "must fail on exactly the 13 audited occurrences" TDD claim, which is false as written.

### [NIT:consistency] "Geometry structs" threading claim is not uniform
**Section:** Constraints — Told-Geometry Invariant bullet **Issue:** Only `websterengine`'s geometry carries `StencilsDir` (`internal/hubgeom/webstergeom.go:30`); `loomengine.PlanSpec(layout, stencilsDir, cfg, reg)` takes it as a plain parameter and `internal/standalonegeom/stencilsdir.go:19-21` records that the value is "carried by callers as a plain string, never as an engine geometry field". **Fix:** Say specs_dir follows whatever route each consumer already uses for stencilsDir — geometry field for webster, told parameter for loom/shed — rather than implying one mechanism.

### [NIT:design] Allowlist assertion misses a bare `{{` in rubric prose
**Section:** `rubric-marker-allowlist` / Testing **Issue:** The replacement assertion is framed as "the only marker present is `specs_dir`", but the helper now runs `stencil.Fill` over rubric bytes, and any bare `{{` an author writes in prose (not `{{.`) becomes a runtime `parse template` error (`internal/stencil/stencil.go:29-31`) that a marker-name assertion does not catch. **Fix:** Have the rubric test assert the rubric parses under the helper, not only that its marker set is `{specs_dir}`.

## Verdict

REQUEST_CHANGES
Three decided-looking areas — CLI verb coverage, specs source mapping, citation enumeration — are incomplete.
MILL_REVIEW_END
