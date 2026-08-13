# Plan: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
slug: lyxtest-real-hubs
approved: false
started: '20260812-180311'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: gitkit leaf
    file: 01-gitkit-leaf.md
    depends-on: []
    verify: go vet -tags integration ./... && go vet -tags smoke ./... && go vet -tags scout ./... && go test -tags integration ./internal/gitkit/... ./internal/lyxcwd/... ./cmd/lyx/...
  - number: 2
    name: fabrictest dissolution
    file: 02-fabrictest-dissolution.md
    depends-on: [1]
    verify: go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/hubforge/... ./internal/fabricengine/... ./internal/lyxcwd/... ./cmd/lyx/...
  - number: 3
    name: hubforge factory
    file: 03-hubforge-factory.md
    depends-on: [2]
    verify: go vet -tags integration ./... && go test -tags integration ./internal/hubforge/... ./internal/fabricengine/... ./cmd/lyx/...
  - number: 4
    name: small consumers
    file: 04-small-consumers.md
    depends-on: [3]
    verify: go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/webstercli/... ./internal/configcli/... ./internal/idecli/... ./internal/boardengine/... ./internal/perchcli/...
  - number: 5
    name: reedcli
    file: 05-reedcli.md
    depends-on: [4]
    verify: go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/reedcli/...
  - number: 6
    name: stuck packages
    file: 06-stuck-packages.md
    depends-on: [5]
    verify: go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/loomengine/... ./internal/treadleengine/...
  - number: 7
    name: fabriccli
    file: 07-fabriccli.md
    depends-on: [6]
    verify: go vet -tags integration ./... && go test -tags integration ./internal/fabriccli/...
  - number: 8
    name: fabricengine external
    file: 08-fabricengine-external.md
    depends-on: [7]
    verify: go vet -tags integration ./... && go test -tags integration ./internal/fabricengine/...
  - number: 9
    name: fabricengine in-package weft
    file: 09-fabricengine-inpackage-weft.md
    depends-on: [8]
    verify: go vet -tags integration ./... && go test -tags integration ./internal/fabricengine/...
  - number: 10
    name: fabricengine in-package hub
    file: 10-fabricengine-inpackage-hub.md
    depends-on: [9]
    verify: go vet -tags integration ./... && go test -tags integration ./internal/fabricengine/...
  - number: 11
    name: helper deletion
    file: 11-helper-deletion.md
    depends-on: [10]
    verify: go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/gitkit/... ./internal/lyxcwd/... ./cmd/lyx/... ./internal/fabricengine/... ./internal/fabriccli/...
  - number: 12
    name: docs
    file: 12-docs.md
    depends-on: [11]
    verify: go vet -tags integration ./... && go test ./internal/lyxcwd/... ./cmd/lyx/...
