# `internal/lyxcwd`

The **entry gate** that converts "the process started somewhere" into "these are the coordinates of a legal lyx worktree, or here is why this is not one". It is deliberately no longer a geometry owner — it does not construct any path from a structural token — it resolves the active `Location` from a working directory and exposes the handful of typed accessors every caller derives from that `Location`.

**Dependency direction (Go enforces it):** `internal/lyxcwd`'s own imports are capped at stdlib plus `internal/gitexec` — nothing else, ever. This ceiling is what keeps `fabricengine` → `logger` → `lyxcwd` acyclic: any wider import set would risk a cycle back through one of the packages `lyxcwd` itself sits below.

## The problem

The cwd-≠-worktree-root bug recurs because path math is scattered: each module re-derives the worktree root and cwd relationship ad hoc. A single, minimal resolver makes correctness structural, not a matter of discipline — but it stops there. Every per-module path (weft siblings, junctions, `_lyx/plan`, `_pattern`, portals, launchers, and so on) is constructed by the module that owns it, joined onto the coordinates `lyxcwd` hands back.

## Exported API

`internal/lyxcwd` is a three-operation contract: resolve a working directory into a `Location`, then read the handful of derived coordinates off it.

### `Getwd() (string, error)`

A thin wrapper over `os.Getwd`; the only permitted `os.Getwd` call outside `internal/lyxcwd` and `cmd/lyx/main.go`.

**Returns:** On success, the cleaned absolute path of the cwd. On failure, an error (e.g. the cwd no longer exists).

**Use case:** Pre-initialization (before a git repo is accessible), when a caller needs a cwd but no git root yet.

### `Resolve(cwd string) (*Location, error)`

Builds a `Location` from `cwd` by running `git rev-parse --show-toplevel`, reading the recorded `.lyx-anchor` marker for `AnchorRel` (defaulting to `"."` when none is recorded), and then requires `cwd` to equal the anchored directory exactly — the strict cwd gate.

**Returns:** On success, the resolved `*Location`. On failure, `ErrNotAGitRepo` when git fails or `cwd` is outside a git repo, or `ErrCwdOutsideAnchor` when `cwd` is not exactly the anchored directory.

`Resolve` does NOT check for `_lyx/`; that stays in `internal/configengine`.

### `ResolveWithAnchor(cwd, anchor string) (*Location, error)`

Builds a `Location` exactly as `Resolve` does, but takes the anchor as a parameter instead of reading the recorded marker, and applies NO cwd gate.

**This is a deliberate bypass, not a general-purpose resolver.** A caller reaching for `ResolveWithAnchor` to escape a gate failure is misusing it — the correct fix is to stand in the anchored directory. It stays ungated because both its production callers stand somewhere the strict gate would reject: fabric's clone passes the freshly-cloned worktree root while the anchor may be a non-`"."` subpath, and `lyxtest` injects anchors into synthetic hubs to build fixtures.

### `ResolveWorktree(root string) (*Location, error)`

Builds a `Location` like `Resolve` but applies NO cwd gate. It exists for callers holding a worktree root (not an acting cwd) where the gate would spuriously fire — the gate applies only to `Resolve(cwd)`'s entry-point cwd, never to internal sibling-layout construction above a subpath anchor.

### `Location` struct

```go
type Location struct {
    RepoName     string // the repo name, with the hub suffix stripped
    HubPath      string // the hub container directory (parent of the worktree)
    WorktreeName string // this worktree's own directory name within the hub
    AnchorRel    string // the anchored subpath lyx operates at, relative to the worktree root
}
```

`Location` deliberately does not store `Cwd` or the worktree path themselves — under the strict cwd gate, cwd is provably equal to `AnchorPath()` after a successful resolve, and the worktree path is a direct child of `HubPath` by construction, so both are derivable rather than stored.

### `Location` methods

- **`WorktreePath() string`** — `filepath.Join(HubPath, WorktreeName)`. The path to this worktree: a direct child of the hub.
- **`AnchorPath() string`** — `filepath.Join(WorktreePath(), AnchorRel)`. The anchored subpath lyx operates at within this worktree; under the strict cwd gate this equals `cwd` after a successful `Resolve`.

That is the entire exported surface. No other accessor, weft path, junction path, or per-module subdirectory constructor lives here — see "What moved out", below.

## What moved out

Every per-module durable-storage subdirectory (`_lyx/plan`, `_lyx/webster`, `_pattern`, and the rest) is now that module's own private relative-path constant, joined onto `AnchorPath()` directly by the module that owns it — never a `lyxcwd` function call. Weft-sibling paths and junction construction (`WeftWorktree`, `HostLyxLink`, `HostJunctions`, portal and launcher paths, and the `Prime`/sibling-worktree-list lookup they are built from) belong to `internal/fabricengine`. The weft-backed junction name-set is injected from fabric config (`fabric.yaml`'s `pathspec`) — also `fabricengine`'s concern, never `lyxcwd`'s. See `CONSTRAINTS.md`'s Hub Geometry Invariant for the full, current per-token ownership map.

## Design principles

**Geometry-only, minimal.** `lyxcwd` answers *where a legal worktree's coordinates are*, never *what lives inside it*. Every other module's path construction is that module's own private concern, joined onto the coordinates `lyxcwd` resolves.

**Single call per invocation.** Most call sites invoke `Resolve(cwd)` once at the start of a command and re-use the returned `Location` throughout.

**Config resolution stays cwd-authoritative.** `lyxcwd.Resolve` is geometry-only and does NOT check for `_lyx/`. The cwd-authoritative config invariant (`_lyx/` must exist at cwd) remains enforced by `internal/configengine.FindBaseDir`.

## An infrastructure exception, not a permanent home

Cwd resolution staying below Fabric — rather than behind "every module asks Fabric" like the rest of the codebase — is a documented **infrastructure exception**, not the intended end state. The follow-up already recorded: move resolution into `internal/fabricengine` and inject the log directory into `internal/logger` from `cmd/lyx/main.go`, which eliminates this module entirely. That follow-up pulls `logger` initialization rework in with it and is out of scope for this slice.

## The enforcement wall

`internal/lyxcwd/enforcement_test.go` runs two repo-wide AST scans on every `go test ./internal/lyxcwd/...` run:

**`TestEnforcement` (cwd/root primitives ban):** Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/lyxcwd` and `cmd/lyx/main.go`. The scan uses a substring check on the raw file bytes (after blanking comments) and fails the build if either token appears in any non-test `.go` file outside the allowlist.

**`TestEnforcement_GeometryLiterals` (geometry-literal construction ban):** The policed geometry path tokens (`_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_lyx`, `_pattern`) may not appear as string literals in a **path-construction context** in any production file outside that token's registered owner directory (or directories, for a sanctioned dual-owner token). Path-construction contexts are:

- An argument to a `filepath.Join(...)` call.
- An operand of a binary `+` (`token.ADD`) expression.
- The value of a string `const` declaration.

Matching is **whole-token** (exact equality after `strconv.Unquote`, not substring), so compound names like `_boardroom` or `-weft-bare` are not flagged. Test files (`*_test.go`) are excluded from the scan — test geometry is a code-review obligation, not machine-enforced. A `scanned_non_empty` sub-test guards against a misconfigured walk that would silently produce a vacuous pass.

See [CONSTRAINTS.md](../../CONSTRAINTS.md) for the full invariant specification, the current per-token ownership map, and guidance for new code.
