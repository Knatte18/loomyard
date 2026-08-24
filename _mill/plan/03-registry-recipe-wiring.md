# Batch: registry entry, recipe row flip, and wiring

```yaml
task: 'loom: Discussion-Write producer'
batch: 'registry entry, recipe row flip, and wiring'
number: 3
cards: 14
verify: go test ./internal/shedrecipe/... ./internal/loomrecipe/... ./internal/loomcli/...
depends-on: [1, 2]
```

## Batch Scope

This batch makes row 3 real end to end: it adds the two `shedrecipe.Env` passthrough fields and the `DiscussionWrite` registry entry, flips `contracts/recipes/loom-recipe.yaml`'s row 3 from `engine: Stub` to `engine: DiscussionWrite`, updates every `internal/loomrecipe` test whose fixture or expectation assumes that row is a stub, and fills the new `Env` fields plus the so-far-empty `Env.Shuttle` in `internal/loomcli`'s `wire()`.

The three concerns land as one batch because none of them leaves the tree green on its own.
Adding the registry entry alone breaks `TestCoverageGuard_EveryLoomRowHasAnEngine`, whose fourth assertion fails any registry name no row reaches and no allowlist tolerates.
Flipping the recipe row alone breaks every `internal/loomrecipe` fixture, since the new entry rejects a nil `Env.DiscussionSpec` at construction.
And flipping the row without wiring `wire()` would ship a `lyx loom run` that fails to construct its own producer list.
The three are one green step.

This batch consumes `loomengine.DiscussionDirRel` from batch 1 and `loomshed.NewDiscussionWrite` from batch 2, hence both dependency edges.

Batch-local decisions, differing from nothing in the overview: the registry entry lives in its own `entries_discussionwrite.go` rather than in `entries_simple.go`, because it wraps its constructed producer in a decorator and is therefore not the plain single-constructor shape that file's header comment describes;
and `Env.StencilsDir` stays unfilled in `wire()`, because the `DiscussionSpec` closure captures `websterGeom.StencilsDir` directly rather than reading it back off `Env`.

## Cards

### Card 9: Add the two Env passthrough fields

- **Context:**
  - `internal/shedadapters/singlellm.go`
  - `internal/shedrecipe/env.go`
