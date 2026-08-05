# Plan: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
slug: fabric-illusion-core
approved: false
started: 20260805-122944
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: pre-moves
    file: 01-pre-moves.md
    depends-on: []
    verify: go vet -tags "integration smoke scout" ./... && go test ./... && go test -tags integration ./...
  - number: 2
    name: rename-and-reshape
    file: 02-rename-and-reshape.md
    depends-on: [1]
    verify: go build -tags integration ./internal/lyxcwd/... && go vet -tags integration ./internal/lyxcwd/...
  - number: 3
    name: production-sweep
    file: 03-production-sweep.md
    depends-on: [2]
    verify: go build ./...
  - number: 4
    name: test-sweep
    file: 04-test-sweep.md
    depends-on: [3]
    verify: go vet -tags "integration smoke scout" ./... && go test ./... && go test -tags integration ./...
  - number: 5
    name: module-owned-constructors
    file: 05-module-owned-constructors.md
    depends-on: [4]
    verify: go vet -tags "integration smoke scout" ./... && go test ./internal/lyxcwd/... ./internal/loomengine/... ./internal/planparser/... ./internal/builderengine/... ./internal/buildercli/... ./internal/websterengine/... ./internal/webstercli/... ./internal/perchengine/... ./internal/perchcli/... ./internal/scoutengine/... ./internal/pattern/... ./internal/logger/... ./internal/reedengine/... ./internal/reedcli/... ./internal/burlerengine/... ./internal/shuttleengine/... ./cmd/lyx/... && go test -tags integration ./internal/lyxcwd/... ./internal/loomengine/... ./internal/planparser/... ./internal/builderengine/... ./internal/buildercli/... ./internal/websterengine/... ./internal/webstercli/... ./internal/perchengine/... ./internal/perchcli/... ./internal/scoutengine/... ./internal/pattern/... ./internal/logger/... ./internal/reedengine/... ./internal/reedcli/... ./internal/burlerengine/... ./internal/shuttleengine/... ./cmd/lyx/...
  - number: 6
    name: fabric-owns-the-illusion
    file: 06-fabric-owns-the-illusion.md
    depends-on: [5]
    verify: go vet -tags "integration smoke scout" ./... && go test ./internal/lyxcwd/... ./internal/fabricengine/... ./internal/fabriccli/... ./internal/lyxtest/... ./internal/pattern/... ./internal/loomengine/... ./internal/websterengine/... ./internal/webstercli/... ./internal/builderengine/... ./internal/buildercli/... ./internal/perchcli/... ./internal/boardcli/... ./internal/boardengine/... ./internal/configcli/... ./internal/configsync/... ./internal/ideengine/... ./cmd/lyx/... && go test -tags integration ./internal/lyxcwd/... ./internal/fabricengine/... ./internal/fabriccli/... ./internal/lyxtest/... ./internal/pattern/... ./internal/loomengine/... ./internal/websterengine/... ./internal/webstercli/... ./internal/builderengine/... ./internal/buildercli/... ./internal/perchcli/... ./internal/boardcli/... ./internal/boardengine/... ./internal/configcli/... ./internal/configsync/... ./internal/ideengine/... ./cmd/lyx/...
  - number: 7
    name: board-junction
    file: 07-board-junction.md
    depends-on: [6]
    verify: go vet -tags "integration smoke scout" ./... && go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/lyxcwd/... ./cmd/lyx/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./internal/lyxcwd/... ./cmd/lyx/...
  - number: 8
    name: guard-and-docs
    file: 08-guard-and-docs.md
    depends-on: [7]
    verify: go vet -tags "integration smoke scout" ./... && go test ./internal/lyxcwd/... ./cmd/lyx/...
