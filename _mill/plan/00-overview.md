# Plan: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)

```yaml
task: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)
slug: dotlyx-scratch-hygiene
approved: false
started: 20260806-191405
parent: main
root: ""
verify: go build ./... && go vet -tags integration ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: lyxdirs-single-declarer
    file: 01-lyxdirs-single-declarer.md
    depends-on: []
    verify: go vet -tags integration ./... && go test ./...
  - number: 2
    name: treadle-perch-scratch-seam
    file: 02-treadle-perch-scratch-seam.md
    depends-on: [1]
    verify: go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... && go test -tags integration ./internal/perchcli/...
  - number: 3
    name: webster-builder-loom-scratch-seam
    file: 03-webster-builder-loom-scratch-seam.md
    depends-on: [1]
    verify: go test ./internal/websterengine/... ./internal/webstercli/... ./internal/builderengine/... ./internal/buildercli/... ./internal/loomengine/... ./cmd/lyx/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/... ./internal/buildercli/... ./internal/loomengine/...
  - number: 4
    name: dotlyx-group-reanchor-and-logger-sink
    file: 04-dotlyx-group-reanchor-and-logger-sink.md
    depends-on: [3]
    verify: go test ./internal/logger/... ./internal/shuttleengine/... ./internal/burlerengine/... ./internal/reedengine/... ./internal/scoutengine/... ./internal/scoutcli/... ./cmd/lyx/... && go test -tags integration ./internal/reedengine/...
  - number: 5
    name: no-transients-under-lyx-guard
    file: 05-no-transients-under-lyx-guard.md
    depends-on: [2, 4]
    verify: go test ./cmd/lyx/...
  - number: 6
    name: retire-cross-module-excludes
    file: 06-retire-cross-module-excludes.md
    depends-on: [5]
    verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 7
    name: structural-dirs-and-never-committed-routing
    file: 07-structural-dirs-and-never-committed-routing.md
    depends-on: [6]
    verify: go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/configsync/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
  - number: 8
    name: dotlyx-junction-wiring-and-unwire
    file: 08-dotlyx-junction-wiring-and-unwire.md
    depends-on: [7]
    verify: go test ./internal/fabricengine/... ./internal/fabriccli/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: lyxdirs-is-the-only-declarer

- **Decision:** after batch 1, the string literals `"_lyx"` and `".lyx"` exist in exactly one production file, `internal/lyxdirs/dirs.go`, as `LyxDirName` and `DotLyxDirName`.
  Every other production reference is `lyxdirs.LyxDirName` / `lyxdirs.DotLyxDirName`.
  `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals` polices both tokens with `internal/lyxdirs` as their sole owner from batch 1 onward, so any later batch that reintroduces a literal fails the build.
- **Rationale:** the two names are one structural pair (tracked vs never-tracked);
  splitting their ownership is what let five private `dotLyxDirName` copies drift.
  `internal/lyxdirs` is stdlib-only, so every package that needs either token — `configengine`, `logger`, `lyxtest`, `fabricengine`, the module engines — can import it with no cycle risk.
- **Applies to:** all batches

### Decision: told-never-derived-scratch-dir

- **Decision:** no engine derives its own `.lyx` path.
  Each module exposes a `ScratchDir(l *lyxcwd.Location)` accessor beside its existing durable accessor, and every engine-internal or CLI consumer is *handed* that path.
  Accessors naming only a transient keep their single-`dir` shape and simply receive the scratch dir instead of the durable one;
  accessors that must straddle both trees (`LoadState`/`SaveState`, treadle's `loadOrInitState`/`saveState`/`TerminalOutcome`) gain a second `scratchDir` parameter.
- **Rationale:** one directory argument cannot express a split where `state.json` stays under `_lyx` while `state.json.lock` moves to `.lyx`.
  Keeping the choice in the signature is what makes a caller that still passes the durable dir a compile error rather than a silently-broken pause flag.
- **Applies to:** treadle-perch-scratch-seam, webster-builder-loom-scratch-seam

### Decision: mirrored-subpath

- **Decision:** every relocated transient lands at the **same relative subpath** under `.lyx` that it had under `_lyx`.
  `_lyx/webster/state.json.lock` becomes `.lyx/webster/state.json.lock`;
  `_lyx/perch/<block>/run.lock` becomes `.lyx/perch/<block>/run.lock`;
  `_lyx/status.json.lock` becomes `.lyx/status.json.lock`.
  A module's scratch accessor is therefore its durable accessor with `lyxdirs.LyxDirName` swapped for `lyxdirs.DotLyxDirName`, nothing else.
- **Rationale:** mechanical and reviewable, and it keeps per-module ownership of relative subpaths intact per the Cwd Resolution Invariant.
- **Applies to:** all batches

### Decision: dotlyx-anchors-on-AnchorPath

- **Decision:** every worktree-level `.lyx` path is `l.AnchorPath()`-anchored, exactly like `_lyx`.
  The single exception is `reedengine.HubLogsDir`, which stays `l.HubPath`-anchored.
- **Rationale:** the junction fabric wires lands at `<worktree>/<AnchorRel>/.lyx`.
  Leaving any consumer on `l.WorktreePath()` gives a subpath-anchored repo two distinct `.lyx` roots — one junctioned and excluded, one not.
- **Applies to:** dotlyx-group-reanchor-and-logger-sink, webster-builder-loom-scratch-seam, treadle-perch-scratch-seam

### Decision: every-commit-leaves-the-tree-green

- **Decision:** batch order is transients-first, then exclusion-machinery removal, then geometry.
  Batch 7 declares `structuralNeverCommittedDirs` and uses it for pathspec construction, commit routing, and slug reservation, but deliberately does **not** fold it into `WiredNames`/`RepoWiredNames`;
  batch 8 does that in the same commit range as the `.lyx` content-adoption branch.
- **Rationale:** folding `.lyx` into the wired name-set before adoption exists would make the very next `lyx fabric reconcile` hard-error in every worktree that already holds a real `.lyx` — which is all of them.
  Fixture-based tests would not catch it, so the ordering is the guard.
- **Applies to:** structural-dirs-and-never-committed-routing, dotlyx-junction-wiring-and-unwire

### Decision: docs-land-with-their-change

- **Decision:** every doc that asserts behaviour this task changes is corrected in the batch that changes it, not in a trailing docs batch. `CONSTRAINTS.md` is edited in five different batches (1, 5, 6, 7, 8);
  each batch touches only its own clauses.
- **Rationale:** the repo's Documentation Lifecycle rule.
  The `CONSTRAINTS.md` overlap is why batches 5→6→7→8 form a strict chain rather than parallel siblings.
- **Applies to:** all batches

### Decision: go-verify-shape

- **Decision:** this is a Go repo, so no `PYTHONPATH=` prefix.
  Each batch's `verify:` runs `go test` over the packages it touches, and adds a `-tags integration` run whenever the batch edits an `//go:build integration` test file.
  The module-wide `verify:` in this overview's frontmatter is `go build ./... && go vet -tags integration ./...` — it type-checks every package plus every tagged test file, catching cross-package signature breakage at the batch that introduces it without paying for the whole tagged suite.