- **Edits:**
  - `internal/shedrecipe/recipe.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two fields to `shedrecipe.Env` in `internal/shedrecipe/recipe.go`: `DiscussionSpec shedadapters.SpecSource` and `CommitDiscussion func() error`.
  Place both in the injected-seam block alongside `Shuttle`, `Burler`, `WebsterRun`, `WebsterDeps`, and `Landing`, not in the told-roots block above it.
  Give each a doc comment in the surrounding style: `DiscussionSpec` is the injected `shedadapters.SpecSource` the `DiscussionWrite` entry evaluates once per `Call`, named per-producer rather than carried in a generic keyed map because `Env` already carries per-producer named fields;
  `CommitDiscussion` is the injected closure that commits the discussion output directory into the weft, invoked by the entry's commit decorator on a `Done` outcome.
  State on `DiscussionSpec` that it is a closure rather than a resolved value precisely because building the Spec needs a `*lyxcwd.Location`, which the Shed Recipe Registry Invariant bars this package from importing directly.
  Change no existing field and no import — `shedadapters` is already imported here.
- **Commit:** `feat(shedrecipe): add Env.DiscussionSpec and Env.CommitDiscussion`

### Card 10: Implement the DiscussionWrite registry entry

- **Context:**
  - `internal/shedrecipe/entries_simple.go`
  - `internal/shedrecipe/entries_singlellm.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/loomshed/discussionwrite.go`
  - `internal/shedadapters/singlellm.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_discussionwrite.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedrecipe/entries_discussionwrite.go` declaring `discussionWriteEntry`, a `Constructor` with this package's fixed `func(name string, cfg Config, env Env) (shedengine.ShedProducer, error)` signature.
  It takes an empty `Config` — everything it needs is on `Env` — so it must call `configRejectUnknown(cfg)` with no allowed key names as its first act, exactly as `stubEntry` and `discussionValidateEntry` do.
  It then validates three `Env` fields with `requireSeam`, in this order and each naming the entry string `"DiscussionWrite"` and the field name: `env.DiscussionSpec`, `env.CommitDiscussion`, and `env.Shuttle`.
  `requireSeam` is the correct helper for all three rather than `requireAbsRoot`, and it already detects a typed-nil func value, which matters because two of the three are func types.
  On success it returns `loomshed.NewDiscussionWrite(name, shedadapters.NewSingleLLMProducer(name, env.DiscussionSpec, env.Shuttle, env.Now), env.CommitDiscussion)`.
  Pass `name` to both constructors so the decorator and the wrapped producer share the row's durable identity.
  Do not validate `env.Now` — a nil clock is legal and `NewSingleLLMProducer` defaults it to `time.Now`.
  Do not validate `env.StencilsDir`, `env.AnchorPath`, or `env.WorktreeRoot`: unlike `singleLLMEntry`, this entry composes no stencil path and resolves no output file itself — the injected `SpecSource` closure owns both.
  Write a file-header comment and a doc comment on `discussionWriteEntry` in the package's established style, stating what the entry constructs, why the Spec arrives as a closure rather than as recipe `Config`, and why the generic `SingleLLM` entry is not reused here — `{{.slug}}` and `{{.mode_rules}}` are per-run values a static `tokens` map cannot carry, and a generic row's own `model`/`effort` config would bypass the `discussion` role's model-spec resolution and its timeout entirely.
  The file's imports must stay within `internal/shedrecipe`'s existing allowlist: `shedengine`, `shedadapters`, and `loomshed` are all already allowed.
- **Commit:** `feat(shedrecipe): add the DiscussionWrite registry entry`

### Card 11: Register DiscussionWrite and correct the registry's count comment

- **Context:**
  - `internal/shedrecipe/entries_discussionwrite.go`
- **Edits:**
  - `internal/shedrecipe/registry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `"DiscussionWrite": discussionWriteEntry,` to the `registry` map literal in `internal/shedrecipe/registry.go`.
  Place it adjacent to the existing `"DiscussionValidate"` key so the literal keeps reading in the loose grouping it already has;
  the map's iteration order is irrelevant, since `Names` sorts.
  Correct the `registry` doc comment, which currently reads "The table is complete at twelve keys.
  Any thirteenth entry must arrive with a coverage-guard update in the same commit." — this entry is that thirteenth, so the count becomes thirteen and the instruction now names a fourteenth entry.
  Leave the `init()`-self-registration-was-rejected paragraph, `Lookup`, and `Names` unchanged.
- **Commit:** `feat(shedrecipe): register the DiscussionWrite engine`

### Card 12: Fill the two new Env fields in the shedrecipe test fixture

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shuttleengine/spec.go`
- **Edits:**
  - `internal/shedrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `newTestEnv` in `internal/shedrecipe/fixture_test.go` to fill `DiscussionSpec` and `CommitDiscussion`, so every other entry's test in this package keeps constructing against a fully-filled `Env`.
  Fill `DiscussionSpec` with a closure returning a `shuttleengine.Spec` whose `OutputFiles` holds one absolute path under the same `t.TempDir()` root the other fields derive from — absolute is mandatory, since `SingleLLMProducer.Call` rejects a relative entry outright — with a non-empty `Prompt` and `Interactive: false`, and a nil error.
  Fill `CommitDiscussion` with a closure returning nil.
  Update `newTestEnv`'s doc comment to name both new fields alongside the ones it already enumerates.
  Do not change the existing fake types or any existing field of the returned `Env`.
- **Commit:** `test(shedrecipe): fill the new Env closures in newTestEnv`

### Card 13: Test the DiscussionWrite registry entry

