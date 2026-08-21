# Discussion: loom: convert to a Shed recipe

```yaml
task: 'loom: convert to a Shed recipe'
slug: loom-convert-to-shed-recipe
status: discussing
parent: main
```

## Problem

`internal/loomshed.New()` builds loom's thirteen-row `[]shedengine.ProducerDef` as a hand-written Go literal (`internal/loomshed/loomshed.go:137-151`).
The "Shed recipe" initiative exists to replace that literal with a declarative recipe file, and three of its four pieces have already shipped:
the engine registry (`internal/shedrecipe`), the recipe loader/builder (`internal/shedbuild`), and the Shed-setup validity checker (`internal/shedcheck`).
This task is piece 4 — the conversion itself, and the mechanism's first real consumer.

Why now: five further roadmap items (`loom: real LLM producers`) each add or replace rows in this exact list, and they are deliberately sequenced *after* this task so they author their rows directly in recipe form rather than as Go-literal rows that then need converting twice.
`manifest/designs/shed-recipe.md` also carries an explicit banner that piece 4 is "an early concept sketch, not a settled design … do not implement piece 4 from this doc as written", so the location, consumer, and test-ownership decisions below are what settle it.

## Scope

**In:**

- A new production recipe file, `contracts/recipes/loom-recipe.yaml`, expressing loom's current thirteen rows in `internal/shedbuild`'s format.
- A new embed package, `contracts/recipes` (one Go file, `recipes.go`), exporting the recipe's shipped-default bytes — the same shape `contracts/stencils` uses for prompts.
- A new package, `internal/loomrecipe`, which parses the embedded recipe, builds it through `shedbuild.Build` against a caller-supplied `shedrecipe.Env`, and returns the assembled `*shedengine.Shed`.
- Rewiring `internal/loomcli` (`wiring.go`, `drive.go`) to fill a `shedrecipe.Env` instead of a `loomshed.Deps`, and to call `loomrecipe.New` instead of `loomshed.New`.
- Deleting `loomshed.New` and `loomshed.Deps`.
  The thirteen `NamePreflight`…`NameFinalize` constants are **kept** — see the `row-name-authority` Decision.
- Moving the tests that drive `loomshed.New` (all of `loomshed_test.go` bar one deletion, `sequence_test.go`, and `resume_test.go` bar one test that stays) into `internal/loomrecipe`, repointed at the recipe-built list — see `test-ownership` for the per-test disposition.
- Splitting `internal/shedrecipe/coverage_guard_test.go`: two of its three tests move into `internal/loomrecipe` repointed at the recipe file, and `TestRegistry_ShipsTwelveEntries` stays put (see `test-ownership`).
- Deleting `internal/shedbuild/equivalence_test.go` and `internal/shedbuild/testdata/loom-recipe.yaml`.
- Doc updates in the same commit: `manifest/designs/shed-recipe.md`, `manifest/designs/loom.md`, `docs/overview.md`, `CONSTRAINTS.md` (two edits), `manifest/roadmap.md`, `manifest/parallel-work.md`.

**Out:**

- Any **behavioural** change to `internal/shedengine`, `internal/shedcheck`, `internal/shedbuild`, or `internal/shedrecipe` production code.
  This task is those packages' first consumer, not their reviser.
  A genuine defect found in one of them is a finding to report, not a licence to widen scope.
  **Carve-out:** doc comments in those packages that name symbols this task deletes or moves *are* repaired here — a comment pointing at a deleted symbol is not "unchanged", it is wrong.
  The four known sites: `internal/shedcheck/doc.go:8` ("Neither `shedengine.Run` nor `loomshed.New` calls `Check`"), `internal/shedrecipe/recipe.go:69` ("told wholesale by `loomshed.Deps.Landing` today"), and `internal/shedrecipe/entries_simple.go:33-34` and `:53-55` (both naming `coverage_guard_test.go` as the pin for the `Publish`/`Finalize` row keys, a pin that now lives in `internal/loomrecipe`).
  Two further sites are already known and are **not** matched by those three tokens:
  `internal/loomshed/doc.go:1-2` ("Package loomshed owns loom's own ordered producer list and returns a constructed `*shedengine.Shed`"), falsified by deleting `New`;
  and `internal/preflightshed/preflight_test.go:33`, which names `internal/loomshed/resume_test.go`'s `TestCancellation_RealProducersReturnErrorNotStuck` by file path.
  Sweep on a wider token set than the first three: add bare `internal/loomshed/` path mentions and the moved test-file names (`resume_test`, `sequence_test`, `loomshed_test`, `equivalence_test`), and treat the enumerated sites as a starting set rather than a complete one.
- Any change to which producers loom runs, in what order, with what routing.
  The five stubbed rows (`Discussion-Write`, `Discussion-Review`, `Plan-Write`, `Plan-Review`, `Webster-Review`) stay stubs, verbatim.
  No row gains a `segment`, and no `Bouncer`/`BurlerRound` row appears — those belong to the five `loom: real LLM producers` items.
- Filling `Env.Landing`.
  `internal/loomcli` does not fill `loomshed.Deps.Landing` today either — see the `landing-parity` Decision.
- Operator-editable recipes: no seeding, no hash-stamping, no edit detection, no `_lyx/recipes/` directory, no override path.
- Any second recipe (a `Hardener` recipe, a standalone-mode recipe).
  The format is proven by loom's own; a second product's recipe arrives with that product.
- Moving loom's six producer constructors out of `internal/loomshed`.
  The import cycle is avoided by putting the consumer above `loomshed`, not by emptying it.

## Decisions

### recipe-location — embedded shipped default under `contracts/`

- Decision: the recipe file is `contracts/recipes/loom-recipe.yaml`, embedded via `//go:embed` in a new one-file package `contracts/recipes` (`package recipes`, exporting `var LoomRecipe []byte`).
  It is read through `shedbuild.Parse(recipes.LoomRecipe)`, never `shedbuild.Load`.
  There is no on-disk runtime location, no seeding pass, and no operator override.
- Rationale: a producer graph is a structural definition of what loom *is*, not per-hub configuration.
  A hand-edited graph does not degrade gracefully — it produces a run-time validation failure or, worse, a silently mis-routed run — so the editability that stencils deliberately buy is a liability here rather than a feature.
  Embedding also keeps the file inside the compiled binary, so a recipe and the registry it names can never version-skew across a deploy.
  `contracts/` is already this repo's home for declarative product contracts (`contracts/specs/`, `contracts/stencils/`), and `contracts/stencils/stencils.go` is the exact precedent for a production Go embed package outside `internal/`.
- Rejected: **seeded on disk under `<hub>/_board/_lyx/recipes/`, stencil-style.**
  This is the Stencil Ownership Invariant's shape, and it does not transfer: that invariant exists so a human can tune a *prompt* at run time, and `internal/stencilstore` owns seeding, hash-stamping, edit detection, and the `board.lock`-holding positive-pathspec commit for that purpose.
  Reusing it here means either extending `stencilstore` to a non-stencil file type or writing a second copy of that machinery, for a file nobody has asked to edit.
- Rejected: **a repo file read at run time by an absolute path the caller derives.**
  This makes `lyx` depend on its own source tree being present at run time, which no other producer input does, and adds a geometry question (which root does the path resolve against?) with no benefit.
