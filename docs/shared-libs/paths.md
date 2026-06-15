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
outside `internal/paths` (and `cmd/mhgo/main.go`).

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
    Cwd           string  // current working directory (filepath.Clean'd)
    WorktreeRoot  string  // git repository root (normalized via filepath.FromSlash + filepath.Clean)
    Container     string  // parent of WorktreeRoot
    RelPath       string  // relative path from WorktreeRoot to Cwd
    MainWorktree  string  // path to the main (first) worktree from List()
}
```

### Layout methods

- **`MhgoDir() string`** — `filepath.Join(Cwd, "_mhgo")`. The mhgo config/state
  directory at the current location.
- **`WorktreePath(slug string) string`** — `filepath.Join(Container, slug)`. Path to
  a sibling worktree.
- **`PortalsDir() string`** — `filepath.Join(Container, "_portals")`. Path to the
  portals system directory.
- **`PortalTarget(slug string) string`** — `filepath.Join(Container, slug, RelPath,
  "_mhgo")`. The junction target for a given worktree's portal.
- **`LaunchersDir() string`** — `filepath.Join(Container, "_launchers")`. Path to
  the launchers system directory.
- **`LauncherDir(slug string) string`** — `filepath.Join(LaunchersDir(), slug)`.
  Path to a specific worktree's launcher directory.
- **`HubName() string`** — `filepath.Base(MainWorktree)`. The main worktree's
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
does NOT check for `_mhgo/`. The cwd-authoritative config invariant (`_mhgo/` must
exist at cwd) remains enforced by `internal/config.FindBaseDir`. Board and other
modules keep passing `cwd` to their `LoadConfig` (obtained via `paths.Getwd`). This
lets `board init` (pre-init, no `_mhgo/`) and other early-stage commands call into
`paths` without a spurious "not initialized" failure.

## The enforcement wall

**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned** outside
`internal/paths` and `cmd/mhgo/main.go`. This ban is enforced at `go test` / CI
time by `internal/paths/enforcement_test.go`, which walks the entire source tree
and fails the build if either literal token is found in any non-test `.go` file
outside the allowlist.

See [CONSTRAINTS.md](../../CONSTRAINTS.md) for details and guidance for new code.
