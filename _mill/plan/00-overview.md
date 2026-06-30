# Plan: Harden the Path Invariant: close enforcement hole + fix geometry leaks

```yaml
task: 'Harden the Path Invariant: close enforcement hole + fix geometry leaks'
slug: harden-path-invariant
approved: true
started: 20260630-045749
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: paths-foundation
    file: 01-paths-foundation.md
    depends-on: []
    verify: go test ./internal/paths/...
  - number: 2
    name: warp-geometry
    file: 02-warp-geometry.md
    depends-on: [1]
    verify: go test ./internal/warpengine/... ./internal/warpcli/...
  - number: 3
    name: board-paths-owned
    file: 03-board-paths-owned.md
    depends-on: [1]
    verify: go test ./internal/boardengine/... ./internal/boardcli/... ./internal/configsync/...
  - number: 4
    name: lyxtest-geometry
    file: 04-lyxtest-geometry.md
    depends-on: [1]
    verify: go test ./internal/lyxtest/...
  - number: 5
    name: enforcement-and-docs
    file: 05-enforcement-and-docs.md
    depends-on: [2, 3, 4]
    verify: go build ./... && go test ./...
```

## Shared Decisions

### Decision: paths owns the geometry vocabulary

- **Decision:** `internal/paths` is the single source of the geometry literals
  (`-weft`, `_board`, `-HUB`) via exported consts `WeftSuffix`, `BoardDirName`, `HubSuffix`;
  pure bootstrap funcs `WeftSiblingPath(hub, slug)`, `BoardDir(hub)`, `HubPath(parent, name)`;
  and the reverse parser `WeftHostSlug(name) (string, bool)`. No other package may construct
  these paths from string literals.
- **Rationale:** Batches 2–4 route every geometry construction site through this API so the
  tightened enforcement test (batch 5) passes with an allowlist of `internal/paths` only.
- **Applies to:** all batches.

### Decision: behaviour parity is mandatory

- **Decision:** Every warp/lyxtest conversion must produce byte-identical paths to the literal
  it replaces (the pure funcs are the same `filepath.Join`s). Existing package tests staying
  green is the parity proof.
- **Rationale:** This is a hardening task; no observable behaviour may change except the board
  data-dir resolution source (config → paths) and its latent sub-path-bug fix.
- **Applies to:** paths-foundation, warp-geometry, board-paths-owned, lyxtest-geometry.

### Decision: enforcement test tightening lands last

- **Decision:** The AST geometry-literal ban is added to `enforcement_test.go` only in batch 5,
  after every conversion batch (2, 3, 4) has merged. The ban fails the build the moment any
  unconverted site exists, so it cannot precede the conversions.
- **Rationale:** Ordering the DAG so batch 5 depends on [2, 3, 4] guarantees the tree is fully
  converted before the ban activates; no intermediate red state.
- **Applies to:** warp-geometry, board-paths-owned, lyxtest-geometry, enforcement-and-docs.

### Decision: whole-token match, production-only scan

- **Decision:** The enforcement detector matches a geometry token only when a string literal's
  full value **equals** a token (not substring), only in path-construction context
  (`filepath.Join` arg, `+` operand, or string-const declaration), and only in non-`*_test.go`
  files. Token set: `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_codeguide`, `_lyx`.
- **Rationale:** Substring matching false-positives on compound fixture names (`-weft-bare`) and
  prose; scanning test files would flag legitimate fixtures. Allowlist = `internal/paths` only.
- **Applies to:** enforcement-and-docs (defines it); all conversion batches (must satisfy it).

### Decision: geometry is paths-owned and not config/env-overridable

- **Decision:** The board data dir (`<hub>/_board`) is geometry: removed from the config
  template, resolved as `--board-path` flag (transient) > `paths.BoardDir(l.Hub)`. Non-geometry
  config keys (`home`, `sidebar`, `proposal_prefix`) keep their `${env:NAME:-default}` form
  untouched — only the `path:` key is removed.
- **Rationale:** Distinguishes geometry (never config/env-overridable) from non-geometry
  filenames (optional overrides are the desired design). No env-removal initiative here.
- **Applies to:** board-paths-owned.

## All Files Touched

- `CONSTRAINTS.md`
- `docs/shared-libs/configengine.md`
- `docs/shared-libs/paths.md`
- `docs/shared-libs/yamlengine.md`
- `internal/boardcli/cli.go`
- `internal/boardcli/cli_test.go`
- `internal/boardengine/config.go`
- `internal/boardengine/config_test.go`
- `internal/boardengine/template.yaml`
- `internal/boardengine/template_test.go`
- `internal/configsync/configsync_test.go`
- `internal/lyxtest/lyxtest.go`
- `internal/paths/enforcement_test.go`
- `internal/paths/geometry_test.go`
- `internal/paths/paths.go`
- `internal/warpcli/clone.go`
- `internal/warpengine/clone.go`
- `internal/warpengine/clone_integration_test.go`
- `internal/warpengine/prune.go`
- `internal/warpengine/reconcile.go`
- `internal/warpengine/status.go`
