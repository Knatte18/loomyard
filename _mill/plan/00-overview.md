# Plan: Rename internal/paths to internal/hubgeometry

```yaml
task: "Rename internal/paths to internal/hubgeometry"
slug: rename-paths-to-hubgeometry
approved: false
started: 20260630-161302
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: code-rename
    file: 01-code-rename.md
    depends-on: []
    verify: go build ./... && go test ./...
  - number: 2
    name: docs
    file: 02-docs.md
    depends-on: [1]
    verify: null
```

## Shared Decisions

### Decision: package-name-hubgeometry

- **Decision:** Rename the package `internal/paths` → `internal/hubgeometry`: directory,
  `package paths` → `package hubgeometry` (and `package paths_test` → `package
  hubgeometry_test` for the black-box test files), and every `paths.` package-selector
  qualifier → `hubgeometry.`. No exported symbol, `Layout` field/method, constant, or
  resolution behaviour changes. `hubgeometry` (not the shorter `geometry`) was chosen for
  precision and the hub-centric model.
- **Rationale:** `paths` names the data type, not the responsibility — the package owns
  hub-topology geometry. Behaviour-preserving rename only.
- **Applies to:** all batches

### Decision: rename-via-git-mv

- **Decision:** Perform every directory/file rename with `git mv` FIRST, then make only
  surgical edits (package clause, imports, qualifiers, comments, the two guard literals).
  No full-file rewrites. The package files `paths.go`/`paths_test.go`/`paths_unit_test.go`
  are renamed to `hubgeometry.go`/`hubgeometry_test.go`/`hubgeometry_unit_test.go`; the
  doc `docs/shared-libs/paths.md` → `hubgeometry.md`.
- **Rationale:** Preserves git rename history/blame and keeps the diff reviewable as a
  rename + small edits. Standing repo convention.
- **Applies to:** all batches

### Decision: comprehensive-reference-sweep

- **Decision:** Update **every** file that names `internal/paths` or the "Path Invariant"
  invariant — production code, test code, string literals, comments, `CONSTRAINTS.md`,
  `CLAUDE.md`, and all docs — not only the proposal's explicit list. Verified end-state:
  `grep -rn "internal/paths" .`, `grep -rn "Path Invariant" .`, `grep -rn "package
  paths(_test)?\b" .`, `grep -rn "#path-invariant" .`, `grep -rn '"paths.go"' .` all
  return nothing (excluding `_mill/`).
- **Rationale:** A rename that leaves dangling old-name references defeats the point; the
  name is the deliverable. Operator decision in discussion.
- **Applies to:** all batches

### Decision: invariant-rename

- **Decision:** Rename the invariant "Path Invariant" → "Hub Geometry Invariant" in
  `CONSTRAINTS.md`, `CLAUDE.md`, and the `## Path Invariants` heading in
  `docs/overview.md` (→ `## Hub Geometry Invariants`). The heading's GitHub anchor slug
  changes from `#path-invariants` to `#hub-geometry-invariants`, so the dependent link in
  `docs/modules/loom.md` is updated in the same card.
- **Rationale:** The invariant is named after the package; both rename together. The
  heading-anchor + cross-doc link is load-bearing and evades the text greps.
- **Applies to:** docs

### Decision: per-batch-implementer-model-via-orchestrator-flip

- **Decision:** The mill-go orchestrator switches `roles.implementer.model` in
  `.millhouse/config.local.yaml` **between batches** (mill re-loads config on every batch
  dispatch; there is no per-batch model field). Batch 1 (code-rename) runs on **opushigh**
  — the large, unsplittable atomic rename whose context Opus holds comfortably; before
  Batch 2 (docs) the orchestrator flips the key to **sonnethigh** (3 trivial prose cards).
  The key is currently set to `opushigh` (ready for Batch 1).
- **Rationale:** The atomic rename cannot be split across batches (a half-renamed tree
  does not `go build`), so its importer set lands in one batch whose size estimate exceeds
  the default. Running it on Opus makes the raised `max_batch_context_tokens` honest while
  keeping Sonnet for the cheap docs work. No mill code change required.
- **Applies to:** all batches (orchestration)

### Decision: context-budget-override

