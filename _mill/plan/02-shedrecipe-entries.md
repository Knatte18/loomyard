# Batch: shedrecipe lifecycle entries

```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "shedrecipe lifecycle entries"
number: 2
cards: 5
verify: go test ./internal/shedrecipe/... ./internal/shedbuild/...
depends-on: [1]
```

## Batch Scope

This batch takes `internal/shedrecipe`'s registry from fourteen keys to seventeen: it adds the six `Env` fields the three lifecycle entries read, the `requireNonEmpty` validation helper `Env.Slug` needs, the three registry entries themselves in a new `entries_lifecycle.go`, and the registry-table and pin-test updates that go with them.
It also repairs the second registry-wide test — `internal/shedbuild`'s `TestBuild_EveryRegisteredEngineBuilds`, which drives its assertion off `shedrecipe.Names()` and therefore fails on the three new keys until `newTestEnv` fills their seams.

It is one batch because the `Env` fields, the entries that read them, and the two fixtures that fill them cannot compile apart: adding a registry key without filling the fixture breaks `internal/shedbuild`, and filling the fixture before the `Env` fields exist does not compile.

The external interface batch 3 consumes is the three registry keys `WorktreeCreate`, `LoomRun` and `WorktreeTeardown`, resolvable through `shedrecipe.Lookup`.

Batch-local decision: the `Loom-Run` row's two `Config` keys default to legal values, so an empty `Config` block is legal for every one of the three engines and `engineMinimalConfig` needs no new row.

## Cards

### Card 8: the six Env fields and the non-empty validator

- **Context:**
  - `internal/lifecycleshed/deps.go`
  - `internal/shedrecipe/doc.go`
- **Edits:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/env_test.go`
  - `internal/shedrecipe/seam_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add six fields to `Env` in `internal/shedrecipe/recipe.go`, each with its own field doc naming which entries read it.
  `Slug string` is the run-wide task slug, read by all three lifecycle entries for producer identity and stuck-reason text; its doc must state it is legal on `Env` because `Env` carries roots and run-wide values, and that anything per-row is a `Config` key instead.
  `ScratchDir string` is the told absolute directory the three lifecycle producers write their stuck-reason file into, read by all three.
  `CreateWorktree func(context.Context) error` is a single closure, following the `CommitDiscussion`/`CommitPlan`/`ApprovePlan` convention, because that producer's whole job is one told action with no behaviour of its own a caller must observe.
  `LoomRun lifecycleshed.LoomRunDeps` and `Teardown lifecycleshed.TeardownDeps` are whole-struct passthroughs following `Env.Landing`'s precedent, because each of those two producers has behaviour of its own that per-seam fakes must be able to substitute individually.
  `PrimeLock lifecycleshed.PrimeLock` is read by the two bookend entries only.
  Add the `context` and `internal/lifecycleshed` imports this requires.
  In `internal/shedrecipe/env.go` add `func requireNonEmpty(entry, field, value string) error`, erroring with `shedrecipe: %s: Env.%s must not be empty` — a separate helper rather than a reuse of `requireAbsRoot`, because `Env.Slug` is a plain name and not a path, so the absoluteness half would be wrong for it.
  Cover `requireNonEmpty` in `internal/shedrecipe/env_test.go` for both the empty and non-empty cases.
  Add `github.com/Knatte18/loomyard/internal/lifecycleshed` to `shedrecipeAllowedImports` in `internal/shedrecipe/seam_enforcement_test.go`.
- **Commit:** `feat(shedrecipe): add the six Env fields the lifecycle entries read`

### Card 9: the three registry entries

- **Context:**
  - `internal/shedrecipe/entries_simple.go`
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/lifecycleshed/deps.go`
  - `internal/lifecycleshed/create.go`
  - `internal/lifecycleshed/loomrun.go`
  - `internal/lifecycleshed/teardown.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_lifecycle.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `entries_lifecycle.go` with a file comment stating it holds the three lifecycle registry entries, grouped into their own file because they share the `Env.Slug`/`Env.ScratchDir`/`Env.PrimeLock` validation shape that `entries_simple.go`'s nine entries do not have.
  `worktreeCreateEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error)` calls `configRejectUnknown(cfg)`, `requireNonEmpty("WorktreeCreate", "Slug", env.Slug)`, `requireAbsRoot("WorktreeCreate", "ScratchDir", env.ScratchDir)`, `requireSeam("WorktreeCreate", "CreateWorktree", env.CreateWorktree)`, `requireSeam("WorktreeCreate", "PrimeLock.Acquire", env.PrimeLock.Acquire)` and `requireAbsRoot("WorktreeCreate", "PrimeLock.Path", env.PrimeLock.Path)`, then returns `lifecycleshed.NewWorktreeCreate(name, env.Slug, env.CreateWorktree, env.PrimeLock, env.ScratchDir)`.
  `worktreeTeardownEntry` is its twin: the same `Slug`/`ScratchDir`/`PrimeLock` checks under the `WorktreeTeardown` entry name, plus `requireSeam` on `env.Teardown.Shutdown` and `env.Teardown.Remove`, returning `lifecycleshed.NewWorktreeTeardown(name, env.Slug, env.Teardown, env.PrimeLock, env.ScratchDir)`.
  `loomRunEntry` reads two optional int `Config` keys through `configInt`: `poll_interval_s` and `poll_attempts`, defaulting to `5` and `8640` respectively when the extracted value is zero, then calls `configRejectUnknown(cfg, "poll_interval_s", "poll_attempts")`.
  It rejects a negative value for either key with an error naming that key.
  Its own field doc must state that `configInt` reports an absent key and an explicit zero identically, so both resolve to the default, and that the twelve-hour default the attempt count expresses at the default interval is deliberately generous because a real task run spans hours.
  It then runs `requireNonEmpty` on `Slug`, `requireAbsRoot` on `ScratchDir`, and `requireSeam` on `env.LoomRun.Spawn`, `env.LoomRun.ResolveStatus` and `env.LoomRun.ReadStatus` — and on neither `Now` nor `Sleep`, whose nil values are legitimate and select the production clock and sleep.
  It returns `lifecycleshed.NewLoomRun(name, env.Slug, env.LoomRun, time.Duration(pollIntervalS)*time.Second, pollAttempts, env.ScratchDir)`.
