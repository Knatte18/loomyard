# Batch: move-the-graph-tests-into-loomrecipe

```yaml
task: 'loom: convert to a Shed recipe'
batch: 'move-the-graph-tests-into-loomrecipe'
number: 2
cards: 7
verify: go test ./internal/loomrecipe/... ./internal/loomshed/...
depends-on: [1]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

Every test whose subject is loom's *assembled graph* moves from `internal/loomshed` into `internal/loomrecipe` and is repointed at the recipe-built list, and the one test in the moving set whose subject is loomshed's own constructors is extracted back into a new file that stays.
This is one batch because the four moves are mutually load-bearing: `fixture_test.go` supplies the whole-list fixture the other three consume, and `loomshed_test.go` supplies `fakeAlwaysDoneProducer`, which `fixture_test.go` consumes in turn.
Splitting them would leave `internal/loomshed` failing to compile at a batch boundary.

`loomshed.New` and `loomshed.Deps` still exist throughout this batch — nothing here deletes them, and batch 5 does.
`internal/shedbuild/equivalence_test.go` and `internal/shedrecipe/coverage_guard_test.go` also still drive `loomshed.New` at the end of this batch;
batch 3 handles both.
So the whole tree stays green at this batch's boundary.

Batch-local decision: the moved `loomshed_test.go` lands as `internal/loomrecipe/shape_test.go`, not as `loomrecipe_test.go`.
Its eight tests are all shape-and-identity assertions over the built list, and `shape_test.go` names that;
`loomrecipe_test.go` would suggest it is the package's primary test file, which `recipe_test.go` is.

## Cards

### Card 5: Move the whole-list fixture and duplicate the helpers it needs

- **Context:**
  - `internal/loomshed/discussionvalidate_test.go`
  - `internal/loomshed/planvalidate_test.go`
  - `internal/loomshed/webster_test.go`
  - `internal/loomshed/batchifier_test.go`
  - `internal/loomshed/seed.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/loomrecipe/loomrecipe.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/loomshed/fixture_test.go` -> `internal/loomrecipe/fixture_test.go`
- **Requirements:** After `git mv`, rewrite the moved file's package declaration to `package loomrecipe` and convert `buildSequenceFixture` from returning `(anchorPath string, deps Deps)` to returning `(anchorPath string, env shedrecipe.Env, paths ShedPaths)`.

  Keep the on-disk seeding exactly as it is: `writeDiscussionFixture` into a `discussion` subdirectory of one `t.TempDir()`, `seedPlanValidateFixture(t, dir, true)`, the `loomshed.Seed(statusPath, statusLockPath, "fixture-slug", "fixture-parent")` call (now qualified, since `Seed` stays in `internal/loomshed`), and `landing.PushSkipped = true` plus `landing.Config.RequirePRToBase = []string{landing.ParentBranch}` so `Publish`'s own told-skip gate still reports Stuck and blocks the run at row 12.
  Keep `LockPath` and `StatusLockPath` at two distinct paths — `shedengine` rejects them naming one file.

  Fill the returned `shedrecipe.Env` with exactly the ten fields loom's thirteen rows read: `Cwd` (a new `mustMkdir("cwd")` under the same temp dir — `preflightEntry` requires an absolute `Cwd`), `AnchorPath` and `WorktreeRoot` (both the temp dir, as today), `StatusPath`, `StatusLockPath`, `DecisionRecordPath`, `SupportLogPath`, `WebsterRun`, `WebsterDeps`, and `Landing`.
  Leave `StencilsDir`, `RunRoot`, `Shuttle`, `Burler`, and `Now` zero — only `SingleLLM`, `Bouncer`, and `BurlerRound` read them and no row uses those engines.
  `WebsterRun` is `(&fakeWebsterRun{}).run` as today.
  `WebsterDeps` is **new work relative to the old fixture**: `websterEntry` `requireSeam`-checks four inner fields `loomshed.New` never checked, so `WebsterDeps` must be a `websterengine.RunDeps` with non-nil `Starter`, `Reed`, `Engine`, and `RefMatcher`.
  Copy the four embedded-interface placeholder types (`fakeMasterStarter`, `fakeReedOps`, `fakeShuttleEngine`, `fakeRefMatcher`) from `internal/shedbuild/fixture_test.go` rather than inventing new ones — each embeds the seam interface in an empty struct, yielding a non-nil value satisfying the interface without implementing a method.
  Fill the returned `ShedPaths` with `StatusPath`, `LockPath`, `StatusLockPath`, and `MaxBounces: 3`, matching the old `Deps`.

  Duplicate into this file, verbatim, the five helpers it needs that live in files staying in `internal/loomshed`: `writeDiscussionFixture` and the `validDecisionRecord` constant, `seedPlanValidateFixture`, the `fakeWebsterRun` type with its `run` method, and `writeBatcherConfig` (needed by the moved resume tests in card 8).
  Add `fakeAlwaysDoneProducer` here too — it arrives with card 6's move but belongs beside the fixture that fills it, so card 6 relocates it into this file rather than leaving it in `shape_test.go`.
  Note in the file header that these are deliberate duplications, not an oversight, and that `testLandingDeps` already existed in two independent copies before this task.

  Rewrite the file's own header comment: it currently names `_mill/plan/03-sequence-and-integration.md` and "card 11" from a long-past task, and describes `Deps` as the thing it returns.
  Also update `buildSequenceFixture`'s doc comment where it says "Preflight and WebsterRun are the only two injectable rows" — `Preflight` is no longer injectable at all, and its replacement is the post-build row-1 substitution the callers perform.
- **Commit:** `test(loomrecipe): move the whole-list fixture off Deps onto Env`

### Card 6: Move the shape-and-identity tests off `loomshed.New`

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomshed/loomshed.go`
  - `internal/shedbuild/check.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/landingshed/publish.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/loomshed/loomshed_test.go` -> `internal/loomrecipe/shape_test.go`