- **Context:**
  - `internal/shedrecipe/entries_singlellm_test.go`
  - `internal/shedrecipe/entries_simple_test.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedrecipe/entries_discussionwrite.go`
  - `internal/shedengine/producer.go`
  - `internal/shuttleengine/spec.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_discussionwrite_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedrecipe/entries_discussionwrite_test.go`, mirroring `entries_singlellm_test.go`'s existing table shape and using `newTestEnv(t)` as its base `Env`.
  Cover the construction-time rejections first: a nil `Env.DiscussionSpec`, a nil `Env.CommitDiscussion`, and a nil `Env.Shuttle` each fail with an error naming both the entry string `DiscussionWrite` and the offending field name.
  Add a typed-nil case for each of the two func fields — assigning a nil-valued variable of the field's own func type, not the untyped `nil` literal — so `requireSeam`'s reflect-based typed-nil detection is genuinely exercised rather than only its plain-nil branch.
  Cover an unknown `Config` key being rejected, and cover the happy path returning a non-nil `shedengine.ShedProducer` with a nil error.
  Then drive that happy-path producer's `Call` once with `context.Background()` against a `fakeShuttle` whose `result` is set to `shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}`, and assert three things: the injected `SpecSource` was evaluated (the fake records the `Spec` it received, so assert on that recorded value), the returned `shedengine.OutputPointer.Path` equals the Spec's first `OutputFiles` entry, and the injected commit closure was invoked exactly once.
  Add one further case where the shuttle reports `shuttleengine.OutcomeAsking`, asserting the outcome maps to `shedengine.Stuck` and the commit closure was not invoked — the mapping the decorator must preserve untouched.
  Keep every path inside the test's own `t.TempDir()`, per this package's own no-real-repo-paths rule.
- **Commit:** `test(shedrecipe): cover the DiscussionWrite entry's validation and Call mapping`

### Card 14: Move the exact-names pin from twelve to thirteen

- **Context:**
  - `internal/shedrecipe/registry.go`
- **Edits:**
  - `internal/shedrecipe/registry_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/shedrecipe/registry_test.go`, rename `TestRegistry_ShipsTwelveEntries` to `TestRegistry_ShipsThirteenEntries` and insert `"DiscussionWrite"` into its `want` slice in the correct sorted position — immediately after `"DiscussionValidate"` and before `"Finalize"`, since `Names` sorts byte-wise and the test compares element by element.
  The slot is after `"DiscussionValidate"`, not before it: the two names share the `Discussion` prefix and diverge at `V` versus `W`, so `DiscussionValidate` sorts first.
  Update the test's own doc comment so its "exactly the sorted twelve engine names" phrasing reads thirteen.
  Leave `TestLookup` and `TestNames` unchanged: `TestNames`' sortedness assertion holds with the new key inserted, and its `MatchesRegistryKeys` subtest is derived from `len(registry)` rather than a literal.
- **Commit:** `test(shedrecipe): pin the registry at thirteen entries`

### Card 15: Flip recipe row 3 to the DiscussionWrite engine

- **Context:**
  - `contracts/recipes/recipes.go`
  - `internal/shedrecipe/registry.go`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `contracts/recipes/loom-recipe.yaml`, change the `Discussion-Write` row's `engine: Stub` to `engine: DiscussionWrite`.
  Change nothing else about that row: it keeps `name: Discussion-Write` and `on_done: Discussion-Validate`, and it deliberately gains no `on_stuck`.
  A `Stuck` from this row means the agent ended its turn without writing both files, and a self-bounce would archive the partial work and respawn an agent carrying no information about why the previous one stopped — `Discussion-Validate` already owns the one live bounce path into this row, and that path at least carries the signal that the files were the problem.
  Add no config keys under the row — the entry takes an empty `Config` and rejects any stray key.
  Leave the file's header comment and all twelve other rows untouched.
- **Commit:** `feat(recipes): back loom row 3 with the DiscussionWrite engine`

### Card 16: Give the loomrecipe sequence fixture a real shuttle

