# Batch: inner-run-neutralization

```yaml
task: "Shed-generic watchdog for ly-drive and loom's CLI verbs"
batch: "inner-run-neutralization"
number: 2
cards: 4
verify: go test ./internal/lifecycleshed/... ./internal/shedrecipe/... ./internal/lifecyclerecipe/... ./internal/shedbuild/... ./internal/lifecyclecli/... && go test -tags integration ./internal/lifecyclecli/...
depends-on: [1]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch is a pure rename with no logic change: the lifecycle recipe's middle producer stops naming loom, moving onto the name **`InnerRun`**, which `_mill/discussion.md`'s `inner-run-engine-goes-product-neutral` Decision fixed rather than leaving to the plan.
The rename is all-or-nothing within one batch because the recipe YAML's `engine:` value, the registry key, both coverage guards and the entry's own `requireSeam`/`requireNonEmpty`/`requireAbsRoot` error texts must all land on one string together — a partial rename leaves the registry lookup failing at `shedbuild.Build` time.

The split between what is renamed and what is not follows from what is durable.
The status file persists `CurrentProducer`, which is the *row* name, so renaming a row would break resume for an in-flight run;
an `engine:` value is resolved by `shedbuild.Build` at construction time and is persisted nowhere, so renaming it is safe.
Unchanged, deliberately: the `Loom-Run` row name in `contracts/recipes/lifecycle-recipe.yaml`, `lifecyclerecipe.NameLoomRun`'s string value `"Loom-Run"`, the recipe's `entry`/`terminals`, the `poll_interval_s`/`poll_attempts` Config keys and their defaults, and the producer's own logic.
Card 9 adds a guard asserting exactly that, so a later symmetry-minded rename of the durable identity fails loudly rather than silently breaking resume.

This batch writes no second lifecycle recipe and creates no Hardener artefact.
It depends on batch 1 because the two batches edit three files in common — `internal/lifecyclecli/wire.go`, `internal/lifecyclecli/wire_test.go` and `internal/shedbuild/fixture_test.go` — where batch 1 retargets the `ShedPaths` type and this batch renames the `LoomRun` field.
The edge sequences them rather than expressing a logical need: nothing here reads `shedbuild.NewShed`.

## Cards

### Card 6: neutralize lifecycleshed's inner-run producer

- **Context:**
  - `internal/lifecycleshed/doc.go`
  - `internal/lifecycleshed/teardown.go`
  - `_mill/discussion.md`
  - `internal/lifecycleshed/stuck.go`
  - `internal/lifecycleshed/ctx.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/status.go`
- **Edits:**
  - `internal/lifecycleshed/deps.go`
  - `internal/lifecycleshed/innerrun.go`
  - `internal/lifecycleshed/innerrun_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/lifecycleshed/loomrun.go` -> `internal/lifecycleshed/innerrun.go`
  - `internal/lifecycleshed/loomrun_test.go` -> `internal/lifecycleshed/innerrun_test.go`
- **Requirements:** rename `LoomRunDeps` to `InnerRunDeps` in `internal/lifecycleshed/deps.go`, `NewLoomRun` to `NewInnerRun`, and `loomRunProducer` to `innerRunProducer` in the moved file, updating every reference and every `var _ shedengine.ShedProducer = (*innerRunProducer)(nil)` style assertion.
  Rename the test helpers and test functions in the moved test file the same way: `newLoomRunDeps` to `newInnerRunDeps`, and each `TestLoomRun_*` function to `TestInnerRun_*`.
  Change the producer's two required log lines so they say "inner shed run" in place of "loom session" — the spawn line and the wait-complete line — and change its stuck reasons the same way, so no string in the moved file or in `InnerRunDeps` names loom.
  The clause is scoped to those two places deliberately: `internal/lifecycleshed/doc.go` and `internal/lifecycleshed/teardown.go` also name the loom session, and both are out of scope — `_mill/discussion.md` scopes this neutralization to the inner-run producer's own names, log lines and stuck reasons, and `teardown.go` describes what the shipped lifecycle recipe actually tears down.
  Both log lines stay, unconditionally: the Live-Substrate Spawn Observability invariant requires them because this producer waits for its child rather than detaching.
  Change no logic: the poll loop, the exhaustive verdict table over `shedengine.Status.State` (`StateDone` to `Done`; `StateBlocked`/`StatePaused`/`StateFailed` to `Stuck` naming the state, `Error` and `CurrentProducer`; `StateRunning` consumes an attempt; anything else a hard error), the treatment of a `ResolveStatus`/`ReadStatus` error as a hard error rather than a verdict, the treatment of a `Spawn` error and of `found == false` as `Stuck`, and the nil-resolution of `Now`/`Sleep` to `time.Now`/`time.Sleep` in the constructor all stay exactly as they are.
  Weaken no assertion in the moved test file — every existing case keeps its shape, and only identifiers and the asserted log/reason substrings change.
  Update the doc comments in `internal/lifecycleshed/deps.go` and at the top of the moved file to match the new names, keeping the explanation of why `Now`/`Sleep` are nil-resolvable and why the deps struct exists.
- **Commit:** `refactor(lifecycleshed): neutralize the inner-run producer to InnerRun`

### Card 7: rename the registry key, entry and Env field

- **Context:**
  - `internal/lifecycleshed/deps.go`
  - `internal/lifecycleshed/innerrun.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/shedrecipe/config.go`
  - `internal/shedbuild/build.go`
- **Edits:**
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_lifecycle.go`
  - `internal/shedrecipe/entries_lifecycle_test.go`
  - `internal/shedrecipe/registry_test.go`
  - `internal/lifecyclecli/wire.go`
  - `internal/lifecyclecli/wire_test.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/lifecyclerecipe/fixture_test.go`
  - `internal/lifecyclecli/run_test.go`
  - `internal/lifecyclecli/lifecycle_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `internal/shedrecipe/registry.go`, change the registry key `"LoomRun"` to `"InnerRun"`, keeping its position in the map literal and keeping the registry at seventeen keys — this is a rename, never an addition.
  In `internal/shedrecipe/recipe.go`, rename the `Env.LoomRun` field to `Env.InnerRun` and change its type to `lifecycleshed.InnerRunDeps`, updating its doc comment and the `Env.Slug` doc comment's parenthetical list of the three lifecycle entries.
  In `internal/shedrecipe/entries_lifecycle.go`, rename `loomRunEntry` to `innerRunEntry`, `defaultLoomRunPollIntervalS` to `defaultInnerRunPollIntervalS` and `defaultLoomRunPollAttempts` to `defaultInnerRunPollAttempts`, keeping both default values exactly as they are (`5` and `8640`).
  Change every entry-name string this entry passes to its validators so the new name appears in the error text: the two `fmt.Errorf("shedrecipe: LoomRun: config key %q …")` calls, `requireNonEmpty("LoomRun", "Slug", …)`, `requireAbsRoot("LoomRun", "ScratchDir", …)`, and the three `requireSeam("LoomRun", "LoomRun.Spawn"|"LoomRun.ResolveStatus"|"LoomRun.ReadStatus", …)` calls — both the entry-name argument and the field-name argument move to `InnerRun`.
  The two Config key strings themselves, `poll_interval_s` and `poll_attempts`, do not change.
  Update the final construction call to `lifecycleshed.NewInnerRun(name, env.Slug, env.InnerRun, time.Duration(pollIntervalS)*time.Second, pollAttempts, env.ScratchDir)`.
  Keep the entry's deliberate non-validation of `Env.InnerRun.Now` and `Env.InnerRun.Sleep`, whose nil values are legitimate and select the production clock and sleep.
  Update `internal/shedrecipe/entries_lifecycle_test.go` and `internal/shedrecipe/registry_test.go` so every `"LoomRun"` registry key, `loomRunEntry` reference and `Env.LoomRun.*` seam-nil case names the new identifiers, weakening no assertion;
  `registry_test.go`'s expected-key list must still assert seventeen keys.
  Update `internal/lifecyclecli/wire.go`'s `LoomRun: lifecycleshed.LoomRunDeps{…}` literal to `InnerRun: lifecycleshed.InnerRunDeps{…}`, changing no closure body — the `ResolveStatus`, `Spawn` and `ReadStatus` closures stay exactly as they are, including `Spawn`'s `exec.Command(exe, "loom", "start", "--no-attach")`, which spawns loom because that is what this hub's lifecycle recipe actually wraps today.
  Find every remaining construction and field-read site with a repo-wide grep rather than the hand list below, which is this plan's own inventory at authoring time and is the floor, not the ceiling: grep for `LoomRun:`, `.LoomRun.` and `LoomRunDeps`.
  The sites this plan found beyond the two production files above are `internal/lifecyclecli/wire_test.go`'s three `c.env.LoomRun.*` field reads, and the `LoomRun: lifecycleshed.LoomRunDeps{…}` fixtures in `internal/shedbuild/fixture_test.go`, `internal/shedrecipe/fixture_test.go`, `internal/lifecyclerecipe/fixture_test.go` and `internal/lifecyclecli/run_test.go`, plus `internal/lifecyclecli/lifecycle_integration_test.go`'s two `c.env.LoomRun.Spawn`/`c.env.LoomRun.ReadStatus` overrides and the doc comment above them that names the same fields.
  Retarget each onto the new field name, weakening no assertion and changing no closure body.
- **Commit:** `refactor(shedrecipe): rename the LoomRun registry key and Env field to InnerRun`

### Card 8: move the recipe's engine value and the coverage guards

- **Context:**
  - `internal/shedrecipe/registry.go`
  - `internal/shedbuild/build.go`
  - `internal/lifecyclerecipe/lifecyclerecipe.go`
  - `internal/lifecyclerecipe/names.go`
- **Edits:**
  - `contracts/recipes/lifecycle-recipe.yaml`
  - `internal/lifecyclerecipe/coverage_guard_test.go`
  - `internal/lifecyclerecipe/recipe_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `contracts/recipes/lifecycle-recipe.yaml`, change the `Loom-Run` row's `engine: LoomRun` value to `engine: InnerRun`.
  Change nothing else in that file: the row's own `name: Loom-Run` stays, the file header comment explaining that `Loom-Run`'s empty `on_stuck` is load-bearing stays, the `on_done: Loom-Run` edge from the create row stays, and the recipe's `entry`/`terminals` stay.
  The empty `on_stuck` itself is load-bearing and must survive untouched: a stuck verdict there escalates to a human with the task worktree fully intact, which is what keeps the destructive `Worktree-Teardown` row unreachable from any failure path.
  In `internal/lifecyclerecipe/coverage_guard_test.go`, change the guard's expected engine name for `NameLoomRun` from `"LoomRun"` to `"InnerRun"`, leaving the map's key — the row-name constant `NameLoomRun` — alone.
  In `internal/lifecyclerecipe/recipe_test.go`, change the engine-name expectation `want := []string{"LoomRun", "WorktreeCreate", "WorktreeTeardown"}` to name `"InnerRun"` in `"LoomRun"`'s place, re-sorting the slice if the assertion compares it sorted.
  That one assertion belongs to this card and not to card 9, which adds a new guard and changes no existing assertion in that file.
  Search `internal/shedrecipe` for the cross-consumer coverage guard that asserts every registry key is reachable from some shipped recipe and update its expectation to the new key if it names engines literally;
  if it derives the set from the parsed recipes rather than a literal, it needs no edit — `lifecyclerecipe.RecipeEngines()` already derives its set from the parsed recipe, so verify before editing rather than assuming.