- **Requirements:** After `git mv`, rewrite the package declaration to `package loomrecipe` and repoint all eight tests off `New(deps)` onto `loomrecipe.New(env, paths)`.

  Relocate `fakeAlwaysDoneProducer` out of this file into `internal/loomrecipe/fixture_test.go` (card 5) — it is fixture scaffolding, not a shape assertion.
  Replace `testDeps` with a `testEnv(t *testing.T) (shedrecipe.Env, ShedPaths)` helper of the same shape: one `t.TempDir()`, every path field joined off it, `Cwd` an absolute created subdirectory, `Landing: testLandingDeps(dir)`, `WebsterRun` and the four `WebsterDeps` seams filled the way card 5 fills them, and `MaxBounces: 3` on the `ShedPaths` half.
  Rewrite `wantProducerTable`'s entries to key off `loomshed.Name*` constants, exactly as they do today.

  Per-test dispositions:

  - `TestNew_ProducerTable` — keep unchanged in substance: thirteen rows in table order with the expected `Name`, `OnStuck`, `OnDone`, an empty `Segment`, a zero `MaxBounces`, and a non-nil `Producer`.
  - `TestNew_PublishAndFinalizeAreRealProducers` — keep unchanged: the rows named `loomshed.NamePublish`/`loomshed.NameFinalize` type-assert to `*landingshed.Publish`/`*landingshed.Finalize` and both keep `OnStuck: ""`.
  - `TestNew_ProducerTableOrderUnchangedByWiring` — keep, restating that row order is now the recipe's list order rather than the Go literal's.
  - `TestNew_PassesShedValidation` — keep, calling `loomshed.Seed` (now qualified) before `Run` as it does today.
  - `TestNew_RoutingGraphIsClean` — keep, still calling `shedcheck.Check(shed.Producers, loomshed.NamePreflight, []string{loomshed.NameFinalize})`.
    Delete the four-line paragraph in its doc comment predicting that this guard "must move onto the recipe-assembled list at that point" — this batch is that point, and the prediction is now stale.
  - `TestNew_ToldShedFields` — keep, repointed from `Deps`'s four fields at `ShedPaths`' four, asserting `loomrecipe.New` threads each onto the returned `*shedengine.Shed` unchanged.
  - `TestNew_MissingLandingClosureReturnsError` — keep, restated: an `Env` whose `Landing` lacks its `OpenFabric` closure fails the build, and the error names the offending row.
    Assert the error text contains the row name `Publish`, which `shedbuild` prefixes with the row's zero-based index and quoted name — strictly more information than `loomshed.New`'s old "build Publish row" wrapper carried.
  - `TestNew_NilPreflightReturnsError` — **delete outright**.
    The guard it covers (`New` rejecting a nil `deps.Preflight`) no longer exists: the row is built by `preflightEntry` from `Env.Cwd`, not injected.
    Its replacement is card 7's construction-failure test.

  None of these eight tests calls `Run` against a real fixture except `TestNew_PassesShedValidation`, whose `Env` points at paths that do not exist on disk so `Discussion-Validate` bounces until its budget is exhausted — that is an ordinary blocked outcome, not a validation failure, and needs no row-1 substitution because row 1 is `fakeAlwaysDoneProducer`... which it no longer is.
  `TestNew_PassesShedValidation` **does** now build the real `Preflight` producer, so it must substitute row 1 per the `row1-substitution-is-a-seam-not-a-fixed-fake` Shared Decision before calling `Run`.
  The other seven only build and inspect the list and must add no substitution at all, so the real row 1's construction stays covered.
