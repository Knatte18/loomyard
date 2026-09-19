# Plan: Shed-generic watchdog for ly-drive and loom's CLI verbs

```yaml
task: "Shed-generic watchdog for ly-drive and loom's CLI verbs"
slug: "shed-generic-watchdog"
approved: false
started: "20260919-130542"
parent: "main"
root: ""
verify: go build ./...
discussion_sha: 25a8fe211e385a2001feef2ae3145ddaef3269d7
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shedbuild-shedpaths-hoist
    file: 01-shedbuild-shedpaths-hoist.md
    depends-on: []
    verify: go test ./internal/shedbuild/... ./internal/loomrecipe/... ./internal/lifecyclerecipe/... ./internal/loomcli/... ./internal/lifecyclecli/...
  - number: 2
    name: inner-run-neutralization
    file: 02-inner-run-neutralization.md
    depends-on: [1]
    verify: go test ./internal/lifecycleshed/... ./internal/shedrecipe/... ./internal/lifecyclerecipe/... ./internal/shedbuild/... ./internal/lifecyclecli/... && go test -tags integration ./internal/lifecyclecli/...
  - number: 3
    name: shedverbs-package
    file: 03-shedverbs-package.md
    depends-on: []
    verify: go test ./internal/shedverbs/...
  - number: 4
    name: module-rearm
    file: 04-module-rearm.md
    depends-on: [1, 3]
    verify: go test ./internal/loomcli/... ./internal/lifecyclecli/... ./internal/shedverbs/... && go test -tags integration ./internal/loomcli/... ./internal/lifecyclecli/...
  - number: 5
    name: shed-subtree
    file: 05-shed-subtree.md
    depends-on: [2, 4]
    verify: go test ./internal/shedcli/... ./cmd/lyx/... ./internal/loomcli/... ./internal/lifecyclecli/...
  - number: 6
    name: docs-invariant-skill
    file: 06-docs-invariant-skill.md
    depends-on: [5]
    verify: go test ./cmd/lyx/... ./tools/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: run-result-type-is-shedengine-Result

- **Decision:** the `PostRun` hook's result parameter is typed `shedengine.Result`, not `shedengine.RunResult`.
- **Rationale:** `_mill/discussion.md`'s `optional-hooks-for-module-specific-work` Decision writes `PostRun func(ctx, result shedengine.RunResult, runErr error)`, but `internal/shedengine/shed.go` declares the type as `Result` and `func (s *Shed) Run(ctx context.Context) (Result, error)` returns it.
  There is no `RunResult` identifier in the tree.
  The discussion's name is a slip, not a request to rename the engine type — `shedengine` is explicitly out of scope, so the plan uses the real name everywhere.
- **Applies to:** all batches

### Decision: build-time-texts-run-time-spec

- **Decision:** `shedverbs` splits its arming across two arguments with different lifetimes.
  `shedverbs.VerbTexts` carries each verb's `Use`/`Short`/`Long` strings and is passed by value at `Command()`-construction time;
  `*shedverbs.Spec` carries the resolved paths, told strings, absent-file disposition, hooks and `BuildShed`, and is filled in place by the arming module's own `PersistentPreRunE`.
  The constructor is `shedverbs.Verbs(texts VerbTexts, spec *Spec) []*cobra.Command`.
- **Rationale:** cobra builds the whole command tree before any `PersistentPreRunE` runs, so help text must exist at build time — `cmd/lyx/helptree_test.go`, `drift_test.go` (non-blank `Short` on every command) and `longlist_test.go` all read the tree with no invocation at all, and `lyx loom --help` must print loom's own byte-identical text with nothing resolved.
  Resolution-dependent values cannot travel the same way, because they are not known until cwd and `--recipe` have been read.
  Filling a pointed-to `Spec` in the pre-run is exactly how `loomCLI` and `lifecycleCLI` already work today: the receiver is populated by `resolvePersistentPreRun` and every `RunE` closure reads it at run time.
- **Applies to:** all batches

### Decision: exported-Arm-is-the-single-resolution-entry-point

- **Decision:** each arming module exposes `Arm(cwd string, verb string, args []string) (shedverbs.Spec, error)` — `loomcli.Arm` and `lifecyclecli.Arm` — and its own `PersistentPreRunE` calls that same function and assigns the result through its spec pointer (`*c.spec = armed`).
  No second resolution path exists.
- **Rationale:** `_mill/discussion.md`'s `generic-package-resolves-nothing` Decision requires it: `cmd/lyx/main.go` sets `cobra.EnableTraverseRunHooks = true`, so the hooks that fire are the executed command's *ancestor* chain, and `lifecycle` is not an ancestor of `shed run` — lifecycle's `lyxcwd.Resolve`, its `fabricengine.PrimeName` lookup, its non-prime refusal and its `args[0]` slug read would all be skipped under `lyx shed run --recipe lifecycle` unless both paths call one shared function.
- **Applies to:** shedverbs-package, module-rearm, shed-subtree

### Decision: no-surface-change-to-existing-verbs

- **Decision:** `lyx loom run|step|status|pause` and `lyx lifecycle run|status` keep their present `Use`, `Short`, `Long`, flags, exit codes, envelope key sets and refusal wording byte-for-byte.
  The only agreed surface changes in this whole task are the three additive ones `_mill/discussion.md`'s Testing section names: `lyx lifecycle run`'s envelope gains `history_length`, `lyx lifecycle` gains `pause`, and `lyx lifecycle status` gains `--watch`/`--interval`.
- **Rationale:** the existing behavioural suites are the proof the extraction preserved behaviour, and they only prove it if they keep passing unchanged.
  A changed assertion in `internal/loomcli`'s `step_test.go`/`status_test.go`/`smoke_test.go`/`wiring_test.go`/`parity_test.go` or `internal/lifecyclecli`'s `run_test.go`/`lifecycle_integration_test.go` is a signal the extraction changed behaviour and must be justified in the card that changes it.
- **Applies to:** all batches

### Decision: told-strings-never-derived

- **Decision:** every user-facing string that differs between the two shipped consumers travels as a told field on `shedverbs.Spec`, never as a value the generic body composes from another field.
  That covers the `ErrShedBusy` message (per verb), the status decode-error prefix, the absent-file disposition, `pause`'s absent-file refusal, and the `--watch` status label.
  An empty told busy message means passthrough — the generic body reports `err.Error()` verbatim.
- **Rationale:** `_mill/discussion.md`'s `busy-refusal-comes-from-the-arming-spec` Decision: the prefix is not derivable from the status label (`loom`/`lifecycle` against `loom:`/`lifecyclecli:`), and collapsing the three shipped `ErrShedBusy` treatments to one would be a user-visible regression in two of them.
- **Applies to:** shedverbs-package, module-rearm, shed-subtree

### Decision: go-test-verify-no-pythonpath-prefix

- **Decision:** every `verify:` command in this plan is a bare `go test` invocation with no `PYTHONPATH=` prefix.
- **Rationale:** the `verify-not-isolated` validator check applies the `PYTHONPATH= ` prefix rule to Python/mill projects only;
  this is a Go repository and the native runner is used directly.
  Batches touching `internal/loomcli` or `internal/lifecyclecli` chain a second `go test -tags integration` invocation over the same packages, because both carry `//go:build integration` suites (`lifecycle_integration_test.go`, `testmain_integration_test.go`, `wiring_commitstatus_integration_test.go`) that a bare `go test` never compiles.
