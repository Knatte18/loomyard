# Plan: fabric: clone doesn't commit written module configs

```yaml
task: 'fabric: clone doesn''t commit written module configs'
slug: 'fabric-clone-commit-module-configs'
approved: true
started: '20260826-113442'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: configfilerel-accessor
    file: 01-configfilerel-accessor.md
    depends-on: []
    verify: go test -count=1 ./internal/configengine/...
  - number: 2
    name: fixture-empty-stage-tolerance
    file: 02-fixture-empty-stage-tolerance.md
    depends-on: []
    verify: go test -count=1 -tags integration ./internal/hubforge/... ./internal/preflight/... ./internal/preflightshed/...
  - number: 3
    name: clone-commits-module-configs
    file: 03-clone-commits-module-configs.md
    depends-on: [1, 2]
    verify: go test -count=1 ./internal/configengine/... ./internal/fabriccli/... && go test -count=1 -tags integration ./internal/fabriccli/... ./internal/fabricengine/... ./internal/hubforge/... ./internal/preflight/... ./internal/preflightshed/...
```

## Shared Decisions

### Decision: spike-verified-no-regression-sweep

- **Decision:** the discussion's Testing §8 "regression sweep" produces **zero** fixture edits beyond the two `internal/preflight*` sites already scoped into batch 2.
  No batch carries a speculative "update whatever breaks" card, and an implementer that finds an unrelated failing test must report it rather than edit it.
- **Rationale:** during Phase: Plan the whole change was applied as a throwaway spike (the `CommitAnchoredPaths` call in `internal/fabriccli/clone.go`, the `ConfigFileRel` accessor, and `--allow-empty` at all three fixture sites) and both halves of the repo gate were run against it.
  `go test ./...` passed;
  `go test -tags integration ./...` failed only on `internal/stencilcli`'s pre-existing `TestStencilCLI_ListAndValidate`, which also fails on the untouched worktree tip.
  `internal/fabricengine`, `internal/fabriccli`, `internal/hubforge`, `internal/preflight` and `internal/preflightshed` all reported `ok`.
  The spike was then fully reverted (`git checkout --`) before any plan file was written.
  This turns the discussion's "any test that breaks is a test asserting the bug" contingency into a measured empty set, so the plan does not have to leave an unbounded `Edits:` list anywhere.
- **Applies to:** all batches

### Decision: preexisting-stencilcli-failure-blocks-done-gate

- **Decision:** `internal/stencilcli`'s `TestStencilCLI_ListAndValidate` fails deterministically on this branch's tip **before** any of this task's work, and nothing in this plan fixes it.
  `mill-config.yaml`'s `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) will therefore fail at mill-go's Handoff pre-done gate on that one test.
- **Rationale:** the failure is `validate findings missing the expected warning finding for "loom-template-discussion"; got [map[marker:ZZZ_UNKNOWN_MARKER name:landing-template-conflict severity:error]]` — a stencil-marker validation defect in `internal/stencilcli`, unrelated to fabric's clone path, and confirmed reproducible with the worktree clean at commit `2d98c05d`.
  Fixing it is outside this task's Scope, and `mill-config.yaml` is deliberately not edited (a config mutation mid-flight would trip the `wiki-config-mutation` gate for a problem this task did not create).
  The operator handling that done-gate halt should confirm the failure set is exactly `{TestStencilCLI_ListAndValidate}` and treat it as pre-existing debt rather than a regression from this task.
- **Applies to:** all batches

### Decision: no-python-prefix-on-verify

- **Decision:** every `verify:` command is a bare `go test` invocation with no `PYTHONPATH= ` prefix.
- **Rationale:** this is a Go repository;
  the `PYTHONPATH= ` isolation prefix exists only for Python/mill projects whose test subprocess would otherwise inherit the mill cache scripts dir.
  `-count=1` is present on every invocation so a batch's verify never reports a cached pass from before its own edits.
- **Applies to:** all batches

### Decision: fixture-tolerance-lands-before-the-fix

