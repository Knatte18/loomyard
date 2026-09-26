# Plan: Loom persists done only after post-run friction reflection

```yaml
task: Loom persists done only after post-run friction reflection
slug: loom-done-after-friction
approved: true
started: 20260926-111835
parent_branch: main
root: ""
verify: null
discussion_sha: ee813d88d6b4bc548fb433f6bc2c87179e02ffe7
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: friction-reflect-row
    file: 01-friction-reflect-row.md
    depends-on: []
    verify: go test ./internal/loomshed/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/loomrecipe/... ./internal/loomcli/... ./internal/landingshed/... && go test -tags integration ./internal/landingshed/...
  - number: 2
    name: loomcli-row-wiring
    file: 02-loomcli-row-wiring.md
    depends-on: [1]
    verify: go test ./internal/loomcli/... ./internal/loomengine/... ./internal/loomrecipe/... ./internal/shedcli/... && go test -tags integration -skip TestIntegrationDriverBootstrap_ReturnsWithoutWaitingOnTheDriver ./internal/loomcli/... && go test -tags smoke ./internal/loomcli/...
```

## Shared Decisions

### Decision: row-shaped-fix-no-engine-change

- **Decision:** Reflection on the done path moves into a new terminal recipe row, `Friction-Reflect` (engine `FrictionReflect`), after `Finalize`.
  `internal/shedengine`, `internal/shedverbs`, `internal/battenshed` and `internal/battencli` are not edited by any card.
- **Rationale:** the discussion's "Mechanism: a terminal recipe row, not an engine hook" Decision; the Shed Producer-Seam, Shed Verb-Set and Batten Bookend Invariants.
- **Applies to:** all batches

### Decision: reflection-closure-on-env

- **Decision:** `shedrecipe.Env` gains `ReflectFriction func() string`.
  `loomshed.NewFrictionReflect` refuses a nil closure; `shedrecipe`'s `frictionReflectEntry` validates no Env field itself and wraps the constructor's error, following `publishEntry`.
  loomcli's `wire` fills the field with the method value `c.reflectFrictionRow`, which gates on Tier 2 (`c.frictionDir != ""`) and on the arming verb (`c.armedVerb == "run"`), and records the returned status on the receiver.
- **Rationale:** keeps `frictionengine`, `lock` and `loomengine` imports out of the Told-Geometry-bound packages; gating on the arming verb, never the recorded seed driver, keeps the Driver Choice Single-Site Invariant.
- **Applies to:** all batches

### Decision: row-waits-postrun-skips

- **Decision:** `reflectFriction` takes a `wait bool` parameter.
  The row path (`reflectFrictionRow`) passes `true` and takes `loomengine.LoomFrictionLock` with the blocking `lock.AcquireWriteLock`; `loomPostRun`'s blocked path passes `false` and keeps today's `lock.TryAcquireWriteLock` skip.
- **Rationale:** the discussion's "The row waits for a concurrent reflection instead of skipping it" Decision; a parameter is the smaller diff than two callers over a shared body.
- **Applies to:** loomcli-row-wiring

### Decision: intermediate-state-between-batches

- **Decision:** after batch 1 lands, the shipped recipe requires `Env.ReflectFriction`, which loomcli's `wire` fills only in batch 2, so a real `lyx loom run` would fail at `loomrecipe.New` between the two batches.
  Inside batch 1, `internal/shedrecipe`'s cross-consumer coverage guard fails between card 2 (registry entry) and card 3 (recipe row that reaches it).
  Both are accepted intermediate states; each batch's own `verify:` passes at its end, and nothing ships between batches.
- **Rationale:** splitting on the package boundary (shed-side rows vs. loomcli wiring) keeps each batch's context small; no untagged loomcli test builds the real recipe through `wire` (the one that would, `TestBuildLoomShed_OutputShape`, skips without a fabric).
- **Applies to:** all batches

### Decision: no-perishable-row-counts

