# Plan: Extract internal/fslink cross-OS link primitive

```yaml
task: "Extract internal/fslink cross-OS link primitive"
slug: "extract-fslink"
approved: false
started: "20260622-110905"
parent: "main"
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: fslink-package
    file: 01-fslink-package.md
    depends-on: []
    verify: go test ./internal/fslink/... ./internal/paths/...
  - number: 2
    name: migrate-callsites
    file: 02-migrate-callsites.md
    depends-on: [1]
    verify: go test -tags integration ./internal/fslink/... ./internal/worktree/... ./internal/weft/... ./internal/paths/...
```

## Shared Decisions

### Decision: package-owns-the-os-split

- **Decision:** The OS split lives entirely inside `internal/fslink`
  (`fslink_windows.go` / `fslink_other.go`); `internal/worktree` keeps zero
  build-tagged link files after the migration.
- **Rationale:** Cross-OS support is the point of the extraction — callers must not
  need parallel `junction_windows`/`junction_other` files. The one genuinely
  OS-specific concern is centralized behind a unified API.
- **Applies to:** all batches

### Decision: fslink-public-api

- **Decision:** `internal/fslink` exposes exactly five functions, documented in the
  `fslink.go` package header (the single source of truth for the API contract):
  `Create(link, target string) error`, `Remove(link string) error` (idempotent —
  nil when absent), `IsLink(path string) (bool, error)` (`(false, nil)` on
  `os.IsNotExist`, `(false, err)` on other stat failure), `PointsTo(link string)
  (string, error)` (fully-resolved absolute target via `filepath.EvalSymlinks`), and
  `RemoveLinksIn(dir string) (int, error)` (immediate-children link sweep).
- **Rationale:** A fixed, documented contract lets every migration card reference one
  file (`fslink.go`) for the whole API.
- **Applies to:** all batches

### Decision: no-privilege-junctions

- **Decision:** On Windows, links are junctions (mount-point reparse points) created
  via a direct `golang.org/x/sys/windows` reparse-point syscall — never `os.Symlink`
  (which requires `SeCreateSymbolicLink`). On non-Windows, links are `os.Symlink`
  symlinks.
- **Rationale:** Preserves today's no-privilege junction behaviour while removing the
  `cmd /c mklink /J` double process spawn.
- **Applies to:** all batches

### Decision: preserve-behaviour-and-messages

- **Decision:** The migration is behaviour-preserving. Caller-visible error/reason
  strings that tests assert on must be preserved verbatim — especially
  `checkJunction`'s `"host _lyx junction missing"`, `"host _lyx is not a junction"`,
  `"host _lyx junction points elsewhere"`, and `seedLyxJunction`'s `"host repo
  already contains a real _lyx …"` / `"weft _lyx directory does not exist …"`. The
  `remove.go`/`add.go` ordering (host junction removed first; sweep as safety net)
  must not change.
- **Rationale:** The surviving integration tests prove the migration; changing
  messages or ordering would break them and the contract.
- **Applies to:** migrate-callsites

### Decision: test-build-tags

- **Decision:** New `internal/fslink` tests are **untagged** (pure filesystem
  syscalls, no git, no process spawn). The surviving worktree/weft link tests
  (`portals_test.go`, `remove_test.go`, `weft_test.go`, `status_test.go`) **keep**
  their `//go:build integration` tag — they depend on `internal/lyxtest` git
  fixtures, unaffected by this task.
- **Rationale:** The `mklink` spawn was only one reason for the tag; the git-fixture
  dependency is the binding reason for the surviving files, so the tag stays.
- **Applies to:** all batches

### Decision: path-invariant-clean

- **Decision:** `internal/fslink` operates only on caller-supplied paths; it must NOT
  call `os.Getwd` or `git rev-parse --show-toplevel` (banned outside
  `internal/paths` and `cmd/lyx/main.go` by `CONSTRAINTS.md`, enforced by
  `internal/paths/enforcement_test.go`).
- **Rationale:** Keeps the build-time path invariant green; both batches include
  `./internal/paths/...` in `verify:` to catch any violation introduced by `fslink`.
- **Applies to:** all batches

## All Files Touched

- `go.mod`
- `go.sum`
- `internal/fslink/fslink.go`
- `internal/fslink/fslink_other.go`
- `internal/fslink/fslink_test.go`
- `internal/fslink/fslink_windows.go`
- `internal/weft/status.go`
- `internal/weft/status_test.go`
- `internal/worktree/portals.go`
- `internal/worktree/remove.go`
- `internal/worktree/weft.go`
