# Batch: batten-rename

```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: batten-rename
number: 3
cards: 5
verify: go build ./... && go test ./internal/battenshed/... ./internal/battenrecipe/... ./internal/battencli/... ./internal/shedrecipe/... ./internal/shedcli/... ./cmd/lyx/... && go test -tags integration ./internal/shedcli/...
depends-on: [1]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch performs the lifecycle→batten rename in full and in one batch, with no deprecated alias: three packages, the embedded recipe file and its byte var, the `lyx lifecycle` CLI verb, the `Loom-Run` row-name constant and its durable value, the `shedrecipe` entries file, the `shedcli` table key, and the four `CONSTRAINTS.md` lines the rename falsifies.
It is one batch because a partial rename does not compile: `internal/battencli` imports `internal/battenshed` and `internal/battenrecipe`, `internal/shedcli` imports `internal/battencli`, and `internal/shedrecipe` imports `internal/battenshed`, so every package in the cycle of consumers has to move together.

The external interface later batches consume: the renamed import paths, `battenrecipe.NameRunShed` valued `"Run-Shed"`, `recipes.BattenRecipe`, and the `"batten"` key in `shedcli`'s recipes table.

Batch-local decision beyond the overview's: this batch is **rename only**.
No path relocates, no verb is added, no behaviour changes — the relocation onto `_lyx/shed/` lands in batch 6 and the `step` verb in batch 6 as well.
Keeping the rename behaviourally inert is what makes its large diff reviewable.
One durable-identity caveat applies throughout: `NameRunShed`'s **value** changes from `"Loom-Run"` to `"Run-Shed"`, which `shedengine` persists into the status file as `CurrentProducer`, so this rename breaks resume for any in-flight lifecycle run.
That is sanctioned by the overview's `no-migration-and-no-in-flight-runs-at-landing` Shared Decision and must be stated in the commit message.

## Cards

### Card 7: rename lifecycleshed to battenshed

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/registry.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/lifecycleshed/create.go` -> `internal/battenshed/create.go`
  - `internal/lifecycleshed/create_test.go` -> `internal/battenshed/create_test.go`
  - `internal/lifecycleshed/ctx.go` -> `internal/battenshed/ctx.go`
  - `internal/lifecycleshed/ctx_test.go` -> `internal/battenshed/ctx_test.go`
  - `internal/lifecycleshed/deps.go` -> `internal/battenshed/deps.go`
  - `internal/lifecycleshed/doc.go` -> `internal/battenshed/doc.go`
  - `internal/lifecycleshed/innerrun.go` -> `internal/battenshed/innerrun.go`
  - `internal/lifecycleshed/innerrun_test.go` -> `internal/battenshed/innerrun_test.go`
  - `internal/lifecycleshed/seam_enforcement_test.go` -> `internal/battenshed/seam_enforcement_test.go`
  - `internal/lifecycleshed/stuck.go` -> `internal/battenshed/stuck.go`
  - `internal/lifecycleshed/teardown.go` -> `internal/battenshed/teardown.go`
  - `internal/lifecycleshed/teardown_test.go` -> `internal/battenshed/teardown_test.go`
- **Requirements:** After the `git mv` of every file above, change the `package lifecycleshed` declaration to `package battenshed` in each, and retarget every `lifecycleshed:` error-message and log-message prefix inside them to `battenshed:`.
  Those prefixes appear in `innerrun.go`'s `Call`, in `stuck.go`, and in `create.go`/`teardown.go`.
  `seam_enforcement_test.go`'s own package-name literal and the resolver-import ban it scans for must both be re-pointed at `battenshed` — that scan is one of the Batten Bookend Invariant's two mechanical proxies and must not be weakened while being relocated.
  Update the two importing production files listed in `Context:` in this same card, since the module does not compile otherwise: `internal/shedrecipe/recipe.go`'s `Env` field types (`InnerRun lifecycleshed.InnerRunDeps`, `Teardown lifecycleshed.TeardownDeps`, `PrimeLock lifecycleshed.PrimeLock`) and `internal/shedrecipe/registry.go`'s import.
  Do not change any behaviour: no verdict, no path, no signature moves in this card.
- **Commit:** `refactor(battenshed): rename lifecycleshed to battenshed`

### Card 8: rename lifecyclerecipe to battenrecipe and Loom-Run to Run-Shed

- **Context:**
  - `internal/shedbuild/recipe.go`
  - `internal/battenshed/doc.go`