- Consequence, accepted: `shedbuild.Load` then has no production caller.
  That is fine — it is already exported, documented, and covered by `load_test.go`, and it is the entry a future non-embedded consumer needs.
  Do not delete it, and do not add a contrived production call to justify it.

### consumer-package — a new `internal/loomrecipe` above `loomshed`

- Decision: a new package `internal/loomrecipe` owns the conversion.
  It imports `contracts/recipes`, `internal/shedbuild`, `internal/shedrecipe`, and `internal/shedengine`, and exports one constructor that is the drop-in replacement for `loomshed.New`.
  `internal/loomcli` is its only production caller.
- Rationale: `internal/loomshed` cannot be the consumer — `internal/shedrecipe`'s registry already imports `loomshed` for six of its constructors (`entries_simple.go`), so a `loomshed` → `shedbuild` → `shedrecipe` → `loomshed` production import cycle would not compile.
  The roadmap item states this constraint and leaves the choice between "consumer sits above" and "loomshed sheds its constructors" to this task; "sits above" is chosen.
  Putting it in its own package rather than inline in `loomcli` keeps it drivable from a tier-1 test with a hand-built `Env` — `loomcli`'s own wiring is cwd- and cobra-shaped and its tests carry that weight — and gives the moved sequence/resume/coverage/`Check` tests a natural home.
- Rejected: **inline in `internal/loomcli`.**
  `wiring.go` is already the widest function in the CLI, and the assembled-graph tests would then have to live beside cobra plumbing.
- Rejected: **move the six producer constructors out of `loomshed` so `loomshed` can consume the loader.**
  A much larger refactor (six types plus their tests relocated) whose only gain is keeping one package name in one place, and which would leave `loomshed` a near-empty shell anyway.

### constructor-shape — `New(env, paths)` returning `*shedengine.Shed`

- Decision: `loomrecipe.New` takes the caller's `shedrecipe.Env` plus a small told struct carrying Shed's own four fields (`StatusPath`, `LockPath`, `StatusLockPath`, `MaxBounces`), and returns `(*shedengine.Shed, error)` — exactly `loomshed.New`'s return shape.
  It parses the embedded recipe, calls `shedbuild.Build`, and assembles the `Shed`.
- Rationale: a drop-in return type means `drive.go` changes only which constructor it names, and one package remains the single answer to "how is loom's Shed built".
  Shed's four told fields cannot travel in `shedrecipe.Env` — `Env` is defined as roots and run-wide values the *registry entries* read, and no entry reads `LockPath` — so they need their own told carrier.
- Rejected: **returning `[]shedengine.ProducerDef` and letting `loomcli` assemble the `Shed`.**
  Splits the assembly across two packages and pushes the `MaxBounces`-is-a-per-producer-default subtlety (documented in `wiring.go` today) into cobra code.
- Naming is the implementer's call within the package (`New` plus a `ShedPaths`-style struct is the expected shape); the contract above is what matters.
- The told struct is stored on the `loomCLI` value, not built inside `drive.go`: `status`, `pause`, and `run` all read `StatusPath`/`StatusLockPath`/`LockPath` off `c.deps` today (see Technical context), so the replacement carrier must be reachable from every verb, exactly as `Deps` is.
  The `shedrecipe.Env` is stored beside it and read only by `drive`.

### delete-loomshed-new — one authoritative definition of the list

- Decision: `loomshed.New` and `loomshed.Deps` are deleted, not deprecated.
  `internal/loomshed` keeps its six producer constructors, `Seed`/`ErrSeedExists`, its ctx helpers, and **all thirteen `Name*` constants** — see the `row-name-authority` Decision below for why none of them is deleted.
- Rationale: the whole point of the conversion is that the recipe file becomes *the* definition.
  Keeping a Go literal beside it — even deprecated — guarantees the two drift, and the drift is silent because nothing would compare them once `equivalence_test.go` is gone.
- Rejected: **keep `New` as a deprecated fallback.**
  There is no caller to fall back for; `loomcli` is the only one.
- Note: `internal/loomshed/seam_enforcement_test.go`'s import allowlist shrinks by exactly one entry once `New` goes.
  `landingshed` becomes droppable — `loomshed.go` was its only production importer, via `Deps.Landing` and the two `landingshed.New*` calls.
  `shedadapters` and `websterengine` **stay**: `webster.go:9-13` imports both for `NewWebsterProducer`, independent of `New`.
  Verify by grep rather than by assumption, and tighten to exactly what production still imports — a membership allowlist that over-permits is the failure mode that test exists to prevent.

### row-name-authority — the Go constants stay authoritative, the recipe is checked against them

- Decision: `internal/loomshed`'s thirteen `Name*` constants remain the authority for loom's row names.
  The recipe file spells the same thirteen names as yaml strings, and the moved coverage guard is what pins the two declarations together: its row table keys off `loomshed.NamePreflight`, `loomshed.NameLoomPreflight`, … rather than off string literals, so a rename on either side fails the build or the guard.
  All thirteen constants are kept, including the eleven with no production consumer.
- Rationale: two rows are load-bearing for **seed and resume**, and neither goes through the recipe.
  `loomshed.Seed` writes `CurrentProducer: NamePreflight` into a fresh status file (`internal/loomshed/seed.go:57`), and `loomPreflightProducer.Call` passes `NameLoomPreflight` as the expected name plus `[]string{NamePreflight, NameLoomPreflight}` as the tolerated history set to `loomengine.CheckSeed` (`internal/loomshed/loompreflight.go:59`).
  Today those strings equal the row names by construction, because one Go literal declares both.
  Once the recipe is the row-name source, nothing connects them — a recipe row renamed from `Preflight` to something else would leave `Seed` writing a `current_producer` that names no row, and `CheckSeed`'s tolerated set would stop matching.
  That failure is silent at build time and surfaces as a broken resume for an in-flight task, which is exactly the durable-identity hazard `loomshed.go`'s own constant block warns about.
  Keying the guard off the constants makes the connection machine-checked.
- Why the constants cannot simply move into `internal/loomrecipe`: `loomshed` itself reads two of them (`seed.go`, `loompreflight.go`), so it would have to import `loomrecipe` — and `loomrecipe` imports `shedbuild` → `shedrecipe` → `loomshed`.
  That is the same production cycle the `consumer-package` Decision exists to avoid.
  The constants stay where the packages that read them can reach them.
- Why all thirteen rather than the two with production consumers: the remaining eleven are referenced only by tests today, but those are exactly the tests this task **moves** into `internal/loomrecipe` (`sequence_test.go` and `resume_test.go` alone carry 17 and 29 references).
  Keeping the constants lets every moved test and the moved guard name rows through the same symbols the production seed path uses, which is what makes the pin uniform across all thirteen rows instead of special-casing two.
  The retention test is therefore "any reference, production or moved test", stated explicitly so the implementer does not delete eleven constants and re-spell them as literals.
- Rejected: **the recipe becomes the authority and `Seed`/`CheckSeed` are repointed at values read out of it.**
  That makes `loomshed` depend on the recipe (the same cycle), and it makes seeding depend on parsing a recipe at a point in the bootstrap where nothing else does.
