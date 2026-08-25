# Plan: loom: Discussion-Review producer

```yaml
task: 'loom: Discussion-Review producer'
slug: 'loom-discussion-review-producer'
approved: true
started: '20260825-063828'
parent: 'main'
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: rubric-stencil
    file: 01-rubric-stencil.md
    depends-on: []
    verify: go test ./contracts/stencils/...
  - number: 2
    name: shedrecipe-entries
    file: 02-shedrecipe-entries.md
    depends-on: []
    verify: go test ./internal/shedrecipe/...
  - number: 3
    name: loomengine-config-and-paths
    file: 03-loomengine-config-and-paths.md
    depends-on: []
    verify: go test ./internal/loomengine/...
  - number: 4
    name: loomcli-wiring
    file: 04-loomcli-wiring.md
    depends-on: [2, 3]
    verify: go test ./internal/loomcli/...
  - number: 5
    name: recipe-rows
    file: 05-recipe-rows.md
    depends-on: [1, 2, 4]
    verify: go test ./internal/loomrecipe/... ./internal/loomshed/...
  - number: 6
    name: docs
    file: 06-docs.md
    depends-on: [5]
    verify: go vet -tags smoke ./internal/loomcli/...
```

## Shared Decisions

### Decision: two-rows-not-one

- **Decision:** the stubbed `Discussion-Review` recipe row is replaced by two rows, `Discussion-Bouncer` (engine `Bouncer`) and `Discussion-Burler` (engine `BurlerRound`).
  `loomshed.NameDiscussionReview` is deleted; `loomshed.NameDiscussionBouncer` and `loomshed.NameDiscussionBurler` replace it.
  loom's recipe goes from thirteen rows to fourteen.
- **Rationale:** `manifest/roadmap.md`'s item names both rows explicitly, and CLAUDE.md's perch terminology note describes exactly this two-row hand-wiring.
  A row name is a durable on-disk identity in `current_producer`, but the row being renamed is a stub that has never produced anything, so there is no in-flight state to break.
- **Applies to:** batch 5, batch 6

### Decision: routing-and-segment

- **Decision:** the four routing edges are exactly `Discussion-Validate.on_stuck: Discussion-Write` (unchanged) and `on_done: Discussion-Bouncer`; `Discussion-Bouncer.on_stuck: Discussion-Burler`, `on_done: Plan-Write`; `Discussion-Burler.on_stuck: Discussion-Bouncer`, `on_done: Discussion-Bouncer`.
  Both new rows carry `segment: Discussion-Review` and `max_bounces: 5`.
- **Rationale:** `internal/shedengine`'s validator rejects an `OnStuck` naming a producer in a different `Segment`, so both rows need the same non-empty label for the two mutual edges to build at all.
  `Discussion-Burler.on_done` is set rather than left empty because an empty `OnDone` is load-bearing and ends the whole run silently;
  `BurlerProducer` never returns `Done`, so the edge is unreachable but harmless.
  `internal/shedcheck`'s done-cycle walk follows done edges only, and `Discussion-Bouncer`'s done edge leaves the segment for `Plan-Write`, so the mutual pair forms no done cycle.
- **Applies to:** batch 5

### Decision: one-rubric-stencil-read-by-both-rows

- **Decision:** the rubric lives in exactly one place, the new stencil `loom-rubric-discussion-review`.
  `Discussion-Bouncer` names it through `bouncerEntry`'s existing `rubric_stencil` key.
  `Discussion-Burler` names it through a new `rubric_stencil` key inside `burlerRoundEntry`'s `profile:` map, mutually exclusive with that map's existing literal `rubric` key — exactly one of the two must be present, and both or neither is a construction error.
- **Rationale:** `BouncerConfig.RubricStencil` takes a stencilstore name while `burlerengine.Profile.Rubric` takes literal prose, so without this key the same rubric would have to be written twice, which the Producer Pointer-Rule Invariant forbids.
  The Burler cannot reach the stencils directory any other way: `stencilstore` resolves against the hub's `_board`, and the Hub Containment Invariant keeps `_board` out of every worktree.
- **Applies to:** batch 2, batch 5

### Decision: review-model-comes-from-loom-yaml-not-the-recipe

- **Decision:** loom's config template gains `review:` and `review_timeout_min:`, validated at load time via `modelspec.Parse` exactly as `discussion:` and `plan:` already are.
  `shedrecipe.Env` gains four run-wide fields (`ReviewModel`, `ReviewEffort`, `ReviewVersion`, `ReviewTimeout`) that `bouncerEntry` and `burlerRoundEntry` fall back to when the corresponding per-row Config key is absent.
  The recipe's two new rows omit all four keys and therefore take the `Env` values.
