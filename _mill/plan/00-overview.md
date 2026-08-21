# Plan: loom: convert to a Shed recipe

```yaml
task: 'loom: convert to a Shed recipe'
slug: 'loom-convert-to-shed-recipe'
approved: false
started: '20260821-143326'
parent: 'main'
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: recipe-file-and-loomrecipe-package
    file: 01-recipe-file-and-loomrecipe-package.md
    depends-on: []
    verify: go test ./contracts/... ./internal/loomrecipe/...
  - number: 2
    name: move-the-graph-tests-into-loomrecipe
    file: 02-move-the-graph-tests-into-loomrecipe.md
    depends-on: [1]
    verify: go test ./internal/loomrecipe/... ./internal/loomshed/...
  - number: 3
    name: coverage-guard-move-and-fixture-retirement
    file: 03-coverage-guard-move-and-fixture-retirement.md
    depends-on: [2]
    verify: go test ./internal/loomrecipe/... ./internal/shedrecipe/... ./internal/shedbuild/...
  - number: 4
    name: loomcli-rewiring
    file: 04-loomcli-rewiring.md
    depends-on: [1]
    verify: go test ./internal/loomcli/...
  - number: 5
    name: delete-loomshed-new-and-deps
    file: 05-delete-loomshed-new-and-deps.md
    depends-on: [2, 3, 4]
    verify: go test ./internal/loomshed/... ./internal/loomrecipe/... ./internal/loomcli/... ./internal/shedrecipe/... ./internal/shedbuild/...
  - number: 6
    name: docs-and-comment-sweep
    file: 06-docs-and-comment-sweep.md
    depends-on: [5]
    verify: go test ./internal/lyxcwd/ ./internal/preflightshed/ ./internal/shedrecipe/ ./internal/shedcheck/ ./internal/shedbuild/ ./internal/loomcli/ && go vet -tags smoke ./internal/loomcli/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: run-the-equivalence-test-green-first

- **Decision:** Before card 1 copies `internal/shedbuild/testdata/loom-recipe.yaml` into `contracts/recipes/loom-recipe.yaml`, the implementer runs `go test ./internal/shedbuild/ -run TestLoomEquivalence -count=1` against the current tree and confirms it passes.
  That green run is the proof the fixture still matches `internal/loomshed/loomshed.go`'s live thirteen-row literal, and this task deletes both the test and the literal.
  As of plan authoring (2026-08-21) that command was run and reported `ok github.com/Knatte18/loomyard/internal/shedbuild`, so the fixture and the literal are in sync at plan time;
  the implementer re-runs it anyway, because the tree may have moved between planning and implementation.
- **Rationale:** copying the fixture first and deleting its prover first destroys the only remaining verification that the recipe expresses loom's real list.
- **Applies to:** batch 1 (the run), batch 3 (the deletion).

### Decision: no-production-change-to-the-consumed-packages

- **Decision:** `internal/shedengine`, `internal/shedcheck`, `internal/shedbuild`, and `internal/shedrecipe` receive no behavioural production change in this task.
  The only edits any of them take are doc-comment repairs (batch 6) naming symbols this task deletes or moves, and test-file changes in `internal/shedrecipe` and `internal/shedbuild` (batch 3).
- **Rationale:** this task is those packages' first real consumer, not their reviser.
  A genuine defect found in one of them is a finding to report in the batch's own notes, never a licence to widen scope.
- **Applies to:** all batches.

### Decision: env-webster-run-is-filled-explicitly

- **Decision:** `internal/loomcli`'s `wire` sets `Env.WebsterRun = websterengine.Run` explicitly.
- **Rationale:** `internal/shedrecipe/entries_simple.go`'s `websterEntry` calls `requireSeam("Webster", "WebsterRun", env.WebsterRun)` and errors on a nil value, whereas `internal/loomcli/wiring.go` deliberately leaves `Deps.WebsterRun` nil today and relies on `shedadapters.NewWebsterProducer`'s own nil-defaulting.
  A straight port leaving it nil fails at build time with `shedrecipe: Webster: Env.WebsterRun must not be nil`.
  Relaxing `websterEntry` instead is out of scope per `no-production-change-to-the-consumed-packages`.
- **Applies to:** batch 4.

### Decision: landing-parity

- **Decision:** `Env.Landing` is left unfilled by `internal/loomcli`, exactly as `Deps.Landing` is unfilled today.
  Every test that builds the real thirteen-row list must still fill `Env.Landing` with a synthetic-but-valid `landingshed.Deps`, because `landingshed.NewPublish`/`NewFinalize` reject nil closures at construction.
- **Rationale:** `Publish`/`Finalize` construction already fails in production today for want of `Landing`;
  the parent-fabric resolution chain belongs to a later item, which card 24 adds to `manifest/roadmap.md` because `internal/landingshed/deps.go` already asserts such an item exists and none does.
  Preserving the existing failure keeps this task a conversion, and must not read as a regression introduced here.
- **Applies to:** batches 2, 3, 4.

### Decision: row1-substitution-is-a-seam-not-a-fixed-fake

- **Decision:** every moved test that calls `Run` substitutes the built row 1's producer in place — `shed.Producers[0].Producer = <fake>` — after **each** `loomrecipe.New` call and before `Run`.
  The default fake is `fakeAlwaysDoneProducer{}`;
  a test whose subject is row 1's call count substitutes its own instance instead, and substitutes the **same** instance at every `New` call in that test.
  Tests that only build and inspect the list add no substitution at all, so the real row 1's construction stays covered.
- **Rationale:** `preflightEntry` builds row 1 from `Env.Cwd`, removing the `Deps.Preflight` injection point the moved tests relied on.
  Construction is safe (nothing in `preflightEntry` touches disk), but the real producer's `Call` spawns `git`, which breaks the Test Tier Purity Invariant.
  Both `shedengine.Shed.Producers` and `shedengine.ProducerDef.Producer` are exported, so this needs no test-only API on `internal/loomrecipe`.
- **Applies to:** batch 2.

### Decision: row-name-authority-stays-with-the-go-constants

- **Decision:** `internal/loomshed`'s thirteen `Name*` constants stay, all thirteen, and remain the authority for loom's row names.
  The recipe spells the same thirteen names as yaml strings, and the moved coverage guard's row table keys off `loomshed.NamePreflight`, `loomshed.NameLoomPreflight`, … rather than off string literals.
- **Rationale:** `internal/loomshed/seed.go` writes `CurrentProducer: NamePreflight` into a fresh status file and `internal/loomshed/loompreflight.go` passes `NameLoomPreflight` plus a tolerated history set to `loomengine.CheckSeed`;
  neither path goes through the recipe.
  Once the recipe is the row-name source nothing connects the two, and a rename would silently break resume for an in-flight task.
  The constants cannot move into `internal/loomrecipe` — `loomshed` reads two of them, and `loomrecipe` imports `shedbuild` → `shedrecipe` → `loomshed`.
- **Applies to:** batches 2, 3, 5.

### Decision: duplicate-test-helpers-rather-than-share-them

- **Decision:** helpers the moved fixture needs that live in files staying in `internal/loomshed` — `writeDiscussionFixture` and `validDecisionRecord` (`discussionvalidate_test.go`), `seedPlanValidateFixture` (`planvalidate_test.go`), `fakeWebsterRun` (`webster_test.go`), `writeBatcherConfig` (`batchifier_test.go`) — are **duplicated** into `internal/loomrecipe/fixture_test.go`, never moved.
  No shared exported test-support package is created.
- **Rationale:** moving them breaks the per-producer tests that stay;
  leaving them means `internal/loomrecipe` does not compile.
  Duplication is established repo practice here: `testLandingDeps` already exists in two independent copies (`internal/loomshed/fixture_test.go` and `internal/shedbuild/fixture_test.go`).
  A shared package would be production-visible API whose only consumers are tests.
- **Applies to:** batch 2.

### Decision: renames-go-through-git-mv

- **Decision:** every relocated file in this plan is expressed as a `Moves:` pair and performed with `git mv` first, followed by surgical edits only.
  No relocated file is rewritten from scratch and its original deleted.
- **Rationale:** a `Creates:` + `Deletes:` pair destroys git rename history and inflates the review diff.
  `rename_detect_pct` is 30 in this hub's config, so a moved-then-heavily-edited file still renders as a rename.
- **Applies to:** batches 2, 3.

### Decision: go-verify-commands-carry-no-pythonpath-prefix

- **Decision:** every `verify:` command in this plan is a native `go test` invocation with no `PYTHONPATH= ` prefix.
- **Rationale:** the `verify-not-isolated` validator check applies the `PYTHONPATH= ` rule to Python projects only;
  this is a Go module (`go.mod` declares `github.com/Knatte18/loomyard`, go 1.26).
- **Applies to:** all batches.

## All Files Touched

- `CONSTRAINTS.md`
- `contracts/recipes/loom-recipe.yaml`
- `contracts/recipes/recipes.go`
- `docs/overview.md`
- `internal/loomcli/cli.go`
- `internal/loomcli/cli_test.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomcli/drive.go`
- `internal/loomcli/pause.go`
- `internal/loomcli/run.go`
- `internal/loomcli/status.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomrecipe/coverage_guard_test.go`
- `internal/loomrecipe/doc.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomrecipe/loomrecipe.go`
- `internal/loomrecipe/recipe_test.go`
- `internal/loomrecipe/resume_test.go`
- `internal/loomrecipe/seam_enforcement_test.go`
- `internal/loomrecipe/sequence_test.go`
- `internal/loomrecipe/shape_test.go`
- `internal/loomshed/cancellation_test.go`
- `internal/loomshed/doc.go`
- `internal/loomshed/loompreflight.go`
- `internal/loomshed/loomshed.go`
- `internal/loomshed/seed.go`
- `internal/loomshed/seam_enforcement_test.go`
- `internal/preflightshed/preflight_test.go`
- `internal/shedbuild/fixture_test.go`
- `internal/shedcheck/doc.go`
- `internal/shedrecipe/entries_simple.go`
- `internal/shedrecipe/recipe.go`
- `internal/shedrecipe/registry_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/shed-recipe.md`
- `manifest/parallel-work.md`
- `manifest/roadmap.md`
