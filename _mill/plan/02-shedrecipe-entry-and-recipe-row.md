# Batch: shedrecipe-entry-and-recipe-row

```yaml
task: 'loom: Plan-Write producer'
batch: 'shedrecipe-entry-and-recipe-row'
number: 2
cards: 7
verify: go test ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/loomrecipe/...
depends-on: [1]
```

## Batch Scope

This batch takes `internal/shedrecipe`'s registry from thirteen entries to fourteen and flips the `Plan-Write` recipe row from `Stub` to the new `PlanWrite` engine: two new named `Env` seams, a `planWriteEntry` constructor in its own file, the registry key, the two coverage fixtures that break the moment a fourteenth engine appears, and the recipe row itself.
It is one batch rather than two because `internal/loomrecipe`'s coverage guard rejects any registry entry no row reaches and that is absent from `coverageGuardAllowedUnreachableEngines` — so registering `PlanWrite` without flipping the row in the same batch would leave `go test ./...` red at a batch boundary, and the two halves are one unit of work even though they span four Go packages plus a data file.
The external interface batch 3 consumes is `shedrecipe.Env`'s new `PlanSpec shedadapters.SpecSource` and `CommitPlan func() error` fields, which `internal/loomcli`'s `wire()` is what fills for real.
Batch-local decision, differing from nothing in `## Shared Decisions`: cards 4 through 10 must land in this stated order, because card 6's registry key is what makes cards 7 and 9 necessary and card 8's fixture widening is what keeps card 9's row flip from failing construction for every test in `internal/loomrecipe`.
Running the batch verify before card 9 lands will report failures that card 9 is what fixes.
Card 8 owns the `fakeDiscussionShuttle` → `fakeLoomShuttle` rename across all four `internal/loomrecipe` test files that name the type, rather than leaving each file's rename to the card that otherwise edits it — the type is declared in one file and read from three others, so a rename split across cards leaves the package uncompilable between commits.

## Cards

### Card 4: two named Env seams for the PlanWrite entry

- **Context:**
  - `internal/loomengine/plan.go`
  - `internal/planparser/parse.go`
  - `internal/shedadapters/singlellm.go`