- **Commit:** `refactor(recipes): point the lifecycle recipe's inner row at the InnerRun engine`

### Card 9: pin the durable identity that must not follow the rename

- **Context:**
  - `contracts/recipes/lifecycle-recipe.yaml`
  - `internal/lifecyclerecipe/lifecyclerecipe.go`
  - `internal/lifecyclerecipe/coverage_guard_test.go`
  - `internal/shedengine/status.go`
- **Edits:**
  - `internal/lifecyclerecipe/names.go`
  - `internal/lifecyclerecipe/recipe_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add one test to `internal/lifecyclerecipe/recipe_test.go` asserting that `NameLoomRun`'s *value* is still the literal string `"Loom-Run"`, and that the built recipe's three row names are still exactly `NameWorktreeCreate`, `NameLoomRun`, `NameWorktreeTeardown` in that order.
  State in the test's own doc comment why it exists: `shedengine` persists `CurrentProducer` — the row name — into the status file, so renaming a row breaks resume for an in-flight run, and this guard makes a later symmetry-minded rename of the durable identity fail loudly rather than silently.
  Extend the comment on `NameLoomRun` in `internal/lifecyclerecipe/names.go` to record the same split: the constant's *name* may follow a neutralization but its *value* is durable and pinned by that test.
  Change no existing assertion in `recipe_test.go`: the `Worktree-Create` to `Loom-Run` to `Worktree-Teardown` `OnDone` chain and `Loom-Run`'s empty `OnStuck` are still correct facts, and the engine-name list's `"LoomRun"` entry is card 8's to update, not this card's.
- **Commit:** `test(lifecyclerecipe): pin Loom-Run's durable row name against a symmetry rename`

## Batch Tests

`verify:` runs the five packages this batch touches plus the chained integration tier.
The proof for a pure rename is that nothing moved: `internal/lifecycleshed`'s moved `innerrun_test.go` and `internal/shedrecipe`'s `entries_lifecycle_test.go` must pass with identifiers renamed and no assertion weakened, and both coverage guards must assert the new engine key with the registry still at seventeen.
`internal/lifecyclerecipe`'s full suite — `recipe_test.go` including card 9's new durable-identity guard, `coverage_guard_test.go`, `seam_enforcement_test.go` — covers the recipe side.
`internal/shedbuild` is in scope because its `fixture_test.go` builds a lifecycle `Env` and would stop compiling on the field rename;
`internal/lifecyclecli` is in scope for `wire.go`'s literal and `wire_test.go`'s three seam-presence assertions.

The chained `go test -tags integration ./internal/lifecyclecli/...` run is required rather than optional: `lifecycle_integration_test.go` and `testmain_integration_test.go` both carry `//go:build integration`, so a bare `go test` never compiles them, and a rename that broke either would otherwise surface only at the repo-wide done gate.