```

The DAG is a straight chain, and that is a decision rather than an oversight.
Batches 4 through 10 touch disjoint packages and could nominally run in parallel, but each one teaches the next: the fixture-field mapping and the `SeedConfig` triage are settled on seventeen sites in batch 4 before being applied to `fabricengine`'s eighty-two in batches 8 through 10.
The discussion's own migration order — smallest package first — is what this chain encodes.

## Shared Decisions

### Decision: three packages, split by role

- **Decision:**
  `internal/gitkit` is the below-fabric leaf holding git primitives;
  `internal/hubforge` is the repo-wide real-hub factory;
  fabric's live-state assertions become `package fabricengine_test` files inside `internal/fabricengine/`.
  Neither `lyxtest` nor `fabrictest` survives as a package name.
- **Rationale:**
  The three have different jobs. `gitkit` hands out git primitives and asserts nothing. `hubforge` builds hubs and asserts nothing — it is a factory, not a test suite, which is why its name does not end in `test`.
  Fabric's live-state assertions test fabric, so they belong with fabric. `hubforge`'s importability is load-bearing (its consumers live in ~15 directories); `fabrictest`'s importability was used by nobody.
- **Applies to:** all batches

### Decision: gitkit is a compile constraint, not a preference

- **Decision:**
  `internal/gitkit` imports only stdlib plus `lyxcwd`, `weftname`, `configengine`, `lyxdirs`, and never imports fabric.
- **Rationale:**
  23 packages call `HermeticGitEnv` from `TestMain`, and 11 of them sit inside `internal/fabriccli`'s dependency set (`gitexec`, `gitrepo`, `lyxcwd`, `boardengine`, `burlerengine`, `loomengine`, `perchengine`, `treadleengine`, `websterengine`, `fabricengine`, `fabriccli`).
  A fabric import in the only module offering `HermeticGitEnv` would stop those `TestMain` files compiling.
  This is enforced by `internal/gitkit/leaf_enforcement_test.go`, ported in batch 1 before any migration starts.
- **Applies to:** all batches

### Decision: the fixture-field mapping table

- **Decision:**
  Every migrated call site replaces its old fixture with `hubforge.NewHub(tb, ".")` and rewrites its field reads by this table.

| old | new | note |
|---|---|---|
| `f.Hub` used as a git repo or a cwd | `h.PrimeWorktree()` | the old field named `Hub` held a repo that was not a hub |
| `f.Hub` used as a container/parent directory | `h.Path` | the real `<name>-HUB` container, which is not a git repo |
| `f.WeftPrime` | `h.PrimeWeft()` | un-anchored weft worktree root |
| `f.Bare` | `h.WarpBare` | this hub's own copy of the warp bare |
| `f.WeftBare` | `h.WeftBare` | this hub's own copy of the weft bare |
| `f.Container` | `h.Container` | the `tb.TempDir()` the hub was cloned into |
| `f.Layout` | `h.Location` | obtained from `lyxcwd.Resolve`, never constructed by hand |
| `WeftFixture.WeftPath` | `h.PrimeWeft()` | — |
| `WeftFixture.Bare` | `h.WeftBare` | — |
| a hand-built junction/worktree path | `h.PairWarpWorktree(slug)`, `h.PairWeftSibling(slug)`, `h.PairPortalLink(slug)`, `h.PairLauncherDir(slug)`, `h.BoardDir()` | never string concatenation |

- **Rationale:**
  `f.Hub` is the ambiguous one and the reason this is a table rather than a rename: the old field held a git repo standing in for a hub, so the correct replacement depends on which of the two roles the call site was using it for, and choosing wrong fails at a distance rather than at the line.
- **Applies to:** batches 4, 5, 6, 7, 8, 9, 10

### Decision: the three-way SeedConfig triage

- **Decision:**
  Each of the 56 `SeedConfig` call sites resolves to exactly one of three outcomes, decided per site by reading what it seeds and where:
  1. **Drop the call.** `fabriccli.CloneAndWire` runs `configsync.ReconcileAll` and `ReconcileFabricAt`, so a real hub arrives with materialized default config for every registered module.
     A site seeding a module's plain registered `ConfigTemplate()` deletes its call outright.
  2. **`hubforge.SeedConfig(tb, h, map[string]string{...})`** for a site that overrides a value.
     It writes into `h.WeftBase` and commits in the weft worktree.
  3. **`hubforge.SeedFabricConfig(tb, h, yaml)`** for repo-wide fabric config, which goes to `h.BoardDir()` and commits through `fabricengine.NewBolt`.
- **Rationale:**
  Neither of `gitkit.SeedConfig`'s current arguments works on a real hub: 32 of the 56 sites pass `fixture.Hub`, which is now the `<name>-HUB` container and not a git repo at all, so the commit fails outright.
  Seeding the warp worktree fails too — `<worktree>/_lyx` is a weft junction excluded from the warp index via `.git/info/exclude`, so `git add .` stages nothing.
  A single `SeedConfig` that guessed its base from the path shape was rejected: it would pick wrong on the three ad-hoc sites, silently.
  Those three sites get named per-site resolutions rather than the general rule, because none of them fits it: the `nested` and `warpSubdir` sites are resolved in batch 4 card 27 (both hand-build an anchor a real hub records for itself, so both become `hubforge.NewHub` at the real anchor), and the `sibling` site is resolved in batch 5 card 33 (it is a plain second clone, so it stays on `gitkit.SeedConfig` unchanged).
- **Applies to:** batches 4, 5, 6, 7, 8

### Decision: WeftBase is anchor-joined, and getting it wrong is silent

- **Decision:**
  `hubforge.SeedConfig`'s base is `h.WeftBase`, populated verbatim from `CloneResult.WeftBase`, never `h.PrimeWeft()`.
- **Rationale:**
  `fabricengine`'s `CloneHub` computes `WeftBase` as `filepath.Join(WeftWorktree(l), l.AnchorRel)`, whereas `PrimeWeft()` returns the un-anchored weft sibling.
  They coincide at the `"."` anchor and diverge at `"backend"`, where seeding the un-anchored path writes `<weft>/_lyx/config` while every module loader reads `<weft>/backend/_lyx/config` — the override silently does not take effect, with no error.
  This is why batch 3 card 18's seeding test runs at both anchors: a `"."`-only test passes even when the base is wrong.
- **Applies to:** batches 3, 4, 5, 6, 7, 8

### Decision: every broken assertion is read, never silenced

- **Decision:**
  A real hub is ~155 files against the old templates' ~36 and carries `_board`, `_portals`, `_launchers`, junctions, an anchor marker, a hub-level `.lyx` and a repo-wide `fabric.yaml` the old fixtures lacked.
  Directory listings, file counts and "this path should not exist" assertions will break.
  Each break is re-expressed against the real shape.
  A migration that deletes an assertion rather than re-pointing it states why in its commit message.
- **Rationale:**
  Every break marks a place currently asserting against an invented shape — that is the finding the task exists to surface, not noise to suppress.
  An assertion that still passes unchanged on a real hub is worth re-reading rather than treating as a relief.
- **Applies to:** batches 4, 5, 6, 7, 8, 9, 10

### Decision: build tags — production untagged, git-spawning tests tagged

- **Decision:**
  `internal/gitkit` and `internal/hubforge` production code is untagged.
  Each package's own git-spawning tests carry `//go:build integration`, and each keeps an untagged `doc.go` and an untagged `testmain_test.go`.
