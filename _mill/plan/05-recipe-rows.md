# Batch: recipe-rows

```yaml
task: 'loom: Discussion-Review producer'
batch: 'recipe-rows'
number: 5
cards: 8
verify: go test ./internal/loomrecipe/... ./internal/loomshed/...
depends-on: [1, 2, 4]
```

## Batch Scope

This is the batch that makes the segment real: the stubbed `Discussion-Review` recipe row becomes the two-row `Discussion-Bouncer`/`Discussion-Burler` perch, `internal/loomshed`'s row-name constants follow, and every guard in `internal/loomrecipe` moves with them.
It depends on batch 1 (the rubric stencil must be registered before a production `NewBouncer` can probe it), batch 2 (the `profile.rubric_stencil` key and the `Env.Review*` fallback the rows rely on), and batch 4 (a `wire()` that fills `RunRoot`, `StencilsDir`, and `Burler`, without which `loomrecipe.New` fails at construction in production the moment these rows go live).
It ships no external interface of its own;
batch 6 consumes only the row names and counts it establishes.

## Cards

### Card 14: replace the stub row with the two-row perch

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/entries_burler.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/burler.go`
  - `internal/shedengine/validate.go`
  - `internal/loomengine/config.go`
  - `contracts/stencils/loom/loom-rubric-discussion-review.md`
  - `_mill/discussion.md`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/doc.go`
  - `internal/loomshed/stub.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/loomshed/loomshed.go`, delete the `NameDiscussionReview` constant and add `NameDiscussionBouncer = "Discussion-Bouncer"` and `NameDiscussionBurler = "Discussion-Burler"` in its place, keeping the const block in recipe-row order.
  Update this file's own header comment and the const block's doc comment from "thirteen" to "fourteen" row names throughout, and update `internal/loomshed/doc.go`'s package comment the same way.
  In `internal/loomshed/stub.go`, three separate claims go stale and all three move together: the file's own header comment says "13-row producer list", `stubProducer`'s doc comment says "backs three rows of loom's 13-row producer list", and that same doc comment lists `Discussion-Review`, `Plan-Review`, and `Webster-Review` as the stubbed rows.
  After this card the list is fourteen rows and exactly two of them are stubbed, so both "13-row" occurrences become "14-row", "three rows" becomes "two rows", and `Discussion-Review` drops out of the list.
  In `contracts/recipes/loom-recipe.yaml`, replace the single `Discussion-Review` row with two rows in its place, keeping every other row byte-identical.
  Re-point the preceding `Discussion-Validate` row's `on_done` from `Discussion-Review` to `Discussion-Bouncer`, leaving its `on_stuck: Discussion-Write` unchanged — a mechanical validation failure is a writing failure, not a judgment one.
  The first new row is `name: Discussion-Bouncer`, `engine: Bouncer`, `segment: Discussion-Review`, `max_bounces: 5`, `on_stuck: Discussion-Burler`, `on_done: Plan-Write`, with a `config:` map carrying `run_subdir: discussion`, `artifact_paths` listing the two worktree-relative discussion files (`_lyx/discussion/decision-record.md` and `_lyx/discussion/support-log.md`), and `rubric_stencil: loom-rubric-discussion-review`.
  Set no `model`, `effort`, or `version` key: their absence is what makes the row take the run-wide `Env.Review*` values loom.yaml supplies.
  The second new row is `name: Discussion-Burler`, `engine: BurlerRound`, `segment: Discussion-Review`, `max_bounces: 5`, `on_stuck: Discussion-Bouncer`, `on_done: Discussion-Bouncer`, with a `config:` map carrying the same `run_subdir: discussion` (the shared value is what makes both rows write into one directory) and a `profile:` map holding: `target.paths` naming the same two worktree-relative discussion files;
  `fasit.instructions` stating in prose that the authority for this round is the rubric supplied in the prompt and that the mechanical section contract is already enforced upstream by `Discussion-Validate` and is not this round's subject, with no `fasit.paths`;
  `rubric_stencil: loom-rubric-discussion-review`;
  `fix-scope: source`;
  and `tool-use: true`.
  Omit `cluster-fan` entirely.
  Set no `model`, `effort`, or `timeout_s` key, for the same `Env` fallback reason as the Bouncer row.
  Update the yaml file's own header comment: it currently says "The thirteen row names below" — make it fourteen, and add a sentence naming the segment label and the mutual `on_stuck` pair as the reason both new rows carry `segment: Discussion-Review`, since `internal/shedengine/validate.go` rejects an `OnStuck` naming a producer in a different segment.
  Explain in a comment on `Discussion-Burler` why its `on_done` is set at all: `BurlerProducer` documents that it never returns `Done`, so the edge is unreachable, but an empty `OnDone` is load-bearing and ends the whole run silently, which is a worse failure than a redundant edge.