- **Decision:** text that pins loom's row count or calls `Finalize` the last row is reworded to name the source instead of a number, except ly-drive's step-cap arithmetic (`plugins/ly/skills/ly-drive/SKILL.md` § The loop), where the count is load-bearing and is re-derived: fifteen rows, thirty-six steps, cap 40 unchanged.
  The sweep found, beyond the discussion's floor list: `internal/loomcli/sharedbootstrap_test.go` (comment "all fourteen" plus a stale `17` assertion in an always-skipping test), `internal/shedrecipe/registry.go`/`registry_test.go` ("sixteen keys"), `internal/shedbuild/build_engines_test.go` ("fourteen engines", "fifteenth"), `internal/shedrecipe/entries_simple.go` ("seven registry entries"), `internal/landingshed/deps.go` (`Deps.CommitStatus`'s doc calls Finalize "the last row of a loom run"), `internal/shedrecipe/entries_simple_test.go` ("seven value-only entries"), `internal/shedbuild/fixture_test.go` ("two of the sixteen engines"), `docs/overview.md` ("registers sixteen engine names") and `contracts/specs/shed-recipe-spec.md` ("imports `loomshed` for eight of its constructors").
  The sweep method: grep `internal`, `docs`, `contracts`, `plugins` and `manifest/designs` for spelled-out and digit counts (fourteen through seventeen, seven, eight) next to engine/entry/row/producer/key/constructor, plus `last row`/`final row`/`terminal` phrasing next to `Finalize`.
  `plugins/scribe/skills/prose/SKILL.md`'s "fourteen rows" is an illustrative example of the rule itself, not a description of loom's recipe, and stays.
  `internal/frictionengine/doc.go` was checked against the discussion's reword list: it never says reflection runs after the run lock is released, so it is not edited.
- **Rationale:** the discussion's "Stale row-count and last-row text" Decision and the prose rule against perishable tallies.
- **Applies to:** all batches

### Decision: no-lint-in-done-gate

- **Decision:** the effective `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) already covers the whole module tree, so no change to it is recommended.
  `golangci-lint run` was run against the current worktree tip and exits 1 (pre-existing repo-wide lint findings unrelated to this task), so no lint command is recommended either.
- **Rationale:** mill-plan's done-gate guidance: recommend lint only when it passes on the unmodified tree.
- **Applies to:** all batches

### Decision: pre-existing-integration-failure-skipped

- **Decision:** `TestIntegrationDriverBootstrap_ReturnsWithoutWaitingOnTheDriver` (integration-tagged, in `internal/loomcli`) fails on the unmodified tree at planning time, three runs out of three (`startLLMDriverArm took 5.07s; want well under the stub's own 2s settle delay`).
  It tests the llm-driver arm's startup latency, which no card touches.
  Batch 2's `verify:` runs loomcli's integration suite with `-skip` on exactly that test, plus the smoke suite unmodified.
  The effective `pipeline.done_gate` (`go test -tags integration ./...`) will still hit this failure at mill-go's pre-done gate; that is a pre-existing defect outside this task's scope, not a regression of it, and the operator should expect it there.
- **Rationale:** batch 2 changes loomcli behaviour, so its tagged suites belong in its own `verify:`, but a known-red test unrelated to the change would make the batch unverifiable.
- **Applies to:** loomcli-row-wiring

## All Files Touched

- `contracts/recipes/loom-recipe.yaml`
- `contracts/specs/shed-recipe-spec.md`
- `docs/overview.md`
- `internal/landingshed/deps.go`
- `internal/loomcli/arm.go`
- `internal/loomcli/bootstrap.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/friction_test.go`
- `internal/loomcli/run.go`
- `internal/loomcli/sharedbootstrap_test.go`
- `internal/loomcli/start.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/config.go`
- `internal/loomrecipe/coverage_guard_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomrecipe/frictionreflect_test.go`
- `internal/loomrecipe/shape_test.go`
- `internal/loomshed/doc.go`
- `internal/loomshed/frictionreflect.go`
- `internal/loomshed/frictionreflect_test.go`
- `internal/loomshed/interruptpolicy.go`
- `internal/loomshed/loomshed.go`
- `internal/shedbuild/build_engines_test.go`
- `internal/shedbuild/fixture_test.go`
- `internal/shedrecipe/entries_simple.go`
- `internal/shedrecipe/entries_simple_test.go`
- `internal/shedrecipe/fixture_test.go`
- `internal/shedrecipe/recipe.go`
- `internal/shedrecipe/registry.go`
- `internal/shedrecipe/registry_test.go`
- `plugins/ly/skills/ly-drive/SKILL.md`