- **Applies to:** all batches

### Decision: done-gate-already-configured

- **Decision:** `pipeline.done_gate` is left exactly as the hub already sets it — `go test ./... && go test -tags integration ./...` — and this plan adds nothing to it.
- **Rationale:** the batch-scoped `verify:` commands above deliberately do not cover the whole module tree, which is the case the done gate exists for, and the hub's existing value already runs both tiers repo-wide.
  `golangci-lint` is not added: this plan authors no config change, and the existing value already catches a cross-package regression through the full test suite.
- **Applies to:** all batches

### Decision: build-prerequisite-cgo

- **Decision:** every batch's `verify:` assumes `CGO_ENABLED=1` and a C compiler on `PATH`.
- **Rationale:** `lyx` links quarry's tree-sitter grammars through cgo, so `go test ./...` over any package in this module requires it;
  it is already the default on a developer machine with a compiler installed and needs no per-batch action.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/main.go`
- `cmd/lyx/sandbox_coverage_test.go`
- `contracts/recipes/lifecycle-recipe.yaml`
- `docs/overview.md`
- `internal/lifecyclecli/arm.go`
- `internal/lifecyclecli/cli.go`
- `internal/lifecyclecli/cli_test.go`
- `internal/lifecyclecli/lifecycle_integration_test.go`
- `internal/lifecyclecli/run.go`
- `internal/lifecyclecli/run_test.go`
- `internal/lifecyclecli/status.go`
- `internal/lifecyclecli/wire.go`
- `internal/lifecyclecli/wire_test.go`
- `internal/lifecyclerecipe/coverage_guard_test.go`
- `internal/lifecyclerecipe/lifecyclerecipe.go`
- `internal/lifecyclerecipe/names.go`
- `internal/lifecyclerecipe/recipe_test.go`
- `internal/lifecycleshed/deps.go`
- `internal/lifecycleshed/innerrun.go`
- `internal/lifecycleshed/innerrun_test.go`
- `internal/loomcli/arm.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/pause.go`
- `internal/loomcli/run.go`
- `internal/loomcli/status.go`
- `internal/loomcli/step.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomrecipe/loomrecipe.go`
- `internal/shedbuild/fixture_test.go`
- `internal/shedbuild/newshed.go`
- `internal/shedbuild/newshed_test.go`
- `internal/shedbuild/seam_enforcement_test.go`
- `internal/shedcli/cli.go`
- `internal/shedcli/cli_test.go`
- `internal/shedcli/doc.go`
- `internal/shedcli/parity_test.go`
- `internal/shedcli/table.go`
- `internal/shedcli/table_test.go`
- `internal/shedrecipe/entries_lifecycle.go`
- `internal/shedrecipe/entries_lifecycle_test.go`
- `internal/shedrecipe/recipe.go`
- `internal/shedrecipe/registry.go`
- `internal/shedrecipe/registry_test.go`
- `internal/shedverbs/doc.go`
- `internal/shedverbs/pause.go`
- `internal/shedverbs/pause_test.go`
- `internal/shedverbs/run.go`
- `internal/shedverbs/run_test.go`
- `internal/shedverbs/seam_enforcement_test.go`
- `internal/shedverbs/spec.go`
- `internal/shedverbs/status.go`
- `internal/shedverbs/status_test.go`
- `internal/shedverbs/step.go`
- `internal/shedverbs/step_test.go`
- `internal/shedverbs/testsupport_test.go`
- `internal/shedverbs/verbs.go`
- `manifest/designs/shed-generic-watchdog.md`
- `manifest/roadmap.md`
- `plugins/ly/skills/INDEX.md`
- `plugins/ly/skills/ly-drive/SKILL.md`
