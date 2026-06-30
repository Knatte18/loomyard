# `internal/paths`

The **canonical geometry resolver** — the single owner of all worktree and Hub
path math. Centralizes cwd/worktree-root handling so the `cwd ≠ git-repo-path` bug
class never recurs.

**Dependency direction (Go enforces it):** `internal/paths` imports only
`internal/gitexec` + stdlib and **never** a domain module. All domain modules
(`warp`, `board`, `ide`, `muxpoc`) import `paths` for geometry.

## The problem

The cwd-≠-worktree-root bug recurs because path math is scattered: each module
re-derives Hub, worktree-root, and relative-path ad hoc. A single resolver
makes correctness structural, not a matter of discipline.

## Exported API

### Constants

The following constants centralize every geometry and layout literal so no other package needs to repeat a string value. All production code that constructs paths from these names must import `internal/paths` and use these constants — never inline string literals.

#### Layout constants

- **`LyxDirName`** (`"_lyx"`) — directory name for the lyx system directory within a worktree. Used by `LyxDir()`, `HostLyxLink`, `WeftLyxDir`, etc.

#### Geometry vocabulary constants

These three constants are the single source of the geometry tokens for the whole repo. They are the only place these string values are permitted to appear in `filepath.Join` args, `+` operands, or `const` declarations (the geometry-literal ban in `TestEnforcement_GeometryLiterals` enforces this allowlist).

- **`WeftSuffix`** (`"-weft"`) — suffix appended to a host-worktree slug to form its weft sibling directory name (e.g. `"feat"` → `"feat-weft"`). Use `WeftSiblingPath` / `WeftRepoRoot` / `WeftWorktreePath` rather than constructing the path from this constant directly.
- **`BoardDirName`** (`"_board"`) — name of the board data directory inside the hub (i.e. `<hub>/_board`). Use `BoardDir(hub)` to obtain the full path.
- **`HubSuffix`** (`"-HUB"`) — suffix appended to a repo name to form its hub container directory (e.g. `"loomyard"` → `"loomyard-HUB"`). Use `HubPath(parent, name)` to obtain the full path.

### Functions

### `Getwd() (string, error)`

Returns the current working directory.

**Behavior:** A thin wrapper over `os.Getwd`; the only permitted `os.Getwd` call
outside `internal/paths` (and `cmd/lyx/main.go`).

**Returns:** On success, the cleaned absolute path of the cwd. On failure, an error
(e.g., the cwd no longer exists).

**Use case:** Pre-initialization (before a git repo is accessible), when you need a
cwd but no git root yet.

### `Resolve(cwd string) (*Layout, error)`

Builds a complete geometry `Layout` from a cwd once, resolving all path math upfront.

**Behavior:**

1. Runs `git rev-parse --show-toplevel` from `cwd` to find the repo root.
2. Normalizes the root via `filepath.FromSlash` + `filepath.Clean` (reconciles forward
   slashes from git vs backslashes from `os.Getwd()`).
3. Computes Hub = `parent(root)`, relative-path = `rel(root, cwd)`.
4. Calls `worktreeList(cwd)` to fetch the `Main=true` entry and stores its `.Path`.
5. Returns a `Layout` struct with all fields set.

**Returns:** On success, a pointer to the `Layout`. On failure, an error (typically
`ErrNotAGitRepo` if `cwd` is not in a git repository).

**Error types:**
- `ErrNotAGitRepo` — the given cwd is not within a git repository, or `git
  rev-parse --show-toplevel` failed.

### `Layout` struct

```go
type Layout struct {
    Cwd          string  // current working directory (filepath.Clean'd)
    WorktreeRoot string  // git repository root (normalized via filepath.FromSlash + filepath.Clean)
    Hub          string  // top-level Hub directory (non-git); parent of WorktreeRoot
    RelPath      string  // relative path from WorktreeRoot to Cwd
    Prime        string  // path to the Prime worktree checkout (main branch)
}
```

### Config path helpers

These functions resolve configuration file paths. They take a `baseDir` (the directory containing `_lyx/`) as a parameter, not a `Layout`.

- **`ConfigDir(baseDir string) string`** — Returns `filepath.Join(baseDir, LyxDirName, "config")`. The directory where module configuration YAML files are stored.
- **`ConfigFile(baseDir, module string) string`** — Returns `filepath.Join(ConfigDir(baseDir), module+".yaml")`. The path to a specific module's configuration file (e.g., `_lyx/config/board.yaml`).
- **`DotEnv(baseDir string) string`** — Returns `filepath.Join(baseDir, ".env")`. The path to the environment variable overrides file.

### Geometry bootstrap functions

These pure functions construct geometry paths without requiring a resolved `Layout`. They are the correct way for early-stage callers (pre-init, pre-layout, bootstrap code) to form geometry paths. They consume the geometry constants above — no caller needs to repeat the raw suffix strings.

- **`WeftSiblingPath(hub, slug string) string`** — Returns `filepath.Join(hub, slug+WeftSuffix)`. The canonical `<hub>/<slug>-weft` weft sibling path. Used by `WeftRepoRoot()`, `WeftWorktreePath()`, and `WeftWorktree()`.
- **`BoardDir(hub string) string`** — Returns `filepath.Join(hub, BoardDirName)`. The canonical `<hub>/_board` board data path. Used by the board engine for path resolution when no `Layout` is available.
- **`HubPath(parent, name string) string`** — Returns `filepath.Join(parent, name+HubSuffix)`. The canonical `<parent>/<name>-HUB` hub container path.

### Reverse parser