- **Context:**
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/archive.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shedrecipe/recipe.go`
- **Edits:**
  - `internal/loomrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomrecipe/fixture_test.go`, declare a `fakeDiscussionShuttle` implementing `shedadapters.Shuttle`, carrying a `writeOutputs bool` field, a recorded call count, and the file contents to write.
  Its `Run` reports `shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}` and, when `writeOutputs` is true, first writes every entry of the received `Spec.OutputFiles` — the decision record with `validDecisionRecord`'s contents (already declared in this file) and the support log with any non-empty placeholder — creating the parent directory if needed.
  Then extend `buildSequenceFixture` to fill three `Env` fields it currently leaves zero: `Shuttle` with a `fakeDiscussionShuttle{writeOutputs: true}`, `DiscussionSpec` with a closure returning a `shuttleengine.Spec` whose `OutputFiles` is `[]string{decisionRecordPath, supportLogPath}` — the same two absolute paths the fixture already computes for `DecisionRecordPath`/`SupportLogPath` — with a non-empty `Prompt` and `Interactive: false`, and `CommitDiscussion` with a closure recording its invocation count and returning nil.
  Give `fakeDiscussionShuttle` a second counter field recording commit invocations, and build `CommitDiscussion`'s closure over that same fake so one handle carries both signals.
  Do NOT change `buildSequenceFixture`'s return signature: it keeps returning exactly `(anchorPath string, env shedrecipe.Env, paths ShedPaths)`.
  Seven call sites across `sequence_test.go` and `resume_test.go` destructure it as `_, env, paths := buildSequenceFixture(t)`, and Go requires an exact arity match on `:=`, so adding a fourth return value or wrapping the three in a struct would fail to compile at five call sites no card in this batch otherwise touches.
  A test that needs the fake reaches it through the returned `Env` instead, by type-asserting `env.Shuttle.(*fakeDiscussionShuttle)` — the fixture is the only thing that ever fills that field, so the assertion is total.
  State that reasoning in the fake's own doc comment so a later fixture edit does not reintroduce the arity change.
  Update `buildSequenceFixture`'s doc comment: row 3 is no longer skipped over by a stub — it now runs a real `SingleLLMProducer` behind the commit decorator, and the fake shuttle writing both output files is what keeps `Discussion-Validate` passing, because `archiveStaleOutputs` renames the fixture's pre-written files away on every `Call`.
- **Commit:** `test(loomrecipe): add a discussion shuttle fake to the sequence fixture`

### Card 17: Update the shape test's producer table and Env builder

- **Context:**
  - `internal/loomshed/discussionwrite.go`
  - `internal/loomrecipe/fixture_test.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shuttleengine/spec.go`
- **Edits:**
  - `internal/loomrecipe/shape_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomrecipe/shape_test.go`, change `wantProducerTable`'s `Discussion-Write` row's `producerType` from `reflect.TypeOf(loomshed.NewStub(""))` to `reflect.TypeOf(loomshed.NewDiscussionWrite("", nil, nil))`.
  Leave that row's `name`, `onStuck` (empty), and `onDone` columns unchanged, and leave every other row of the table unchanged.
  Extend `testEnv` to fill the same three `Env` fields card 16 fills — `Shuttle`, `DiscussionSpec`, and `CommitDiscussion` — but with the non-writing variant: a `fakeDiscussionShuttle{writeOutputs: false}`, so the discussion paths this builder points at stay absent on disk.
  That absence is load-bearing for `TestNew_ValidateSucceedsOnTheRealList`, whose whole premise is that `Discussion-Validate` never reaches `Done` and exhausts its own bounce budget;
  update that test's inline comment, which currently explains the bounce in terms of a stub row 3, to say instead that row 3's fake shuttle deliberately writes nothing, so each bounce re-runs a real producer that leaves the record absent.
  Update `testEnv`'s own doc comment to name the three new fields.
  Leave `TestNew_RoutingGraphIsClean` unchanged — row 3's routing is untouched by this task.
- **Commit:** `test(loomrecipe): expect the DiscussionWrite producer type in the shape table`

### Card 18: Move the coverage guard's row 3 entry off Stub

- **Context:**
  - `internal/shedrecipe/registry.go`
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `internal/loomrecipe/coverage_guard_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomrecipe/coverage_guard_test.go`, change `loomRowEngines`' `loomshed.NameDiscussionWrite` value from `"Stub"` to `"DiscussionWrite"`.
  Do not add `"DiscussionWrite"` to `coverageGuardAllowedUnreachableEngines` — it is now reached by a real row, which is exactly what makes the guard's fourth assertion pass.
  Correct that allowlist's doc comment, which currently says the five "loom: real LLM producers" roadmap items "still stub out Discussion-Write/-Review, Plan-Write/-Review, and Webster-Review": `Discussion-Write` is no longer among them, so the count becomes four and its name drops out.
  Leave `SingleLLM`, `Bouncer`, and `BurlerRound` in that allowlist — all three are still unreached, and the generic `SingleLLM` entry stays in place and untouched for a future recipe row whose Spec really is static recipe config.
  Change neither of the two test functions' bodies.
- **Commit:** `test(loomrecipe): point the coverage guard's row 3 at the DiscussionWrite engine`

