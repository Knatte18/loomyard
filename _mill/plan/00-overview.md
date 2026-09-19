# Plan: Worktree spawn/teardown as Shed producers

```yaml
task: "Worktree spawn/teardown as Shed producers"
slug: "worktree-lifecycle-shed-producers"
approved: true
started: "20260919-051253"
parent: "main"
root: ""
verify: null
discussion_sha: "e06b75bb9266e68f5f2071bbec6842d5e49909ee"
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: lifecycleshed producers
    file: 01-lifecycleshed-producers.md
    depends-on: []
    verify: go test ./internal/lifecycleshed/...
  - number: 2
    name: shedrecipe lifecycle entries
    file: 02-shedrecipe-entries.md
    depends-on: [1]
    verify: go test ./internal/shedrecipe/... ./internal/shedbuild/...
  - number: 3
    name: lifecycle recipe and coverage guard
    file: 03-lifecycle-recipe-and-coverage.md
    depends-on: [2]
    verify: go test ./internal/lifecyclerecipe/... ./internal/loomrecipe/... ./internal/shedrecipe/... ./internal/shedbuild/... ./contracts/...
  - number: 4
    name: loom run --no-attach
    file: 04-loom-run-no-attach.md
    depends-on: []
    verify: go test ./internal/loomcli/...
  - number: 5
    name: lifecyclecli module
    file: 05-lifecyclecli-module.md
    depends-on: [3, 4]
    verify: go test ./internal/lifecyclecli/... && go test -tags integration ./internal/lifecyclecli/...
  - number: 6
    name: registration and docs
    file: 06-registration-and-docs.md
    depends-on: [5]
    verify: go test ./cmd/lyx/...
```

## Shared Decisions

### Decision: stuck-reason-carrier-is-log-and-file

- **Decision:** A lifecycle producer surfaces its stuck reason through an `internal/logger` warning plus a one-line reason file under a told scratch directory, copying `internal/landingshed/stuck.go`'s shipped `reportStuck` helper.
  That told directory reaches the producers as a new `shedrecipe.Env` field, `ScratchDir`, filled by the CLI tier with the per-slug lifecycle directory.
- **Rationale:** The discussion states repeatedly that a `Stuck` "records the reason in the lifecycle status file where a human can read it", and that is not what the engine does: on a stuck verdict with no bounce target it persists the fixed string `stuck with no OnStuck target`, discarding whatever the producer knew.
  Every reason requirement this task states — naming which teardown half failed, naming the lock path, carrying the polled run's own error and producer, naming the interval and attempt count — is unsatisfiable through the engine's own reason field, and `internal/landingshed`'s two producers already solved exactly this with exactly this helper.
  The `Env.ScratchDir` field is the price: a producer bound by the Told-Geometry Invariant cannot derive that directory itself.
- **Applies to:** all batches

### Decision: primelock-carries-its-told-path

- **Decision:** `lifecycleshed.PrimeLock` is a struct carrying both `Acquire` and the told absolute `Path`, not the bare `Acquire` closure the discussion names.
- **Rationale:** The discussion requires a contention refusal whose reason names the lock path, and correctly rules out naming the holding slug, which nothing can report.
  An `Acquire` closure alone returns a release, a bool and an error — no path — so the producer would have nothing to put in the reason.
  Carrying the path as a told value keeps the derivation in the CLI tier where it belongs and costs one field.
- **Applies to:** lifecycleshed producers, shedrecipe lifecycle entries, lifecyclecli module

### Decision: loomrun-seams-add-a-sleep-and-carry-the-whole-status

- **Decision:** `LoomRunDeps.ReadStatus` returns `shedengine.Status` rather than a bare state string, and `LoomRunDeps` carries a `Sleep func(time.Duration)` seam alongside `Now`.
- **Rationale:** The verdict table requires the stuck reason to carry the polled run's own error field and current producer, which a bare state string cannot supply, and `internal/state`'s strict read already returns that exact struct, so the seam costs no new type and makes the CLI-side closure a one-liner.
  A fake clock alone does not keep the attempt-cap test out of real time: without a sleep seam the test would sleep the real configured interval on every attempt, and the five-second default would put an untagged file over the Test Tier Purity Invariant's one-second sleep threshold on the second attempt.
