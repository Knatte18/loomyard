# Plan: fabric: clone-does-everything + subpath-in-weft + init dissolution

```yaml
task: 'fabric: clone-does-everything + subpath-in-weft + init dissolution'
slug: fabric-clone-subpath
approved: true
started: 2026-08-01T07:21:03Z
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: hubgeometry-recorded-anchor
    file: 01-hubgeometry-recorded-anchor.md
    depends-on: []
    verify: go test -tags integration ./internal/hubgeometry/...
  - number: 2
    name: reconcile-declarative-convergence
    file: 02-reconcile-declarative-convergence.md
    depends-on: [1]
    verify: go test -tags integration ./internal/fabricengine/...
  - number: 3
    name: configsync-fabric-repowide
    file: 03-configsync-fabric-repowide.md
    depends-on: []
    verify: go test -tags integration ./internal/configsync/... ./internal/initengine/...
  - number: 4
    name: clone-does-everything
    file: 04-clone-does-everything.md
    depends-on: [1, 2, 3]
    verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
  - number: 5
    name: worktree-add-eager-wiring
    file: 05-worktree-add-eager-wiring.md
    depends-on: [1, 2]
    verify: go test -tags integration ./internal/fabricengine/...
  - number: 6
    name: init-dissolution-and-unwire
    file: 06-init-dissolution-and-unwire.md
    depends-on: [2, 4, 5]
    verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/... ./internal/loomengine/...
  - number: 7
    name: docs-and-sandbox-suites
    file: 07-docs-and-sandbox-suites.md
    depends-on: [6]
    verify: go test ./cmd/lyx/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: two repo-wide facts live on the `weft:main` branch

- **Decision:** The lyx-anchor subpath and the junction `pathspec` are per-repo facts stored on the `weft:main` branch, read at runtime from its checkout at `hubgeometry.BoardDir(Hub)`. Two files: (1) a plain single-line `.fabric-anchor` at the `weft:main` root (`<BoardDir>/.fabric-anchor`), holding only the subpath string (e.g. `backend` or `.`), read by `hubgeometry` with `os.ReadFile`+`TrimSpace` (YAML-free); (2) the repo-wide `fabric.yaml` (holding `pathspec`+`branch_prefix`) at `<BoardDir>/_lyx/config/fabric.yaml`, resolved through `hubgeometry.ConfigFile(BoardDir,"fabric")` and read via `configengine.Load`/`fabricengine.LoadConfig`.
- **Rationale:** `hubgeometry` imports only stdlib+`gitexec` and must stay YAML-free (hence the plain marker); `pathspec`/`branch_prefix` are fabric config and the Hub Geometry Invariant requires every `<module>.yaml` to resolve through `ConfigFile`/`ConfigDir`. `BoardDir(Hub)` is a fixed, `RelPath`-independent location every command reaches (`Hub = filepath.Dir(WorktreeRoot)`), breaking the circularity of storing the subpath under `RelPath`.
- **Applies to:** all batches

### Decision: record wins, cwd is a hard at-or-below gate, marker-absent falls back to cwd

- **Decision:** `hubgeometry.Resolve` reads `.fabric-anchor` and sets `RelPath` from it (record is truth), then validates cwd is at or below `<WorktreeRoot>/<anchor>`; cwd outside the anchored subtree is a **hard error**. Marker absent → today's cwd-derived `RelPath`. `SiblingLayout` reads the same anchor but WITHOUT the cwd gate (it derives another worktree's geometry from its root, above a subpath anchor).
- **Rationale:** For a root-anchored repo (`.`) the gate never fires. For a subpath-anchored repo, running lyx outside the anchored subtree is genuinely wrong and must fail loudly. The absent-marker fallback covers mid-clone, lyxtest synthetic hubs, and non-fabric git repos only.
- **Applies to:** hubgeometry-recorded-anchor, reconcile-declarative-convergence

### Decision: Go project — native test runner, no PYTHONPATH prefix

- **Decision:** This is a Go codebase. `verify:` commands use `go test` directly (no `PYTHONPATH= ` prefix, which is Python/mill-only). Batches whose tests include `//go:build integration` files run `go test -tags integration <pkgs>` (a superset that runs both tagged and untagged files); pure-unit batches omit the tag.
- **Rationale:** The `verify-not-isolated` validator check is conditional on project language; Go batches use the native runner.
- **Applies to:** all batches