- **Edits:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/shedrecipe/recipe.go`, add two fields to the `Env` struct immediately after the existing `CommitDiscussion` field, keeping them in the same injected-seam block rather than moving them up among the told-path fields: `PlanSpec shedadapters.SpecSource` and `CommitPlan func() error`. `PlanSpec`'s field doc must state that it is the injected `shedadapters.SpecSource` the `PlanWrite` entry evaluates once per `Call`, that it arrives as a closure rather than as recipe `Config` because building the Spec needs a `*lyxcwd.Location` the Shed Recipe Registry Invariant bars this package from importing directly, that `internal/loomcli`'s `wire()` is what supplies it, and that it is a second per-producer named field rather than the first entry of a generic keyed map because `Env` already carries per-producer named fields and forking that convention on the second instance would abandon a decision one commit old. `CommitPlan`'s field doc must state that it is the injected closure committing the plan output directory, invoked by the `PlanWrite` entry's commit decorator on a `Done` outcome. Add no import to this file — `shedadapters` is already imported.

  In `internal/shedrecipe/fixture_test.go`, extend `newTestEnv` to fill both new fields, mirroring exactly how it fills `DiscussionSpec`/`CommitDiscussion` today: `PlanSpec` returns a `shuttleengine.Spec` with a short prompt string, `OutputFiles` holding one absolute path under the same `t.TempDir()` root (a `plan-output.md` sibling of the existing `discussion-output.md`), and `Interactive: false`; `CommitPlan` returns nil. Update `newTestEnv`'s own doc comment, which currently describes filling `DiscussionSpec` and `CommitDiscussion` by name, so it names all four seams. Do not reference any path outside the function's own `t.TempDir()`, per that file's own stated rule.
- **Commit:** `feat(shedrecipe): add the PlanSpec and CommitPlan Env seams`

### Card 5: the PlanWrite registry entry

- **Context:**
  - `internal/shedrecipe/entries_discussionwrite.go`
  - `internal/shedrecipe/entries_discussionwrite_test.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/loomshed/planwrite.go`
  - `internal/shedadapters/singlellm.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/entries_planwrite.go`
  - `internal/shedrecipe/entries_planwrite_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/shedrecipe/entries_planwrite.go` declares `planWriteEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error)`, the Constructor for the `"PlanWrite"` registry row, modelled line-for-line on `discussionWriteEntry` in `internal/shedrecipe/entries_discussionwrite.go`. In order it calls `configRejectUnknown(cfg)`, then `requireSeam("PlanWrite", "PlanSpec", env.PlanSpec)`, `requireSeam("PlanWrite", "CommitPlan", env.CommitPlan)`, `requireSeam("PlanWrite", "Shuttle", env.Shuttle)`, then `requireAbsRoot("PlanWrite", "AnchorPath", env.AnchorPath)`, returning each error unwrapped. It then builds `inner := shedadapters.NewSingleLLMProducer(name, env.PlanSpec, env.Shuttle, env.Now)` and returns `loomshed.NewPlanWrite(name, inner, env.CommitPlan, env.AnchorPath, env.Now), nil`. The file header comment must say the entry lives in its own file rather than in `entries_simple.go` because that file's own header describes only the plain single-constructor shape. The function's doc comment must record why the generic `"SingleLLM"` entry is not reused: building the Spec needs a `*lyxcwd.Location` the Shed Recipe Registry Invariant bars this package from importing, and a generic row's own `model`/`effort` `Config` keys would bypass the `plan` role's model-spec resolution and its `plan_timeout_min` timeout entirely. It must also record that `AnchorPath` is validated here and threaded through because `loomshed.NewPlanWrite` resolves the plan directory itself via `planparser.PlanDir`, the same split `planValidateEntry` already uses, which keeps this package free of any `planparser` import. The row carries no `Config` keys of its own, per the Config Strictness Invariant.

  `internal/shedrecipe/entries_planwrite_test.go` is modelled on `internal/shedrecipe/entries_discussionwrite_test.go`. Cover: a `TestPlanWriteEntry_ConstructionFailures` with subtests zeroing `env.PlanSpec`, `env.CommitPlan`, and `env.Shuttle` in turn, each asserting a non-nil error whose text contains both `"PlanWrite"` and the missing field name; a subtest zeroing `env.AnchorPath` asserting an error naming both `"PlanWrite"` and `"AnchorPath"`; and a subtest passing `Config{"bogus_key": "x"}` asserting an error naming the offending key. Add a `TestPlanWriteEntry_HappyPath` asserting a fully-filled `newTestEnv(t)` builds a non-nil producer with a nil error. Add a `TestPlanWriteEntry_CallDone` that replaces `env.PlanSpec` with a closure returning a Spec whose single `OutputFiles` entry is an absolute path under `env.AnchorPath`, replaces `env.CommitPlan` with a counting closure, sets the `fakeShuttle` reached via `env.Shuttle.(*fakeShuttle)` to report `shuttleengine.OutcomeDone`, and asserts the injected `SpecSource` was evaluated exactly once, the returned `OutputPointer.Path` equals that Spec's first `OutputFiles` entry, and the commit closure fired exactly once. Add a `TestPlanWriteEntry_CallAsking` asserting `shuttleengine.OutcomeAsking` maps to `shedengine.Stuck` with the commit closure uninvoked. Note that `env.AnchorPath` from `newTestEnv` is a real directory that contains no plan directory, so the decorator's rotation is a legitimate absent-directory no-op in every one of these tests.
- **Commit:** `feat(shedrecipe): add the PlanWrite registry entry`

### Card 6: register PlanWrite as the fourteenth engine

- **Context:**
  - `internal/shedrecipe/entries_planwrite.go`
  - `internal/loomrecipe/coverage_guard_test.go`
- **Edits:**
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/registry_test.go`
  - `CONSTRAINTS.md`
  - `manifest/designs/shed-recipe.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/shedrecipe/registry.go`, add `"PlanWrite": planWriteEntry` to the `registry` map literal, placed immediately after the existing `"PlanValidate"` key so the literal's existing grouping order is preserved. Update the `registry` doc comment, which currently reads "The table is complete at thirteen keys. Any fourteenth entry must arrive with a coverage-guard update in the same commit" — it now reads that the table is complete at fourteen keys and that any fifteenth entry must arrive with a coverage-guard update in the same commit. Leave the `init()`-self-registration paragraph unchanged.

  In `internal/shedrecipe/registry_test.go`, rename `TestRegistry_ShipsThirteenEntries` to `TestRegistry_ShipsFourteenEntries`, add `"PlanWrite"` to its `want` slice in sorted position (between `"PlanValidate"` and `"Preflight"`), and update the test's doc comment where it says "the sorted thirteen engine names this task's registry ships" to say fourteen.

  In `CONSTRAINTS.md`, the Shed Recipe Registry Invariant's **Enforced by** bullet names `internal/shedrecipe/registry_test.go` (`TestRegistry_ShipsThirteenEntries`), which pins the registry's exact thirteen names — change both the test symbol and the count to the fourteen forms. Change nothing else in `CONSTRAINTS.md`: this task records no new invariant, and this edit is a factual correction to an existing invariant's enforcement pointer only.

  In `manifest/designs/shed-recipe.md`, two statements go stale with this card. The "The consumer." bullet says `internal/shedrecipe`'s registry already imports `loomshed` for seven of its constructors — change `seven` to `eight`. The Motivation paragraph says that `Discussion-Write` "turns out not to fit that mold" and ships as its own registry entry over an injected `shedadapters.SpecSource` closure — extend that passage to name `Plan-Write` as the second such row, stating that its Spec is resolved from the `plan` role's own model-spec and timeout rather than from recipe strings and that it needs a `*lyxcwd.Location` to build the plan paths, so it ships as its own `PlanWrite` entry for the same reason. Follow this repo's semantic-line-break rule in both edits: one sentence per line, breaking long sentences at internal independent-clause boundaries, never hard-wrapping at a fixed column.
