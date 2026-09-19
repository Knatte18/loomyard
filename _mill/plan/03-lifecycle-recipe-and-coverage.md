# Batch: lifecycle recipe and coverage guard

```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "lifecycle recipe and coverage guard"
number: 3
cards: 6
verify: go test ./internal/lifecyclerecipe/... ./internal/loomrecipe/... ./internal/shedrecipe/... ./internal/shedbuild/... ./contracts/...
depends-on: [2]
```

## Batch Scope

This batch delivers the embedded lifecycle recipe, the `internal/lifecyclerecipe` builder that parses and assembles it, and the coverage-guard restructuring the second registry consumer forces.
The restructuring is four coordinated changes that must land together: `RecipeEngines()` on each recipe package, a new cross-consumer guard in `internal/shedrecipe`'s external test package, the removal of exactly the fourth half plus its now-dead allowlist from `internal/loomrecipe`'s guard, and `internal/lifecyclerecipe`'s own three-direction guard.
Splitting them would leave the tree red in between: loom's fourth half fails the moment batch 2's keys are registered, and deleting it before the replacement exists drops the closed-coverage claim entirely.

The external interface batch 5 consumes is `lifecyclerecipe.New(env, paths)` and `lifecyclerecipe.ShedPaths`.

Batch-local decision: `lifecyclerecipe.New` performs no coherence check across its two arguments, unlike `loomrecipe.New`'s two — no lifecycle registry entry reads `Env.StatusPath` or `Env.StatusLockPath`, so there is no duplicate copy for a check to guard, and adding one for symmetry would guard nothing.

## Cards

### Card 13: the embedded recipe file

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/shedbuild/parse.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `contracts/recipes/recipes.go`
- **Creates:**
  - `contracts/recipes/lifecycle-recipe.yaml`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `lifecycle-recipe.yaml` with `version: 1`, `entry: Worktree-Create`, `terminals: [Worktree-Teardown]`, and three producer rows: `Worktree-Create` on engine `WorktreeCreate` with `on_done: Loom-Run`; `Loom-Run` on engine `LoomRun` with `on_done: Worktree-Teardown`; and `Worktree-Teardown` on engine `WorktreeTeardown` with an explicitly empty `on_done: ""`.
  No row declares a `segment`, a `max_bounces`, or an `on_stuck`.
  A file header comment, in the shape `loom-recipe.yaml`'s own header uses, must record three things: the three row names are durable on-disk identities that resume depends on, pinned against this task's own Go constants by the recipe package's guard; `Loom-Run`'s empty `on_stuck` is load-bearing and escalates to a human with the task worktree fully intact, which is what makes the destructive row unreachable from any failure path; and `Worktree-Teardown`'s empty `on_done` is likewise load-bearing and is what ends the run quietly.
  In `contracts/recipes/recipes.go` add a second exported byte var, `LifecycleRecipe`, with its own `//go:embed lifecycle-recipe.yaml` directive and a doc comment naming it as the task-worktree lifecycle producer graph.
- **Commit:** `feat(recipes): embed the three-row lifecycle recipe`

### Card 14: the lifecyclerecipe builder