### Decision: weft:main writes route through the fabricengine commit choke point

- **Decision:** The two new repo-wide files are committed onto `weft:main` by clone through `fabricengine.CommitWeftAt`/`PushWeftAt` (the sanctioned `weft:main` writer that `boardengine`'s `Sync` uses), never raw git. Their commit pathspec is exclusion-safe: neither `.fabric-anchor` nor `_lyx/config/fabric.yaml` is matched by the `**/_lyx/*/{*.lock,pause,prompts/}` exclude globs.
- **Rationale:** Weft Git Invariant — every git op on the weft repo goes through `internal/fabricengine`. `CommitWeftAt` wildcard-stages the board worktree (`git add -A` via `StageAllAndCommit`), which is correct for these root/config files.
- **Applies to:** clone-does-everything

### Decision: declarative junction convergence, fail-closed on a broken pathspec

- **Decision:** Junction wiring is declarative convergence to the repo-wide `pathspec`: add junctions missing on disk, remove on-disk junctions absent from `pathspec`, no-op correct ones. Stale-removal enumerates by scanning on-disk link entries under the host worktree root (via `fslink.IsLink`) and excludes `hubgeometry.HubReservedNames()`. A missing/unparseable repo-wide `fabric.yaml` **aborts stale-removal, touches nothing** (never a blanket sweep).
- **Rationale:** A single declarative "make wiring match config" self-heals drift both directions and is the entry point for setup and later per-repo module activation. An empty/errored `pathspec` has no safe destructive degraded meaning across every worktree.
- **Applies to:** reconcile-declarative-convergence, clone-does-everything, worktree-add-eager-wiring, init-dissolution-and-unwire

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/jsonhelp_test.go`
- `cmd/lyx/main.go`
- `docs/overview.md`
- `internal/boardengine/config.go`
- `internal/builderengine/config.go`
- `internal/configsync/configsync.go`
- `internal/configsync/configsync_test.go`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/unwire.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/add_branch_exists_test.go`
- `internal/fabricengine/add_rollback_adopt_test.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/checkout_index_refresh_test.go`
- `internal/fabricengine/checkout_rollback_test.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/clone_adopt_test.go`
- `internal/fabricengine/config.go`
- `internal/fabricengine/config_test.go`
- `internal/fabricengine/config_driven_junctions_integration_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/drift.go`
- `internal/fabricengine/hostlayout.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/junction_pattern_integration_test.go`
- `internal/fabricengine/junction_repoint_test.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/reconcile_stale_registration_test.go`
- `internal/fabricengine/reconcile_stale_removal_test.go`
- `internal/fabricengine/remove.go`
- `internal/fabricengine/remove_junctions_integration_test.go`
- `internal/fabricengine/status.go`
- `internal/fabricengine/unwire.go`
- `internal/fabricengine/unwire_test.go`
- `internal/fabricengine/weftgit.go`
- `internal/hubgeometry/anchor.go`
- `internal/hubgeometry/anchor_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/siblinglayout_test.go`
- `internal/initengine/init_test.go`
- `internal/initengine/undo_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `internal/perchengine/config.go`
- `internal/reedengine/config.go`
- `internal/shuttleengine/config.go`
- `internal/websterengine/config.go`
- `manifest/designs/fabric-unified-view.md`
- `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
- `tools/sandbox/SANDBOX-BURLER-SUITE.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- `tools/sandbox/SANDBOX-PERCH-SUITE.md`
- `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