- **Edits:**
  - `contracts/recipes/recipes.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/lifecyclerecipe/coverage_guard_test.go` -> `internal/battenrecipe/coverage_guard_test.go`
  - `internal/lifecyclerecipe/doc.go` -> `internal/battenrecipe/doc.go`
  - `internal/lifecyclerecipe/fixture_test.go` -> `internal/battenrecipe/fixture_test.go`
  - `internal/lifecyclerecipe/lifecyclerecipe.go` -> `internal/battenrecipe/battenrecipe.go`
  - `internal/lifecyclerecipe/names.go` -> `internal/battenrecipe/names.go`
  - `internal/lifecyclerecipe/recipe_test.go` -> `internal/battenrecipe/recipe_test.go`
  - `internal/lifecyclerecipe/seam_enforcement_test.go` -> `internal/battenrecipe/seam_enforcement_test.go`
  - `contracts/recipes/lifecycle-recipe.yaml` -> `contracts/recipes/batten-recipe.yaml`
- **Requirements:** After the `git mv`s, change `package lifecyclerecipe` to `package battenrecipe` in every moved Go file and retarget every `lifecyclerecipe:` message prefix to `battenrecipe:`.
  In the moved `internal/lifecyclerecipe/names.go`, rename the constant `NameLoomRun` to `NameRunShed` **and change its value** from `"Loom-Run"` to `"Run-Shed"`, rewriting its doc comment so it still says the value is a durable on-disk identity resume depends on, and adding that this task changed it deliberately under a no-in-flight-runs landing precondition.
  Retarget `RecipeEngines`'s `shedbuild.Parse(recipes.LifecycleRecipe)` call to `recipes.BattenRecipe` and its panic prefix to `battenrecipe:`.
  In the moved `internal/lifecyclerecipe/lifecyclerecipe.go`, retarget `New`'s `shedbuild.NewShed(recipes.LifecycleRecipe, …)` call and its error prefix, and update the doc comment's `recipes.LifecycleRecipe` mention.
  In `contracts/recipes/recipes.go`, rename the byte var `LifecycleRecipe` to `BattenRecipe`, point its `//go:embed` directive at `batten-recipe.yaml`, and update its doc comment from "the task-worktree lifecycle's producer graph" to name batten.
  In the moved `contracts/recipes/lifecycle-recipe.yaml`, rename the `Loom-Run` producer to `Run-Shed` and update the `on_done` reference in the `Worktree-Create` row that points at it, plus the header comment's two mentions of `Loom-Run` and its mention of the old filename and of `internal/lifecyclerecipe`.
  Leave `Loom-Run`'s `on_stuck` semantics and the header's load-bearing-empty-`on_stuck` paragraph exactly as they are — batch 5 rewrites that paragraph when it changes the routing, and changing it here would leave the comment describing a mechanism that has not moved yet.
  In the moved `internal/lifecyclerecipe/recipe_test.go`, update the coverage guard so it pins the **value** `"Run-Shed"` explicitly, the same way it pins `"Loom-Run"` today against a symmetry-minded rename.
- **Commit:** `refactor(battenrecipe): rename lifecyclerecipe to battenrecipe, Loom-Run to Run-Shed`

### Card 9: rename lifecyclecli to battencli and the lyx lifecycle verb

