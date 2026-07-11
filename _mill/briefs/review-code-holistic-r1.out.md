MILL_REVIEW_BEGIN
# Review: Build modelspec - the model-spec parser + registry — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-11
```

## Findings

None. All three batches were verified end-to-end against the plan:

- `internal/modelspec` (Card 1-7): types/vocabularies, `Parse`, `Registry.Resolve`/`builtins`, `LoadRegistry`, `ConfigTemplate`/`template.yaml`, and `leaf_enforcement_test.go` all match the plan's requirements verbatim, including the fail-loud error-naming rules, bracket-over-default precedence, whole-entry override semantics, and the allowlist-style leaf test (mirrors `lyxtest`'s technique but inverted, as required).
- `configreg`/`configsync`/`configcli` (Batch 2): `Module.SeedOnly`, named-field `Modules()` literals with `models` in alphabetical position, and the `configsync.ReconcileAll` seed-only branch (verbatim materialize via `MissingKeys` when absent, untouched when present, ordinary prune preserved for non-seed-only modules) all match Card 8/9 exactly, and tests cover every case the batch specifies (extends, whole-entry override, anti-resurrection, non-seed-only guard).
- `shuttleengine`/`claudeengine` (Batch 3): `Spec.Version` is added parallel to `Effort` with `validate` provably untouched (`TestSpec_Validate_VersionUntouched`); `resolveModelID` implements the exact four-case bare-word/version composition rule with no closed alias list, wired into `Prepare` before any artifact write (mirroring the existing `validateEffort` before-artifacts guarantee), and `buildLaunchCmd` is unchanged as required.
- Docs (Cards 7, 10, 13): `CONSTRAINTS.md`'s Modelspec Leaf Invariant, `docs/reference/model-spec.md`'s built-in-list and version-translation amendments, `docs/overview.md`'s source-tree/shared-infra/config/shuttle bullets, and `docs/shared-libs/README.md`'s entry all match the required text and placement. `docs/roadmap.md` is confirmed untouched per the no-roadmap-edit decision.
- No out-of-plan files: every file in `internal/modelspec/` and `internal/shuttleengine/claudeengine/` matches the batches' Creates/Edits lists exactly, with no surprises.
- No duplicated helpers: `configsync` correctly reuses `yamlengine.MissingKeys`/`fsx.AtomicWriteBytes` rather than reimplementing verbatim-write logic; `claudeengine` reuses `internal/shell` for all pane-shell composition (Shell Mechanics Seam respected).
- Cross-batch contracts hold: `modelspec.ConfigTemplate` (batch 1) is consumed correctly by `configreg` (batch 2); `shuttleengine.Spec.Version` (batch 3) has no code dependency on batch 2, consistent with the stated doc-serialization-only rationale.

## Verdict

APPROVE
All three batches match the plan, shared decisions, and CONSTRAINTS.md with no deviations found.
MILL_REVIEW_END
