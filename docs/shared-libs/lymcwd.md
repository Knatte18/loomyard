# `internal/lymcwd`

The **entry gate** from "the process started somewhere" to the coordinates of a legal lym worktree.
It returns those coordinates, or the reason the directory is not a lym worktree.
It owns no geometry and constructs no path from a structural token.
It resolves the active `Location` from a working directory.
It exposes the typed accessors every caller derives from that `Location`.

**Dependency direction (test-enforced):** the package's imports are capped at an allowlist.
The allowlist is the standard library, `internal/dotgit` and the leaf that names a worktree's records sibling.
With no `gitexec` import the package cannot spawn git: it finds the worktree root in-process.
This ceiling keeps `fabricengine` → `logger` → `lymcwd` acyclic.
A wider import set would risk a cycle back through a package `lymcwd` sits below.
The Go compiler does not enforce the cap.
`TestLeafInvariant_AllowlistOnly` in `internal/lymcwd/leaf_enforcement_test.go` does:
an out-of-allowlist import that creates no cycle compiles fine.

## The problem

The cwd-≠-worktree-root bug recurs because path math is scattered.
Each module re-derives the relationship between the worktree root and the cwd ad hoc.
A single, minimal resolver makes correctness structural, not a matter of discipline — but it stops there.
Every per-module path (records siblings, junctions, `_lym/plan`, launchers) is built by the module that owns it.
The module joins it onto the coordinates `lymcwd` hands back.

## Exported API

`internal/lymcwd` resolves a working directory into a `Location`, then exposes the few coordinates derived from it.

### `Getwd() (string, error)`

A thin wrapper over `os.Getwd`;
the only permitted `os.Getwd` call outside `internal/lymcwd` and `cmd/lym/main.go`.

**Returns:** On success, the cleaned absolute path of the cwd.
On failure, an error (e.g. the cwd no longer exists).

**Use case:** Pre-initialization (before a git repo is accessible), when a caller needs a cwd but no git root yet.

### `Resolve(cwd string) (*Location, error)`

Builds a `Location` from `cwd`.
It finds the worktree root in-process through `internal/dotgit`.
It reads the worktree's recorded `.lym-anchor` marker for `AnchorRel`.
It then requires `cwd` to equal the anchored directory exactly: the strict cwd gate.
The walk to the worktree root starts from `cwd` with its symlinks resolved, as `git rev-parse --show-toplevel` does.
A hub reached through a symlinked path, to the root or below it, therefore gets the same `Location` as through its real path.

**Returns:** On success, the resolved `*Location`.
On failure, one of two errors:

- `ErrNotAGitRepo` when `cwd` is outside a git repo;
  it is also returned for a pruned worktree and for one whose gitfile names a missing git dir.
- `ErrCwdOutsideAnchor` when `cwd` is not exactly the anchored directory.

`Resolve` does NOT check for `_lym/`;
that stays in `internal/configengine`.

### `ResolveWithAnchor(cwd, anchor string) (*Location, error)`

Builds a `Location` as `Resolve` does, but takes the anchor as a parameter instead of reading the recorded marker.
It applies NO cwd gate.

**This is a bypass, not a general-purpose resolver.**
A caller reaching for `ResolveWithAnchor` to escape a gate failure is misusing it — the correct fix is to stand in the anchored directory.
It stays ungated because its callers stand somewhere the strict gate would reject.
For example, a fresh clone passes the new worktree root while the anchor may be a non-`"."` subpath.

### The recorded anchor

A worktree's anchor is read in this order:

1. the worktree's own `.lym-anchor`, at the root of its records sibling;
   an empty one counts as absent, and one that fails `ValidateAnchorRel` is an error naming the file;
2. the legacy hub-wide `.lym-anchor` at the board root;
   a stale pre-rename `.fabric-anchor` with no renamed marker beside it is `ErrStaleAnchorMarker`;
   the guard applies to this read only;
3. `"."`.

Only `fabricengine` writes the marker or reads it from git; `lymcwd` reads the working tree alone.

### `NamedWorktree(hubPath, name string) (*Location, error)`

Builds the `Location` of the worktree `name` in the hub at `hubPath`.
It reads that worktree's anchor in the order above.
It applies no cwd gate and spawns no git.
It needs neither the worktree nor its records sibling to exist: with both absent, the anchor is the board marker's value or `"."`.
It is how production code outside `lymcwd` gets the `Location` of a worktree other than the acting one.

### `ResolveWorktree(root string) (*Location, error)`