- **Rationale:** the recipe is embedded in the binary, so a recipe-literal model is untunable without a rebuild.
  A single review model shared by all three review segments is a genuinely run-wide value, which is what `Env` is contractually allowed to carry.
- **Applies to:** batch 2, batch 3, batch 4, batch 5

### Decision: bouncer-has-no-timeout-key

- **Decision:** `Env.ReviewTimeout` is read by `burlerRoundEntry` only.
  `bouncerEntry` gains no timeout fallback, because `shedadapters.BouncerConfig` carries no timeout field and `bouncerEntry` declares no `timeout_s` key today.
- **Rationale:** this task consumes `internal/shedadapters` unmodified (an explicit Out item in `_mill/discussion.md`), so adding a timeout to the Bouncer's shuttle spawn is out of scope.
  The Bouncer's seed and judge spawns therefore run at the provider default deadline;
  that is an accepted consequence of the unmodified-adapters boundary, not an oversight, and is the thing to revisit if a judge call ever hangs.
- **Applies to:** batch 2

### Decision: review-run-artifacts-are-ephemeral

- **Decision:** `Env.RunRoot` resolves to `<AnchorPath>/.lyx/loom/reviews/` via a new `loomengine.LoomReviewsDir` accessor, and both rows carry `run_subdir: discussion`, so every round report, verdict, ledger, focus file, and archive sibling lands in `<AnchorPath>/.lyx/loom/reviews/discussion/`.
- **Rationale:** the Durable-vs-Ephemeral State Invariant requires every never-tracked file to live under `.lyx` at the mirrored subpath of its `_lyx` content, and requires each module to expose its own scratch accessor rather than deriving the path inline.
  There is no commit seam for a `Bouncer` row, so round artifacts under `_lyx/` would be untracked dirt in a tree the following rows and `internal/fabricengine`'s sync both care about.
- **Applies to:** batch 3, batch 5

### Decision: docs-land-with-the-behaviour-change

- **Decision:** `manifest/designs/loom.md`, `manifest/roadmap.md`, and the stale row-count prose in `internal/loomcli/smoke_test.go` are updated in batch 6, which depends on batch 5 and lands in the same task.
- **Rationale:** CLAUDE.md's task-completion rule requires the module doc and roadmap to move with the change.
  They sit in their own batch rather than inside batch 5's cards because their verify command differs (`go vet -tags smoke` compiles the tag-gated smoke suite without running it, which no other batch needs), and because a doc edit shares no `Context:` with the Go wiring cards.
- **Applies to:** batch 6

### Decision: loomrecipe-fixtures-seed-a-real-stencils-dir

- **Decision:** `internal/loomrecipe`'s two Env builders (`testEnv` in `shape_test.go`, `buildSequenceFixture` in `fixture_test.go`) gain a seeded stencils directory carrying `bouncer-template-seed`, `bouncer-template-judge`, and `loom-rubric-discussion-review`, seeded from `contracts/stencils`' embedded bytes, plus a `RunRoot` under `t.TempDir()` and a fake `shedadapters.BurlerRunner`.
- **Rationale:** `shedadapters.NewBouncer` probes its rubric stencil eagerly at construction, so `New` fails for every test in the package without a seeded rubric.
  The two bouncer templates are read at call time by `seedCall` and `judgeCall`, and an unreadable template degrades to `Stuck`, which would make `shedengine.Done` unreachable for the sequence test.
  Seeding the real embedded bytes rather than dummy files keeps `internal/stencil`'s `Fill` marker set identical to production.
- **Applies to:** batch 5

## All Files Touched

- `contracts/recipes/loom-recipe.yaml`
- `contracts/stencils/loom/loom-rubric-discussion-review.md`
- `contracts/stencils/rubric_test.go`
- `contracts/stencils/stencils.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/review.go`
- `internal/loomengine/review_test.go`
- `internal/loomengine/template.yaml`
- `internal/loomrecipe/coverage_guard_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomrecipe/recipe_test.go`
- `internal/loomrecipe/sequence_test.go`
- `internal/loomrecipe/shape_test.go`
- `internal/loomshed/doc.go`
- `internal/loomshed/loomshed.go`
- `internal/loomshed/stub.go`
- `internal/shedrecipe/entries_bouncer.go`
- `internal/shedrecipe/entries_bouncer_test.go`
- `internal/shedrecipe/entries_burler.go`
- `internal/shedrecipe/entries_burler_test.go`
- `internal/shedrecipe/recipe.go`
- `manifest/designs/loom.md`
- `manifest/roadmap.md`