- **Context:**
  - `internal/battenrecipe/battenrecipe.go`
  - `internal/battenshed/deps.go`
  - `internal/shedverbs/verbs.go`
  - `internal/shedverbs/spec.go`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/lifecyclecli/arm.go` -> `internal/battencli/arm.go`
  - `internal/lifecyclecli/cli.go` -> `internal/battencli/cli.go`
  - `internal/lifecyclecli/cli_test.go` -> `internal/battencli/cli_test.go`
  - `internal/lifecyclecli/lifecycle_integration_test.go` -> `internal/battencli/lifecycle_integration_test.go`
  - `internal/lifecyclecli/paths.go` -> `internal/battencli/paths.go`
  - `internal/lifecyclecli/paths_test.go` -> `internal/battencli/paths_test.go`
  - `internal/lifecyclecli/refusal.go` -> `internal/battencli/refusal.go`
  - `internal/lifecyclecli/refusal_test.go` -> `internal/battencli/refusal_test.go`
  - `internal/lifecyclecli/run_test.go` -> `internal/battencli/run_test.go`
  - `internal/lifecyclecli/testmain_integration_test.go` -> `internal/battencli/testmain_integration_test.go`
  - `internal/lifecyclecli/testmain_test.go` -> `internal/battencli/testmain_test.go`
  - `internal/lifecyclecli/wire.go` -> `internal/battencli/wire.go`
  - `internal/lifecyclecli/wire_test.go` -> `internal/battencli/wire_test.go`
- **Requirements:** After the `git mv`s, change `package lifecyclecli` to `package battencli` in every moved file, rename the `lifecycleCLI` receiver type to `battenCLI`, rename the hook methods `lifecyclePreRun`/`lifecyclePostRun`/`lifecycleStatusExtras` to `battenPreRun`/`battenPostRun`/`battenStatusExtras`, rename `lifecycleVerbTexts` to `battenVerbTexts`, and retarget every `lifecyclecli:` message prefix to `battencli:`.
  Rename the cobra group command's `Use` from `lifecycle` to `batten`, and the `cmd.Name() == "lifecycle"` short-circuit in `resolvePersistentPreRun` to `"batten"`.
  Rewrite every `lyx lifecycle …` string inside `Short`, `Long` and `Example` text — in the group command and in all three verb texts — to `lyx batten …`, keeping each `Short` non-empty per the CLI/Cobra Invariant.
  Rewrite the user-facing refusal and message strings that name the old verb, including `PauseAbsentMessage`'s `run "lyx lifecycle run <slug>" first`, `battenPreRun`'s already-completed refusal, and `RunBusyMessage`'s text.
  Rename `StatusLabel` from `"lifecycle"` to `"batten"` and `DecodeErrPrefix` from `"lifecyclecli:"` to `"battencli:"`.
  In the moved `internal/lifecyclecli/paths.go` rename `LifecycleDir` to `BattenDir` and the private `lifecycleDirName` constant's identifier to `battenDirName`, but **leave its value `"lifecycle"` unchanged and leave every path this file constructs exactly where it is** — the relocation onto `_lyx/shed/<run-id>/` is batch 6's, and moving it here would make this card's diff behavioural.
  Retarget the `lifecyclerecipe.New`/`lifecyclerecipe.NameWorktreeCreate` references to `battenrecipe`, and the `lifecycleshed.*` references to `battenshed`.
  In `cmd/lyx/main.go` update the import and the `lifecyclecli.Command()` entry to `battencli.Command()`, keeping its position in the command list unchanged.
  Rename the moved `lifecycle_integration_test.go` in place afterwards is **not** required; leave its filename as is so its `git mv` rename detection stays clean.
- **Commit:** `refactor(battencli): rename lifecyclecli to battencli and lyx lifecycle to lyx batten`

### Card 10: rename the shedrecipe entries file and the shedcli table key

- **Context:**
  - `internal/battenshed/deps.go`
  - `internal/battencli/arm.go`
- **Edits:**
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/seam_enforcement_test.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedcli/table.go`
  - `internal/shedcli/cli.go`
  - `internal/shedcli/doc.go`
  - `internal/shedcli/parity_test.go`
  - `internal/shedcli/testmain_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/shedrecipe/entries_lifecycle.go` -> `internal/shedrecipe/entries_batten.go`
  - `internal/shedrecipe/entries_lifecycle_test.go` -> `internal/shedrecipe/entries_batten_test.go`
- **Requirements:** After the `git mv`s, update the two moved files' own file doc comments so they name batten rather than lifecycle, and retarget their `lifecycleshed.*` constructor calls to `battenshed.*`.
  The registry key strings `"WorktreeCreate"`, `"InnerRun"` and `"WorktreeTeardown"` in `internal/shedrecipe/registry.go` stay exactly as they are — they are engine names pinned against the recipe file's `engine:` values, which this batch does not change.
  Update `registry.go`'s and `seam_enforcement_test.go`'s and `fixture_test.go`'s prose and import references from lifecycle to batten.
  In `internal/shedcli/table.go`, rename the `"lifecycle"` map key to `"batten"`, retarget its `Arm` value from `lifecyclecli.Arm` to `battencli.Arm`, and rewrite the map's own doc comment, which today explains why lifecycle has no `step` analogue — keep that explanation, renamed, since batten does not gain `step` until batch 6.
  In `internal/shedcli/cli.go`, rewrite the `--recipe lifecycle` examples in the group and verb `Long` text to `--recipe batten`; leave the `--recipe` flag itself in place, since batch 7 removes it.
  Update `internal/shedcli/doc.go`, `parity_test.go` and `testmain_integration_test.go`'s lifecycle references, including `parity_test.go`'s recipe-name literals.