- **Decision:** batch 2 (`--allow-empty` at the three enumerated fixture sites) is a root batch with no dependency on batch 1, and batch 3 depends on **both**.
- **Rationale:** the spike measured what happens if the clone-commit lands without the fixture tolerance: `internal/preflight` fails six subtests and `internal/preflightshed` fails three, all with `nothing to commit, working tree clean`.
  Landing the tolerance first makes every intermediate commit in the DAG green, which the one-commit-per-card execution convention requires.
  `--allow-empty` is a no-op on the pre-fix tree (the nine untracked configs still stage), so batch 2 is independently verifiable in either order.
- **Applies to:** clone-commits-module-configs

### Decision: seedconfig-trap-is-latent-not-live

- **Decision:** `internal/hubforge`'s own integration suite is **not** the thing that proves the `SeedConfig` `--allow-empty` change is needed;
  a new regression test in batch 3 is.
- **Rationale:** with `--allow-empty` reverted and the clone-commit in place, `internal/hubforge` still reported `ok` in the spike — no existing hubforge test seeds a module config byte-identical to the reconciled file, so the trap the `seedconfig-tolerates-empty-stage` Decision describes is real but currently unreached.
  This is why batch 3 carries the redundant-seed test (discussion Testing scenario 6) rather than relying on the existing suite to cover the helper edit.
- **Applies to:** fixture-empty-stage-tolerance, clone-commits-module-configs

### Decision: positive-pathspec-commit-shape

- **Decision:** the commit goes through `fabricengine.CommitAnchoredPaths(rec, l, relPaths, msg, fabricengine.SyncOptions{})` with `relPaths` built from `configengine.ConfigFileRel` — never a stage-all, never a `Bolt` commit, never a raw git call.
- **Rationale:** `CONSTRAINTS.md`'s Fabric Git Invariant requires a positive-only pathspec built via `fabricengine.ScopedPathspec` for any commit touching the `_lyx` tree, which every round-loop module shares;
  `CommitAnchoredPaths` is the wrapper that resolves the weft worktree and `AnchorRel` from `l` itself, so the CLI layer never names a weft path (Fabric Vocabulary Invariant) and never violates the Cwd Resolution Invariant.
  The Mutation Record Invariant is satisfied without a call-site recording: `CommitWeftPaths` appends `KindCommitCreated` only when the commit observably landed.
- **Applies to:** clone-commits-module-configs

### Decision: docs-land-with-their-own-behaviour-change

- **Decision:** each doc edit is committed by the card that changes the behaviour the doc describes — `docs/shared-libs/configengine.md` with the `ConfigFileRel` accessor (card 1), `SeedConfig`'s doc comment with its `--allow-empty` flag (card 2), and `internal/fabriccli/clone.go`'s two comments plus `internal/hubforge`'s `hub.go`/`doc.go` post-clone-state description with the commit itself (card 3).
- **Rationale:** `CLAUDE.md`'s Documentation Lifecycle rule requires the touched module's doc in the same commit.
  `internal/hubforge`'s post-clone-state description is caused by card 3's behaviour change, not by card 2's helper edit, so it travels with card 3 even though the file lives in the package card 2 also touches.
  No `CONSTRAINTS.md` change (no new cross-cutting invariant), no `docs/overview.md` change (no module-table or execution-stack move), no `manifest/roadmap.md` move (this is a bugfix).
- **Applies to:** all batches

### Decision: markdown-semantic-line-breaks

- **Decision:** every `.md` file this plan touches uses one sentence per line with soft newlines, never a fixed-column hard wrap and never trailing double-spaces.
- **Rationale:** `CLAUDE.md`'s Markdown rule, which applies to every `.md` file in this repo rather than only newly-written ones.
- **Applies to:** configfilerel-accessor

## All Files Touched

- `docs/shared-libs/configengine.md`
- `internal/configengine/config.go`
- `internal/configengine/config_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/cloneconfigcommit_integration_test.go`
- `internal/hubforge/doc.go`
- `internal/hubforge/hub.go`
- `internal/hubforge/seed.go`
- `internal/hubforge/seed_test.go`
- `internal/preflight/preflight_integration_test.go`
- `internal/preflightshed/preflight_integration_test.go`
