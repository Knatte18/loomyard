# Plan: Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks

```yaml
task: 'Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks'
slug: test-suite-regression
approved: false
started: 20260712-070313
parent: main
root: ""
verify: go test ./... -count=1
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: fix-red-packages
    file: 01-fix-red-packages.md
    depends-on: []
    verify: go test -tags integration ./internal/initengine ./internal/ideengine -count=1
  - number: 2
    name: retier-offline-loop
    file: 02-retier-offline-loop.md
    depends-on: []
    verify: go test -tags integration ./internal/boardcli ./internal/perchcli ./internal/muxcli ./internal/configcli ./cmd/lyx -count=1
  - number: 3
    name: tier-purity-guard
    file: 03-tier-purity-guard.md
    depends-on: [2]
    verify: go test ./cmd/lyx -run TestTierPurity -count=1
  - number: 4
    name: rebaseline-docs
    file: 04-rebaseline-docs.md
    depends-on: [1, 2, 3]
    verify: null
```

## Shared Decisions

### Decision: test-names-are-preserved

- **Decision:** No test function, subtest, or assertion is deleted or renamed
  anywhere in this task. Tests move between build tags (untagged ↔
  `//go:build integration`) and between files within the same package; the
  union of test names under `go test -tags integration ./... -list '.*'` must
  be identical before and after each re-tiering card.
- **Rationale:** The trend log's equivalence-guardrail discipline
  (docs/benchmarks/test-suite-timing.md): re-tiering is placement, never
  coverage change.
- **Applies to:** all batches

### Decision: build-tag-form

- **Decision:** Integration gating uses exactly the line `//go:build integration`
  as the first line of the file, followed by one blank line, then the file's
  doc comment and `package` clause. The package name never changes (test files
  moving within a package keep the package's existing test-package form:
  `boardcli_test` stays `boardcli_test`, `perchcli` stays `perchcli`, etc.).
- **Rationale:** Matches every existing gated file in the repo
  (`internal/idecli/cli_test.go`, `internal/warpengine/*_test.go`, …); Go 1.17+
  build-tag syntax only, no legacy `// +build` second line.
- **Applies to:** batch fix-red-packages (initengine file is already tagged),
  retier-offline-loop

### Decision: helper-placement-under-split-tags

- **Decision:** When a package has both an untagged and an integration-tagged
  test file, a helper used by BOTH lives in the *untagged* file (the tagged
  build sees a superset of files, so untagged symbols are always visible to
  it); a helper used ONLY by tagged tests lives in the *tagged* file
  (otherwise the untagged build breaks on unused imports or drags spawn code
  into Tier 1). Concretely: `runCLI` (boardcli) stays visible to both by
  living in the new untagged file; `seedCwd` (boardcli), `seedPerchFixture`
  (perchcli), `gitLsFiles`/`gitLogOneline` (perchcli) live in tagged files.
- **Rationale:** Same-package build-tag mechanics; verified compiling in the
  2026-07-12 dry run recorded in `_mill/discussion.md`.
- **Applies to:** retier-offline-loop

### Decision: no-verify-cadence-change

- **Decision:** Nothing in this plan changes per-batch `verify:` scope
  conventions or mill config. Batch verifies are narrowly package-scoped as
  usual; the module-wide overview `verify: go test ./... -count=1` is the
  standard Tier 1 offline loop (which this very task returns to fast), not a
  broadened gate.
- **Rationale:** Explicit guardrail from the task body and
  `_mill/discussion.md` § guardrail-verify-stays-package-scoped.
- **Applies to:** all batches

### Decision: moved-code-is-verbatim

- **Decision:** Test functions and helpers relocated between files are moved
  **verbatim** — body, doc comment, and name unchanged; only the surrounding
  file (build tag, imports, file doc comment) differs. Import blocks in both
  files are trimmed/extended to exactly what each file references.
- **Rationale:** Keeps review diffs mechanical and the equivalence guarantee
  trivially checkable.
- **Applies to:** retier-offline-loop

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/crosscompile_test.go`
- `cmd/lyx/main_integration_test.go`
- `cmd/lyx/main_test.go`
- `cmd/lyx/tierpurity_test.go`
- `docs/benchmarks/running-tests.md`
- `docs/benchmarks/test-suite-timing.md`
- `internal/boardcli/cli_test.go`
- `internal/boardcli/cli_unit_test.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configcli/configcli_test.go`
- `internal/configcli/reconcile_integration_test.go`
- `internal/configcli/reconcile_test.go`
- `internal/ideengine/menu.go`
- `internal/initengine/init_test.go`
- `internal/muxcli/cli_integration_test.go`
- `internal/muxcli/cli_test.go`
- `internal/perchcli/cli_integration_test.go`
- `internal/perchcli/cli_test.go`
- `internal/perchcli/run_integration_test.go`
- `internal/perchcli/run_test.go`
