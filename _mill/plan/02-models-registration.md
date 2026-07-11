# Batch: models-registration

```yaml
task: Build modelspec - the model-spec parser + registry
batch: models-registration
number: 2
cards: 3
verify: go test ./internal/configreg/... ./internal/configsync/... ./internal/configcli/... ./internal/initengine/... ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

Registers `models` as a config module with seed-only reconcile semantics: `configreg`
gains a `SeedOnly` flag and the `models` entry (template from batch 1's
`modelspec.ConfigTemplate`); `configsync.ReconcileAll` honors the flag — materialize
when absent, never rewrite when present. This closes the round-1 discussion-review gap:
default reconcile prunes keys absent from the template
(`TestReconcileAll_DropsStaleMuxClaudeKey`), which would delete operator-added aliases
and break the pinned new-model-without-recompile requirement. `lyx config` list/edit and
`lyx init` pick the module up mechanically through `configreg.Modules()` /
`configreg.Names()`.

Batch-local notes: `internal/initengine/init_test.go` is integration-tagged
(`//go:build integration`) and already stale relative to the current six-module registry
(it asserts `len(result.Modules) != 3`); the default `go test` gate does not compile it
and this batch does not repair it — pre-existing rot unrelated to this change.
`internal/configcli` tests iterate `configreg.Names()`/`Modules()` dynamically and need
no pinned-list edits; the one pinned list is `configreg_test.go` (card 8).

## Cards

### Card 8: configreg — SeedOnly flag and models entry

- **Context:**
  - `internal/configsync/configsync.go`
  - `internal/configcli/configcli.go`
  - `internal/modelspec/template.go`
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `SeedOnly bool` to `configreg.Module` with a doc comment:
  seed-only modules have an open-ended key set owned by the operator; `configsync`
  materializes their template when the file is absent and never rewrites a present file
  (neither adds nor prunes keys). Convert the `Modules()` composite literals from
  positional to named fields (`{Name: "board", Template: boardengine.ConfigTemplate}` …)
  and insert `{Name: "models", Template: modelspec.ConfigTemplate, SeedOnly: true}` in
  alphabetical position (between `board` and `mux`). Import
  `github.com/Knatte18/loomyard/internal/modelspec`; `modelspec.ConfigTemplate` has
  signature `func() string` (created in card 5). Update `TestNames` in
  `configreg_test.go` to `{"board", "models", "mux", "perch", "shuttle", "warp", "weft"}`
  and add a test asserting the `models` module (and only it) has `SeedOnly == true`.
- **Commit:** `feat(configreg): register models module with seed-only flag`

### Card 9: configsync — honor SeedOnly

- **Context:**
  - `internal/configreg/configreg.go`
  - `internal/yamlengine/reconcile.go`
  - `internal/modelspec/template.go`
- **Edits:**
  - `internal/configsync/configsync.go`
  - `internal/configsync/configsync_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `ReconcileAll`, before calling `yamlengine.Reconcile` for a
  module: when `m.SeedOnly` and the file EXISTS, append
  `Result{Module: m.Name, Applied: false}` (no Added, no Removed — the file is never
  parsed, diffed, or written) and continue to the next module. When `m.SeedOnly` and the
  file is ABSENT, do NOT route through `yamlengine.Reconcile` — its merged output is
  marshalled from the node tree and is only *equivalent* to the template, not
  byte-identical (indentation, blank lines, and comment placement get normalized, which
  would degrade the operator-facing annotated seed). Instead: compute
  `added := yamlengine.MissingKeys([]byte(m.Template()), nil)` (empty existing → every
  template leaf key-path), and when `apply` write `[]byte(m.Template())` VERBATIM via
  `fsx.AtomicWriteBytes`, reporting `Result{Module: m.Name, Added: added, Applied: true}`
  (`Applied: false` and no write on a dry run, same `Added` report). `Removed` stays
  empty, so initengine's existing `status == "created"` heuristic
  (`Applied && len(Added) > 0 && len(Removed) == 0`) behaves correctly with zero
  initengine changes. Update the `ReconcileAll` doc comment to document the seed-only
  branch (verbatim-materialize when absent, untouched when present). Tests (extend `configsync_test.go`, `t.TempDir()` +
  `hubgeometry.ConfigFile` paths, table/subtest style matching the file): (a) absent
  models.yaml + `apply=true` → file materialized byte-identical to
  `modelspec.ConfigTemplate()`, `Applied=true`; (b) present models.yaml containing an
  operator-added alias (`zephyr:` with `engine`/`model`) + `apply=true` → file bytes
  UNCHANGED and `Applied=false` (the pinned anti-prune property); (c) present
  models.yaml with a template key deliberately removed (`sonnet:` block's
  `defaults:`/`effort` deleted) + `apply=true` → file bytes UNCHANGED (no silent
  resurrection); (d) a non-seed-only module in the same run still gets the existing
  prune behavior (guard against over-broad skip).
- **Commit:** `feat(configsync): seed-only modules materialize once, never rewritten`

### Card 10: Overview config bullet — seed-only nuance

- **Context:**
  - `internal/configsync/configsync.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Amend the `config` module bullet in `docs/overview.md`'s
  `## Modules` section: `lyx config reconcile` reconciles module configs against live
  templates EXCEPT seed-only modules (today: `models`), which are materialized when
  absent and never rewritten — the file is operator-owned; one sentence, matching the
  bullet's existing density. No other overview edits in this batch (tree/infra lines are
  card 7; the shuttle bullet is card 13).
- **Commit:** `docs(config): document seed-only reconcile semantics for models.yaml`

## Batch Tests

`verify:` covers the two edited packages plus the mechanical consumers:
`./internal/configreg/...` (pinned names + SeedOnly), `./internal/configsync/...`
(seed-only branch + existing prune tests), `./internal/configcli/...` (help text
`Known modules:` generated from `Names()` — its sync tests iterate dynamically),
`./internal/initengine/...` (unit surface; integration-tagged tests are excluded by
default build tags), `./cmd/lyx/...` (helptree/drift/longlist guards prove no cobra
surface changed). The wider-than-edited scope is justified: `configreg.Names()` output
feeds help text and iteration in exactly these consumer packages.