- **Commit:** `feat(loom): replace the Discussion-Review stub with a Bouncer/Burler perch`

### Card 15: seed the loomrecipe Env fixtures for a live segment

- **Context:**
  - `contracts/stencils/stencils.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/burler.go`
  - `internal/shedadapters/burler_test.go`
  - `internal/shedrecipe/entries_bouncer_test.go`
  - `internal/shedrecipe/fixture_test.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/shape_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Both Env builders in this package — `testEnv` in `shape_test.go` and `buildSequenceFixture` in `fixture_test.go` — must fill `StencilsDir`, `RunRoot`, `Burler`, and `Now`, or `New` fails at construction for every test in the package once card 14's rows are live.
  Add a shared `seedBouncerStencils(t *testing.T, dir string)` helper to `fixture_test.go` (the file that already holds this package's shared fixture helpers) writing three stencils into `dir` at `stencilstore.Path(dir, name)`, creating parent directories as needed: `bouncer-template-seed` and `bouncer-template-judge` from `stencils.BouncerTemplateSeed` and `stencils.BouncerTemplateJudge`, and `loom-rubric-discussion-review` from `stencils.LoomRubricDiscussionReview`.
  Seed the real embedded bytes rather than dummy content: `shedadapters.NewBouncer` probes the rubric eagerly at construction, and `seedCall`/`judgeCall` read the two templates at call time and degrade to `Stuck` when either is unreadable, so dummy templates would make `shedengine.Done` unreachable and would also diverge from the marker set `internal/stencil`'s `Fill` requires in production.
  Add a `fakeLoomBurler` type to `fixture_test.go` implementing `shedadapters.BurlerRunner`, mirroring `internal/shedadapters/burler_test.go`'s own `fakeBurlerRunner` in shape.
  Its `Run` writes the `ReviewPath` and `FixerReportPath` the handed `burlerengine.Profile` names, each with short non-empty placeholder content, and returns a `burlerengine.Result` whose `Outcome` is `shuttleengine.OutcomeDone` — that pair-on-disk plus `OutcomeDone` is what makes `BurlerProducer.Call` return `Stuck` with a real report rather than erroring, which is what the Bouncer's next call then judges.
  Record the call count on the fake so a later card can assert the segment ran exactly one round.
  In both builders, set `RunRoot` to a fresh directory under the builder's own `t.TempDir()`, set `StencilsDir` to another and seed it via the new helper, set `Burler` to a `&fakeLoomBurler{}`, and set `Now` to a fixed-instant closure so archive-sibling filenames are deterministic.
  `buildSequenceFixture`'s return signature stays exactly `(anchorPath string, env shedrecipe.Env, paths ShedPaths)` — seven call sites destructure it with `:=` and Go requires an exact arity match, so a test needing the fake reaches it by type-asserting `env.Burler.(*fakeLoomBurler)`, exactly as the existing tests already do for `env.Shuttle`.
  The recipe's rows resolve `run_subdir: discussion` under `RunRoot` and `artifact_paths`/`profile.target.paths` under `WorktreeRoot`, so the fixture's `WorktreeRoot` must be the anchor the discussion files are already written beneath.
  Note the exception documented in `internal/shedrecipe`: `profile.target.paths` is passed through relative and unjoined, and `burlerengine.Profile.validate` is never invoked by `BurlerProducer`, so an on-disk mismatch there is inert for these fixtures — do not add fixture files to satisfy it.
- **Commit:** `test(loomrecipe): seed the Env fixtures for a live Discussion-Review segment`

### Card 16: extend fakeLoomShuttle for the bouncer seed and judge passes

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/bouncerfiles.go`
  - `internal/shedadapters/round.go`