- **Rationale:** the seam changes in batches 2 and 3 change exported signatures consumed from other packages and from integration-tagged test files;
  a package-scoped `go test` alone would not compile those.
- **Applies to:** all batches

### Decision: repo-wide-done-gate-already-configured

- **Decision:** `mill-config.yaml`'s `pipeline.done_gate` is already `go test ./... && go test -tags integration ./...`;
  this plan does not change it.
  Per-batch `verify:` scopes stay narrow and rely on that gate for the whole-tree sweep before the task is marked done.
- **Rationale:** the batch scopes here are package-focused;
  the repo-wide regression net is the done gate's job.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/exitcode_test.go`
- `cmd/lyx/main_integration_test.go`
- `cmd/lyx/notransients_test.go`
- `docs/overview.md`
- `docs/reference/builder-contract.md`
- `docs/shared-libs/README.md`
- `internal/boardcli/cli_test.go`
- `internal/boardengine/boardtest/bench_test.go`
- `internal/boardengine/config_test.go`
- `internal/buildercli/cli.go`
- `internal/buildercli/pause.go`
- `internal/buildercli/pause_test.go`
- `internal/buildercli/poll.go`
- `internal/buildercli/poll_test.go`
- `internal/buildercli/run.go`
- `internal/buildercli/smoke_test.go`
- `internal/buildercli/spawnbatch.go`
- `internal/buildercli/spawnbatch_test.go`
- `internal/buildercli/status.go`
- `internal/buildercli/status_test.go`
- `internal/buildercli/weft.go`
- `internal/buildercli/weft_integration_test.go`
- `internal/builderengine/pause.go`
- `internal/builderengine/pause_test.go`
- `internal/builderengine/runlevel.go`
- `internal/builderengine/runlevel_test.go`
- `internal/builderengine/spawn.go`
- `internal/builderengine/spawn_test.go`
- `internal/builderengine/state.go`
- `internal/builderengine/state_test.go`
- `internal/burlerengine/doc.go`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/engine_test.go`
- `internal/configengine/config.go`
- `internal/configengine/config_test.go`
- `internal/configengine/edit_test.go`
- `internal/configengine/set_test.go`
- `internal/configsync/configsync_test.go`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/unwire.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/add_test.go`
- `internal/fabricengine/classify.go`
- `internal/fabricengine/classify_test.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/clone_test.go`
- `internal/fabricengine/commit.go`
- `internal/fabricengine/config_driven_junctions_integration_test.go`
- `internal/fabricengine/config_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/dotlyxjunction_integration_test.go`
- `internal/fabricengine/fabric.go`
- `internal/fabricengine/hostjunction_test.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/junction_pattern_integration_test.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/junctionnames_test.go`
- `internal/fabricengine/portals.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/status.go`
- `internal/fabricengine/structuraldirs_test.go`
- `internal/fabricengine/template.yaml`
- `internal/fabricengine/template_test.go`
- `internal/fabricengine/unwire.go`
- `internal/fabricengine/unwire_test.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabricengine/weftgit_exclude_test.go`
- `internal/fabricengine/weftwiring.go`
- `internal/ideengine/menu.go`
- `internal/ideengine/menu_test.go`
- `internal/logger/logger.go`
- `internal/logger/logsdir_test.go`
- `internal/logger/sink.go`
- `internal/logger/sink_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/discussionpath_test.go`
- `internal/loomengine/loomstatus_test.go`
- `internal/loomengine/planpath_test.go`
- `internal/loomengine/preflight.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/lyxcwd/enforcement_test.go`
- `internal/lyxdirs/dirs.go`
- `internal/lyxdirs/doc.go`
- `internal/lyxtest/doc.go`
- `internal/lyxtest/lyxtest.go`
- `internal/perchcli/cli.go`
- `internal/perchcli/cli_integration_test.go`
- `internal/perchcli/pause.go`
- `internal/perchcli/run.go`
- `internal/perchcli/run_integration_test.go`
- `internal/perchcli/run_test.go`
- `internal/perchengine/config_test.go`
- `internal/perchengine/engine.go`
- `internal/perchengine/identity.go`
- `internal/perchengine/identity_test.go`
- `internal/perchengine/run_test.go`
- `internal/planparser/parse.go`
- `internal/reedengine/config_test.go`
- `internal/reedengine/contract_integration_test.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/lock.go`
- `internal/reedengine/lock_test.go`
- `internal/reedengine/spawn.go`
- `internal/reedengine/spawn_test.go`
- `internal/reedengine/strand.go`
- `internal/reedengine/strand_test.go`
- `internal/scoutcli/cli.go`
- `internal/scoutengine/daemonstate.go`
- `internal/scoutengine/ensureserver.go`
- `internal/scoutengine/leaf_enforcement_test.go`
- `internal/scoutengine/refs.go`
- `internal/shuttleengine/config_test.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/run_test.go`
- `internal/shuttleengine/rundir.go`
- `internal/shuttleengine/rundir_test.go`
- `internal/treadleengine/engine.go`
- `internal/treadleengine/engine_test.go`
- `internal/treadleengine/run.go`
- `internal/treadleengine/state.go`
- `internal/treadleengine/state_test.go`
- `internal/webstercli/awaitbatch.go`
- `internal/webstercli/beginbatch.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/ownership.go`
- `internal/webstercli/pause.go`
- `internal/webstercli/recordbatch.go`
- `internal/webstercli/recoverbatch.go`
- `internal/webstercli/run.go`
- `internal/webstercli/status.go`
- `internal/webstercli/verbs_test.go`
- `internal/webstercli/weft.go`
- `internal/webstercli/weft_integration_test.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/beginbatch_test.go`
- `internal/websterengine/integration_test.go`
- `internal/websterengine/pause.go`
- `internal/websterengine/pause_test.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/runlevel_test.go`
- `internal/websterengine/state.go`
- `internal/websterengine/state_test.go`
- `internal/websterengine/webstergeom_test.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
