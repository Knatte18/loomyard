# `internal/paths`

The **canonical geometry resolver** — the single owner of all worktree and container
path math. Centralizes cwd/worktree-root handling so the `cwd ≠ git-repo-path` bug
class never recurs.

**Dependency direction (Go enforces it):** `internal/paths` imports only
`internal/git` + stdlib and **never** a domain module. All domain modules
(`worktree`, `board`, `ide`, `muxpoc`) import `paths` for geometry.

## The problem

The cwd-≠-worktree-root bug recurs because path math is scattered: each module
re-derives container, worktree-root, and relative-path ad hoc. A single resolver
makes correctness structural, not a matter of discipline.

## Exported API

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
3. Computes container = `parent(root)`, relative-path = `rel(root, cwd)`.
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
    Hub          string  // top-level container directory that is NOT a git repo; parent of WorktreeRoot
    RelPath      string  // relative path from WorktreeRoot to Cwd
    Prime        string  // path to the main/first worktree checkout (on main branch)
}
```

### Layout methods

- **`LyxDir() string`** — `filepath.Join(Cwd, "_lyx")`. The Loomyard config/state
  directory at the current location.
- **`WorktreePath(slug string) string`** — `filepath.Join(Hub, slug)`. Path to
  a sibling worktree.
- **`PortalsDir() string`** (un-mirrored root) — `filepath.Join(Hub, "_portals")`. The portals
  system container directory (prune boundary, not mirrored by subpath). **Deprecated — portals are superseded by the weft overlay model. Removal planned for task 006.**
- **`PortalLink(slug string) string`** (mirrored leaf) — `filepath.Join(Hub, "_portals", RelPath, slug)`. The portal junction link,
  mirrored into the repo subpath structure. At `RelPath == "."`, collapses to the
  flat `<Hub>/_portals/<slug>`. **Deprecated — portals are superseded by the weft overlay model. Removal planned for task 006.**
- **`PortalTarget(slug string) string`** — `filepath.Join(Hub, slug, RelPath,
  "_lyx")`. The junction target for a given worktree's portal. **Deprecated — portals are superseded by the weft overlay model. Removal planned for task 006.**
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
exist at cwd) remains enforced by `internal/config.FindBaseDir`. Board and other
modules keep passing `cwd` to their `LoadConfig` (obtained via `paths.Getwd`). This
lets `board init` (pre-init, no `_lyx/`) and other early-stage commands call into
`paths` without a spurious "not initialized" failure.

**Mirrored system dirs never enumerate the worktree.** `paths` only derives Loomyard's
own system directories (`_lyx`, `_portals`, `_launchers`) from `RelPath` and never
enumerates or mirrors user content. A nested or git-ignored `_codeguide` sibling
(or any other sibling repo) is never mirrored as a subpath-specific copy.

## The enforcement wall

**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned** outside
`internal/paths` and `cmd/lyx/main.go`. This ban is enforced at `go test` / CI
time by `internal/paths/enforcement_test.go`, which walks the entire source tree
and fails the build if either literal token is found in any non-test `.go` file
outside the allowlist.

See [CONSTRAINTS.md](../../CONSTRAINTS.md) for details and guidance for new code.