- **Edits:**
  - `internal/loomrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `shedrecipe.Env` carries one `Shuttle` field, so the existing `fakeLoomShuttle` now serves the Bouncer's two spawn roles as well as `Discussion-Write` and `Plan-Write`.
  Add a branch on `spec.Role == "bouncer-judge"` that writes the three files named in `spec.OutputFiles` — in `shedadapters`' fixed order, the round's verdict path, its ledger path, and the next round's focus path — and returns `shuttleengine.OutcomeDone`.
  The verdict file must satisfy `parseVerdict`: closed YAML frontmatter carrying `verdict: APPROVED` and a non-empty `rationale`.
  The ledger file must satisfy `parseLedger`: closed YAML frontmatter carrying a positive integer `round` and a `ledger` list, which may legally be empty.
  Write the focus file with closed frontmatter carrying a positive `round` plus empty `exclude_lenses` and `focus` lists, or omit it — `Bouncer.settle` on an APPROVED verdict never reads it.
  Derive each file's round number from `spec.Round`, which the Bouncer fills with the round it is judging, rather than hardcoding `1`.
  Add a `bouncerVerdict` field on the fake, defaulting to `APPROVED`, so a later test can script a `BLOCKING` round without a second fake.
  Do not add a `bouncer-seed` branch: `seedCall` calls `ensureFocus(1)` regardless of what the spawn reported, synthesizing an empty-but-parsing focus file when none is on disk, so the default no-write branch is already correct for the seed pass.
  Record the seed and judge call counts on the fake, keyed by role, so a later card can assert the segment's exact spawn sequence.
  Update `fakeLoomShuttle`'s doc comment: it currently claims the fake serves rows 3 and 6 only, and that claim is now false.
- **Commit:** `test(loomrecipe): teach fakeLoomShuttle the bouncer judge pass`

### Card 17: move the coverage guard to the live rows

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomshed/loomshed.go`
  - `internal/shedrecipe/registry.go`
  - `manifest/roadmap.md`
- **Edits:**
  - `internal/loomrecipe/coverage_guard_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `loomRowEngines`, replace the `NameDiscussionReview: "Stub"` entry with `NameDiscussionBouncer: "Bouncer"` and `NameDiscussionBurler: "BurlerRound"`, keeping the map in recipe-row order, and update the map's doc comment from thirteen row names to fourteen.
  In `coverageGuardAllowedUnreachableEngines`, drop the `"Bouncer"` and `"BurlerRound"` entries, leaving only `"SingleLLM"`.
  Rewrite that variable's doc comment: it currently says the three remaining "loom: real LLM producers" roadmap items will each consume one of these three engines when it lands.
  The replacement states that this task landed `Bouncer` and `BurlerRound`, that `Stub` stays reachable via the still-stubbed `Plan-Review` and `Webster-Review` rows, and that `SingleLLM` is the sole remaining tolerated entry.
  The allowlist must shrink in the same change that lands the rows, or it silently tolerates a regression that unwires either one.
- **Commit:** `test(loomrecipe): shrink the coverage-guard allowlist to SingleLLM`

### Card 18: extend the producer table with segment and max-bounces columns

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomshed/loomshed.go`
  - `internal/shedengine/validate.go`
  - `internal/shedcheck/check.go`
  - `internal/shedcheck/finding.go`