```

## Shared Decisions

### Decision: eight batches, strictly serial, sized by implementer session rather than by logic

- **Decision:** Eight batches, each depending on the one before, none running in parallel. The discussion's `batching` decision draws five; batches 1-4 here are its batch 1 split four ways (pre-moves, rename-and-reshape, production-sweep, test-sweep), and batches 5-8 are its batches 2-5 unchanged.
- **Rationale:** The discussion's batch 1 is correct as a *logical* unit — the rename cannot be split, and rewriting every field consumer in the same pass means each importer is opened once, not three times. But as a *work* unit it is 19 cards and roughly 200 file-edits in a single implementer session, which is a turn-exhaustion risk, not a context one: mill-go would land in `stuck_type: incomplete` and recover by warm resume, and planning to rely on that recovery is planning badly. The split keeps the logical unit intact — the four batches are consecutive, serial, and no other work interleaves — while capping any one implementer session at seven cards. Batches 5-8 need no split: the largest is ten cards of pure relocation.
- **Applies to:** all batches

### Decision: a red intermediate tree is allowed only between batches 2 and 4, and never ungated

- **Decision:** Batch 2 verifies with `go build -tags integration ./internal/lyxcwd/... && go vet -tags integration ./internal/lyxcwd/...`; batch 3 verifies with `go build ./...`; batch 4 restores the repo-wide gate — tagged vet, the full untagged `go test ./...`, and `go test -tags integration ./...`. No batch anywhere in this plan uses `verify: null`.
- **Rationale:** The discussion requires each batch to leave the tree green, and batches 1 and 4-8 do. Batches 2 and 3 cannot: the moment `Layout` becomes `Location` the ~190 field consumers stop compiling, and sweeping them in the same batch is exactly the oversized unit the split above exists to avoid. The deviation is bounded to two batches and each still carries the strongest gate that is *true* at that point — batch 2 proves the renamed module compiles and vets on its own, batch 3 proves all ~85 production cutovers type-check together. `verify: null` was the alternative and is rejected: it would leave those batches with no gate at all, which is a strictly worse trade than a narrower one. `pipeline.done_gate` (`go test ./...`) is the repo-wide backstop before mill-go marks the task done.
- **Applies to:** rename-and-reshape, production-sweep

### Decision: within a batch, one card is one subsystem

- **Decision:** No card's `Edits:` spans more than one package family. The ~190-file consumer sweep is ten cards split by family — fabric, webster, builder/burler/loom, config/board/ide/leaf-libs, and the runtime engines — five production cards in batch 3 and five test cards in batch 4. The largest is 27 files.
- **Rationale:** A card is one commit and one unit of implementer attention. A 100-file card is neither reviewable as a diff nor holdable as a unit of work, however mechanical its content. Splitting by package family rather than by file count keeps each card's diff one subsystem's cutover, so a reviewer reads a coherent change instead of an alphabetical slice.
- **Applies to:** all batches

### Decision: the anchoring table — there is no single base

- **Decision:** Each relocated constructor keeps the base it has today. `AnchorPath()` for the durable, weft-synced, git-tracked `_lyx` group: `PlanDir`, `PlanOverview`, `DiscussionDir`, `DiscussionDecisionRecord`, `DiscussionSupportLog`, `LoomStatusFile`, `LoomStatusLock`, `BuilderDir`, `BuilderReportsDir`, `WebsterDir`, `WebsterReportsDir`, `WebsterPromptsDir`, `PerchRunsDir`, `PatternDir`, `PatternFile`, `PatternFileHere`. `WorktreePath()` for the ephemeral, machine-bound, never-git-tracked `.lyx` group: `WorktreeLogsDir`, `ScoutDaemonStateFile`, `ScoutDaemonLock`. `HubPath` for `HubLogsDir` alone.
- **Rationale:** A blanket "join onto `AnchorPath()`" would silently relocate four constructors. `HubLogsDir` is hub-anchored on purpose so one reed server per hub resolves to one deterministic place. The other three use `.lyx`, not `_lyx` — these are PIDs, sockets and rotating logs, which must never be git-tracked. The `_lyx` group moves from `WorktreeRoot` to `AnchorPath()` because the `_lyx` junction itself lives at worktree-root plus anchor; for an unanchored repo that is the same directory, and for a subpath-anchored repo the old base was pointing above the junction. The equivalence test in batch 2 is therefore **anchor-aware, not byte-identical**, and runs over both an unanchored and a subpath-anchored fixture so the intended move is asserted rather than assumed.
- **Applies to:** rename-and-reshape, module-owned-constructors

### Decision: `_lyx` is joined per segment, never as a fused literal

- **Decision:** A module must not declare `"_lyx/plan"`. It joins per segment — `filepath.Join(l.AnchorPath(), configengine.LyxDirName, planDirName)` — where `configengine.LyxDirName` supplies the policed token and the module's own private constant is an unpoliced ordinary name.
- **Rationale:** `TestEnforcement_GeometryLiterals` matches **whole tokens only**: `geometryToken` is a switch over exact values after `strconv.Unquote`, so `"_lyx/plan"` is not a match and would slip past unowned in all three scanned contexts. Fusing the segments would make the `_lyx` ownership row vacuous — it would police a token nobody declares. Per-segment joining is what keeps the single-declarer rule enforced rather than merely stated, and it is also why batch 2 registers **no** `_lyx` owners: no relocated constructor declares the token.
- **Applies to:** module-owned-constructors, guard-and-docs

### Decision: the guard is staged token by token, in lockstep with each owner

- **Decision:** `TestEnforcement_GeometryLiterals` is not rewritten once at the end. Each token's ownership row lands in the same card that declares or deletes that token's literal: batch 1's cards 1 and 2 convert the allowlist to the per-token ownership map and register `-weft` and `_lyx` in the same cards that declare them; batch 4's card 19 only retargets the surviving directory values from `internal/hubgeometry` to `internal/lyxcwd`; batch 5's card 25 moves `_pattern`'s row when it declares `pattern.DirName`, keeping a transitional `internal/lyxcwd` co-owner for the retained `PatternDirName` const; batch 6 registers `_portals`, `_launchers`, `_raddle`, `_board` and `-HUB` (card 36) and finishes `_pattern`'s row (card 35, swapping the transitional entry for `internal/fabricengine` when the pathspec const moves); batch 8 collapses the remaining scaffolding and removes the transitional `_lyx` co-owner.
- **Rationale:** The guard is only a real assertion when it names an ownership that already exists. Written up front it would assert a layout the tree does not have; written only at the end it would leave five batches red, because `enforcement_test.go:420` allowlists the literal directory `internal/hubgeometry` and the rename alone breaks it — which is also why the guard's own directory switch sits in batch 4, the first batch after the rename whose gate actually runs the test. Moving each entry with its owner is also what proves each batch **moved** ownership rather than copying code.
- **Applies to:** pre-moves, test-sweep, module-owned-constructors, fabric-owns-the-illusion, guard-and-docs

### Decision: every gate that can run Tier 2 does, because this slice's new tests are all Tier 2

- **Decision:** Each batch's `verify` is up to three commands: `go vet -tags "integration smoke scout" ./...`, then `go test` over the packages that batch touches, then `go test -tags integration` over the same packages. Batches 1 and 4 use `./...` for both test runs. Batches 2 and 3 have no test run at all — see the red-tree decision above. Batch 8 runs no tagged tests because its only code change is to an untagged file. `pipeline.done_gate` is `go test ./... && go test -tags integration ./...`.
- **Rationale:** The tagged `go vet` alone is not a test — it type-checks tagged files and executes nothing. Every new runtime assertion this slice adds is Tier 2: the `_board` junction's wiring, its absence from `filterHubReserved`/`ScopedPathspec`, its invisibility to `Healthy`, and the stale-`.fabric-anchor` hard error all need a real clone and real filesystem links. Without an explicit `-tags integration` execution they would be compiled and never checked, which is worse than not writing them. The vet stage still earns its place: it is the only thing that catches a *broken* tagged file in batches 2 and 3, where nothing can run. Cost is roughly 128 s per tagged run (`docs/benchmarks/running-tests.md`), and the repo's own guidance is that `-tags integration` is warranted precisely when changing fabric / hubgeometry / board / ide git behaviour — which is this entire task.
- **Applies to:** all batches

### Decision: test call sites are in scope and are the bulk of the diff

- **Decision:** Every relocation list in the discussion and in these batch files is production-only unless it says otherwise, and the test call sites are planned, budgeted and listed explicitly alongside them.
- **Rationale:** `go test ./...` must stay green, so a test file calling a moved or privatized symbol breaks a batch exactly as a production file does. Measured, not estimated: the config-symbol move alone is 228 hits across 34 files, and roughly 60 further synthetic `Layout` literals live in test files across ~20 packages. Batches 1-4's real shape is a small number of substantive rewrites inside a large mechanical sweep — review them accordingly, because a reviewer expecting the mechanical portion to be small will be looking at roughly five times that.
- **Applies to:** all batches

### Decision: `PlanDirRel` goes to `internal/planparser`, not `internal/loomengine`

- **Decision:** `internal/planparser` declares `PlanDirName` and `PlanDirRel()`; `internal/loomengine.PlanDir` joins onto `planparser.PlanDirName`.
- **Rationale:** Deviation from the discussion's `per-module-constructors` decision, which lists `PlanDirRel` with the loom group. Its sole production consumer is `planparser/parse.go:236`, which stamps the token into `Card.SourcePath`; sending it to `loomengine` would make a pure parser library import a feature engine, inverting the layering the Planparser Sole-Parser Invariant assumes. `loomengine` → `planparser` is the natural direction and is acyclic — `planparser` imports only `configengine` after the move. The `plan` segment still has exactly one declarer.
- **Applies to:** module-owned-constructors

### Decision: batch-size caps were raised in `mill-config.yaml` before planning

- **Decision:** `pipeline.max_cards_per_batch` is 20 and `pipeline.max_batch_context_tokens` is 700000. Both were raised as an orchestrator config change before this plan was written; no card edits `mill-config.yaml`.
- **Rationale:** The defaults (10 cards, 120000 tokens) are sized for a 200k-context implementer. mill-go's implementer here is Sonnet with a 1M window, and the consumer sweep's file union is ~200 files at roughly 514000 estimated tokens — a repo-wide package rename cannot be split without leaving the tree uncompilable, so the cap must not be what decides the batching. The raised value is deliberately slack: it exists to stop the validator from vetoing a correct design, not to license large batches. The two rules above — one card is one package family, and no batch exceeds ten cards in practice — are what actually bound implementer and reviewer load.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/exitcode_test.go`
- `cmd/lyx/main_integration_test.go`
- `cmd/lyx/registration_test.go`
- `cmd/lyx/unknown_subcommand_test.go`
- `docs/overview.md`
- `docs/reference/builder-contract.md`
- `docs/reference/discussion-format.md`
- `docs/reference/model-spec.md`
- `docs/reference/plan-format.md`
- `docs/reference/status-schema.md`
- `docs/shared-libs/README.md`
- `docs/shared-libs/configengine.md`
- `docs/shared-libs/envsource.md`
- `docs/shared-libs/lyxcwd.md`
- `internal/boardcli/cli.go`
- `internal/boardcli/cli_test.go`
- `internal/boardcli/notes_test.go`
- `internal/boardengine/boardtest/bench_cli_test.go`
- `internal/boardengine/boardtest/bench_test.go`
- `internal/boardengine/config.go`
- `internal/boardengine/config_test.go`
- `internal/boardengine/template_test.go`
- `internal/buildercli/cli.go`
- `internal/buildercli/pause_test.go`
- `internal/buildercli/poll.go`
- `internal/buildercli/poll_test.go`
- `internal/buildercli/run.go`
- `internal/buildercli/run_test.go`
- `internal/buildercli/smoke_test.go`
- `internal/buildercli/spawnbatch.go`
- `internal/buildercli/spawnbatch_test.go`
- `internal/buildercli/status_test.go`
- `internal/buildercli/testdata_test.go`
- `internal/buildercli/validate.go`
- `internal/buildercli/weft.go`
- `internal/buildercli/weft_integration_test.go`
- `internal/buildercli/weft_test.go`
- `internal/builderengine/doc.go`
- `internal/builderengine/plan.go`
- `internal/builderengine/spawn.go`
- `internal/builderengine/spawn_test.go`
- `internal/builderengine/state.go`
- `internal/burlercli/cli.go`
- `internal/burlerengine/config.go`
- `internal/burlerengine/config_test.go`
- `internal/burlerengine/doc.go`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/engine_test.go`
- `internal/burlerengine/prompt.go`
- `internal/burlerengine/smoke_cluster_test.go`
- `internal/burlerengine/smoke_round_test.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configcli/configcli_test.go`
- `internal/configcli/menu.go`
- `internal/configcli/reconcile_integration_test.go`
- `internal/configcli/reconcile_test.go`
- `internal/configengine/config.go`
- `internal/configengine/config_test.go`
- `internal/configengine/edit.go`
- `internal/configengine/edit_test.go`
- `internal/configengine/set.go`
- `internal/configengine/set_test.go`
- `internal/configsync/configsync.go`
- `internal/configsync/configsync_test.go`
- `internal/envsource/envsource.go`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/unwire.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/add_branch_exists_test.go`
- `internal/fabricengine/add_rollback_adopt_test.go`
- `internal/fabricengine/add_test.go`
- `internal/fabricengine/boardjunction_integration_test.go`
- `internal/fabricengine/branchname.go`
- `internal/fabricengine/branchname_test.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/checkout_index_refresh_test.go`
- `internal/fabricengine/checkout_rollback_test.go`
- `internal/fabricengine/cleanup.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/clone_adopt_test.go`
- `internal/fabricengine/commit.go`
- `internal/fabricengine/commit_integration_test.go`
- `internal/fabricengine/config_driven_junctions_integration_test.go`
- `internal/fabricengine/config_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/drift.go`
- `internal/fabricengine/fabric.go`
- `internal/fabricengine/hook.go`
- `internal/fabricengine/hook_test.go`
- `internal/fabricengine/hostclean.go`
- `internal/fabricengine/hostjunction_test.go`
- `internal/fabricengine/hostlayout.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/junction_pattern_integration_test.go`
- `internal/fabricengine/junction_repoint_test.go`
- `internal/fabricengine/junction_test.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/junctionnames_test.go`
- `internal/fabricengine/launchers.go`
- `internal/fabricengine/list.go`
- `internal/fabricengine/portallauncher_test.go`
- `internal/fabricengine/portals.go`
- `internal/fabricengine/prune.go`
- `internal/fabricengine/pull.go`
- `internal/fabricengine/pull_integration_test.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/reconcile_stale_registration_test.go`
- `internal/fabricengine/reconcile_stale_removal_test.go`
- `internal/fabricengine/remove.go`
- `internal/fabricengine/remove_junctions_integration_test.go`
- `internal/fabricengine/snapshot_integration_test.go`
- `internal/fabricengine/status.go`
- `internal/fabricengine/topology.go`
- `internal/fabricengine/unwire.go`
- `internal/fabricengine/unwire_test.go`
- `internal/fabricengine/warpforward_integration_test.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabricengine/weftgit_exclude_test.go`
- `internal/fabricengine/weftpaths_test.go`
- `internal/fabricengine/weftwiring.go`
- `internal/fabricengine/weftwiring_test.go`
- `internal/fabricengine/worktreelist.go`
- `internal/fabricengine/worktreelist_test.go`
- `internal/gitrepo/doc.go`
- `internal/hubgeometry/enforcement_test.go`
- `internal/hubgeometry/geometry_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_test.go`
- `internal/hubgeometry/hubgeometry_unit_test.go`
- `internal/hubgeometry/pattern_test.go`
- `internal/hubgeometry/weft_test.go`
- `internal/idecli/cli.go`
- `internal/idecli/cli_test.go`
- `internal/ideengine/menu.go`
- `internal/ideengine/menu_test.go`
- `internal/ideengine/spawn.go`
- `internal/ideengine/spawn_test.go`
- `internal/logger/logger.go`
- `internal/logger/retention.go`
- `internal/logger/sink.go`
- `internal/logger/sink_test.go`
- `internal/logger/worktreelogs_test.go`
- `internal/loomengine/discussion.go`
- `internal/loomengine/discussion_test.go`
- `internal/loomengine/discussionpath_test.go`
- `internal/loomengine/loomstatus_test.go`
- `internal/loomengine/plan.go`
- `internal/loomengine/plan_test.go`
- `internal/loomengine/planpath_test.go`
- `internal/loomengine/preflight.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/lyxcwd/anchor.go`
- `internal/lyxcwd/anchor_test.go`
- `internal/lyxcwd/discussionpath_test.go`
- `internal/lyxcwd/enforcement_test.go`
- `internal/lyxcwd/gate_test.go`
- `internal/lyxcwd/geometry_test.go`
- `internal/lyxcwd/loomstatus_test.go`
- `internal/lyxcwd/lyxcwd.go`
- `internal/lyxcwd/lyxcwd_test.go`
- `internal/lyxcwd/lyxcwd_unit_test.go`
- `internal/lyxcwd/pattern_test.go`
- `internal/lyxcwd/planpath_test.go`
- `internal/lyxcwd/raddle_guard_test.go`
- `internal/lyxcwd/scoutdaemon_test.go`
- `internal/lyxcwd/testmain_test.go`
- `internal/lyxcwd/webstergeom_test.go`
- `internal/lyxcwd/weft_test.go`
- `internal/lyxcwd/worktreelogs_test.go`
- `internal/lyxtest/doc.go`
- `internal/lyxtest/leaf_enforcement_test.go`
- `internal/lyxtest/lyxtest.go`
- `internal/lyxtest/lyxtest_test.go`
- `internal/modelspec/leaf_enforcement_test.go`
- `internal/modelspec/load.go`
- `internal/modelspec/load_test.go`
- `internal/modelspec/modelspec.go`
- `internal/modelspec/template_test.go`
- `internal/pattern/doc.go`
- `internal/pattern/leaf_enforcement_test.go`
- `internal/pattern/pattern.go`
- `internal/pattern/pattern_test.go`
- `internal/pattern/patternpath_test.go`
- `internal/perchcli/cli.go`
- `internal/perchcli/cli_integration_test.go`
- `internal/perchcli/run.go`
- `internal/perchcli/run_integration_test.go`
- `internal/perchengine/config_test.go`
- `internal/perchengine/doc.go`
- `internal/perchengine/engine.go`
- `internal/perchengine/identity_test.go`
- `internal/perchengine/run_test.go`
- `internal/planparser/parse.go`
- `internal/reedcli/cli.go`
- `internal/reedcli/cli_integration_test.go`
- `internal/reedcli/smoke_lifecycle_test.go`
- `internal/reedengine/config_test.go`
- `internal/reedengine/contract_integration_test.go`
- `internal/reedengine/header_test.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/lock.go`
- `internal/reedengine/lock_test.go`
- `internal/reedengine/mouse_boot_integration_test.go`
- `internal/reedengine/server.go`
- `internal/reedengine/spawn.go`
- `internal/reedengine/spawn_test.go`
- `internal/reedengine/strand.go`
- `internal/reedengine/strand_test.go`
- `internal/scoutcli/cli.go`
- `internal/scoutcli/cli_test.go`
- `internal/scoutengine/daemonstate.go`
- `internal/scoutengine/doc.go`
- `internal/scoutengine/ensureserver.go`
- `internal/scoutengine/ensureserver_integration_test.go`
- `internal/scoutengine/ensureserver_test.go`
- `internal/scoutengine/leaf_enforcement_test.go`
- `internal/scoutengine/load.go`
- `internal/scoutengine/load_test.go`
- `internal/scoutengine/refs_integration_test.go`
- `internal/scoutengine/scoutdaemon_test.go`
- `internal/scoutengine/supervised_integration_test.go`
- `internal/scoutengine/supervised_scout_test.go`
- `internal/scoutengine/supervised_test.go`
- `internal/scoutengine/toolchain.go`
- `internal/shuttlecli/cli.go`
- `internal/shuttlecli/cli_test.go`
- `internal/shuttlecli/smoke_interrupt_test.go`
- `internal/shuttleengine/config_test.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/run_inject_test.go`
- `internal/shuttleengine/run_test.go`
- `internal/shuttleengine/rundir.go`
- `internal/shuttleengine/rundir_test.go`
- `internal/shuttleengine/wait.go`
- `internal/shuttleengine/wait_test.go`
- `internal/tokenvocab/doc.go`
- `internal/tokenvocab/leaf_enforcement_test.go`
- `internal/tokenvocab/tokenvocab.go`
- `internal/tokenvocab/tokenvocab_test.go`
- `internal/treadleengine/doc.go`
- `internal/treadleengine/engine.go`
- `internal/treadleengine/seam_enforcement_test.go`
- `internal/treadleengine/smoke_judge_test.go`
- `internal/vscode/color.go`
- `internal/vscode/color_test.go`
- `internal/webstercli/beginbatch.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/recordbatch.go`
- `internal/webstercli/recoverbatch.go`
- `internal/webstercli/run.go`
- `internal/webstercli/smoke_test.go`
- `internal/webstercli/validate.go`
- `internal/webstercli/verbs_test.go`
- `internal/webstercli/weft.go`
- `internal/webstercli/weft_integration_test.go`
- `internal/websterengine/audit.go`
- `internal/websterengine/audit_test.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/beginbatch_test.go`
- `internal/websterengine/config_test.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/recordbatch.go`
- `internal/websterengine/recordbatch_test.go`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/recoverbatch_test.go`
- `internal/websterengine/render.go`
- `internal/websterengine/report.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/runlevel_test.go`
- `internal/websterengine/state.go`
- `internal/websterengine/template_test.go`
- `internal/websterengine/webstergeom_test.go`
- `internal/weftname/weftname.go`
- `internal/weftname/weftname_test.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/designs/loom.md`
- `manifest/designs/pattern.md`
- `manifest/roadmap.md`