- Rejected: **leave them unpinned and rely on review.**
  A silent resume break for an in-flight task is the highest-cost failure this conversion can produce, and it is trivially machine-checkable.

### env-webster-run — fill `Env.WebsterRun` explicitly

- Decision: `internal/loomcli` sets `Env.WebsterRun = websterengine.Run` explicitly.
- Rationale: this is a real behavioural difference the conversion must absorb, not a detail.
  Today `wiring.go` deliberately leaves `Deps.WebsterRun` nil, with a comment saying `shedadapters.NewWebsterProducer` defaults nil to the production entry point (`internal/shedadapters/webster.go:50-55`).
  But `shedrecipe`'s `websterEntry` calls `requireSeam("Webster", "WebsterRun", env.WebsterRun)` and **errors on nil** — so a straight port leaving it nil fails at build time with `shedrecipe: Webster: Env.WebsterRun must not be nil`.
  `shedadapters.WebsterRunner` is a function type and `var _ WebsterRunner = websterengine.Run` already compiles, so naming it is a one-line change.
- Rejected: **relax `websterEntry` to accept nil.**
  That is a production change to a package this task consumes rather than revises, and it inverts the registry's told-not-derived posture to preserve an implicit default that only existed because the Go literal could rely on the adapter's own fallback.

### preflight-row — constructed by the registry, not injected

- Decision: the `Preflight` row is built by the registry's `preflightEntry` from `Env.Cwd`, and `loomcli` stops pre-constructing it.
  `Env.Cwd` is filled from `c.cwd`, the same value `wiring.go` passes to `preflightshed.NewPreflight` today.
- Rationale: `preflightEntry` already does exactly `preflightshed.NewPreflight(name, env.Cwd)`, so the injection field that `Deps.Preflight` existed for has no purpose once the recipe drives construction.
  Deleting it also removes `loomshed.New`'s nil-`Preflight` guard, whose job the registry's `requireAbsRoot("Preflight", "Cwd", …)` now does.
- Note for the implementer: this changes *when* the row name is told — from `loomshed.NamePreflight` in Go to the `name:` key in the recipe file.
  The recipe's `name: Preflight` must match, and the coverage guard is what pins it.

### row1-substitution — the moved Run tests swap row 1 after `Build`

- Decision: **every** moved test that calls `Run` substitutes the built row 1's producer in place — `shed.Producers[0].Producer = <fake>` — after each `loomrecipe.New` call and before `Run`.
  Both `shedengine.Shed.Producers` and `shedengine.ProducerDef.Producer` are exported, so this needs no new API on `loomrecipe` and no test-only constructor variant.
  Row 1 is the *only* substitution those tests make: every other seam they need is already injectable through `shedrecipe.Env`.