- **Commit:** `refactor(shedrecipe,shedcli): rename the lifecycle entries file and table key to batten`

### Card 11: retarget the cross-cutting tests and the four falsified CONSTRAINTS lines

- **Context:**
  - `internal/battencli/cli.go`
  - `internal/battenshed/seam_enforcement_test.go`
  - `internal/battencli/paths_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `cmd/lyx/notransients_test.go`
  - `internal/loomrecipe/recipe_test.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedbuild/newshed_test.go`
  - `internal/shedrecipe/coverage_guard_test.go`
  - `internal/shedverbs/pause_test.go`
  - `internal/shedverbs/status_test.go`
  - `internal/shedverbs/seam_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget every remaining lifecycle reference in the test files listed above: `cmd/lyx/helptree_test.go`'s `requiredModules` entry `"lifecycle"` becomes `"batten"` and its per-module subcommand row moves with it; `cmd/lyx/sandbox_coverage_test.go`'s `"shed"` exclusion reason names batten instead of lifecycle, and any module-key entry for `lifecycle` becomes `batten`; `cmd/lyx/registration_test.go` and `cmd/lyx/notransients_test.go` take the renamed import and command name; `internal/shedrecipe/coverage_guard_test.go` takes `battenrecipe.RecipeEngines()`; `internal/shedbuild/fixture_test.go`, `internal/shedbuild/newshed_test.go` and `internal/loomrecipe/recipe_test.go` take the renamed recipe byte var and row names; `internal/shedverbs`' three test files take the renamed label and prefix strings.
  Then amend the four `CONSTRAINTS.md` lines this rename falsifies, all in this same commit.
  (1) In `## Told-Geometry Invariant`'s bound-packages list, replace `lifecycleshed` with `battenshed` and `lifecyclerecipe` with `battenrecipe`.
  (2) In `## CLI / Cobra Invariant`'s interactive-handoff exception, replace `lyx lifecycle status --watch` with `lyx batten status --watch`.
  (3) In `## CLI / Cobra Invariant`'s package-naming deviations, replace the `lifecyclecli` deviation with `battencli` naming `internal/battenshed`, `internal/battenrecipe`, and update `shedcli`'s own deviation line, which names `internal/lifecyclecli` among its imports.
  (4) Rename the `## Lifecycle Bookend Invariant` heading to `## Batten Bookend Invariant`, rewrite its body's "The lifecycle Shed" opening to name batten, and re-point both mechanical proxies in its enforcement bullet at `internal/battenshed` and `internal/battencli`.
  Leave that invariant's "prime's own ephemeral tree" clause untouched here — it becomes false only when batch 6 makes the status durable, and it is amended there.
  Leave the `## Shed Verb-Set Invariant`'s no-resolver bullet untouched here too; batch 7 falsifies and amends it.
- **Commit:** `refactor(constraints,tests): retarget the lifecycle-to-batten rename across tests and CONSTRAINTS`

## Batch Tests

`verify: go build ./... && go test ./internal/battenshed/... ./internal/battenrecipe/... ./internal/battencli/... ./internal/shedrecipe/... ./internal/shedcli/... ./cmd/lyx/...` is scoped to the six package trees this batch touches plus a whole-module build.
The `go build ./...` half is load-bearing rather than belt-and-braces: a rename of three packages breaks compilation in any consumer the card list missed, and only a whole-module build finds one — the six test trees alone would pass while, say, `internal/loomrecipe` failed to compile.

`internal/battenshed`'s own `seam_enforcement_test.go` is the Batten Bookend Invariant's first mechanical proxy and `internal/battencli/paths_test.go` its second; both are in scope and must stay green with their assertions intact rather than relaxed.
`cmd/lyx/...` covers the help-tree, registration, drift and sandbox-coverage gates the rename moves.
`internal/shedrecipe`'s cross-consumer coverage guard is what proves `battenrecipe.RecipeEngines()` still closes the engine set after the recipe file moved.

No integration-tagged test runs in this batch's verify: the moved `internal/battencli/lifecycle_integration_test.go` is `integration`-tagged and is exercised by batch 8's verify, which is where the end-to-end run is asserted against the finished shape.