- **Rationale:**
  `hubforge` merges an untagged source (`lyxtest.go`) with an integration-tagged one (`fabrictest/hub.go`).
  Tagging every file `integration` would leave the package with zero files in the untagged build, which the `go vet ./...` gate runs.
  Untagged production is safe because the Test Tier Purity Invariant bans untagged *tests* from calling `gitkit.Copy*` and `hubforge.NewHub` by token, not by build tag — and all 132 current `Copy*` call sites already sit in tagged files, so nothing regresses.
  The untagged `testmain_test.go` is `fabrictest`'s own existing pattern: it must be compiled into the test binary on a plain `go test` as well as under `-tags integration`, or the tagged suites would run with no hermetic environment installed.
- **Applies to:** batches 1, 2, 3

### Decision: the verify shape is vet-both-tags plus scoped tests

- **Decision:**
  Every batch's `verify:` starts with `go vet -tags integration ./...`, plus one further `go vet` per custom tag the batch's edited test files carry — `go vet -tags smoke ./...` when it touches a smoke-tagged file, and `go vet -tags scout ./...` when it touches a scout-tagged one (batch 1 does, via `internal/scoutengine`'s three `//go:build scout` files) — then runs `go test -tags integration` scoped to the packages the batch changed.
  Each tag gets its own `go vet` invocation rather than being appended to an existing `-tags` list, because Go reads a comma-separated `-tags` value as a conjunction: `-tags integration,scout` would require both tags at once and compile neither suite.
- **Rationale:**
  `go vet` type-checks test files, so it is the cheapest whole-repo gate that catches a migration breaking a package the batch's scoped tests never load — which matters when a single batch edits files across thirteen directories.
  `smoke`-tagged suites are compile-checked and **never executed**: they spawn live tmux sessions and real LLM agents.
  That is a stated coverage limit, not an oversight — batches 4, 5 and 6 name the exact call sites it leaves untested, and the repo-wide `done_gate` does not close it either, since it runs no `smoke`-tagged tests.
- **Applies to:** all batches

### Decision: t.Chdir stays, t.Parallel does not arrive

- **Decision:**
  No call site is migrated off `t.Chdir` and no `t.Parallel` is enabled anywhere in this task.
  Only the path handed to `t.Chdir` changes.
- **Rationale:**
  `hubforge.NewHub`'s parallel safety is structural — a `sync.Once` template read-only, then per-test `tb.TempDir()` — and batch 3 proves it with a concurrency test.
  But roughly 20 fixture-using files call `t.Chdir`, which Go makes incompatible with `t.Parallel`, and unblocking them means touching CLI signatures, which the CLI/Cobra Invariant puts outside this task.
  Filed as the wiki task `hubforge-parallel-chdir`, `depends_on: lyxtest-real-hubs`.
- **Applies to:** batches 4, 5, 6, 7, 8, 9, 10

### Decision: Windows-correct by construction, unmeasured

- **Decision:**
  No Windows benchmark and no measurement gate, but every path must be Win11-correct by construction: all links go through `internal/fslink` (`CreateDirLink`, directory-only), remote URLs pass through `filepath.ToSlash` before reaching git, teardown removes junctions before `os.RemoveAll`, and nothing relies on Windows file symlinks.
- **Rationale:**
  Even a 5× worse clone-versus-copy ratio is roughly +15 s on a 132 s Tier 2 run, so it is not a design blocker — but a junction inside a temp dir Go will `os.RemoveAll` 132 times per suite run is a correctness question, not a speed one.
- **Applies to:** batches 2, 3

### Decision: NewPairedForTest is narrowed, not deleted — a scope deviation

- **Decision:**
  The `NewPairedForTest` shim in `internal/fabricengine/export_test.go` is renamed to `NewPairedFromPathsForTest` and kept for one consumer, rather than deleted as the discussion's scope line states.
- **Rationale:**
  18 of its 22 call sites exist only because the old fixtures could not produce a genuine warp/weft pair, and those 18 go away.
  The remaining consumer is `internal/fabricengine/fabric_test.go`, an **untagged** unit test of the `newPaired` constructor itself: it hands the shim two empty `os.Mkdir` directories and asserts the warp and weft fields come back non-nil.
  That spawns no git, has nothing to do with hub fixtures, and cannot move onto `hubforge.NewHub` because the Test Tier Purity Invariant bans an untagged test from calling it.
  Deleting the shim would delete that coverage for no gain.
  The discussion's blanket deletion line was derived from the 18 fixture sites and did not account for this one;
  batch 11's grep gate is written against the fixture usage accordingly.
- **Applies to:** batches 8, 11

### Decision: scope boundaries carried from the discussion

- **Decision:**
  `internal/gitrepo` and `internal/lyxcwd` never get hub fixtures.
  No new production code lands in `internal/fabricengine` or `internal/fabriccli`, and in particular no hub-teardown verb.
  No CLI signature changes.
  The sandbox Hub (`lyx-test-HUB`) is untouched.
- **Rationale:**
  Both low-level packages sit inside fabric's dependency set, so importing `hubforge` is a compile error regardless — and independently of that, keeping them on primitive fixtures preserves a layer that still fails on its own when a fabric clone bug lands, instead of every package in the repo failing at once.
  `fabricengine.Unwire` was rejected as a teardown mechanism because it is per-warp-worktree rather than per-hub, deliberately never touches repo-wide `weft:main` records, and carries refusal semantics that would fail teardown on exactly the deliberately-corrupt fixtures the live-state matrix plants.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens)._