- **Commit:** `feat(shedrecipe): register PlanWrite as the fourteenth engine`

### Card 7: widen the shedbuild coverage fixture for the fourteenth engine

- **Context:**
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedrecipe/entries_planwrite.go`
- **Edits:**
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedbuild/build_engines_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `TestBuild_EveryRegisteredEngineBuilds` in `internal/shedbuild/build_engines_test.go` drives its assertion off `shedrecipe.Names()`, so the new `"PlanWrite"` engine fails it until this package's fixture fills the seams that entry requires. In `internal/shedbuild/fixture_test.go`, extend `newTestEnv` to fill `PlanSpec` with a closure returning a `shuttleengine.Spec` over one absolute output path under the same `t.TempDir()` root and `CommitPlan` with a closure returning nil, exactly mirroring how it already fills `DiscussionSpec`/`CommitDiscussion`. `AnchorPath` is already filled with an absolute `mustMkdir` path, so `requireAbsRoot("PlanWrite", "AnchorPath", ...)` passes with no further change. Update `newTestEnv`'s doc comment, which currently names "the two per-producer DiscussionWrite seams", so it names all four per-producer seams across the two write engines.

  In `internal/shedbuild/build_engines_test.go`, `engineMinimalConfig`'s doc comment says "The other ten engines take no config at all" and singles out `DiscussionWrite` as one of the ten — update the count to eleven and extend the `DiscussionWrite` sentence to cover `PlanWrite` alongside it, since both wrap a single-LLM producer whose Spec arrives as an injected `Env` closure rather than as recipe `Config`. `TestBuild_EveryRegisteredEngineBuilds`'s own doc comment says a fourteenth registered engine fails the test until the fixture covers it — change `fourteenth` to `fifteenth`. Add no entry to `engineMinimalConfig`'s returned map: `PlanWrite` takes no config keys, and a non-empty config block on it is an error from its constructor.
- **Commit:** `test(shedbuild): fill the PlanWrite seams in the coverage fixture`

### Card 8: generalise the loomrecipe sequence fixture to serve two real LLM rows

- **Context:**
  - `internal/shedrecipe/recipe.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
  - `internal/loomshed/planwrite.go`
  - `internal/shedadapters/singlellm.go`
