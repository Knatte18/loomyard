# Plan: Seeded Shed core: run addressing, seed contract, batten

```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
slug: seeded-shed-core
approved: true
started: '20260919-171012'
parent: main
root: ""
verify: go build ./...
discussion_sha: cbc91bd080b37b09198b7a76c5ece6b95ddd0904
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shedrun-leaf
    file: 01-shedrun-leaf.md
    depends-on: []
    verify: go test ./internal/shedrun/...
  - number: 2
    name: board-type-field
    file: 02-board-type-field.md
    depends-on: []
    verify: go test ./internal/boardengine/...
  - number: 3
    name: batten-rename
    file: 03-batten-rename.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/battenshed/... ./internal/battenrecipe/... ./internal/battencli/... ./internal/shedrecipe/... ./internal/shedcli/... ./cmd/lyx/... && go test -tags integration ./internal/shedcli/...
  - number: 4
    name: loom-run-directory
    file: 04-loom-run-directory.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/loomengine/... ./internal/loomcli/...
  - number: 5
    name: batten-producers
    file: 05-batten-producers.md
    depends-on: [3]
    verify: go build ./... && go test ./internal/battenshed/... ./internal/battenrecipe/... ./internal/shedrecipe/... ./internal/shedengine/...
  - number: 6
    name: batten-wiring
    file: 06-batten-wiring.md
    depends-on: [1, 2, 3, 5]
    verify: go build ./... && go test ./internal/battencli/...
  - number: 7
    name: shed-addressing
    file: 07-shed-addressing.md
    depends-on: [1, 3, 4, 6]
    verify: go build ./... && go test ./internal/shedcli/... ./internal/loomcli/... ./internal/battencli/... ./cmd/lyx/... && go test -tags integration ./internal/shedcli/...
  - number: 8
    name: docs-and-integration
    file: 08-docs-and-integration.md
    depends-on: [4, 6, 7]
    verify: go build ./... && go test ./cmd/lyx/... && go test -tags integration ./internal/battencli/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: shedrun-owns-the-recipe-name-vocabulary

- **Decision:** `internal/shedrun` declares the closed recipe-name vocabulary (`RecipeLoom = "loom"`, `RecipeBatten = "batten"`, `RecipeNames()`, `ValidateRecipe`) alongside the driver vocabulary it already owns, and `internal/shedcli/table_test.go` gains a sync meta-test pinning the `recipes` map's key set against `shedrun.RecipeNames()`.
  `internal/shedcli`'s map literal remains the sole name→arming-function table, untouched in that role.
- **Rationale:** the discussion places recipe-name validation "at the seeding site, against `internal/shedcli`'s table", but one seeding site cannot reach that table: `Seed-Child` is wired from `internal/battencli`, and `internal/shedcli` already imports `internal/battencli`, so a reverse import is a compile-time cycle.
  The seed's `recipe` value is part of the seed contract `shedrun` already owns end to end — it validates `driver` against a closed two-value set for exactly the same reason — so the vocabulary belongs beside it, and the sync meta-test keeps the two declaration sites from drifting apart in the way the `refKind`↔`allRefKinds` meta-test already does elsewhere in this repo.
- **Applies to:** all batches

### Decision: loomDirName-survives-the-status-relocation

- **Decision:** `internal/loomengine/config.go` deletes `LoomStatusRel`, `LoomStatusFile`, `LoomStatusLock` and `LoomRunLock`, but **keeps** the `loomDirName` constant.
- **Rationale:** the discussion lists `loomDirName` among the symbols deleted "in favour of `shedrun`'s equivalents", but that constant also backs seven accessors that are not run-directory paths and have no `shedrun` equivalent — `LoomDriverLog`, `LoomBootstrapLock`, `LoomSelfreportFiled`, `LoomSelfreportFiledLock`, `LoomStepHandoff`, `LoomStepHandoffLock` and `LoomScratchDir`, the last of which in turn backs the reviews and friction directories.
  Deleting the constant would strand all seven.
  The Shed Run-Directory Invariant is unaffected: those paths name the `loom` segment, never `shed`, so `shedrun`'s sole-declarer claim over `shed` still holds with nothing left over.
- **Applies to:** all batches

### Decision: seed-encoding-stays-behind-a-seam-in-battenshed

- **Decision:** `internal/battenshed`'s `Seed-Child` producer never imports `internal/shedrun`.
  Its `SeedChildDeps` carries injected closures — `ReadBoardType`, `ChildDriver`, `WriteSeed`, `CommitSeed`, `PushSeed` — and `internal/battencli/wire.go` is the only place that calls `shedrun.WriteSeed` and `shedrun.ValidateRecipe`.
- **Rationale:** the Batten Bookend Invariant's mechanical proxy is `battenshed`'s seam-enforcement scan barring a direct resolver import, and `shedrun` imports `internal/lyxcwd`.
  Keeping the encode call in the wiring layer preserves that proxy, satisfies the Told-Geometry Invariant's told-paths rule, and keeps every verdict in the producer drivable from stub closures in a Tier 1 test.
  The Shed Run-Directory Invariant's sole-encoder clause is satisfied because the closure calls `shedrun`, not a hand-rolled encoder.
- **Applies to:** batten-producers, batten-wiring

### Decision: per-module-docs-ride-their-own-card

- **Decision:** a `CONSTRAINTS.md` amendment lands in the card whose change falsifies that line, not in the final docs batch.
  The final batch carries only the cross-cutting prose sweep that cannot be written until the whole shape exists: `docs/overview.md`, `manifest/roadmap.md`, `manifest/designs/seeded-shed.md`, `contracts/specs/loom-status-spec.md`, the stencil and recipe prompt text, `plugins/ly/skills/ly-drive/SKILL.md`, and both sandbox suite documents.
- **Rationale:** CLAUDE.md requires docs in the same commit as the change they describe.
  The five falsified `CONSTRAINTS.md` lines each have one identifiable falsifying change, so each can ride it; the prose sweep describes the finished surface and would be rewritten twice if split.
- **Applies to:** all batches

### Decision: no-migration-and-no-in-flight-runs-at-landing

- **Decision:** no code reads, moves or deletes a leftover `.lyx/lifecycle/` or `_lyx/loom/status.json`.
  Every commit message for the batches that relocate state, and `docs/overview.md`, state that landing requires no lifecycle or loom run in flight.
- **Rationale:** stated outright in the discussion's `no-migration-legacy-layouts-refuse-loudly` decision — a migration would have to guess the run-id, the recipe and the driver, which are the three values the seed exists to record.
- **Applies to:** all batches

### Decision: go-verify-commands-carry-no-PYTHONPATH-prefix

- **Decision:** every `verify:` in this plan is a bare `go build`/`go test` invocation with no `PYTHONPATH= ` prefix.
- **Rationale:** the prefix exists to stop a Python test subprocess inheriting mill's cache scripts directory.
  This is a Go module; the `verify-not-isolated` validator check is language-conditional and does not apply.
  `CGO_ENABLED=1` is required for every build here per the Quarry CGO Requirement Invariant, and is already the native default when a C compiler is on `PATH`, so no command sets it explicitly.
- **Applies to:** all batches

### Decision: reserved-run-id-refusal-sites

- **Decision:** `shedrun.ValidateRunID` accepts `self`; `shedrun.IsReserved` answers true for `self` alone and is consulted only where a run-id derives from a Board slug.
  `internal/battencli` separately refuses a `self` **address** by name, before its auto-seed gate, because prime hosts only slug-addressed batten runs.
- **Rationale:** addressing `self` is always legal and only claiming it for a task is not, so the two refusals are different checks at different sites with different messages — the reservation refusal names the collision, the address refusal names the omitted slug argument.
- **Applies to:** shedrun-leaf, batten-wiring

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens)._

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/notransients_test.go`
- `cmd/lyx/registration_test.go`
- `cmd/lyx/sandbox_coverage_test.go`
- `contracts/recipes/batten-recipe.yaml`
- `contracts/recipes/loom-recipe.yaml`
- `contracts/recipes/recipes.go`
- `contracts/specs/loom-status-spec.md`
- `contracts/stencils/discussiontemplate_test.go`
- `contracts/stencils/loom/loom-rubric-webster-review.md`
- `contracts/stencils/loom/loom-template-discussion.md`
- `docs/overview.md`
- `internal/battencli/arm.go`
- `internal/battencli/arm_seed_test.go`
- `internal/battencli/cli.go`
- `internal/battencli/cli_test.go`
- `internal/battencli/commitstatus.go`
- `internal/battencli/commitstatus_test.go`
- `internal/battencli/lifecycle_integration_test.go`
- `internal/battencli/paths.go`
- `internal/battencli/paths_test.go`
- `internal/battencli/refusal.go`
- `internal/battencli/refusal_test.go`
- `internal/battencli/run_test.go`
- `internal/battencli/step_test.go`
- `internal/battencli/testmain_integration_test.go`
- `internal/battencli/testmain_test.go`
- `internal/battencli/wire.go`
- `internal/battencli/wire_test.go`
- `internal/battenrecipe/battenrecipe.go`
- `internal/battenrecipe/coverage_guard_test.go`
- `internal/battenrecipe/doc.go`
- `internal/battenrecipe/fixture_test.go`
- `internal/battenrecipe/names.go`
- `internal/battenrecipe/recipe_test.go`
- `internal/battenrecipe/seam_enforcement_test.go`
- `internal/battenshed/create.go`
- `internal/battenshed/create_test.go`
- `internal/battenshed/ctx.go`
- `internal/battenshed/ctx_test.go`
- `internal/battenshed/deps.go`
- `internal/battenshed/doc.go`
- `internal/battenshed/innerrun.go`
- `internal/battenshed/innerrun_test.go`
- `internal/battenshed/seam_enforcement_test.go`
- `internal/battenshed/seamchild.go`
- `internal/battenshed/seamchild_test.go`
- `internal/battenshed/stuck.go`
- `internal/battenshed/teardown.go`
- `internal/battenshed/teardown_test.go`
- `internal/boardengine/board.go`
- `internal/boardengine/board_test.go`
- `internal/boardengine/store.go`
- `internal/boardengine/store_test.go`
- `internal/boardengine/task.go`
- `internal/boardengine/task_test.go`
- `internal/loomcli/arm.go`
- `internal/loomcli/arm_seed_test.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/landingdeps.go`
- `internal/loomcli/parity_test.go`
- `internal/loomcli/sharedbootstrap.go`
- `internal/loomcli/sharedbootstrap_test.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/loomstatus_test.go`
- `internal/loomrecipe/recipe_test.go`
- `internal/shedbuild/fixture_test.go`
- `internal/shedbuild/newshed_test.go`
- `internal/shedcli/cli.go`
- `internal/shedcli/cli_test.go`
- `internal/shedcli/doc.go`
- `internal/shedcli/parity_test.go`
- `internal/shedcli/seed.go`
- `internal/shedcli/seed_test.go`
- `internal/shedcli/table.go`
- `internal/shedcli/table_test.go`
- `internal/shedcli/testmain_integration_test.go`
- `internal/shedengine/run.go`
- `internal/shedrecipe/coverage_guard_test.go`
- `internal/shedrecipe/entries_batten.go`
- `internal/shedrecipe/entries_batten_test.go`
- `internal/shedrecipe/fixture_test.go`
- `internal/shedrecipe/recipe.go`
- `internal/shedrecipe/registry.go`
- `internal/shedrecipe/registry_test.go`
- `internal/shedrecipe/seam_enforcement_test.go`
- `internal/shedrun/doc.go`
- `internal/shedrun/paths.go`
- `internal/shedrun/paths_test.go`
- `internal/shedrun/runid.go`
- `internal/shedrun/runid_test.go`
- `internal/shedrun/seed.go`
- `internal/shedrun/seed_test.go`
- `internal/shedverbs/pause_test.go`
- `internal/shedverbs/seam_enforcement_test.go`
- `internal/shedverbs/status_test.go`
- `internal/shedverbs/step.go`
- `manifest/designs/seeded-shed.md`
- `manifest/roadmap.md`
- `plugins/ly/skills/INDEX.md`
- `plugins/ly/skills/ly-drive/SKILL.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