- **Applies to:** lifecycleshed producers, shedrecipe lifecycle entries, lifecyclecli module

### Decision: design-doc-is-deleted-not-rewritten

- **Decision:** `manifest/designs/worktree-lifecycle-shed-producers.md` is deleted when this module lands, and its corrections — chiefly that the self-heal item is not a hard dependency — go into the new packages' own documentation and the roadmap's Done entry.
- **Rationale:** The discussion's Scope calls for rewriting that doc to an as-built design.
  The Documentation Lifecycle, recorded in `docs/overview.md` and referenced from `CONSTRAINTS.md`, says the opposite for exactly this doc class: a module-design doc is a draft for a planned, not-yet-built module and is deleted when the module lands, with its purpose and rationale moving into the Go package header comment next to the code.
  `manifest/roadmap.md`'s own Maintenance section repeats the rule and says a Done entry points at the module's package documentation instead.
  Ten shipped modules are already recorded in `docs/overview.md` as "module doc deleted per the documentation lifecycle", so rewriting this one would be the exception, not the convention.
- **Applies to:** registration and docs

### Decision: vscode-embedding-is-out-of-scope

- **Decision:** The optional VS Code embedding the roadmap's Planned entry names as one of this item's four ingredients is deliberately not delivered and not deferred silently: it stays an operator action outside the driven run, and the Done entry says so, pointing at the Someday entry named `VS Code as opt-in per worktree, not spun up by default` as where that work lives.
- **Rationale:** The Planned entry folds four ingredients into one run — the create verb, reed's self-healing bootstrap, optional VS Code embedding, and the task's own producer list — and three of them are delivered here.
  Embedding is not, because a VS Code window is a passive client attaching to a session the driven path brings up anyway, so it is an interactive convenience with no place in a driven run, which is what the discussion's own Out list records.
  Stating the disposition matters because the Done entry would otherwise read as though all four shipped.
- **Applies to:** registration and docs

### Decision: sandbox-runners-need-no-edit

- **Decision:** The sandbox work is one new scenario in `tools/sandbox/SANDBOX-FABRIC-SUITE.md` and its verdict line in that file's report template.
  Nothing under `sandbox/posix/` or `sandbox/win/` changes.
- **Rationale:** The discussion's Scope calls for "the matching runner step" in both runner directories.
  Those files are one-line launchers, one per suite, that invoke the sandbox tool with a suite name; they carry no per-scenario steps at all.
  The suite document itself is embedded into the tool, and the tool enumerates suites rather than scenarios, so a new scenario inside an existing suite is picked up with no Go or script change.
- **Applies to:** registration and docs

### Decision: verify-commands-are-native-go

- **Decision:** Every `verify:` command in this plan is a bare `go test` invocation with no `PYTHONPATH=` prefix.
- **Rationale:** The prefix exists to stop a Python test subprocess inheriting a cache module path.
  This is a Go repository with no Python test surface, and the validator's isolation check applies the prefix rule conditionally on project language.
- **Applies to:** all batches

### Decision: cgo-is-a-build-prerequisite

- **Decision:** Every batch's `verify:` assumes `CGO_ENABLED=1` and a C compiler on `PATH`.
- **Rationale:** The binary links tree-sitter grammars through cgo, so any `go build` or `go test` in this repository needs both.
  Nothing in this task changes that, and no batch should try to work around a missing compiler — a failure there is an environment problem, not a plan defect.
- **Applies to:** all batches

### Decision: the-three-new-packages-may-not-name-either-side-of-the-pair