- `CLAUDE.md`
- `cmd/lyx/boardguard_test.go`
- `cmd/lyx/destructiveguard_test.go`
- `cmd/lyx/hermeticenv_test.go`
- `cmd/lyx/testmain_test.go`
- `cmd/lyx/tierpurity_test.go`
- `cmd/lyx/tiersleep_test.go`
- `crucible/review-prompt-template.md`
- `docs/benchmarks/fixture-copy.md`
- `docs/benchmarks/running-tests.md`
- `docs/benchmarks/scout-vs-grep.md`
- `docs/benchmarks/test-suite-timing.md`
- `docs/overview.md`
- `docs/shared-libs/lyxcwd.md`
- `internal/batcher/config_test.go`
- `internal/boardcli/testmain_test.go`
- `internal/boardengine/boardtest/sync_test.go`
- `internal/boardengine/boardtest/testmain_test.go`
- `internal/boardengine/sync_integration_test.go`
- `internal/boardengine/testmain_test.go`
- `internal/burlerengine/smoke_cluster_test.go`
- `internal/burlerengine/smoke_round_test.go`
- `internal/burlerengine/testmain_test.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configcli/testmain_test.go`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/pushbypass_integration_test.go`
- `internal/fabriccli/testmain_test.go`
- `internal/fabricengine/add_branch_exists_test.go`
- `internal/fabricengine/add_rollback_adopt_test.go`
- `internal/fabricengine/bolt_integration_test.go`
- `internal/fabricengine/checkout_index_refresh_test.go`
- `internal/fabricengine/checkout_rollback_test.go`
- `internal/fabricengine/cleanreason_integration_test.go`
- `internal/fabricengine/cleanup_primary_integration_test.go`
- `internal/fabricengine/clone_adopt_test.go`
- `internal/fabricengine/clone_emptyweft_integration_test.go`
- `internal/fabricengine/clone_test.go`
- `internal/fabricengine/coalesce_integration_test.go`
- `internal/fabricengine/commit_integration_test.go`
- `internal/fabricengine/commitweftat_test.go`
- `internal/fabricengine/config_driven_junctions_integration_test.go`
- `internal/fabricengine/destroy_test.go`
- `internal/fabricengine/destructivegaps_integration_test.go`
- `internal/fabricengine/diff_integration_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/dotlyxjunction_integration_test.go`
- `internal/fabricengine/export_test.go`
- `internal/fabricengine/fabric_test.go`
- `internal/fabricengine/fabrictest/hub.go`
- `internal/fabricengine/fabrictest/testmain_test.go`
- `internal/fabricengine/healthreason_integration_test.go`
- `internal/fabricengine/hook_test.go`
- `internal/fabricengine/index_integration_test.go`
- `internal/fabricengine/junction_pattern_integration_test.go`
- `internal/fabricengine/junction_repoint_test.go`
- `internal/fabricengine/livestate_doc_test.go`
- `internal/fabricengine/livestate_helpers_test.go`
- `internal/fabricengine/livestate_manifest_selftest_test.go`
- `internal/fabricengine/livestate_manifest_test.go`
- `internal/fabricengine/livestate_matrix_test.go`
- `internal/fabricengine/livestate_mutationoracle_selftest_test.go`
- `internal/fabricengine/livestate_mutationoracle_test.go`
- `internal/fabricengine/livestate_refusal_selftest_test.go`
- `internal/fabricengine/livestate_refusal_test.go`
- `internal/fabricengine/livestate_states_selftest_test.go`
- `internal/fabricengine/livestate_states_test.go`
- `internal/fabricengine/livestate_verbs_selftest_test.go`
- `internal/fabricengine/livestate_verbs_test.go`
- `internal/fabricengine/mutation.go`
- `internal/fabricengine/mutation_record_integration_test.go`
- `internal/fabricengine/open_integration_test.go`
- `internal/fabricengine/prune_dirty_integration_test.go`
- `internal/fabricengine/prune_unowned_integration_test.go`
- `internal/fabricengine/pull_integration_test.go`
- `internal/fabricengine/ready_integration_test.go`
- `internal/fabricengine/reconcile_stale_registration_test.go`
- `internal/fabricengine/reconcile_stale_removal_test.go`
- `internal/fabricengine/refusalof_test.go`
- `internal/fabricengine/remove_guard_integration_test.go`
- `internal/fabricengine/remove_junctions_integration_test.go`
- `internal/fabricengine/snapshot_integration_test.go`
- `internal/fabricengine/status_pollution_integration_test.go`
- `internal/fabricengine/syncweft_integration_test.go`
- `internal/fabricengine/testmain_test.go`
- `internal/fabricengine/unwire_test.go`
- `internal/fabricengine/warpbinding_clone_integration_test.go`
- `internal/fabricengine/warpbinding_reconcile_integration_test.go`
- `internal/fabricengine/warpforward_integration_test.go`
- `internal/fabricengine/warplayout_test.go`
- `internal/fabricengine/weftgit_exclude_test.go`
- `internal/fabricengine/weftgit_pathspec_integration_test.go`
- `internal/fabricengine/weftgit_unborn_warp_test.go`
- `internal/fabricengine/worktreelist_test.go`
- `internal/gitexec/testmain_test.go`
- `internal/gitkit/bench_test.go`
- `internal/gitkit/callerset_enforcement_test.go`
- `internal/gitkit/doc.go`
- `internal/gitkit/gitkit.go`
- `internal/gitkit/gitkit_test.go`
- `internal/gitkit/hermetic.go`
- `internal/gitkit/leaf_enforcement_test.go`
- `internal/gitkit/reexecguard.go`
- `internal/gitkit/reexecguard_test.go`
- `internal/gitrepo/commitempty_integration_test.go`
- `internal/gitrepo/fetch_integration_test.go`
- `internal/gitrepo/gitrepo_test.go`
- `internal/gitrepo/gogit_test.go`
- `internal/gitrepo/keyvalidation_test.go`
- `internal/gitrepo/parity_test.go`
- `internal/gitrepo/push_test.go`
- `internal/gitrepo/testmain_test.go`
- `internal/gitrepo/worktree_test.go`
- `internal/hubforge/bench_test.go`
- `internal/hubforge/doc.go`
- `internal/hubforge/hub.go`
- `internal/hubforge/hub_test.go`
- `internal/hubforge/seed.go`
- `internal/hubforge/testmain_test.go`
- `internal/idecli/cli_test.go`
- `internal/idecli/testmain_test.go`
- `internal/ideengine/testmain_test.go`
- `internal/loomengine/export_test.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/loomengine/testmain_test.go`
- `internal/lyxcwd/anchor.go`
- `internal/lyxcwd/anchor_test.go`
- `internal/lyxcwd/enforcement_test.go`
- `internal/lyxcwd/gate_test.go`
- `internal/lyxcwd/lyxcwd.go`
- `internal/lyxcwd/lyxcwd_test.go`
- `internal/lyxcwd/testmain_test.go`
- `internal/lyxdirs/doc.go`
- `internal/perchcli/cli_integration_test.go`
- `internal/perchcli/cli_test.go`
- `internal/perchcli/run_integration_test.go`
- `internal/perchcli/run_test.go`
- `internal/perchcli/testmain_test.go`
- `internal/perchengine/testmain_test.go`
- `internal/reedcli/cli_integration_test.go`
- `internal/reedcli/smoke_attach_test.go`
- `internal/reedcli/smoke_debuglog_test.go`
- `internal/reedcli/smoke_lifecycle_test.go`
- `internal/reedcli/smoke_resume_test.go`
- `internal/reedcli/smoke_teardown_test.go`
- `internal/reedcli/smoke_test.go`
- `internal/reedcli/testmain_test.go`
- `internal/scoutengine/ensureserver_integration_test.go`
- `internal/scoutengine/refs_integration_test.go`
- `internal/scoutengine/toolchain_integration_test.go`
- `internal/shuttlecli/smoke_guardrail_test.go`
- `internal/shuttlecli/smoke_interrupt_test.go`
- `internal/shuttlecli/smoke_run_test.go`
- `internal/shuttlecli/testmain_test.go`
- `internal/shuttleengine/seam_enforcement_test.go`
- `internal/treadleengine/export_test.go`
- `internal/treadleengine/gate_lingering_test.go`
- `internal/treadleengine/smoke_judge_test.go`
- `internal/treadleengine/testmain_test.go`
- `internal/webstercli/testmain_test.go`
- `internal/webstercli/verbs_test.go`
- `internal/websterengine/config_test.go`
- `internal/websterengine/integration_test.go`
- `internal/websterengine/recoverbatch_test.go`
- `internal/websterengine/runlevel_test.go`
- `internal/websterengine/testmain_test.go`
- `internal/weftname/weftname.go`
- `internal/weftname/weftname_test.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/roadmap.md`