- **Decision:** `pipeline.max_batch_context_tokens` is raised to `200000` in
  `.millhouse/config.local.yaml` (gitignored, per-worktree). The code-rename batch is also
  executed as a scripted bulk find-replace (git mv + sed/gofmt sweep + `go build`) so real
  context stays well below the estimate.
- **Rationale:** The unsplittable code-rename batch's byte-estimate (~142k) exceeds the
  120k default and would trip `batch-oversized`; with Batch 1 on Opus the 200k ceiling is
  honest. Renamed files ride in `Moves:` and are not counted by the validator.
- **Applies to:** code-rename

## All Files Touched

_Full union of every `Edits:` / `Creates:` plus every `Moves:` target across
every batch, sorted alphabetically. mill-go reads this to warn if two parallel
batches touch the same file. The old pre-rename paths (`internal/paths/*`,
`docs/shared-libs/paths.md`) are the `Moves:` sources and are intentionally absent._

- `CLAUDE.md`
- `CONSTRAINTS.md`
- `cmd/lyx/exitcode_test.go`
- `cmd/lyx/main_test.go`
- `cmd/lyx/registration_test.go`
- `docs/benchmarks/test-suite-timing.md`
- `docs/modules/loom.md`
- `docs/modules/mux.md`
- `docs/overview.md`
- `docs/roadmap.md`
- `docs/shared-libs/README.md`
- `docs/shared-libs/envsource.md`
- `docs/shared-libs/hubgeometry.md`
- `internal/boardcli/cli.go`
- `internal/boardcli/cli_test.go`
- `internal/boardengine/boardtest/bench_test.go`
- `internal/boardengine/config_test.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configcli/configcli_test.go`
- `internal/configcli/menu.go`
- `internal/configcli/reconcile_test.go`
- `internal/configengine/config.go`
- `internal/configengine/config_test.go`
- `internal/configengine/edit.go`
- `internal/configengine/edit_test.go`
- `internal/configsync/configsync.go`
- `internal/configsync/configsync_test.go`
- `internal/envsource/envsource.go`
- `internal/hubgeometry/codeguide_guard_test.go`
- `internal/hubgeometry/enforcement_test.go`
- `internal/hubgeometry/geometry_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_test.go`
- `internal/hubgeometry/hubgeometry_unit_test.go`
- `internal/hubgeometry/weft_test.go`
- `internal/hubgeometry/worktreelist.go`
- `internal/hubgeometry/worktreelist_test.go`
- `internal/idecli/cli.go`
- `internal/ideengine/menu.go`
- `internal/ideengine/menu_test.go`
- `internal/ideengine/spawn.go`
- `internal/ideengine/spawn_test.go`
- `internal/initcli/initcli.go`
- `internal/initcli/initcli_test.go`
- `internal/lyxtest/doc.go`
- `internal/lyxtest/leaf_enforcement_test.go`
- `internal/lyxtest/lyxtest.go`
- `internal/lyxtest/lyxtest_test.go`
- `internal/muxpoccli/cli.go`
- `internal/vscode/color.go`
- `internal/vscode/color_test.go`
- `internal/warpcli/clone.go`
- `internal/warpcli/warp.go`
- `internal/warpcli/warp_test.go`
- `internal/warpengine/add.go`
- `internal/warpengine/checkout.go`
- `internal/warpengine/cleanup.go`
- `internal/warpengine/clone.go`
- `internal/warpengine/clone_integration_test.go`
- `internal/warpengine/config_test.go`
- `internal/warpengine/drift.go`
- `internal/warpengine/drift_test.go`
- `internal/warpengine/hook.go`
- `internal/warpengine/hook_test.go`
- `internal/warpengine/junction.go`
- `internal/warpengine/launchers.go`
- `internal/warpengine/launchers_test.go`
- `internal/warpengine/list.go`
- `internal/warpengine/portals.go`
- `internal/warpengine/portals_test.go`
- `internal/warpengine/prune.go`
- `internal/warpengine/reconcile.go`
- `internal/warpengine/reconcile_test.go`
- `internal/warpengine/remove.go`
- `internal/warpengine/remove_test.go`
- `internal/warpengine/status.go`
- `internal/warpengine/status_test.go`
- `internal/warpengine/weftwiring.go`
- `internal/weftcli/cli.go`
- `internal/weftcli/cli_test.go`
- `internal/weftengine/config_test.go`
- `internal/weftengine/weft_integration_test.go`