- **Decision:** No identifier, string literal, or comment in `internal/lifecycleshed`, `internal/lifecyclerecipe` or `internal/lifecyclecli` names either side of the worktree pair.
  Write "the task worktree", "the pair", and "the hub's prime worktree".
- **Rationale:** The Fabric Vocabulary Invariant's owner set does not include these three packages, and the ban is machine-enforced by an AST walk over every identifier in `internal/lyxcwd`'s own enforcement test, so a violation is a build failure rather than a review note.
  `internal/landingshed/doc.go` records the same constraint for itself and is the model for how to phrase around it.
- **Applies to:** all batches

### Decision: one-card-one-commit

- **Decision:** Each card produces exactly one commit, using that card's own `Commit:` line, and no card's work is folded into another card's commit.
- **Rationale:** This is the execution convention the orchestrator applies, and the harness never amends a pushed commit.
  Where two changes genuinely must land together — the registration line and the guards it trips, the coverage-guard halves — they are written as one card rather than as two cards linked by an instruction.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens)._

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/notransients_test.go`
- `contracts/recipes/lifecycle-recipe.yaml`
- `contracts/recipes/recipes.go`
- `docs/overview.md`
- `internal/lifecyclecli/cli.go`
- `internal/lifecyclecli/cli_test.go`
- `internal/lifecyclecli/lifecycle_integration_test.go`
- `internal/lifecyclecli/paths.go`
- `internal/lifecyclecli/paths_test.go`
- `internal/lifecyclecli/refusal.go`
- `internal/lifecyclecli/refusal_test.go`
- `internal/lifecyclecli/run.go`
- `internal/lifecyclecli/run_test.go`
- `internal/lifecyclecli/status.go`
- `internal/lifecyclecli/testmain_integration_test.go`
- `internal/lifecyclecli/testmain_test.go`
- `internal/lifecyclecli/wire.go`
- `internal/lifecyclecli/wire_test.go`
- `internal/lifecyclerecipe/coverage_guard_test.go`
- `internal/lifecyclerecipe/doc.go`
- `internal/lifecyclerecipe/fixture_test.go`
- `internal/lifecyclerecipe/lifecyclerecipe.go`
- `internal/lifecyclerecipe/names.go`
- `internal/lifecyclerecipe/recipe_test.go`
- `internal/lifecyclerecipe/seam_enforcement_test.go`
- `internal/lifecycleshed/create.go`
- `internal/lifecycleshed/create_test.go`
- `internal/lifecycleshed/ctx.go`
- `internal/lifecycleshed/ctx_test.go`
- `internal/lifecycleshed/deps.go`
- `internal/lifecycleshed/doc.go`
- `internal/lifecycleshed/loomrun.go`
- `internal/lifecycleshed/loomrun_test.go`
- `internal/lifecycleshed/seam_enforcement_test.go`
- `internal/lifecycleshed/stuck.go`
- `internal/lifecycleshed/teardown.go`
- `internal/lifecycleshed/teardown_test.go`
- `internal/loomcli/bootstrap.go`
- `internal/loomcli/bootstrap_test.go`
- `internal/loomcli/cli_test.go`
- `internal/loomcli/run.go`
- `internal/loomrecipe/coverage_guard_test.go`
- `internal/loomrecipe/names.go`
- `internal/loomrecipe/recipe_test.go`
- `internal/shedbuild/build_engines_test.go`
- `internal/shedbuild/fixture_test.go`
- `internal/shedrecipe/coverage_guard_test.go`
- `internal/shedrecipe/entries_lifecycle.go`
- `internal/shedrecipe/entries_lifecycle_test.go`
- `internal/shedrecipe/env.go`
- `internal/shedrecipe/env_test.go`
- `internal/shedrecipe/fixture_test.go`
- `internal/shedrecipe/recipe.go`
- `internal/shedrecipe/registry.go`
- `internal/shedrecipe/registry_test.go`
- `internal/shedrecipe/seam_enforcement_test.go`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