- **Commit:** `test(loomrecipe): move the graph shape assertions onto the recipe`

### Card 7: The recipe's own shape, structural check, and construction-failure tests

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/shape_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/shedbuild/equivalence_test.go`
  - `internal/shedbuild/check.go`
  - `internal/shedbuild/parse.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/loomshed/seed.go`
  - `internal/loomshed/loompreflight.go`
  - `internal/loomshed/loomshed.go`
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/recipes/recipes.go`
- **Edits:** none
- **Creates:**
  - `internal/loomrecipe/recipe_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomrecipe/recipe_test.go` in `package loomrecipe`, carrying four tests.

  *Shape assertion.* This is `internal/shedbuild/equivalence_test.go`'s assertion loop with its `loomshed.New` side replaced by an expected-value table.
  Build the embedded recipe through `loomrecipe.New` from `testEnv(t)`, assert exactly thirteen rows, and for each row assert `Name`, `OnDone`, `OnStuck`, an empty `Segment`, a zero `MaxBounces`, and the expected concrete `Producer` type via `reflect.TypeOf`.
  The expected table's row names key off `loomshed.Name*` constants per the `row-name-authority-stays-with-the-go-constants` Shared Decision.
  The expected producer types are the ones `internal/shedbuild/equivalence_test.go` proves today — read them off that file rather than guessing;
  it is still present in the tree at this batch and batch 3 is what deletes it.

  *Structural check.* Parse `recipes.LoomRecipe` through `shedbuild.Parse` directly, build it through `shedbuild.Build` against the same `Env`, and assert `shedbuild.Check(recipe, built)` returns no findings.
  This needs the parsed `Recipe` value, which `loomrecipe.New` does not return, so it goes through `Parse`/`Build` rather than through `New` — state that in the test's doc comment so a reader does not "simplify" it back onto `New`.

  *Construction failure surfaces.* An `Env` with an empty `Cwd` makes `loomrecipe.New` return a non-nil error and a nil `*shedengine.Shed`, and the error names the offending row — `requireAbsRoot("Preflight", "Cwd", …)` inside `preflightEntry`, wrapped by `shedbuild` with the row's zero-based index and quoted name.
  Assert on a non-nil error, a nil Shed, and the presence of the row name in the error text;
  do not assert the exact full string.
  This test is the replacement for the deleted `TestNew_NilPreflightReturnsError` and covers the same class of failure at the layer that now owns it.

  *Seed/resume name pin.* Assert that the two row names loom's seed and resume paths hard-code outside the recipe both name rows the recipe actually has: `loomshed.NamePreflight`, which `Seed` writes as `CurrentProducer` into a fresh status file, and `loomshed.NameLoomPreflight`, which `loomPreflightProducer.Call` passes to `loomengine.CheckSeed` as the expected name alongside `[]string{NamePreflight, NameLoomPreflight}` as the tolerated history set.
  Build the list and assert both constants appear as row names.
  The test's doc comment must state what it protects: once the recipe is the row-name source, a recipe row renamed from `Preflight` would leave `Seed` writing a `current_producer` naming no row and `CheckSeed`'s tolerated set no longer matching — a failure that is silent at build time and surfaces as a broken resume for an in-flight task.
  Read the exact tolerated-set expression out of `internal/loomshed/loompreflight.go` rather than trusting this description.
- **Commit:** `test(loomrecipe): pin the recipe's shape, structure, and seed row names`