- **Edits:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/resume_test.go`
  - `internal/loomrecipe/sequence_test.go`
  - `internal/loomrecipe/shape_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `shedrecipe.Env` carries one `Shuttle` field, not one per row, so the single fake in `internal/loomrecipe/fixture_test.go` must now serve both `Discussion-Write` and `Plan-Write`. Rename the `fakeDiscussionShuttle` type to `fakeLoomShuttle`, rename its `commitCalls` field to `commitDiscussionCalls`, add a `commitPlanCalls int` field, and add a `planDir string` field.

  This card owns the whole rename, across every reference in the package — the type is declared in `internal/loomrecipe/fixture_test.go` but read from three other test files, and a partial rename leaves the package uncompilable. In `internal/loomrecipe/fixture_test.go` update the type declaration, the `var _ shedadapters.Shuttle = (*fakeDiscussionShuttle)(nil)` assertion, the `Run` method receiver, the three `fmt.Errorf` strings naming the type, `buildSequenceFixture`'s own construction of it, and both doc comments that name it. In `internal/loomrecipe/resume_test.go` update the two `env.Shuttle.(*fakeDiscussionShuttle).writeOutputs = false` type assertions. In `internal/loomrecipe/sequence_test.go` update the `env.Shuttle.(*fakeDiscussionShuttle)` type assertion and the `t.Errorf` string naming the type, and retarget the `commitCalls` field read to `commitDiscussionCalls`. In `internal/loomrecipe/shape_test.go` update `testEnv`'s `Shuttle: &fakeDiscussionShuttle{writeOutputs: false}` construction and the two doc comments that name the type.

  `internal/loomrecipe/shape_test.go`'s `testEnv` additionally needs the two new `Env` seams filled, or `New` fails construction for every test that uses it — `TestNew_ProducerTable` and both coverage-guard tests — the moment card 9 flips the recipe row. Fill `PlanSpec` with a closure returning a `shuttleengine.Spec` over one absolute path under that builder's own `t.TempDir()` root and `CommitPlan` with a closure returning nil, mirroring how it already fills `DiscussionSpec`/`CommitDiscussion`, and set `Role: "plan"` on that Spec and `Role: "discussion"` on the existing `DiscussionSpec` Spec so the fake's branch is total there too. Extend `testEnv`'s doc comment, which currently names `Shuttle`, `DiscussionSpec`, and `CommitDiscussion`, to name the two new seams as well. `testEnv`'s fake is the non-writing variant (`writeOutputs: false`) and stays that way: `TestNew_ProducerTable` asserts on the assembled table's shape rather than running the list, so no plan-directory fixture is needed there.

  Change `Run` to branch on `spec.Role`. When `spec.Role` is `"plan"`, it writes the whole plan-directory fixture — `os.MkdirAll(f.planDir, 0o755)`, then `01-first-card.md` and `00-overview.md` with the same bytes `seedPlanValidateFixture` writes for an approved plan — and reports `shuttleengine.OutcomeDone`. Otherwise it keeps today's behaviour verbatim: when `f.writeOutputs` is true, write every `spec.OutputFiles` entry with this fake's configured discussion contents, creating any missing parent directory, then report `OutcomeDone`. To keep the two writers from drifting, first extract the two plan-fixture bodies out of `seedPlanValidateFixture` into package-level test helpers — a `planFixtureCard` string constant holding the one-card body, and a `planFixtureOverview(approved bool) string` function holding the `fmt.Sprintf` overview — and have both `seedPlanValidateFixture` and the fake call them.

  Writing the whole plan directory rather than only `spec.OutputFiles` is required, not incidental: `loomshed.NewPlanWrite`'s rotation archives every top-level `.md` file — including the card file `seedPlanValidateFixture` pre-wrote — before the shuttle runs, so a fake writing only the overview would leave the Card Index naming a card file that no longer exists in the plan directory and `Plan-Validate` would report `Stuck` and bounce.

  In `buildSequenceFixture`, set `planDir` on the fake to `filepath.Join(dir, lyxdirs.LyxDirName, "plan")` — the same expression `seedPlanValidateFixture` already builds, so keep the two consistent by having the fixture compute it once and pass it to both. Add `Role: "discussion"` to the existing `DiscussionSpec` closure's returned Spec so the fake's branch is total rather than relying on the default arm. Add a `PlanSpec` closure returning a Spec with a short prompt, `OutputFiles` holding the single absolute overview path (`filepath.Join(planDir, "00-overview.md")`), `Role: "plan"`, and `Interactive: false`. Add a `CommitPlan` closure incrementing `commitPlanCalls` on the same fake and returning nil. Update `buildSequenceFixture`'s own doc comment, which currently explains only row 3's fake-shuttle wiring, to explain row 6's alongside it, including why the plan branch rewrites the whole directory. Number `Plan-Write` as row 6 in every doc comment this card writes, never row 7: `manifest/designs/loom.md`'s table numbers it 7 because that table includes the never-built `Plan-Sweep` as row 6, but the built list has no `Plan-Sweep` row at all, so `Plan-Write` is the sixth entry of `wantSequenceOrder` — and this package's existing, unedited comments already use the built-list numbering, calling `Publish` row 12 and `Finalize` row 13. Do not change the function's return signature — seven call sites destructure it as a three-value `:=`.
- **Commit:** `test(loomrecipe): generalise the sequence fixture shuttle for Plan-Write`

