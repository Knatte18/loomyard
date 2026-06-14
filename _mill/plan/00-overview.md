# Plan: Extend worktree module: portals and launchers

```yaml
task: 'Extend worktree module: portals and launchers'
slug: 'mhgo-portals-launchers'
approved: false
started: '20260614-100835'
parent: 'main'
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
    verify: go test ./internal/paths/... ./internal/git/...
  - number: 2
    name: gitignore-lib
    file: 02-gitignore-lib.md
    depends-on: []
    verify: go test ./internal/gitignore/...
  - number: 3
    name: worktree-portals-launchers
    file: 03-worktree-portals-launchers.md
    depends-on: [1]
    verify: go test ./internal/worktree/...
  - number: 4
    name: board-paths-healthcheck
    file: 04-board-paths-healthcheck.md
    depends-on: [1, 2]
    verify: go test ./internal/board/...
  - number: 5
    name: muxpoc-paths-migration
    file: 05-muxpoc-paths-migration.md
    depends-on: [1]
    verify: go test ./internal/muxpoc/...
  - number: 6
    name: ide-module
    file: 06-ide-module.md
    depends-on: [1, 2, 4]
    verify: go test ./internal/ide/... ./cmd/mhgo/...
  - number: 7
    name: enforcement-and-docs
    file: 07-enforcement-and-docs.md
    depends-on: [1, 2, 3, 4, 5, 6]
    verify: go test ./...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: paths is the sole geometry owner

- **Decision:** `internal/paths` is a new geometry-only package that owns ALL
  worktree/container path math and is the ONLY package (besides
  `cmd/mhgo/main.go`) permitted to call `os.Getwd` or run
  `git rev-parse --show-toplevel`. It imports only `internal/git` + stdlib and
  never a domain module. `Resolve(cwd)` builds a `Layout` once; geometry methods
  derive everything else. `Layout` fields: `Cwd`, `WorktreeRoot`
  (`rev-parse --show-toplevel`, normalized), `Container` (`filepath.Dir(WorktreeRoot)`),
  `RelPath` (`filepath.Rel(WorktreeRoot, Cwd)`), `MainWorktree` (the `Main=true`
  entry from the relocated porcelain parser). Methods: `MhgoDir()`,
  `WorktreePath(slug)`, `PortalsDir()`, `PortalTarget(slug)` (=
  `<Container>/<slug>/<RelPath>/_mhgo`), `LaunchersDir()`, `LauncherDir(slug)`,
  `HubName()` (= `filepath.Base(MainWorktree)`). A thin `Getwd() (string, error)`
  wraps `os.Getwd` for callers (e.g. `init`) that need a cwd before a git repo
  exists. Typed error `ErrNotAGitRepo`. Normalization reconciles forward-slash
  `--show-toplevel` output vs backslash `os.Getwd` via `filepath.FromSlash` +
  `filepath.Clean`.
- **Rationale:** the cwd-≠-worktree-root bug recurs because no package owns path
  math; centralizing it makes correctness structural. The operator confirmed the
  migration is repo-wide (board, worktree, git, muxpoc all route through `paths`).
- **Applies to:** all batches.

### Decision: config _mhgo-at-cwd authority stays in internal/config

- **Decision:** `paths.Resolve` is geometry-ONLY and does NOT check for `_mhgo/`.
  The cwd-authoritative config invariant (`_mhgo/` must exist at cwd) remains
  enforced by `internal/config.FindBaseDir` exactly as today; board and worktree
  keep passing `cwd` (now obtained via `paths.Getwd`) to their `LoadConfig`. This
  lets `board init` (pre-init, no `_mhgo/`) and `muxpoc` (no `_mhgo/` config) call
  into `paths` without a spurious "not initialized" failure. Only the *source of
  cwd* changes (raw `os.Getwd` → `paths.Getwd`); config-resolution semantics are
  untouched.
- **Rationale:** reconciles "board unchanged / cwd-authoritative" with "no raw
  `os.Getwd` outside paths". `paths` owns geometry; `config` owns init-state.
- **Applies to:** paths-foundation, worktree-portals-launchers, board-paths-healthcheck, muxpoc-paths-migration.

### Decision: enforcement test lands last and bans two literal tokens

- **Decision:** `internal/paths/enforcement_test.go` walks the repo source tree
  and fails the build if the literal token `os.Getwd` OR `--show-toplevel` appears
  in any **non-`_test.go`** `.go` file outside the allowlist `{internal/paths,
  cmd/mhgo/main.go}`. It scans only those two primitives (never `filepath.Dir`,
  which is used legitimately). `_test.go` files are skipped (so `muxpoc_smoke_test.go`'s
  `os.Getwd` and the bench comment do not trip it). The test also asserts the
  scanner trips on synthetic in-memory snippets containing each banned token. It
  lands in the FINAL batch, green, only after every site is migrated.
- **Rationale:** a doc alone is forgettable; the test is the wall that catches
  every future author at `go test`/CI time. Banning bare `rev-parse` would false-
  positive on `rev-parse --verify` / `--absolute-git-dir` / `HEAD`; the precise
  token is `--show-toplevel`.
- **Applies to:** enforcement-and-docs (authors it); all migration batches (must leave the tree clean for it).

### Decision: ported color palette, VS Code config, junction, and launcher content

- **Decision:** Bake these literal values (ported from mill, do not import):
  - **Color palette** (order): `#2d7d46` green, `#7d2d6b` purple, `#2d4f7d` blue,
    `#7d5c2d` yellow, `#6b2d2d` red, `#2d6b6b` cyan, `#4a2d7d` indigo, `#7d462d`
    orange. Green (`#2d7d46`) is reserved for the main worktree; child worktrees
    get the first **unused non-green** color (scan sibling
    `<dir>/<relpath>/.vscode/settings.json` →
    `workbench.colorCustomizations.titleBar.activeBackground`), wrapping to the
    first non-green if all are in use.
  - **settings.json keys:** `workbench.colorCustomizations` =
    `{titleBar.activeBackground: <color>, titleBar.activeForeground: "#ffffff",
    titleBar.inactiveBackground: <color>, titleBar.inactiveForeground:
    "#ffffffaa"}`; `window.title` = `"<slug>"` (or `"<short>: <slug>"` when a
    short form is available); `workbench.startupEditor: "none"`;
    `workbench.secondarySideBar.defaultVisibility: "hidden"` (verify the exact
    panel-hiding key against the installed VS Code during implementation).
  - **tasks.json:** one `Start Claude` shell task with
    `runOptions.runOn: "folderOpen"` running `claude` in a dedicated integrated
    terminal.
  - **Junction:** Windows = `cmd /c mklink /J <backslash-link> <backslash-target>`
    (normalize both ends to backslash), refuse to clobber an existing link path,
    mkdir the parent first. POSIX = `os.Symlink(target, link)`. Removal never
    recurses into the target (`os.Remove`) and is idempotent.
  - **code launch:** Windows = `cmd /c code <worktree>` (PATH resolution); POSIX =
    error "unsupported on this platform".
  - **Launcher content** (Windows-only; no-op on POSIX): `ide.cmd` =
    `@cd /d "%~dp0..\..\<slug>\<relpath-backslash>" && mhgo ide spawn <slug>`
    (omit the trailing `\<relpath>` when RelPath is empty); `ide-menu.cmd` =
    `@cd /d "%~dp0..\<hubname>\<relpath-backslash>" && mhgo ide menu`.