### Card 8: Move the row-sequence test

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomshed/loomshed.go`
  - `internal/shedengine/shed.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/loomshed/sequence_test.go` -> `internal/loomrecipe/sequence_test.go`
- **Requirements:** After `git mv`, rewrite the package declaration to `package loomrecipe`, qualify every bare `Name*` reference in `wantSequenceOrder` as `loomshed.Name*`, and repoint `TestSequence_FullRunBlocksAtPublish` off `New(deps)` onto `loomrecipe.New(env, paths)` from the moved `buildSequenceFixture`.
  Substitute row 1's producer per the `row1-substitution-is-a-seam-not-a-fixed-fake` Shared Decision — `shed.Producers[0].Producer = fakeAlwaysDoneProducer{}` — after the single `New` call and before `Run`.
  The final `state.ReadJSONStrict` assertion reads `deps.StatusPath`/`deps.StatusLockPath` today;
  repoint both at the `ShedPaths` value the fixture returns.
  Every assertion — the twelve-row expected sequence, the `RunBlocked` outcome, `HaltedProducer == NamePublish`, and the persisted `StateBlocked`/`CurrentProducer` pair — carries over with its subject intact.
- **Commit:** `test(loomrecipe): move the row-sequence guard onto the recipe`

### Card 9: Move the resume and bounce-routing tests

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/sequence_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomshed/loomshed.go`
  - `internal/preflightshed/preflight.go`
  - `internal/shedengine/shed.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/loomshed/resume_test.go` -> `internal/loomrecipe/resume_test.go`
- **Requirements:** After `git mv`, rewrite the package declaration to `package loomrecipe`, qualify every bare `Name*` reference as `loomshed.Name*`, and repoint every `New(deps)` call onto `loomrecipe.New(env, paths)`.
  Repoint every `deps.StatusPath`, `deps.StatusLockPath`, `deps.AnchorPath`, and `deps.DecisionRecordPath` read at the `Env`/`ShedPaths` pair the moved fixture returns, and `deps.MaxBounces = 2` in `TestBounceRouting_BudgetExhaustionBlocks` at `paths.MaxBounces = 2` (set before the `New` call, since `New` copies it onto the Shed).

  Move exactly six of the file's seven tests: `TestResume_DoesNotRestartAtRowOne`, `TestResume_CrashRecoveryRecallsUnconditionally`, `TestResume_PauseStopsAtBoundaryAndClearsFlag`, `TestBounceRouting_StuckContinuesAtDeclaredTarget`, `TestBounceRouting_EmptyTargetBlocksInstead`, and `TestBounceRouting_BudgetExhaustionBlocks`, together with the `countingProducer` type and the `resetCurrentProducer` helper.
  Delete `TestCancellation_RealProducersReturnErrorNotStuck` from this file — card 10 re-creates it in `internal/loomshed`.

  **Row-1 substitution is per `New` call, not per test.**
  Three of the six build the list twice: `TestResume_DoesNotRestartAtRowOne`, `TestResume_CrashRecoveryRecallsUnconditionally`, and `TestResume_PauseStopsAtBoundaryAndClearsFlag`.
  Each of those needs a substitution after *each* `New` call — substituting once per test leaves the second run calling the real row-1 producer, whose `Call` invokes `preflight.Check(p.cwd)` and spawns `git` against a `t.TempDir()`, both failing the run and breaching the Test Tier Purity Invariant.
  That is nine `New` call sites in this file, plus card 8's one, for ten in total across the batch.

  **`TestResume_CrashRecoveryRecallsUnconditionally` substitutes its own fake, and the same instance twice.**
  It holds one `counting := &countingProducer{}` across both builds and asserts `counting.calls == 2` at the end.
  Substituting a fresh `&countingProducer{}` at the second site would leave the count at 1 and quietly invert what the test measures.
  Every other moved test substitutes `fakeAlwaysDoneProducer{}`.

  Update the three doc comments that point at "sequence_test.go's wantSequenceOrder doc comment" — that file is now `internal/loomrecipe/sequence_test.go`, so the reference stays valid as a bare filename and needs no change, but re-read each to confirm it still describes what the test does.
  Update `TestResume_DoesNotRestartAtRowOne`'s comment where it says the run blocks at Batchifier "per the real Batchifier gate genuinely failing on a malformed config -- never a substituted fake row" — that remains true;
  row 1 is the only substituted row and Batchifier is row 9.
- **Commit:** `test(loomrecipe): move the resume and bounce-routing suite onto the recipe`

### Card 10: Keep the cancellation test in `internal/loomshed` with a reduced fixture