- **The default fake is not the only fake.** The shared fixture substitutes an always-done producer (today's `fakeAlwaysDoneProducer`), but an individual test may substitute its own at the same seam — and one does: `TestResume_CrashRecoveryRecallsUnconditionally` (`internal/loomshed/resume_test.go:109-112`) replaces row 1 with `countingProducer{}` because its whole subject is the row-1 **call count** across two runs.
  A rule hard-coded to the always-done fake would erase exactly what that test measures.
  State the seam, not the value.
- **This is seven tests and exactly ten `New` call sites, not two.**
  `sequence_test.go` contributes one (`TestSequence_FullRunBlocksAtPublish`, 1 site) and `resume_test.go` six: `TestResume_DoesNotRestartAtRowOne` (2), `TestResume_CrashRecoveryRecallsUnconditionally` (2), `TestResume_PauseStopsAtBoundaryAndClearsFlag` (2), `TestBounceRouting_StuckContinuesAtDeclaredTarget` (1), `TestBounceRouting_EmptyTargetBlocksInstead` (1), and `TestBounceRouting_BudgetExhaustionBlocks` (1).
  The ones building the list **twice** (a first run, then a second resuming run) need a substitution at *each* `New` — one per test is a silent hole, since the second run would call the real producer.
  `TestCancellation_RealProducersReturnErrorNotStuck` is **not** in this set: it calls neither `New` nor `Run` (see `test-ownership` for where it lands).
- **Where a test's fake carries state, substitute the same instance at every `New`.**
  `TestResume_CrashRecoveryRecallsUnconditionally` holds one `counting := &countingProducer{}` across both `New` calls (`resume_test.go:114`, `:131`) and asserts `counting.calls == 2` at `:142`.
  Substituting a fresh `&countingProducer{}` at each site would leave the count at 1 and quietly invert the test's meaning.
- Rationale: this is a real hole the conversion opens, not a detail.
  `internal/loomshed/fixture_test.go:77-79` records that `Preflight` and `WebsterRun` are the fixture's only two injectable rows, and both moved Run tests rely on it — `sequence_test.go` drives rows 1→12 and `resume_test.go` counts `Preflight` appearing exactly once across two runs, each with `Deps.Preflight: fakeAlwaysDoneProducer{}`.
  The `preflight-row` Decision removes that injection point: `preflightEntry` builds row 1 from `Env.Cwd`, and the real producer's `Call` invokes `preflight.Check(p.cwd)` (`internal/preflightshed/preflight.go:43`), which resolves geometry and spawns `git`.
  Against a `t.TempDir()` that both fails at row 1 and breaks the **Test Tier Purity Invariant** the Constraints section pins for the new package — `internal/preflightshed`'s own `Check`-driving tests are tier 2 for exactly this reason.
  Post-build substitution restores the seam at the only layer that still has one.
- Note that **construction** is safe: `preflightEntry` only calls `requireAbsRoot` and `preflightshed.NewPreflight`, neither of which touches disk, so a `t.TempDir()` `Env.Cwd` builds fine.
  It is `Call` that spawns, and `Call` is what the substitution prevents.
  Tests that only build and inspect the list (the shape assertion, the coverage guard, `Check`) need no substitution at all and must not add one — they should exercise the real row 1 so its construction stays covered.
- Rejected: **a `loomrecipe` variant returning `[]ProducerDef` before assembly, for tests to mutate.**
  Adds production API surface whose only consumer is a test, when the assembled `Shed` already exposes the same slice.
- Rejected: **tagging the two Run tests as integration (tier 2).**
  They would then need a real wired hub for a property that has nothing to do with preflight — the sequence and resume behaviour of the graph — and loom's row-order regression guard would stop running in the default suite.

### landing-parity — leave `Env.Landing` unfilled

- Decision: `Env.Landing` is left unfilled, exactly as `Deps.Landing` is today.
  No behaviour change, no new failure, no fix.
- Rationale: `internal/loomcli/wiring.go` never sets `Deps.Landing`, and `landingshed.NewPublish` rejects a nil `OpenFabric`/`PushBranch` (`internal/landingshed/publish.go:60-66`), so `loomshed.New` **already fails in production today** with `loomshed: build Publish row: …`.
  `landingshed/deps.go:73-76` names the gap and says the resolution chain belongs to a later item.
  The recipe path reproduces this identically: `publishEntry`/`finalizeEntry` call the same constructors with the same zero value.
  Preserving parity keeps this task a conversion.
- Rejected: **build the parent-fabric resolution chain here.**
  A substantial feature (list worktrees, match the parent branch, resolve and open the pair) that would dominate the task and obscure whether the conversion itself is correct.
- Implementer obligation: every test in `internal/loomrecipe` that builds the real thirteen-row list must fill the seams the registry entries reject as nil, which is a **wider** set than `loomshed.New` ever required:
  `Env.Landing` (for `publishEntry`/`finalizeEntry`, via `landingshed.NewPublish`'s own nil checks), plus `Env.WebsterRun` and four inner fields of `Env.WebsterDeps` — `Starter`, `Reed`, `Engine`, and `RefMatcher` — each `requireSeam`-checked by `websterEntry` (`internal/shedrecipe/entries_simple.go:160-171`), a check `loomshed.New` never made.
  `internal/shedbuild/equivalence_test.go:59-64` (the `websterengine.RunDeps` fake set) and its `testLandingDeps`, plus `internal/shedrecipe/coverage_guard_test.go`'s `coverageGuardFakeMergeShuttle`, are the fixture shapes to reuse rather than inventing new ones.

### test-ownership — the assembled-graph tests move to `internal/loomrecipe`

- Decision: `internal/loomrecipe` becomes the home for every test whose subject is loom's assembled graph:
  the row-sequence test (`internal/loomshed/sequence_test.go`), six of `internal/loomshed/resume_test.go`'s seven tests, all of `internal/loomshed/loomshed_test.go` bar one deletion, and two of the three tests in the registry coverage guard (`internal/shedrecipe/coverage_guard_test.go`).
  Each is repointed at the recipe-built list.
- `loomshed_test.go` has no non-`New` half — all eight of its tests drive `New` — so each needs its own disposition:
  - `TestNew_ProducerTable` (row names/routing table), `TestNew_PublishAndFinalizeAreRealProducers`, `TestNew_ProducerTableOrderUnchangedByWiring` (row order is now the recipe's list order), `TestNew_PassesShedValidation`, and `TestNew_RoutingGraphIsClean` (the `shedcheck` invariant) — **move**, repointed at the recipe-built list.
  - `TestNew_ToldShedFields` — **moves**, repointed from `Deps`'s four fields at the new Shed-paths carrier (`constructor-shape`), asserting `loomrecipe.New` threads them onto the returned `*shedengine.Shed` unchanged.
  - `TestNew_MissingLandingClosureReturnsError` — **moves**, restated: an `Env` whose `Landing` lacks its closures fails the build, and the error names the offending row (`shedbuild` prefixes every post-decode error with the row's zero-based index and `name`, which is strictly more than `loomshed.New`'s "build Publish row" wrapper carried).
  - `TestNew_NilPreflightReturnsError` — **deleted outright**.
    The guard it covers (`New` rejecting a nil `deps.Preflight`) is removed by the `preflight-row` Decision, since the row is no longer injected.
    Its replacement is the missing-`Env`-field test in Testing below (an empty `Env.Cwd` is rejected by `requireAbsRoot` inside `preflightEntry`), which covers the same class of failure at the layer that now owns it.
  Tests in `internal/loomshed` whose subject is one producer's own behaviour (`batchifier_test.go`, `discussionvalidate_test.go`, `planvalidate_test.go`, `loompreflight_test.go`, `stub_test.go`, `webster_test.go`, `ctx_test.go`, `seed_test.go`) stay where they are.
- Rationale: those four test the graph, and the graph is now the recipe's output; leaving them in `loomshed` would require hand-building a literal list there, recreating the very duplication being deleted.
- Coverage guard specifics: `internal/shedrecipe/coverage_guard_test.go` holds **three** test functions, and they do not share a disposition.
  Split the file rather than moving it wholesale:
  - `TestCoverageGuard_EveryLoomRowHasAnEngine` — **moves** to `internal/loomrecipe`, repointed from `loomshed.New`'s assembled list at the recipe file, with its `loomRowEngines` table keyed off `loomshed.Name*` per the `row-name-authority` Decision.
  - `TestCoverageGuard_PublishAndFinalizeRowNamesMatchTheirProducerIdentity` — **moves** too; it drives `loomshed.New` via the file's `coverageGuardShed` helper, and its subject (the `Publish`/`Finalize` row names matching `landingshed`'s own `publishName`/`finalizeName` constants, which those constructors substitute for the discarded `name` argument) is now a property of the recipe's rows.
  - `TestRegistry_ShipsTwelveEntries` — **stays** in `internal/shedrecipe`.
    It has no `loomshed` dependency at all: it asserts `Names()` returns exactly the sorted twelve, which is that package's own registry-size pin.
    `internal/shedrecipe/registry_test.go`'s `TestNames` already covers `Names()`↔`registry` key agreement and sortedness, so what is unique to this test is the **exact twelve-name contents pin** — and that pin belongs with the registry it pins, not with one consumer of it.
    Nothing about the cycle argument forces it to move.
  The `coverageGuardShed` helper and the `landingshed`/`mergeresolve` test doubles beside it move with the two tests that use them.
- `TestCancellation_RealProducersReturnErrorNotStuck` (`internal/loomshed/resume_test.go:331-361`) **stays** in `internal/loomshed`, despite living in a file that otherwise moves.
  It calls neither `New` nor `Run`: it constructs five of loomshed's own producers directly (`NewDiscussionValidate`, `NewPlanValidate`, `NewBatchifier`, `NewWebsterProducer`, `NewLoomPreflight`) and calls `Call` on each against a cancelled context.
  Its subject is therefore loomshed's own constructors — the same criterion that keeps `batchifier_test.go` and `planvalidate_test.go` in place — and nothing about it concerns the assembled graph.
  Only `buildSequenceFixture` ties it to the moving file, and it uses that fixture purely as a path bag (`DecisionRecordPath`, `SupportLogPath`, `AnchorPath`, `WorktreeRoot`).
  So `internal/loomshed` keeps a reduced local fixture supplying those paths as a plain struct — `Deps` is gone and cannot be that carrier — while `internal/loomrecipe` gets the full whole-list fixture.
  `internal/preflightshed/preflight_test.go:33` names this test by its `internal/loomshed/resume_test.go` path; keeping it in `loomshed` but in a different file still makes that reference stale, so it is part of the comment sweep below.
- **The moved fixture's helpers are duplicated into `internal/loomrecipe`, not moved.**
  `internal/loomshed/fixture_test.go`'s `buildSequenceFixture` calls helpers that live in files this task deliberately **keeps** in `internal/loomshed`: `writeDiscussionFixture` and `validDecisionRecord` (`discussionvalidate_test.go`), `seedPlanValidateFixture` (`planvalidate_test.go`), `fakeWebsterRun` (`webster_test.go`), plus `fakeAlwaysDoneProducer` and `testLandingDeps` from the files that do move.
  Moving the first four would break the per-producer tests that stay; leaving them means `internal/loomrecipe` does not compile.
  Duplicate them into `internal/loomrecipe/fixture_test.go` instead.
  This follows established repo practice rather than inventing a route: `testLandingDeps` already exists in **two** independent copies today (`internal/loomshed/fixture_test.go` and `internal/shedbuild/fixture_test.go`), and `internal/shedbuild/fixture_test.go` additionally carries the `websterengine.RunDeps` fakes and `nilFabricOpener` this new fixture needs — copy from there rather than re-deriving.
  Do **not** create a shared exported test-support package for this: no such package exists in the tree, and one would be production-visible API whose only consumers are tests.
- The move is forced, not chosen, for the two that move: an in-package `shedrecipe` test cannot import `shedbuild` (`shedbuild` imports `shedrecipe` — an import cycle), and testing the recipe-built list requires `shedbuild`.
- In `internal/loomrecipe` the moved guard keeps both directions of its assertion: every row in the recipe resolves through `shedrecipe.Lookup`, and `shedrecipe.Names()` carries no entry the recipe leaves unreachable beyond a named allowance.
  The registry ships twelve engines while the recipe's thirteen rows use **nine** distinct ones (`Preflight`, `LoomPreflight`, `Stub`, `DiscussionValidate`, `PlanValidate`, `Batchifier`, `Webster`, `Publish`, `Finalize`).
  The three unused are `SingleLLM`, `Bouncer`, and `BurlerRound`, unconsumed until the `loom: real LLM producers` items land.
  Today's `TestCoverageGuard_EveryLoomRowHasAnEngine` makes no claim at all about unused registry entries — it asserts table↔row agreement plus `Lookup` resolution — so this orphan check is a **newly added half carrying a named three-engine allowance**, not a weakening of an existing assertion.
  The allowance must still be written down in the test with a comment pointing at the items that will consume the three, so a reader sees why the list is not empty.
- `CONSTRAINTS.md`'s Shed Recipe Registry Invariant names `internal/shedrecipe/coverage_guard_test.go` as its enforcement point; that line is repointed in the same commit.

### retire-the-fixture — delete `equivalence_test.go` and its testdata

- Decision: `internal/shedbuild/equivalence_test.go` and `internal/shedbuild/testdata/loom-recipe.yaml` are deleted.
  Their replacement is a test in `internal/loomrecipe` asserting that the real embedded recipe parses, builds thirteen rows in the expected order with the expected names/routing/concrete producer types, and that `shedbuild.Check` reports no findings.
- Rationale: the fixture's own header says it is a stand-in and that "the conversion item sequenced after this task depends on this fixture staying in sync".
  Once the real recipe exists, keeping the fixture means two copies of loom's thirteen rows in the tree with nothing comparing them to each other — the exact drift this fixture was built to prevent, relocated.
  Deleting it also keeps `internal/shedbuild` free of loom-specific data, which matters for the "second product's recipe" case.
- Rejected: **repoint `equivalence_test.go` at the real recipe.**
  Makes `internal/shedbuild`'s test suite import `contracts/recipes` and encode loom's row list, inverting the layering — the generic loader would then depend on one product's shape.
- Rejected: **keep the fixture and compare real-vs-fixture.**
  Two files that must be edited together, forever, to express one fact.
- Check first: confirm no other test in `internal/shedbuild` reads `testdata/loom-recipe.yaml` before deleting the directory (`equivalence_test.go` claims to be its only consumer; verify with a grep rather than trusting the comment).

### docs — six files, same commit

- Decision: update, in the implementation commit:
  `manifest/designs/shed-recipe.md` (drop the "do not implement piece 4 from this doc as written" banner, mark piece 4 shipped, record the on-disk-location and consumer decisions the doc explicitly deferred);
  `manifest/designs/loom.md` (loom's producer list is recipe-backed, with a pointer to the recipe file);
  `docs/overview.md` (module table gains `internal/loomrecipe` and `contracts/recipes`, the `internal/loomshed` line at 238 loses "13-row producer list", the loom module line at 307 — which spells loom as "`internal/loomcli` + `internal/loomengine` + `internal/loomshed`" — gains `internal/loomrecipe`, and the Shed-recipe narrative paragraph at lines 322-324 stops saying "leaving only the conversion");
  `CONSTRAINTS.md` (**two** edits — see below);
  `manifest/roadmap.md` (move the item from Planned to Done, close out the "Shed recipe" group's remaining-work framing, **and** correct the present-tense claims inside already-Done entries that this task falsifies — at least line 159 ("a coverage guard … pins the registry against `loomshed.New`'s current, real row list, both directions"), line 160 ("`loomshed.New` keeps its own Go literal producer list … nothing downstream of this piece consumes it yet"), and line 168's description of `internal/shedbuild/equivalence_test.go` as a shipped artefact.
  Treat that as a starting set, not a closed list: sweep the file for `loomshed.New`, `loomshed.Deps`, `coverage_guard_test`, and `equivalence_test` the same way the doc-comment carve-out sweeps production comments);
  `manifest/parallel-work.md` (line 8 states that several sequenced items touch `internal/loomshed/loomshed.go`, which stops being true the moment the literal is deleted — the items it refers to touch the recipe file instead).
- The two `CONSTRAINTS.md` edits:
  1. The **Shed Recipe Registry Invariant**'s enforcement line, repointed off `internal/shedrecipe/coverage_guard_test.go` — that file's loom-driving half moves to `internal/loomrecipe` (see `test-ownership`), while `TestRegistry_ShipsTwelveEntries` stays put, so the line names both homes rather than one.
  2. The **Told-Geometry Invariant**'s **Machine-enforced** bullet (`CONSTRAINTS.md:76`), which enumerates every package whose `seam_enforcement_test.go` runs `TestToldGeometryInvariant_AllowlistOnly`.
     Adding that test to `internal/loomrecipe` — which the Constraints section below requires — makes the enumeration stale in this same commit, so `internal/loomrecipe` joins the list.
     The bullet at `CONSTRAINTS.md:80` says "the eleven tests named above" and becomes twelve; update the count with it.
- On the two roadmap Done entries: correct them in place rather than leaving them as historical record.
  Both are written in the present tense about the tree's current state ("keeps its own Go literal producer list", "proves the format's correctness by hand-authoring…"), not as a record of what a past task did, so leaving them would make the roadmap assert something false about `main`.
  Keep the edit minimal — restate the claim in the past tense and point at this item — rather than rewriting the entries.
- Rationale: project rule — a task adding a module or introducing cross-cutting infrastructure updates docs in the same commit; five of these six carry statements this task falsifies outright, and the sixth (`roadmap.md`) records the item's completion.
- No new invariant is introduced by this task, so `CONSTRAINTS.md` gains no section — only the enforcement-pointer correction.
  If the implementer concludes a new invariant *is* warranted (e.g. "loom's producer list is defined only in the recipe"), it is added to `CONSTRAINTS.md` in the same commit with a named enforcing test, per the project rule.

## Technical context

**The thirteen rows and their engines** (from `internal/shedrecipe/coverage_guard_test.go`'s `loomRowEngines`, which the recipe must match):
`Preflight`→`Preflight`, `Loom-Preflight`→`LoomPreflight`, `Discussion-Write`→`Stub`, `Discussion-Validate`→`DiscussionValidate`, `Discussion-Review`→`Stub`, `Plan-Write`→`Stub`, `Plan-Validate`→`PlanValidate`, `Plan-Review`→`Stub`, `Batchifier`→`Batchifier`, `Webster`→`Webster`, `Webster-Review`→`Stub`, `Publish`→`Publish`, `Finalize`→`Finalize`.

**The recipe's content is already written.**
`internal/shedbuild/testdata/loom-recipe.yaml` is a correct, `Parse`-passing, `Build`-passing expression of exactly these thirteen rows, proven equivalent to `loomshed.New`'s output field-by-field and type-by-type by `equivalence_test.go`.
The production recipe is that file with its four-line "test fixture only" header comment replaced by a production header.
Carry over verbatim: `version: 1`, `entry: Preflight`, `terminals: [Finalize]`, the row order, every `on_done`/`on_stuck`, no `segment` on any row, no `max_bounces` on any row, no `config` block on any row, and the explicit `on_done: ""` on `Finalize` with its load-bearing comment.

Do not copy the fixture blind, though.
Its equivalence to `loomshed.New`'s literal was last proven when `equivalence_test.go` was written, and that test is being deleted in this same task — so re-verify, at implementation time, that the fixture's thirteen rows still match `loomshed.go`'s live literal row-for-row before the literal is deleted.
The cheapest way to do that is ordering: run the existing `internal/shedbuild` equivalence test unchanged as the first step, confirm it is green against today's tree, and only then copy the fixture and start deleting.
A green run of that test is the proof; copying first and deleting the test first destroys it.

**`shedrecipe.Env` fields loom's thirteen rows actually read** — fill exactly these in `loomcli`, and leave the rest zero (each entry validates only what it reads, so a partially-filled `Env` is legal by design):

| Env field | Source in `wiring.go` today | Read by |
|---|---|---|
| `Cwd` | `c.cwd` (the `wire` parameter) | `Preflight` |
| `AnchorPath` | `location.AnchorPath()` | `Batchifier`, `PlanValidate`, `Webster` |
| `WorktreeRoot` | `location.WorktreePath()` | `PlanValidate` |
| `StatusPath` | `loomengine.LoomStatusFile(location)` | `LoomPreflight` |
| `StatusLockPath` | `loomengine.LoomStatusLock(location)` | `LoomPreflight` |
| `DecisionRecordPath` | `loomengine.DiscussionDecisionRecord(location)` | `DiscussionValidate` |
| `SupportLogPath` | `loomengine.DiscussionSupportLog(location)` | `DiscussionValidate` |
| `WebsterRun` | **new** — `websterengine.Run`, see the `env-webster-run` Decision | `Webster` |
| `WebsterDeps` | the assembled `runDeps` | `Webster` |
| `Landing` | unfilled today, stays unfilled — see `landing-parity` | `Publish`, `Finalize` |

`StencilsDir` and `RunRoot` stay zero: only `SingleLLM`, `Bouncer`, and `BurlerRound` read them, and no row uses those engines yet.
`Shuttle` and `Burler` stay zero for the same reason.
`Now` stays nil (the underlying constructors default it to `time.Now`).

Shed's own four told values move out of `Deps` into `loomrecipe.New`'s second argument: `StatusPath` = `loomengine.LoomStatusFile(location)`, `LockPath` = `loomengine.LoomRunLock(location)`, `StatusLockPath` = `loomengine.LoomStatusLock(location)`, `MaxBounces` = 0.
Note `StatusPath` and `StatusLockPath` are told twice — once in `Env` (for `LoomPreflight`) and once here (for `Shed`).
That duplication is inherent to the split and is what `equivalence_test.go`'s paired-fixture helper already does; do not try to collapse it.

**`loomcli` call sites that change.**
Two are structural:
`internal/loomcli/wiring.go:88-109` (the `c.deps = loomshed.Deps{…}` literal becomes a `shedrecipe.Env` plus the Shed-paths value, both stored on `c`) and `internal/loomcli/cli.go:41-43` (the `deps` field's type and its doc comment, which names `loomshed.Deps` and `loomshed.New` explicitly).
One is the constructor swap: `internal/loomcli/drive.go:45` (`loomshed.New(c.deps)` → `loomrecipe.New(…)`).

The rest are plain reads of `c.deps.StatusPath` / `c.deps.StatusLockPath` / `c.deps.LockPath` that must be repointed at whichever field replaces them — enumerate them with a `c\.deps\.` grep rather than working from this list alone, but as of exploration they are:
`drive.go:40-41` (the no-status-file refusal),
`status.go:65,67,71,81,95` (`status` and `status --watch`),
`pause.go:33,35,46`,
`run.go:100` (the `loomshed.Seed` call — `Seed` itself is unchanged) and `run.go:181` (`c.deps.LockPath`).
This is the widest mechanical edit in the task: four of loom's five verbs read the status pair off `Deps` today, so the replacement carrier has to be reachable from every command, not just `drive`.

**`Build` is not filesystem-free in general, but loom's own rows barely touch disk.**
`internal/shedbuild/doc.go:8-12` names the three disk-touching constructors as exactly `Bouncer`, `BurlerRound`, and `SingleLLM` — which are precisely the three engines loom's thirteen rows do **not** use.
So for this recipe the only construction-time filesystem contact is `publishEntry`/`finalizeEntry` calling `landingshed.NewPublish`/`NewFinalize`, which open a fabric pair — and `loomshed.New` calls those same two constructors today, so this is not a behaviour change and not a tier-1 obstacle for `internal/loomrecipe`'s tests.
`loomrecipe.New` must surface a construction error rather than swallow it, exactly as `loomshed.New` does.

**Errors carry position.** `shedbuild` names the offending row's zero-based index and `name` in every error it raises after decode, and the decoder keeps yaml line numbers.
No error-wrapping work is needed in `loomrecipe` beyond a package prefix.

**Do not call `Check` from production.** `shedbuild.Check` is authoring-time only; `internal/shedcheck/doc.go` and `internal/shedbuild/check.go` both state why (a resumed run legitimately starts mid-graph, so reachability-from-entry is the wrong production question).
It belongs in `internal/loomrecipe`'s test suite, which is also how `internal/loomshed/loomshed_test.go` uses it today.

**Reference files worth reading before writing code:**
`internal/loomshed/loomshed.go` (the literal being replaced, and a long doc comment on routing rationale worth preserving somewhere — the recipe file's own header is the natural home for the parts that describe the graph);
`internal/shedbuild/equivalence_test.go` (the paired `Env`/`Deps` fixture, and the assertion shape the new loomrecipe test inherits);
`internal/shedrecipe/coverage_guard_test.go` (the guard being moved, and its `landingshed` test doubles);
`contracts/stencils/stencils.go` (the embed-package precedent).

## Constraints

From `CONSTRAINTS.md`:

- **Shed Recipe Registry Invariant** — the registry stays one central `map[string]Constructor` literal reached only via `Lookup`/`Names`; no `init()` self-registration, no runtime `Register`.
  This task adds no registration mechanism.
  Its enforcement pointer to `internal/shedrecipe/coverage_guard_test.go` must be repointed when that file moves.
- **Told-Geometry Invariant** — `internal/loomrecipe` takes every absolute path from its caller (in `Env` and in the Shed-paths value) and must have no direct production import of `internal/lyxcwd`.
  Add a `seam_enforcement_test.go` to the new package with a membership allowlist and the standard `TestToldGeometryInvariant_AllowlistOnly` name, modelled on `internal/loomshed/seam_enforcement_test.go` and `internal/shedrecipe/seam_enforcement_test.go`.
  Adding that test puts `internal/loomrecipe` on the invariant's Machine-enforced list — edit `CONSTRAINTS.md:76` and its "eleven tests" count at line 80 in the same commit, per the `docs` Decision.
- **Shed Producer-Seam Invariant** — untouched; `internal/shedengine` gains no import.
- **Stencil Ownership Invariant** — not engaged by the recipe (a recipe is not a producer prompt), but it is the reason the seeded-on-disk alternative was rejected; do not extend `internal/stencilstore` to cover recipes.
- **Cwd Resolution Invariant** — `internal/lyxcwd` alone resolves cwd; `loomcli` passes `c.cwd` down, and neither `loomrecipe` nor the recipe file names a path.
- **Test Tier Purity Invariant** — the new package's tests must be tier 1: hand-built `Env` over `t.TempDir()`, test doubles for every seam, no process spawn.
  `internal/shedbuild/equivalence_test.go` is the working example.
- **CLI / Cobra Invariant** — `drive.go`'s command shape, `Short`, and help tree are unchanged; only the constructor it calls changes.
- **Documentation Lifecycle** — the six doc files in the `docs` Decision land in the implementation commit.
- **Config Strictness Invariant** — not engaged: the recipe is not a `_lyx/` module config and goes through neither `Load` nor `LoadOrTemplate`.
- **Fabric Vocabulary Invariant** — its `.md` half covers `contracts/stencils/**/*.md`; the new `.yaml` under `contracts/recipes/` is outside both halves today.
  Do not widen the walk as a side effect of this task; if the implementer judges the recipe should be in scope for the vocabulary walk, that is a separate proposal.

Discovered during exploration:

- `shedrecipe`'s `websterEntry` rejects a nil `Env.WebsterRun` — see the `env-webster-run` Decision.
  This is the one place a naive port silently breaks.
- `internal/shedbuild` imports `internal/shedrecipe`, so no in-package `shedrecipe` test may import `shedbuild`.
  This forces the coverage guard's relocation.
- `Publish`/`Finalize` construction already fails in production for want of `Landing` — see `landing-parity`.
  Do not "fix" this while converting, and do not let it read as a regression introduced here.

## Testing

**`internal/loomrecipe` (new, the bulk of the test work — TDD candidate).** Write the recipe file and the package's tests together; the recipe is data, so the tests are what make it verifiable.

- *Build equivalence, restated as a shape assertion.* Parse and build the embedded recipe from a hand-built `Env` over `t.TempDir()`, and assert thirteen rows in the expected order, each with the expected `Name`, `OnDone`, `OnStuck`, empty `Segment`, zero `MaxBounces`, and expected concrete `Producer` type (`reflect.TypeOf`).
  This is `equivalence_test.go`'s assertion loop with its `loomshed.New` side replaced by an expected-value table.
- *Structural check.* `shedbuild.Check(recipe, built)` reports no findings.
- *Registry coverage guard (moved).* Every recipe row's `engine` resolves through `shedrecipe.Lookup`; the thirteen rows use nine distinct engines; the only registered-but-unused engines are `SingleLLM`, `Bouncer`, `BurlerRound`.
  The row-name half keys off `loomshed.Name*`, not string literals, per `row-name-authority`.
  `shedrecipe.Names()` returning exactly twelve stays asserted in `internal/shedrecipe`, not here.
- *Publish/Finalize row identity (moved).* The recipe's rows named `Publish` and `Finalize` match `landingshed`'s own producer-identity constants, which those constructors substitute for the discarded `name` argument.
- *Seed/resume name pin.* `loomshed.Seed`'s `CurrentProducer` value and `loomPreflightProducer`'s tolerated history set both name rows that exist in the recipe — the machine-checked form of the `row-name-authority` Decision.
- *Row sequence (moved from `loomshed/sequence_test.go`).* A clean `Run` over the built list visits the expected row-name sequence, asserted against a literal expected list.
- *Resume and routing (moved from `loomshed/resume_test.go`).* Six tests, not one — resume-does-not-restart-at-row-one (`Preflight` appearing exactly once across both runs), crash-recovery re-call counting, pause-at-boundary-and-clear-flag, and the three bounce-routing cases (declared target, empty target blocks, budget exhaustion blocks).
  Carry all six over with their subjects intact.
  The file's seventh test, `TestCancellation_RealProducersReturnErrorNotStuck`, stays in `internal/loomshed` — see `test-ownership`.
- Every one of these seven `Run`-driving tests (one from `sequence_test.go`, six here — ten `New` sites in total) substitutes row 1's producer after **each** `New` call, per the `row1-substitution` Decision — the always-done fake by default, and one shared `countingProducer` instance across both sites in the crash-recovery test whose subject is the call count.
  `internal/loomshed/fixture_test.go`'s whole-list fixture is copied over for them, with its `Deps.Preflight` injection replaced by that substitution, its `Deps`/`Env` split rewritten, and its four helpers duplicated per `test-ownership`; `internal/loomshed` keeps a reduced path-bag fixture for the cancellation test that stays.
  Every other seam it fills (`WebsterRun`, `WebsterDeps`, `Landing`) carries over into `Env` unchanged.
- *Construction failure surfaces.* An `Env` missing a field a row reads (e.g. empty `Cwd`) returns an error naming the row, not a panic and not a silently-degraded list.
- *Seam enforcement.* Membership allowlist over the package's production imports.

**`contracts/recipes`.** A test asserting the embedded bytes are non-empty and `shedbuild.Parse`-able is redundant with the loomrecipe tests and should not be duplicated there; the package needs no test of its own beyond what `contracts/stencils` has (its `registry_test.go` exists for registry-vs-disk completeness, which a single-file embed does not need).
If a second recipe is ever added, revisit.

**`internal/shedrecipe`.** `TestRegistry_ShipsTwelveEntries` stays and must pass unchanged.
After the other two guard tests and their helpers leave, confirm the package's remaining test imports no longer reference `loomshed`, and tighten `seam_enforcement_test.go` if its allowlist covered test-only imports.

**`internal/loomshed`.** After deletion, run the remaining suite and confirm the per-producer tests still pass untouched.
Tighten `seam_enforcement_test.go`'s allowlist to what production still imports.
`fixture_test.go` may carry helpers used only by the moved tests — move or delete those with them rather than leaving dead fixtures.

**`internal/loomcli`.** Existing `wire` tests must be updated to assert the new `Env`/Shed-paths values instead of `loomshed.Deps` fields, with a specific assertion that `Env.WebsterRun` is non-nil — that is the regression this conversion is most likely to reintroduce.
`TestWire_PreflightIsTheAdapter` (`internal/loomcli/wiring_test.go:100-121`) asserts `c.deps.Preflight` renders as `*preflightshed.preflightProducer`, and that field disappears entirely under `preflight-row` — so it is **restated, not repointed**: it becomes an assertion that `Env.Cwd == c.cwd`, which is the only preflight-related property `wire` still owns.
The "row 1 is the preflightshed adapter, not a bare func" property moves to where it now lives — `shedrecipe`'s `preflightEntry`, which already has its own test — and the recipe-side coverage guard pins the row's engine name.
`TestVerbRefusals` (`internal/loomcli/cli_test.go:106-148`) covers `drive`'s and `pause`'s no-status-file refusals and hand-builds a `loomshed.Deps{…}` fixture at line 128, so it cannot compile once `Deps` is deleted: its **assertions stay unchanged**, while its `loomCLI` fixture is repointed at the new Shed-paths carrier.

**`internal/shedbuild`.** Run `equivalence_test.go` unchanged and green *before* copying its fixture or deleting anything — that green run is the only remaining proof the fixture matches `loomshed.New`'s literal, and both are deleted by the end of this task (see Technical context).
After deleting `equivalence_test.go` and `testdata/`, the remaining suite (`parse_test`, `build_test`, `build_engines_test`, `load_test`, `check_test`, `seam_enforcement_test`) must pass with no edits.
If any of them turns out to read the deleted testdata, keep the minimum that they need rather than resurrecting the loom fixture.

**Whole-repo gate.** `go build ./...` and `go test ./...` green, and the `docs`/`CONSTRAINTS.md` link-integrity checks pass (Markdown Link Integrity is an enforced invariant and this task edits five docs).

## Q&A log

- **Q:** Where does the recipe file live and how is it delivered? **A:** [auto-pick] Embedded shipped default at `contracts/recipes/loom-recipe.yaml` via a `contracts/recipes` embed package, parsed with `shedbuild.Parse`. **Why:** a producer graph is a structural product definition, not per-hub configuration; the seeded-on-disk alternative demands stencilstore-class seed/hash/commit machinery for a file whose hand-edit yields a broken graph at run time.
- **Q:** Who consumes the loader, given `internal/loomshed` cannot? **A:** [auto-pick] A new package `internal/loomrecipe`, called by `internal/loomcli`. **Why:** the registry already imports `loomshed`, so a back-import is a compile-time cycle; a package above it keeps the graph tests tier-1 and avoids relocating six producer constructors.
- **Q:** Do `loomshed.New`/`Deps` survive? **A:** [auto-pick] Deleted, not deprecated. **Why:** two authoritative copies of the thirteen-row list is exactly the drift the recipe exists to eliminate, and nothing would compare them once the equivalence fixture is gone.
- **Q:** Where do the sequence, resume, and `shedcheck` invariant tests live afterwards? **A:** [auto-pick] Moved into `internal/loomrecipe`, driving the recipe-built list. **Why:** their subject is loom's assembled graph, which is now the recipe's output; leaving them in `loomshed` would require hand-building a literal list there.
- **Q:** What happens to `shedbuild/equivalence_test.go` and its `testdata/loom-recipe.yaml`? **A:** [auto-pick] Both deleted, replaced by a shape-assertion test in `internal/loomrecipe`. **Why:** the fixture's own header calls itself a stand-in until this conversion; keeping it leaves two uncompared copies of the same thirteen rows and drags loom-specific data into the generic loader's suite.
- **Q:** What happens to `shedrecipe/coverage_guard_test.go`, which drives the deleted `loomshed.New`? **A:** [auto-pick] Moved to `internal/loomrecipe` and repointed at the recipe file, with `CONSTRAINTS.md`'s enforcement line updated in the same commit. **Why:** an in-package `shedrecipe` test cannot import `shedbuild` (import cycle), and a table-only guard would pass forever regardless of what loom's list grew.
- **Q:** `websterEntry` rejects a nil `Env.WebsterRun`, but `wiring.go` deliberately leaves the field nil today. Which side gives? **A:** [auto-pick] `loomcli` fills `Env.WebsterRun = websterengine.Run` explicitly. **Why:** relaxing the entry would be a production change to a package this task only consumes, and would invert the registry's told-not-derived posture to preserve an implicit adapter default.
- **Q:** `Env.Landing` is unfilled by `loomcli`, so `Publish`/`Finalize` construction already fails in production. Fix it here? **A:** [auto-pick] No — preserve parity, leave it unfilled. **Why:** the parent-fabric resolution chain is a substantial feature owned by a later item; building it here would dominate the task and obscure whether the conversion itself is correct.
- **Q:** How does `loomrecipe.New` receive Shed's own four told fields, which no registry entry reads? **A:** [auto-pick] As a second told argument beside `shedrecipe.Env`, with `New` returning `*shedengine.Shed` — a drop-in for `loomshed.New`. **Why:** `Env` is defined as values registry entries read, and a drop-in return type keeps `drive.go`'s shape and one single answer to "how is loom's Shed built".
- **Q:** Once the recipe is the row-name source, what stops a recipe rename from silently breaking seed and resume — `loomshed.Seed` writes `CurrentProducer: NamePreflight` and `loomPreflightProducer` passes `NameLoomPreflight` plus a tolerated history set to `loomengine.CheckSeed`? **A:** [auto-resolve, review round 1] The thirteen `loomshed.Name*` constants stay authoritative and all thirteen are kept; the recipe spells the same names and the moved coverage guard pins them by keying its table off the constants rather than string literals. **Why:** the constants cannot move into `internal/loomrecipe` without recreating the production import cycle (`loomshed` reads two of them), and a silent resume break for an in-flight task is both the highest-cost failure this conversion can produce and trivially machine-checkable.
- **Q:** With row 1 no longer injectable, how do the moved `Run` tests stay tier 1 — the real `Preflight` producer's `Call` spawns `git` via `preflight.Check`? **A:** [auto-resolve, review round 3] They build through `loomrecipe.New` as production does, then substitute `shed.Producers[0].Producer` with the always-done fake before `Run`; `Shed.Producers` and `ProducerDef.Producer` are both exported, so no test-only API is added. **Why:** construction is already safe (nothing in `preflightEntry` touches disk), only `Call` spawns — and tagging the two tests as integration would drop loom's row-order and resume regression guards out of the default suite for a reason unrelated to what they test.
- **Q:** Is row-1 substitution a single rule with a single fake? **A:** [auto-resolve, review round 4] No — it is a *seam*, applied after every `New` call in all eight moved `Run`-driving tests (one from `sequence_test.go`, seven from `resume_test.go`, several building the list twice), with the always-done fake as the default and `countingProducer` where a test's subject is the row-1 call count. **Why:** fixing the rule at one fake would erase what `TestResume_CrashRecoveryRecallsUnconditionally` measures, and substituting once per test would leave the second run calling the real git-spawning producer.
- **Q:** The recipe's content already exists as `internal/shedbuild/testdata/loom-recipe.yaml` — is copying it a shortcut around verification? **A:** [orchestrator review] No, but the ordering is load-bearing: run the existing `equivalence_test.go` green against today's tree *first*, then copy the fixture and delete. **Why:** the fixture's equivalence to `loomshed.New`'s literal was proven when the test was written, and this task deletes both the test and the literal — copying first would leave the claim unverified at exactly the moment its only prover is removed.
- **Q:** Which docs land in the same commit? **A:** [auto-pick] Six files: `manifest/designs/shed-recipe.md`, `manifest/designs/loom.md`, `docs/overview.md`, `CONSTRAINTS.md` (two separate edits), `manifest/roadmap.md`, and `manifest/parallel-work.md`. **Why:** the project's documentation rule, and five of the six carry statements this task falsifies — including shed-recipe.md's "do not implement piece 4 from this doc as written" banner and parallel-work.md:8's claim that several sequenced items touch `internal/loomshed/loomshed.go`.