- **Rationale:** the operator is used to mill's exact palette/conventions; baking
  literals keeps each card cold-start-implementable without reading external
  Python.
- **Applies to:** worktree-portals-launchers (junction + launchers), ide-module (palette + VS Code + launch).

### Decision: Go conventions — per-file unit tests, build-tag platform split, JSON I/O

- **Decision:** Per-file unit tests next to source (`foo.go` ↔ `foo_test.go`),
  table-driven where natural. Platform specifics use the established
  `_windows.go` / `_other.go` build-tag split (mirroring `git_windows.go` /
  `git_other.go`). New CLI surface prints `{"ok":true,...}` /
  `{"ok":false,"error":...}` via `internal/output`; `ide menu` interactive picker
  is the one documented exception. Symlink/junction tests `t.Skip` when the
  platform forbids them (existing `links_test.go` pattern). `verify:` uses the
  native `go test` runner scoped to the batch's packages (no PYTHONPATH prefix —
  this is a Go project).
- **Rationale:** matches `docs/overview.md` principles and the existing codebase.
- **Applies to:** all batches.

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/mhgo/main.go`
- `cmd/mhgo/main_test.go`
- `docs/modules/ide.md`
- `docs/modules/worktree.md`
- `docs/overview.md`
- `docs/roadmap.md`
- `docs/shared-libs/README.md`
- `docs/shared-libs/gitignore.md`
- `docs/shared-libs/paths.md`
- `internal/board/board.go`
- `internal/board/board_test.go`
- `internal/board/cli.go`
- `internal/board/init.go`
- `internal/git/git.go`
- `internal/git/git_test.go`
- `internal/gitignore/gitignore.go`
- `internal/gitignore/gitignore_test.go`
- `internal/ide/cli.go`
- `internal/ide/cli_test.go`
- `internal/ide/color.go`
- `internal/ide/color_test.go`
- `internal/ide/launch_other.go`
- `internal/ide/launch_windows.go`
- `internal/ide/menu.go`
- `internal/ide/menu_test.go`
- `internal/ide/spawn.go`
- `internal/ide/spawn_test.go`
- `internal/ide/vscode.go`
- `internal/ide/vscode_test.go`
- `internal/muxpoc/attach.go`
- `internal/muxpoc/cli.go`
- `internal/muxpoc/cmd.go`
- `internal/muxpoc/daemon.go`
- `internal/muxpoc/down.go`
- `internal/muxpoc/muxpoc_smoke_test.go`
- `internal/muxpoc/review.go`
- `internal/muxpoc/state_test.go`
- `internal/muxpoc/status.go`
- `internal/muxpoc/up.go`
- `internal/paths/enforcement_test.go`
- `internal/paths/paths.go`
- `internal/paths/paths_test.go`
- `internal/paths/worktreelist.go`
- `internal/paths/worktreelist_test.go`
- `internal/worktree/add.go`
- `internal/worktree/add_test.go`
- `internal/worktree/cli.go`
- `internal/worktree/junction_other.go`
- `internal/worktree/junction_test.go`
- `internal/worktree/junction_windows.go`
- `internal/worktree/launchers.go`
- `internal/worktree/launchers_test.go`
- `internal/worktree/list.go`
- `internal/worktree/list_test.go`
- `internal/worktree/portals.go`
- `internal/worktree/portals_test.go`
- `internal/worktree/remove.go`
- `internal/worktree/remove_test.go`