- **Context:**
  - `internal/loomshed/discussionvalidate_test.go`
  - `internal/loomshed/planvalidate_test.go`
  - `internal/loomshed/webster_test.go`
  - `internal/loomshed/batchifier_test.go`
  - `internal/loomshed/loompreflight.go`
  - `internal/loomshed/seed.go`
  - `internal/loomrecipe/resume_test.go`
  - `internal/shedengine/shed.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/cancellation_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomshed/cancellation_test.go` in `package loomshed` carrying `TestCancellation_RealProducersReturnErrorNotStuck`, restored verbatim from the copy card 9 deletes out of the moved `internal/loomrecipe/resume_test.go`, plus a reduced local fixture it drives from.

  This test stays in `internal/loomshed` because its subject is loomshed's own constructors, not the assembled graph: it calls neither `New` nor `Run`, constructing `NewDiscussionValidate`, `NewPlanValidate`, `NewBatchifier`, `NewWebsterProducer`, and `NewLoomPreflight` directly and calling `Call` on each against an already-cancelled context.
  That is the same criterion keeping `batchifier_test.go` and `planvalidate_test.go` in place.

  The reduced fixture is a package-local helper in this same file returning a plain struct — `Deps` is gone from the moved fixture's return and will be deleted outright in batch 5, so it cannot be the carrier.
  The struct carries the six values the test reads: `AnchorPath`, `WorktreeRoot`, `DecisionRecordPath`, `SupportLogPath`, `StatusPath`, and `StatusLockPath`.
  Reproduce `buildSequenceFixture`'s on-disk seeding exactly — `writeDiscussionFixture` into a `discussion` subdirectory of one `t.TempDir()`, `seedPlanValidateFixture(t, dir, true)`, and the `Seed(statusPath, statusLockPath, "fixture-slug", "fixture-parent")` call — so the test's behaviour is unchanged;
  drop only the `Deps` construction, the `testLandingDeps` landing passthrough, and the `Preflight`/`WebsterRun` injection, none of which this test reads.
  For the `NewWebsterProducer` row the test builds, pass `(&fakeWebsterRun{}).run` directly rather than threading a `WebsterRun` field through the fixture struct.

  Do not re-add `testLandingDeps`, `nilFabricOpener`, `fakeMergeShuttle`, or `fakeAlwaysDoneProducer` to `internal/loomshed` — no remaining test in the package uses any of them, and card 5 moved them all out with `fixture_test.go`.
- **Commit:** `test(loomshed): keep the cancellation guard with a reduced fixture`

### Card 11: Confirm `internal/loomshed`'s remaining suite is self-contained

- **Context:**
  - `internal/loomshed/cancellation_test.go`
  - `internal/loomshed/batchifier_test.go`
  - `internal/loomshed/discussionvalidate_test.go`
  - `internal/loomshed/planvalidate_test.go`
  - `internal/loomshed/loompreflight_test.go`
  - `internal/loomshed/stub_test.go`
  - `internal/loomshed/webster_test.go`
  - `internal/loomshed/ctx_test.go`
  - `internal/loomshed/seed_test.go`
  - `internal/loomshed/seam_enforcement_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run `go test ./internal/loomshed/... -count=1` and confirm the eight per-producer test files that stay pass untouched.
  Then grep `internal/loomshed`'s remaining `_test.go` files for `testLandingDeps`, `nilFabricOpener`, `fakeMergeShuttle`, `fakeAlwaysDoneProducer`, `buildSequenceFixture`, `wantProducerTable`, and `testDeps` and confirm zero hits — a dead fixture left behind is exactly what this card exists to catch.
  Do not change any file in this card;
  if a hit is found, fix it in the owning card above rather than here.
  This card makes no edit and carries no commit.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/loomrecipe/... ./internal/loomshed/...` covers both sides of every move.
On the `internal/loomrecipe` side it runs the four moved/new test files — `shape_test.go` (seven surviving shape-and-identity tests), `recipe_test.go` (shape assertion, structural check, construction failure, seed/resume name pin), `sequence_test.go` (the row-order regression guard), and `resume_test.go` (three resume tests plus three bounce-routing tests) — plus `seam_enforcement_test.go` from batch 1, which now sees the same production import set and must stay green.
On the `internal/loomshed` side it runs the eight per-producer files that stay plus the newly extracted `cancellation_test.go`, proving the reduced fixture is sufficient and that nothing left behind references a moved helper.

Both halves are tier 1: hand-built `Env` over `t.TempDir()`, test doubles for every seam, no process spawn.
The one place that could breach it is the real `Preflight` producer's `Call`, which the row-1 substitution prevents at all ten `New` sites.
A missed substitution surfaces as a `git`-spawning test rather than a silent pass, because `preflight.Check` against a bare `t.TempDir()` fails.

The module-wide `go build ./...` at the batch boundary is what catches the cross-package risk here: `internal/shedbuild/equivalence_test.go` and `internal/shedrecipe/coverage_guard_test.go` still drive `loomshed.New`, which this batch leaves intact, so they must still compile.