### Card 19: Keep the bounce-routing tests genuinely bouncing

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/shedadapters/archive.go`
- **Edits:**
  - `internal/loomrecipe/resume_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomrecipe/resume_test.go`, both `TestBounceRouting_StuckContinuesAtDeclaredTarget` and `TestBounceRouting_BudgetExhaustionBlocks` remove the decision record from disk and depend on nothing restoring it.
  With row 3 now real, `buildSequenceFixture`'s writing shuttle would restore it and destroy both premises, so switch both tests to the non-writing variant — type-assert `env.Shuttle.(*fakeDiscussionShuttle)` and set its `writeOutputs` field to false, immediately after calling `buildSequenceFixture` and before calling `New`.
  Both tests keep their existing `_, env, paths := buildSequenceFixture(t)` destructuring unchanged: card 16 deliberately leaves the fixture's return signature alone, so no call site in this file needs an arity edit.
  Extend each test's doc comment to record why: `Discussion-Write` is a real producer now, and the bounce it receives must leave the record absent for the bounce to repeat.
  `TestBounceRouting_BudgetExhaustionBlocks`' existing per-producer, episode-scoped budget assertion stays exactly as it is — `Discussion-Write` consumes none of `Discussion-Validate`'s budget, and its own episode restarts on each of its `Done` verdicts.
  Leave `TestBounceRouting_EmptyTargetBlocksInstead` on the default writing fixture: it drives `Batchifier` stuck through a malformed config and needs rows 3 and 4 to pass cleanly on the way there.
  Leave the pause/resume test earlier in this file on the default writing fixture too, for the same reason — it must still reach `Publish`.
- **Commit:** `test(loomrecipe): pin the bounce-routing tests to a non-writing discussion shuttle`

### Card 20: Refresh the sequence test's stale stub framing

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `internal/loomrecipe/sequence_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomrecipe/sequence_test.go`, leave `wantSequenceOrder`'s twelve entries and `TestSequence_FullRunBlocksAtPublish`'s assertions structurally unchanged — the sequence, the halt at `Publish`, the per-entry outcome expectations, and the persisted-status assertions all still hold.
  Extend `wantSequenceOrder`'s doc comment, which currently explains only why row 2 passes against the fixture, with a sentence explaining why row 3 now passes too: it is a real `SingleLLMProducer` behind the commit decorator, and the fixture's shuttle fake writes both discussion output files and reports `Done`, so the decorator's injected commit closure fires and `Discussion-Validate` finds a complete pair.
  Add one assertion to `TestSequence_FullRunBlocksAtPublish` proving the commit side actually ran: type-assert `env.Shuttle.(*fakeDiscussionShuttle)` and assert its commit-invocation counter is exactly 1 after a clean run.
  Keep the existing `_, env, paths := buildSequenceFixture(t)` destructuring — card 16 leaves the fixture's return signature unchanged, so the handle arrives through `env`, not through a fourth return value.
  This is the scenario check that a `Done` from row 3 genuinely reaches the weft-commit seam, rather than the decorator being silently bypassed.
- **Commit:** `test(loomrecipe): assert the discussion commit fires on a clean sequence run`

### Card 21: Fill the new Env fields in wire()

- **Context:**
  - `internal/loomcli/run.go`
  - `internal/loomcli/seedinput.go`
  - `internal/loomcli/landingdeps.go`
  - `internal/loomcli/drive.go`
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/config.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `internal/hubgeom/webstergeom.go`
  - `internal/shedrecipe/recipe.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomcli/wiring.go`'s `wire()`, fill three fields of the `c.env` literal that are currently left zero.
  Set `Shuttle: runner` — `runner` is already built above as `shuttleengine.NewRunner(...)`, and `*shuttleengine.Runner` already satisfies `shedadapters.Shuttle`.
  Set `DiscussionSpec` to a closure over `loomengine.DiscussionSpec(location, websterGeom.StencilsDir, loomCfg, registry, seedSlug(location.WorktreeName), true)`, returning its two results unchanged.
  The `autonomous` argument is the literal `true`, unconditionally, per the `autonomous-only` Shared Decision;
  `websterGeom` and every other value the closure captures is already in scope at this point in `wire()`, and `seedSlug` is the same slug derivation `internal/loomcli/run.go` already uses.
  Evaluating the closure per `Call` rather than once here is what keeps the stencil read at call time, as the Stencil Ownership Invariant requires.
  Set `CommitDiscussion` to a closure that calls `fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.DiscussionDirRel()}, fmt.Sprintf("loom: discussion artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())` and returns only its error, discarding the sha and committed results.
  This mirrors the seed commit `internal/loomcli/run.go` already performs, including its `NewMutations("")` record and its `EnvSyncOptions()`.
  The pathspec is the whole discussion directory deliberately, so `archiveStaleOutputs`' timestamped siblings are committed rather than left as untracked weft dirt.
  Correct the literal's trailing comment, which currently reads "StencilsDir, RunRoot, Shuttle, Burler, and Now are left zero -- only SingleLLM, Bouncer, and BurlerRound read them, and no row in loom's recipe uses those engines yet": `Shuttle` is now filled and row 3 does use it, so the comment must name only `StencilsDir`, `RunRoot`, `Burler`, and `Now`, and must say why `StencilsDir` in particular stays unfilled — the `DiscussionSpec` closure captures the stencils directory directly rather than reading it back off `Env` — and that a nil `Now` is legal, defaulting to `time.Now` inside `NewSingleLLMProducer`.
  Leave the `Landing`-is-assembled-in-drive.go paragraph unchanged.
  Add the `fmt` import if it is not already present.
- **Commit:** `feat(loomcli): wire the discussion spec, commit closure, and shuttle into Env`

### Card 22: Assert wire() fills the discussion seams

- **Context:**
  - `internal/loomcli/wiring.go`
  - `internal/loomengine/config.go`
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/prompt.go`
  - `internal/loomengine/prompt_test.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/hubgeom/webstergeom.go`
  - `contracts/stencils/stencils.go`
- **Edits:**
  - `internal/loomcli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add tests to `internal/loomcli/wiring_test.go` covering card 21, following the file's existing `hubLocation(t, ...)` + `c.wire(loc, cwd)` shape and its `t.Parallel()` convention.
  Add a test asserting `c.env.Shuttle`, `c.env.DiscussionSpec`, and `c.env.CommitDiscussion` are each non-nil after `wire()`, and that `c.env.Shuttle` is the same `*shuttleengine.Runner` value `c.runner` holds.
  Before that second test can evaluate the Spec closure, the discussion stencil must exist on disk.
  Add a `seedDiscussionStencil(t *testing.T, hubPath string)` helper to this file that writes `stencils.LoomTemplateDiscussion`'s embedded bytes to `filepath.Join(fabricengine.StencilsDir(hubPath), "loom", "loom-template-discussion.md")`, creating the parent directories first, and mirroring `internal/loomengine/prompt_test.go`'s `newTestStencilsDir` in shape.
  That path is what the closure actually reads: `hubgeom.WebsterGeometry`'s `StencilsDir` field is `fabricengine.StencilsDir(l.HubPath)`, and `stencilstore.Read` hard-errors on a missing file rather than falling back to the embedded default, so without this seed the closure returns an error and every Spec assertion below is unreachable.
  `hubLocation` currently seeds only `loom.yaml` and `landing.yaml` under the anchor's `_lyx/config/`, so call the new helper from `hubLocation` — every test in this file then keeps a fully-seeded hub, and no existing test changes shape.
  Add a second test evaluating `c.env.DiscussionSpec()` once and asserting on the returned `shuttleengine.Spec`: `Interactive` is false, `Role` is `"discussion"`, `Timeout` equals `time.Duration(loomengine.ConfigTemplate()`'s `discussion_timeout_min`) in minutes — read it from the loaded `c.cfg.DiscussionTimeoutMin` rather than hard-coding 480 — `Model` is non-empty, and `OutputFiles` holds exactly two absolute paths equal to `loomengine.DiscussionDecisionRecord(loc)` and `loomengine.DiscussionSupportLog(loc)` in that order.
  Assert `Prompt` is non-empty and contains no unrendered `{{` marker, which is what proves the stencil actually filled rather than the closure returning a half-built Spec.
  Do not invoke `c.env.CommitDiscussion` in any test: it would run a real fabric commit against a temp directory that is no fabric, which the Test Tier Purity Invariant forbids — assert only that it is non-nil.
  Leave every existing test function in this file unchanged — the only pre-existing symbol this card touches is the `hubLocation` helper, which gains the one seeding call above and nothing else.
- **Commit:** `test(loomcli): assert wire fills the discussion spec, commit closure, and shuttle`

## Batch Tests

`verify: go test ./internal/shedrecipe/... ./internal/loomrecipe/... ./internal/loomcli/...` runs exactly the three packages this batch's production edits touch, plus `contracts/recipes`' embedded recipe, which `internal/loomrecipe` parses and builds on every one of its tests.
It is scoped to three packages rather than the whole tree because no other package reads the two new `Env` fields or the new registry key;
the repo-wide sweep is `pipeline.done_gate`'s job at the end of the run, not this batch's.

The three packages cover the batch's scenarios directly.
`internal/shedrecipe` covers the entry's construction-time validation (including the typed-nil func cases `requireSeam`'s reflect branch exists for), its `Config`-rejection, and one full `Call` proving the `SpecSource`, the shuttle, and the commit closure are all reached and the `OutputPointer` passes through — plus the renamed thirteen-names pin.
`internal/loomrecipe` covers the recipe still parsing and building end to end with row 3 on the new engine (`recipe_test.go`, `shape_test.go`), the row still routing `on_done: Discussion-Validate` with no `on_stuck` (`shape_test.go`'s producer table and `TestNew_RoutingGraphIsClean`), the coverage guard's both-directions row-to-engine pin, a clean twelve-row run in which the commit closure fires exactly once, and the two bounce tests that must keep bouncing.
`internal/loomcli` covers `wire()` filling all three seams and the Spec closure evaluating to the expected shape.

A second `Done` over already-committed artifacts is a no-op rather than an error, and that is covered by construction rather than by a dedicated test: `CommitAnchoredPaths` reports `committed == false` for an already-clean, already-tracked path, which card 21 discards, so the closure returns nil either way.
Every test in this batch is hermetic and untagged: no real agent, no real cwd resolution, and no real fabric — `CommitDiscussion` is a closure the tests either never invoke or replace with a counter.