- **Edits:**
  - `internal/loomrecipe/shape_test.go`
  - `internal/loomrecipe/recipe_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `segment string` and `maxBounces int` fields to the `wantProducerRow` struct in `shape_test.go`, and fill them for every row of `wantProducerTable` — empty and zero for the twelve unchanged rows, `"Discussion-Review"` and `5` for the two new ones.
  The existing eleven rows keep their current values;
  add the two new entries in recipe order between `NameDiscussionValidate` and `NamePlanWrite`, with `reflect.TypeOf(&shedadapters.Bouncer{})` and `reflect.TypeOf(&shedadapters.BurlerProducer{})` as their `producerType`, and update `NameDiscussionValidate`'s own `onDone` to `NameDiscussionBouncer`.
  Update the file's header comment from "thirteen-row producer table" to fourteen.
  In `recipe_test.go`, `TestNew_ShapeMatchesRecipe` currently asserts `len(shed.Producers) == 13`, `len(wantProducerTable) == 13`, and — for every row unconditionally — `Segment == ""` and `MaxBounces == 0`.
  Change both counts to 14 and replace the two unconditional assertions with per-row comparisons against the new `want.segment` and `want.maxBounces` fields.
  Update that test's doc comment, which names thirteen rows and describes the two assertions as unconditional.
  `TestNew_ProducerTable` in `shape_test.go` carries the same two unconditional assertions in its own body — `Segment == ""` and `MaxBounces == 0`, each with an error message claiming no row in the migration gains a non-empty or non-zero value — and both become false the moment card 14 lands.
  Convert them to per-row comparisons against `want.segment` and `want.maxBounces` exactly as in `recipe_test.go`, and rewrite the two error messages, which state the now-obsolete claim as their reason.
  That test's length check needs no edit: it compares against `len(wantProducerTable)` rather than a literal.
  Do not weaken `TestNew_RoutingGraphIsClean`: it is the whole-graph guard that fires when a perch is mis-wired, and its own doc comment already states exactly what it does and does not catch.
- **Commit:** `test(loomrecipe): extend the producer table to fourteen rows with segment and max-bounces`

### Card 19: extend the sequence assertions through the segment

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/burler.go`
  - `internal/shedengine/run.go`
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `internal/loomrecipe/sequence_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `wantSequenceOrder` is a flat list of row names whose outcomes `TestSequence_FullRunBlocksAtPublish` derives by rule (`Done` everywhere except `Publish`).
  That rule no longer holds: the segment produces three history entries where one used to sit, and two of them are `Stuck`.
  Change `wantSequenceOrder` from a `[]string` to a slice of a small struct pairing a row name with its expected `shedengine.Outcome`, and drop the derive-by-rule branch in the test body in favour of reading the expected outcome off each entry.
  The segment's expected sub-sequence, replacing the single `NameDiscussionReview` entry, is exactly three entries in order: `NameDiscussionBouncer` with `Stuck` (the seed call, which spawns and always reports `Stuck`), `NameDiscussionBurler` with `Stuck` (one completed round, which `BurlerProducer` reports as `Stuck` by contract, never `Done`), and `NameDiscussionBouncer` again with `Done` (the judge call, whose APPROVED verdict is what advances the run to `Plan-Write`).
  Every other entry keeps its current name and its `Done` outcome, except `Publish`, which keeps its `Stuck`.
  Update `wantSequenceOrder`'s doc comment to explain that three-entry shape, naming the seed call's unconditional `Stuck`, `BurlerProducer`'s never-`Done` contract, and the judged APPROVED verdict, so a reader is not left thinking two `Stuck` entries mid-run are a failure.
  Add assertions at the end of `TestSequence_FullRunBlocksAtPublish` that the fake burler ran exactly one round and that the fake shuttle recorded exactly one `bouncer-judge` spawn — reached by type-asserting `env.Burler` and `env.Shuttle` to the package's own fakes, the same way the existing `commitDiscussionCalls`/`commitPlanCalls` assertions already do.
  These are the scenario checks that the segment genuinely ran rather than being silently short-circuited.
  Verify `TestNew_PassesShedValidation` in `shape_test.go` still holds without edits: its run exhausts `Discussion-Validate`'s own bounce budget and never reaches the segment.
  If it does now reach the segment, adjust its doc comment to match what actually happens rather than changing the fixture to force the old path.
- **Commit:** `test(loomrecipe): extend the sequence assertions through the review segment`

### Card 20: confirm the resume and bounce-routing tests

- **Context:**
  - `internal/loomrecipe/resume_test.go`
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/sequence_test.go`
  - `internal/shedengine/run.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `resume_test.go` names no row this task renames, but all six of its tests build on `buildSequenceFixture` and now drive runs that pass through a live review segment.
  Run the package's tests and confirm each still holds for the reason its own doc comment gives, not by coincidence.
  Check specifically: `TestResume_DoesNotRestartAtRowOne`'s second run still blocks at `Publish` after passing through the segment;
  `TestResume_CrashRecoveryRecallsUnconditionally`'s counting producer still sees the call count its doc comment claims;
  `TestBounceRouting_BudgetExhaustionBlocks` still exhausts the budget it names rather than a different one now that two rows carry an explicit `max_bounces: 5` from the recipe while `ShedPaths.MaxBounces` in the fixture is 3.
  If a test's stated reason no longer matches what happens, fix its doc comment to state the real reason.
  Change no assertion to make a test pass — a changed assertion here means the segment mis-routes, which is a card-14 bug, not a test-maintenance one.
  This card is verification-only and produces no diff of its own;
  any fix it turns up belongs in the card that owns the file.
- **Commit:** none

### Card 21: confirm the whole-graph routing guard exercises both rows

- **Context:**
  - `internal/loomrecipe/shape_test.go`
  - `internal/shedcheck/check.go`
  - `internal/shedcheck/finding.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `TestNew_RoutingGraphIsClean` and `TestRecipe_StructuralCheckHasNoFindings` are the guards that prove the new edges form a valid graph from `entry: Preflight` to `terminals: [Finalize]` with no dangling target, no unreachable row, no unexpected terminal, and no done-cycle.
  Confirm they exercise the two new rows rather than passing vacuously.
  Concretely, verify by temporary local mutation — reverted before the card commits — that each of these three mis-wirings produces at least one finding: `Discussion-Burler` with an empty `on_done`, `Discussion-Bouncer` with an `on_done` that never leaves the segment, and `Discussion-Burler` with an `on_stuck` that does not name the Bouncer.
  Note what `internal/shedcheck`'s own done-cycle walk does and does not see: it follows done edges only, and `Discussion-Bouncer`'s done edge leaves the segment for `Plan-Write`, so the mutual Bouncer/Burler pair is correctly not reported as a cycle.
  This card is verification-only and produces no diff of its own;
  if a mis-wiring turns out to pass unreported, that is a finding to raise, not a test to add here — `internal/shedcheck` is not in this task's scope.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/loomrecipe/... ./internal/loomshed/...` covers both packages this batch edits Go code in.
`internal/loomrecipe` is the batch's centre of gravity — every guard that pins the recipe's shape, coverage, sequence, and routing lives there, and cards 15 through 21 all land in it.
`internal/loomshed` is included because card 14 edits its constants and two of its doc comments, and its own tests are what prove no other row constructor lost a name it reads (`seed.go` and `loompreflight.go` each read one).
`contracts/recipes/loom-recipe.yaml` has no test package of its own: it is embedded into the binary by `contracts/recipes/recipes.go` and is exercised entirely through `internal/loomrecipe`'s own `New`-driven tests, which is why parsing and building it is in scope here rather than in a third package.
The overview's module-wide `verify: go build ./...` runs at the batch boundary and catches any production caller of the deleted `loomshed.NameDiscussionReview` constant outside these two packages.