- **Context:**
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomrecipe/doc.go`
  - `internal/shedbuild/build.go`
  - `internal/shedbuild/parse.go`
  - `internal/shedengine/shed.go`
  - `contracts/recipes/recipes.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecyclerecipe/doc.go`
  - `internal/lifecyclerecipe/lifecyclerecipe.go`
  - `internal/lifecyclerecipe/names.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `package lifecyclerecipe` as `internal/loomrecipe`'s twin.
  `doc.go` states the package owns the lifecycle recipe's construction, that `internal/lifecyclecli` is its only production caller, that it takes every absolute path from its caller with no direct production import of `internal/lyxcwd` per the Told-Geometry Invariant, and that it is outside the Fabric Vocabulary Invariant's owner set.
  `lifecyclerecipe.go` declares `ShedPaths` carrying `StatusPath`, `LockPath`, `StatusLockPath`, `MaxBounces` and `CommitStatus`, each field's doc pointing at the corresponding `shedengine.Shed` field doc, and `func New(env shedrecipe.Env, paths ShedPaths) (*shedengine.Shed, error)`, which calls `shedbuild.Parse(recipes.LifecycleRecipe)`, then `shedbuild.Build(recipe, env)`, wrapping each error with a `lifecyclerecipe: ` prefix and nothing more, and returns a `*shedengine.Shed` carrying the built producers plus `paths`' five fields.
  `New` never calls `shedbuild.Load` — there is no on-disk runtime location for this recipe — and never calls `shedbuild.Check`, which is authoring-time only because a resumed run legitimately starts mid-graph.
  `New`'s doc comment must state explicitly that it performs no coherence check across its two arguments, unlike `loomrecipe.New`'s two, and why: no lifecycle registry entry reads `Env.StatusPath` or `Env.StatusLockPath`, so there is no duplicated copy to guard, and a check added for symmetry would guard nothing.
  `names.go` declares the three row-name constants `NameWorktreeCreate = "Worktree-Create"`, `NameLoomRun = "Loom-Run"` and `NameWorktreeTeardown = "Worktree-Teardown"` as the authority the recipe file's row names are pinned against, plus `func RecipeEngines() []string`, which parses `recipes.LifecycleRecipe` through `shedbuild.Parse`, collects each row's `Engine`, de-duplicates, and returns the result sorted — derived from the recipe, never a hand-maintained literal, and returning a nil slice alongside an error is not an option, so a parse failure panics with a message naming this package, since a recipe that fails to parse is a build-time defect in an embedded file rather than a runtime condition.
- **Commit:** `feat(lifecyclerecipe): add the recipe builder, row-name constants and RecipeEngines`

### Card 15: lifecyclerecipe's own tests and guards

- **Context:**
  - `internal/lifecyclerecipe/lifecyclerecipe.go`
  - `internal/lifecyclerecipe/names.go`
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/seam_enforcement_test.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/lifecycleshed/deps.go`
  - `internal/shedengine/shed.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecyclerecipe/fixture_test.go`
  - `internal/lifecyclerecipe/recipe_test.go`
  - `internal/lifecyclerecipe/coverage_guard_test.go`
  - `internal/lifecyclerecipe/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `fixture_test.go` provides `testEnv(t)` returning a minimal filled `shedrecipe.Env` and a `ShedPaths`, every path derived from one `t.TempDir()` root, filling only the six fields the three lifecycle entries read and leaving the rest of `Env` zero — which is legal, since each entry validates exactly the fields it reads.
  `recipe_test.go` builds through `New` and asserts: exactly three rows; their names equal the three `Name*` constants, which is the durable-identity guard `internal/loomrecipe`'s own row-name test mirrors; the `Worktree-Create` to `Loom-Run` to `Worktree-Teardown` `OnDone` chain; `Loom-Run`'s `OnStuck` is empty; `Worktree-Teardown`'s `OnDone` is empty; no row declares a `Segment`; and that the returned `*shedengine.Shed` carries `ShedPaths`' five values verbatim.
  It also asserts `RecipeEngines()` returns exactly the three engine names, sorted — this is the input the cross-consumer guard trusts, so a silently empty return would disable that guard rather than fail it.
  `coverage_guard_test.go` carries the three loom-local directions in this package's own terms and nothing more: every row `New` assembles has an entry in a hand-written row-to-engine table, every key in that table names a row `New` actually has, and every engine the table maps to resolves through `shedrecipe.Lookup`.
  It carries no closed-coverage assertion over `shedrecipe.Names()` — that claim now lives in one cross-consumer place, and a second copy here would fail on the other consumer's engines.
  `seam_enforcement_test.go` mirrors `internal/loomrecipe/seam_enforcement_test.go` exactly, with an allowlist of `contracts/recipes`, `internal/shedbuild`, `internal/shedrecipe` and `internal/shedengine`, plus the named `internal/lyxcwd` denial constant.
- **Commit:** `test(lifecyclerecipe): cover the recipe shape, RecipeEngines and the package guards`

### Card 16: RecipeEngines on loomrecipe

- **Context:**
  - `internal/lifecyclerecipe/names.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/shedbuild/parse.go`
  - `contracts/recipes/recipes.go`
- **Edits:**
  - `internal/loomrecipe/recipe_test.go`
