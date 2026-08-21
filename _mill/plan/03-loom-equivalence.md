# Batch: loom-equivalence

```yaml
task: 'Shed recipe: loader/builder'
batch: loom-equivalence
number: 3
cards: 2
verify: go test ./internal/shedbuild/...
depends-on: [2]
```

## Batch Scope

This batch delivers the key test of the whole task: a `testdata/` recipe fixture hand-authoring loom's thirteen rows, and the test that builds it and compares the result against `loomshed.New`'s live output row by row.
It is one batch because the fixture and the test that reads it are meaningless apart — the fixture's only consumer is that test, and the test's only input is that fixture.
It is deliberately last among the code batches because it is the only one that needs `Parse`, `Build`, and `Check` all working at once.
Nothing downstream consumes this batch's output;
batch 4 is documentation only.
One batch-local decision beyond the overview's `## Shared Decisions`: this test builds its own paired fixture rather than calling `newTestEnv`, because both sides must be told the same values from one temp directory — see card 13.

## Cards

### Card 12: the loom recipe fixture

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/shedrecipe/coverage_guard_test.go`
  - `internal/shedbuild/recipe.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/testdata/loom-recipe.yaml`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/testdata/loom-recipe.yaml`, a recipe document expressing loom's current thirteen-row list exactly.
  Set `version: 1`, `entry: Preflight`, and `terminals: [Finalize]`.
  Write the thirteen `producers` rows in loom's own list order, each with its `name`, its `engine`, and its routing.
  The name-to-engine correspondence is the one `loomRowEngines` in `internal/shedrecipe/coverage_guard_test.go` already records, and the routing is the one `New` in `internal/loomshed/loomshed.go` already assembles:
  `Preflight` on engine `Preflight`, done to `Loom-Preflight`;
  `Loom-Preflight` on engine `LoomPreflight`, done to `Discussion-Write`;
  `Discussion-Write` on engine `Stub`, done to `Discussion-Validate`;
  `Discussion-Validate` on engine `DiscussionValidate`, stuck to `Discussion-Write`, done to `Discussion-Review`;
  `Discussion-Review` on engine `Stub`, stuck to `Discussion-Write`, done to `Plan-Write`;
  `Plan-Write` on engine `Stub`, done to `Plan-Validate`;
  `Plan-Validate` on engine `PlanValidate`, stuck to `Plan-Write`, done to `Plan-Review`;
  `Plan-Review` on engine `Stub`, stuck to `Plan-Write`, done to `Batchifier`;
  `Batchifier` on engine `Batchifier`, done to `Webster`;
  `Webster` on engine `Webster`, done to `Webster-Review`;
  `Webster-Review` on engine `Stub`, stuck to `Webster`, done to `Publish`;
  `Publish` on engine `Publish`, done to `Finalize`;
  and `Finalize` on engine `Finalize`, with an explicit `on_done: ""` carrying a comment that the empty value is load-bearing and is what ends the whole run quietly.
  Omit `on_stuck` entirely on the eight rows that escalate rather than bounce, since an absent key and an empty value are the same thing and the absent form reads better.
  Omit `config`, `segment`, and `max_bounces` on every row: loom's current thirteen rows carry no segment and no per-row bounce budget, none of these thirteen engines takes a config key, and a non-empty config block on any of them would be an error from its constructor.
  This file is a test fixture only — the task ships no production recipe file, and converting loom's own list is the separate roadmap item sequenced immediately after this one.
- **Commit:** `test(shedbuild): add the loom recipe testdata fixture`

### Card 13: the loom-equivalence test

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/shedbuild/testdata/loom-recipe.yaml`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedbuild/parse.go`
  - `internal/shedbuild/build.go`
  - `internal/shedbuild/check.go`
  - `internal/shedbuild/recipe.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/preflightshed/preflight.go`
- **Edits:** none
- **Creates:**
  - `internal/shedbuild/equivalence_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedbuild/equivalence_test.go` in `package shedbuild`, holding one paired-fixture builder and one test.
  The builder takes `t` and derives a single `t.TempDir()`, then fills both a `shedrecipe.Env` and a `loomshed.Deps` from that one directory so every told value either side reads is literally the same string.
  Both sides share one `landingshed.Deps` value, built by calling `testLandingDeps` from `internal/shedbuild/fixture_test.go` once and assigning the result to both `Env.Landing` and the loom deps' own landing field.
  Both sides share the same webster runner and the same four required webster seams, reusing the fakes `internal/shedbuild/fixture_test.go` already declares.
  Do not call `newTestEnv` here: it derives its own temp directory with no loom-side counterpart, which is exactly what this test must avoid.
  Fill the loom side's preflight row with a real producer from `preflightshed.NewPreflight`, given loom's own preflight name constant and the same working directory the environment carries — not a fake.
  That constructor only stores its two arguments and spawns nothing, since git is spawned inside its call step and this test never invokes a producer, so it is safe to construct;
  a fake would make row 1's concrete types differ by construction and fail the type assertion on the very first row for a purely fixture-shaped reason.
  Give the loom side's own lock path a distinct value from its status lock path, since `shedengine` rejects the two naming one file — even though this test never runs the shed.
  The test itself reads the fixture bytes with `os.ReadFile` from the `testdata` directory, feeds them to `Parse`, feeds the resulting recipe and the environment to `Build`, and calls `loomshed.New` with the paired deps.
  Assert first that both row counts are thirteen and equal, failing fast if not, since every later assertion indexes both slices.
  Then, per index, assert `Name`, `OnDone`, `OnStuck`, `Segment`, and `MaxBounces` are equal, and assert the two producers' concrete types are equal via `reflect.TypeOf`, which is what proves the right engine was selected.
  Do not compare the producer values themselves — producer identity is not comparable, and only the five data fields and the concrete type are.
  Finally, feed the parsed recipe and the built list to `Check` and assert it returns no findings, which is what pins the fixture's own told entry and terminals against the graph it describes.
  The file's header comment states what this test is for: it is the checkable form of the claim that the format can express loom's real list, and it fails loudly if a future change to loom's list outgrows the format — the regression worth catching, because the conversion item sequenced after this task depends on it.
- **Commit:** `test(shedbuild): assert the loom recipe fixture matches loomshed.New`

## Batch Tests

`verify: go test ./internal/shedbuild/...` runs the whole package, which after this batch adds `equivalence_test.go` to the six test files batches 1 and 2 delivered.
The scope is exactly the batch's own `Creates:` set — no file outside `internal/shedbuild/` is touched, and in particular `internal/loomshed` is read from a test binary only, never edited.
This is a Go project, so the command uses the native runner directly with no `PYTHONPATH=` prefix.
The batch is complete when the thirteen-row comparison passes on all five data fields plus concrete type, and the `Check` call over the built list reports nothing.