Builds a `Location` like `Resolve` but applies NO cwd gate.
It exists for callers holding a worktree root rather than an acting cwd, where the gate would fire spuriously.
The gate applies only to the entry-point cwd of `Resolve(cwd)`.
It never applies to internal sibling-layout construction above a subpath anchor.

### `Location` struct

```go
type Location struct {
    RepoName     string // the repo name, with the hub suffix stripped
    HubPath      string // the hub container directory (parent of the worktree)
    WorktreeName string // this worktree's own directory name within the hub
    AnchorRel    string // the anchored subpath lym operates at, relative to the worktree root
}
```

`Location` stores neither `Cwd` nor the worktree path, because both are derivable.
Under the strict cwd gate, cwd equals `AnchorPath()` after a successful resolve,
and the worktree path is a direct child of `HubPath` by construction.

### `Location` methods

- **`WorktreePath() string`** — `filepath.Join(HubPath, WorktreeName)`.
  The path to this worktree: a direct child of the hub.
- **`AnchorPath() string`** — `filepath.Join(WorktreePath(), AnchorRel)`.
  The anchored subpath lym operates at within this worktree;
  under the strict cwd gate this equals `cwd` after a successful `Resolve`.

The package exports no other accessor, records path, junction path, or per-module subdirectory constructor;
see "What moved out" below.

## What moved out

Every per-module durable-storage subdirectory (`_lym/plan`, `_lym/webster`, and the rest) is that module's own private relative-path constant.
The owning module joins it onto `AnchorPath()` directly, never through a `lymcwd` function call.
Records-sibling paths, junction construction and launcher paths belong to `internal/fabricengine`,
as do the prime and sibling-worktree lookups they are built from (for example `RecordsWorktree` and `CodeLymLink`).
Fabric config (`fabric.yaml`'s `pathspec`) supplies the records-backed junction name-set;
that too is `fabricengine`'s concern, never `lymcwd`'s.
See [PATTERN-cwd-resolution](../../pattern/PATTERN-cwd-resolution.md) for the per-token ownership map.

## Design principles

**Geometry-only, minimal.** `lymcwd` answers *where a legal worktree's coordinates are*, never *what lives inside it*.
Every other module builds its own paths, joined onto the coordinates `lymcwd` resolves.

**Single call per invocation.**
Most call sites invoke `Resolve(cwd)` once at the start of a command and re-use the returned `Location` throughout.

**Config resolution stays cwd-authoritative.** `lymcwd.Resolve` is geometry-only and does NOT check for `_lym/`.
`internal/configengine.FindBaseDir` enforces the cwd-authoritative config invariant: `_lym/` must exist at cwd.

## An infrastructure exception, not a permanent home

Cwd resolution sits below Fabric, not behind "every module asks Fabric" like the rest of the codebase.
This is a documented **infrastructure exception**, not the intended end state.
The recorded follow-up moves resolution into `internal/fabricengine`, which eliminates this module.
It also injects the log directory into `internal/logger` from `cmd/lym/main.go`.
That follow-up requires reworking `logger` initialization.

## The enforcement wall

`internal/lymcwd/enforcement_test.go` runs repo-wide AST scans on every `go test ./internal/lymcwd/...` run:

**`TestEnforcement` (cwd/root primitives ban):**
Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/lymcwd` and `cmd/lym/main.go`.
The scan is a substring check on the raw file bytes, after blanking comments.
It fails the build if either token appears in a non-test `.go` file outside the allowlist.

**`TestEnforcement_GeometryLiterals` (geometry-literal construction ban):**
The policed geometry path tokens are `_bolt`, the records-sibling suffix, `-LYMHUB`, `_portals`, `_launchers`, `_lym` and `.lym`.
A token may appear as a string literal in a **path-construction context** only in its registered owner directory.
A sanctioned dual-owner token has two owner directories.
Path-construction contexts are:

- An argument to a `filepath.Join(...)` call.
- An operand of a binary `+` (`token.ADD`) expression.
- The value of a string `const` declaration.

Matching is **whole-token**: exact equality after `strconv.Unquote`, not substring.
Compound names like `_boltroom` or a suffix-extended name are therefore not flagged.
Test files (`*_test.go`) are excluded from the scan;
test geometry is a code-review obligation, not machine-enforced.
A `scanned_non_empty` sub-test guards against a misconfigured walk that would silently produce a vacuous pass.

A separately filed test, `TestLeafInvariant_AllowlistOnly` in `leaf_enforcement_test.go`, enforces the import cap described above.
It walks the package's own production `.go` files and fails on any import outside the allowlist.

See [PATTERN.md](../../PATTERN.md) for the full invariant specification.
It also holds the per-token ownership map and guidance for new code.