- **Creates:**
  - `internal/loomrecipe/names.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func RecipeEngines() []string` to `internal/loomrecipe` in a new `names.go`, the exact twin of `internal/lifecyclerecipe`'s own: parse `recipes.LoomRecipe` through `shedbuild.Parse`, collect each row's `Engine`, de-duplicate, return sorted, and panic on a parse failure with a message naming this package.
  Its doc comment states it exists as the input to the cross-consumer coverage guard, which unions every recipe consumer's engine set, and that deriving the set from the recipe rather than writing it down is what keeps that union honest without a second hand-maintained table alongside `loomRowEngines`.
  In `internal/loomrecipe/recipe_test.go` add a test asserting `RecipeEngines()` reports exactly loom's own recipe's engine set, sorted and de-duplicated, for the same reason its twin gets one: a silently empty return would disable the cross-consumer guard rather than fail it.
- **Commit:** `feat(loomrecipe): derive RecipeEngines from the embedded recipe`

### Card 17: narrow loom's coverage guard to its three local directions

- **Context:**
  - `internal/loomrecipe/names.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/shedrecipe/registry.go`
- **Edits:**
  - `internal/loomrecipe/coverage_guard_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove exactly the fourth half of `TestCoverageGuard_EveryLoomRowHasAnEngine` — the final loop over `shedrecipe.Names()` asserting every registered engine is either reached by one of loom's rows or allowlisted — and delete the `coverageGuardAllowedUnreachableEngines` map outright rather than emptying it, together with the now-unused `usedEngines` accumulation if nothing else reads it.
  Keep the three loom-local directions untouched: every row `New` assembles has an entry in `loomRowEngines`, every key in `loomRowEngines` names a row `New` actually has, and every engine the table maps to resolves through `shedrecipe.Lookup`.
  Update the test's doc comment from "asserts four things" to three, and record why the fourth was moved rather than dropped: the assertion was correct but was stated in a package that structurally cannot answer it once the registry has two consumers, so it now lives in `internal/shedrecipe`'s own external test package where both consumers are visible.
  The `SingleLLM` and `Stub` allowlist entries and their written reasons are not deleted — they relocate verbatim into that new guard in card 18, so do not paraphrase or shorten them here.
  Leave `TestCoverageGuard_PublishAndFinalizeRowNamesMatchTheirProducerIdentity` unchanged.
  Add a test proving each of the three surviving directions still fails on its own trigger — a row absent from `loomRowEngines`, a table key naming no row, and a table engine that does not resolve — rather than assuming the deletion left them intact.
- **Commit:** `test(loomrecipe): narrow the coverage guard to its three loom-local directions`

### Card 18: the cross-consumer coverage guard

- **Context:**
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/names.go`
  - `internal/lifecyclerecipe/names.go`
  - `internal/shedrecipe/registry.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/coverage_guard_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedrecipe/coverage_guard_test.go` in `package shedrecipe_test`, the external test package, which may import both recipe consumers without an import cycle — the reason this guard could not live in the internal test package, stated in the file comment.
  It unions `loomrecipe.RecipeEngines()` and `lifecyclerecipe.RecipeEngines()` and asserts every name in `shedrecipe.Names()` is in that union or on an allowlist holding `SingleLLM` and `Stub`, whose written reasons move here verbatim from `internal/loomrecipe/coverage_guard_test.go` rather than being restated in new words.
  It carries its own drift direction as a second assertion: an allowlist entry naming an engine that is no longer registered, or that some consumer now does reach, fails rather than lingering.
  The file comment must state that this guard, not any one consumer, is where a new registry key's coverage is now checked, and that a consumer holding a full copy of this assertion would fail on the other consumer's engines — the same bug, doubled.
- **Commit:** `test(shedrecipe): add the cross-consumer registry coverage guard`

## Batch Tests

`verify:` runs `go test ./internal/lifecyclerecipe/... ./internal/loomrecipe/... ./internal/shedrecipe/... ./internal/shedbuild/... ./contracts/...`.
Five roots rather than one because the coverage-guard restructuring spans them: the new guard lives in `internal/shedrecipe`'s external test package, the halves it replaces live in `internal/loomrecipe`, both consumers' `RecipeEngines()` feed it, `internal/shedbuild` compiles the embedded recipe through `Parse`, and `contracts/...` covers the new embed site.
Files covered: `internal/lifecyclerecipe`'s four new test files, `internal/loomrecipe/coverage_guard_test.go` and `recipe_test.go`, and `internal/shedrecipe/coverage_guard_test.go`.
All untagged Tier 1: parsing embedded bytes and building producers over fake seams spawns nothing.