- **`WeftHostSlug(name string) (slug string, ok bool)`** — Reports whether `name` ends with `WeftSuffix` and the stripped prefix (the host slug) is non-empty. When `ok` is true, `slug` is the result of `strings.TrimSuffix(name, WeftSuffix)` and may be passed directly to the geometry constructors. The non-empty guard rejects a bare `"-weft"` entry. Used by `warpengine/prune.go` to identify weft siblings in a hub scan.

### Layout methods

- **`LyxDir() string`** — `filepath.Join(Cwd, LyxDirName)`. The Loomyard config/state
  directory at the current location.
- **`WorktreePath(slug string) string`** — `filepath.Join(Hub, slug)`. Path to
  a sibling worktree.
- **`PortalsDir() string`** (un-mirrored root) — `filepath.Join(Hub, "_portals")`. The portals
  system container directory (prune boundary, not mirrored by subpath). Portals expose each
  worktree's `_lyx/` at a Hub-level, subdir-mirrored location so any worktree can see sibling
  task-state in the same subdir; present and working, kept on hold (see the weft proposal).
- **`PortalLink(slug string) string`** (mirrored leaf) — `filepath.Join(Hub, "_portals", RelPath, slug)`. The portal junction link,
  mirrored into the repo subpath structure. At `RelPath == "."`, collapses to the
  flat `<Hub>/_portals/<slug>`.
- **`PortalTarget(slug string) string`** — `filepath.Join(Hub, slug, RelPath,
  "_lyx")`. The junction target for a given worktree's portal.
- **`LaunchersDir() string`** (un-mirrored root) — `filepath.Join(Hub, "_launchers")`. The launchers
  system container directory (prune boundary, not mirrored by subpath).
- **`LauncherDir(slug string) string`** (mirrored leaf) — `filepath.Join(Hub, "_launchers", RelPath, slug)`.
  Path to a specific worktree's launcher directory, mirrored into the repo subpath
  structure. At `RelPath == "."`, collapses to the flat `<Hub>/_launchers/<slug>`.
- **`MenuLauncherPath() string`** (mirrored leaf) — `filepath.Join(Hub, "_launchers", RelPath, "ide-menu.cmd")`. The per-subpath
  menu launcher script, mirrored into the repo subpath structure. At `RelPath == "."`,
  collapses to `<Hub>/_launchers/ide-menu.cmd`.
- **`LauncherSpawnRel(slug string) string`** — `filepath.Rel(LauncherDir(slug), filepath.Join(WorktreePath(slug), RelPath))`.
  The relative path from a launcher directory to the target worktree's subpath for spawning.
- **`MenuLauncherRel() string`** — `filepath.Rel(filepath.Dir(MenuLauncherPath()), filepath.Join(Prime, RelPath))`.
  The relative path from the menu launcher directory to the Prime worktree's subpath for menu spawning.
- **`PrimeName() string`** — `filepath.Base(Prime)`. The Prime worktree's
  directory name (stable, used in paths like `ide-menu.cmd`).

## Design principles

**Geometry-only.** `paths` computes *where* things are, never *mutates* them.
Worktree creation/removal, junction setup, and config scaffolding stay in the
domain modules. `paths` is the dumb geometry resolver so they can be smart about
state transitions.

**Single call per invocation.** Most callsites invoke `Resolve(cwd)` once at the
start of a command and re-use the returned `Layout` throughout. This amortizes all
git calls and normalization upfront.

**Normalization in one place.** Forward slashes from `git rev-parse --show-toplevel`
vs backslashes from `os.Getwd()` are reconciled once in `paths` via
`filepath.FromSlash` + `filepath.Clean`, so callers never deal with mixed forms.

**Config resolution stays cwd-authoritative.** `paths.Resolve` is geometry-only and
does NOT check for `_lyx/`. The cwd-authoritative config invariant (`_lyx/` must
exist at cwd) remains enforced by `internal/configengine.FindBaseDir`. Board and other
modules keep passing `cwd` to their `LoadConfig` (obtained via `paths.Getwd`). This
lets `board init` (pre-init, no `_lyx/`) and other early-stage commands call into
`paths` without a spurious "not initialized" failure.

**Mirrored system dirs never enumerate the worktree.** `paths` only derives Loomyard's
own system directories (`_lyx`, `_portals`, `_launchers`) from `RelPath` and never
enumerates or mirrors user content. A nested or git-ignored `_codeguide` sibling
(or any other sibling repo) is never mirrored as a subpath-specific copy.

## The enforcement wall

`internal/paths/enforcement_test.go` runs two repo-wide AST scans on every
`go test ./internal/paths/...` run:

**`TestEnforcement` (cwd/root primitives ban):**
Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside
`internal/paths` and `cmd/lyx/main.go`. The scan uses a substring check on the
raw file bytes and fails the build if either token appears in any non-test `.go`
file outside the allowlist.

**`TestEnforcement_GeometryLiterals` (geometry-literal construction ban):**
The geometry path tokens `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`,
`_codeguide`, and `_lyx` may not appear as string literals in a
**path-construction context** in any production file outside `internal/paths`.
Path-construction contexts are:

- An argument to a `filepath.Join(...)` call.
- An operand of a binary `+` (`token.ADD`) expression.
- The value of a string `const` declaration.

Matching is **whole-token** (exact equality after `strconv.Unquote`, not
substring), so compound names like `_boardroom` or `-weft-bare` are not flagged.
Test files (`*_test.go`) are excluded from the scan — test geometry is a
code-review obligation, not machine-enforced. A `scanned_non_empty` sub-test
guards against a misconfigured walk that would silently produce a vacuous pass.

See [CONSTRAINTS.md](../../CONSTRAINTS.md) for the full invariant specification
and guidance for new code.