- **Commit:** `feat(shedrecipe): add the WorktreeCreate, LoomRun and WorktreeTeardown entries`

### Card 10: register the three keys

- **Context:**
  - `internal/shedrecipe/entries_lifecycle.go`
- **Edits:**
  - `internal/shedrecipe/registry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `"WorktreeCreate": worktreeCreateEntry`, `"LoomRun": loomRunEntry` and `"WorktreeTeardown": worktreeTeardownEntry` to the `registry` map literal.
  Replace the doc comment's sentence pinning the table at fourteen keys and requiring a coverage-guard update for any fifteenth entry: the table is now complete at seventeen keys, and the note must point at the new cross-consumer coverage guard in this package's external test package as the single place a new key's coverage is checked, rather than at `internal/loomrecipe`.
  Update the same comment's parenthetical listing the packages the entries span so it names `internal/lifecycleshed` alongside the four already there.
- **Commit:** `feat(shedrecipe): register the three lifecycle engines, taking the table to seventeen`

### Card 11: the registry pin and the entry tests

- **Context:**
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/entries_lifecycle.go`
  - `internal/shedrecipe/entries_simple_test.go`
  - `internal/lifecycleshed/deps.go`
- **Edits:**
  - `internal/shedrecipe/registry_test.go`
  - `internal/shedrecipe/fixture_test.go`
- **Creates:**
  - `internal/shedrecipe/entries_lifecycle_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/shedrecipe/registry_test.go`, rename `TestRegistry_ShipsFourteenEntries` to `TestRegistry_ShipsSeventeenEntries`, add `"LoomRun"`, `"WorktreeCreate"` and `"WorktreeTeardown"` to its sorted `want` slice in their correct sorted positions, and update its doc comment's count wording.
  In `internal/shedrecipe/fixture_test.go`, fill `newTestEnv`'s returned `Env` with the six new fields: a non-empty `Slug`, a `ScratchDir` created under the fixture's own temp root, a `CreateWorktree` closure returning nil, `LoomRun` and `Teardown` structs whose own closures return nil and zero values, and a `PrimeLock` whose `Path` is under the same temp root and whose acquire seam returns a no-op release with `ok == true`.
  Every path must derive from the single temp root, because this package's own guard exists to catch a told-geometry violation.
  In `entries_lifecycle_test.go`, follow `entries_simple_test.go`'s table shape and cover, per entry: the happy path returning a non-nil producer; rejection of an empty `Slug`; rejection of a relative or empty `ScratchDir`; rejection of each nil seam the entry reads, including the typed-nil case `requireSeam` handles; acceptance of a nil `LoomRun.Now` and a nil `LoomRun.Sleep`; rejection of an unknown `Config` key through `configRejectUnknown`; and, for `LoomRun` alone, the two config defaults when the keys are absent, an explicit value for each, and rejection of a negative value for each.
- **Commit:** `test(shedrecipe): pin seventeen registry entries and cover the three lifecycle entries`

### Card 12: fill the second registry-wide fixture

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_lifecycle.go`
  - `internal/lifecycleshed/deps.go`
- **Edits:**
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedbuild/build_engines_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `TestBuild_EveryRegisteredEngineBuilds` drives its assertion off `shedrecipe.Names()`, so it fails on the three new keys until this fixture covers them.
  Fill `newTestEnv` in `internal/shedbuild/fixture_test.go` with the same six fields, in the shape that fixture already fills `DiscussionSpec`/`CommitDiscussion`/`PlanSpec`/`CommitPlan` and `Landing`: closures returning nil, structs whose own closures return nil, a non-empty `Slug`, a `ScratchDir` from `mustMkdir`, and a `PrimeLock` whose `Path` sits under the same `t.TempDir()` root and whose acquire seam returns a no-op release with `ok == true`.
  No fixture value may reference a path outside that root.
  In `internal/shedbuild/build_engines_test.go` update `engineMinimalConfig`'s doc comment: the map still covers only the three engines needing a config, and the count of engines taking no config at all rises from eleven to fourteen because all three lifecycle engines accept an empty `Config` — `Loom-Run`'s two keys both default.
  Update `internal/shedbuild/fixture_test.go`'s own "two of the fourteen engines" comment to name the new engine count.
- **Commit:** `test(shedbuild): fill newTestEnv for the three new registry seams`

## Batch Tests

`verify:` runs `go test ./internal/shedrecipe/... ./internal/shedbuild/...` — both packages, because the two are one unit here: `internal/shedbuild`'s registry-wide build test is driven off `shedrecipe.Names()` and is precisely the second test the three new keys break.
Within those two packages the run covers `registry_test.go`, `entries_lifecycle_test.go`, `env_test.go`, `seam_enforcement_test.go`, `build_engines_test.go`, and every existing test in both packages that shares the two fixtures this batch edits.
All of it is untagged Tier 1.