### Card 9: flip the Plan-Write recipe row to the PlanWrite engine

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/planwrite.go`
  - `internal/loomrecipe/fixture_test.go`
  - `internal/shedrecipe/registry.go`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/shape_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `contracts/recipes/loom-recipe.yaml`, the `Plan-Write` row's `engine: Stub` becomes `engine: PlanWrite`. Change nothing else in that row: its `name`, its position in the list, and its `on_done: Plan-Validate` are already correct, and it deliberately carries no `on_stuck` because it is the bounce target of two gates and nothing in the list produces what a stuck writer would need, so escalation to a human is the correct terminal behaviour per the file's own header comment. Do not add an `on_stuck` key. The row-name string stays byte-identical, since it is the durable on-disk identity in `current_producer` and a rename breaks resume for any in-flight task.

  In `internal/loomrecipe/coverage_guard_test.go`, change `loomRowEngines[loomshed.NamePlanWrite]` from `"Stub"` to `"PlanWrite"`. Leave `coverageGuardAllowedUnreachableEngines` at all three of `"SingleLLM"`, `"Bouncer"`, and `"BurlerRound"`: `Plan-Write` now consumes the dedicated `PlanWrite` engine rather than the generic `SingleLLM`, and the three still-stubbed rows (`Discussion-Review`, `Plan-Review`, `Webster-Review`) leave those three genuinely unreferenced. `"Stub"` stays in the registry and in the table for those three rows. Update that variable's doc comment, which currently says the four remaining "loom: real LLM producers" roadmap items still stub out Discussion-Review, Plan-Write/-Review, and Webster-Review — `Plan-Write` is no longer among them.

  In `internal/loomrecipe/shape_test.go`, the `Plan-Write` row's expected `producerType` changes from `reflect.TypeOf(loomshed.NewStub(""))` to `reflect.TypeOf(loomshed.NewPlanWrite("", nil, nil, "", nil))`. Leave that row's expected `OnStuck` empty and its expected `OnDone` at `loomshed.NamePlanValidate`, and leave the `Discussion-Review` row's `OnDone` and the `Plan-Validate`/`Plan-Review` rows' `OnStuck` all pointing at `loomshed.NamePlanWrite` unchanged.
- **Commit:** `feat(loom): run Plan-Write on the PlanWrite engine`

### Card 10: assert the plan commit fires on a clean sequence run

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomshed/planwrite.go`
- **Edits:**
  - `internal/loomrecipe/sequence_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Card 8 already renamed this file's type assertion and field read as part of owning the whole rename, so this card adds only the new assertion and the doc-comment update. In `internal/loomrecipe/sequence_test.go`, `TestSequence_FullRunBlocksAtPublish` ends with a scenario check that row 3's `Done` reaches the Fabric-commit seam. Add a second assertion in that same block, that `commitPlanCalls` on the same fake is exactly 1 after a clean run, so a `Done` from row 6 is proven to reach its own commit seam rather than the decorator being silently bypassed. Update `wantSequenceOrder`'s doc comment, which explains why rows 1 through 3 pass against this fixture, to add the equivalent sentence for row 6: it is a real `shedadapters.SingleLLMProducer` behind `loomshed`'s rotate-and-commit decorator now, and the fixture's fake shuttle rewrites the whole plan directory on its `"plan"`-role branch so `Plan-Validate` still finds a complete, approved, zero-findings plan after the rotation archived the seeded one. Number `Plan-Write` as row 6, not row 7 — it is the sixth entry of `wantSequenceOrder`, and this file's own existing comments already number against that built list, calling `Publish` row 12 and `Finalize` row 13. Leave `wantSequenceOrder` itself unchanged — the row order and the `Publish`-Stuck halt are unaffected.
- **Commit:** `test(loomrecipe): assert Plan-Write reaches its commit seam`

## Batch Tests

`verify: go test ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/loomrecipe/...` covers the three Go packages this batch touches.
`internal/shedrecipe` carries the new `entries_planwrite_test.go`, the widened `newTestEnv`, and the renamed exact-names registry pin.
`internal/shedbuild` carries `TestBuild_EveryRegisteredEngineBuilds`, which turns a missing fixture seam into a failure and is therefore the real gate on card 7.
`internal/loomrecipe` carries the coverage guard, the shape test, and the full-sequence run — the three that together prove the recipe row genuinely resolves to the new engine and that the producer runs end-to-end inside the real thirteen-row list.
All three are untagged tier 1.
The recipe data file `contracts/recipes/loom-recipe.yaml` has no test package of its own: it is embedded and parsed by `internal/loomrecipe`, so its verification is that package's coverage guard and shape test, both already in scope.
