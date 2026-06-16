# Plan: Extract internal/fsx and build internal/state

```yaml
task: "Extract internal/fsx and build internal/state"
slug: "extract-internal-fsx"
approved: false
started: "20260616-181720"
parent: "main"
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
    name: fsx-package
    file: 01-fsx-package.md
    depends-on: []
    verify: go test ./internal/fsx/...
  - number: 2
    name: board-migration
    file: 02-board-migration.md
    depends-on: [1]
    verify: go test ./internal/board/...
  - number: 3
    name: muxpoc-swap
    file: 03-muxpoc-swap.md
    depends-on: [1]
    verify: go test ./internal/muxpoc/...
  - number: 4
    name: state-package
    file: 04-state-package.md
    depends-on: [1]
    verify: go test ./internal/state/...
  - number: 5
    name: docs
    file: 05-docs.md
    depends-on: [1, 4]
    verify: null
```

## Shared Decisions

### Decision: fsx public API

- **Decision:** `internal/fsx` exposes exactly three functions plus one error type:
  - `func AtomicWriteBytes(absPath string, data []byte) error` — the general primitive:
    `os.MkdirAll(filepath.Dir(absPath), 0o755)`, write `data` to a temp file created in that
    directory via `os.CreateTemp(dir, ".tmp-")`, then `os.Rename` onto `absPath`. No path guard.
    `defer os.Remove(tmpPath)` for cleanup on the error path.
  - `func PathGuard(relPath string) error` — rejects empty, absolute (`filepath.IsAbs`, a leading
    `/`, and the Windows `X:` drive form `relPath[1] == ':'`), and any `..` path component.
  - `func AtomicWrite(dir, relPath, content string) error` — guarded convenience: call
    `PathGuard(relPath)`, then `AtomicWriteBytes(filepath.Join(dir, relPath), []byte(content))`.
  - `type PathError string` with `func (e PathError) Error() string { return string(e) }` —
    returned by `PathGuard`. This is the renamed `board.BoardPathError`.
- **Rationale:** the writer primitive carries no security policy; callers opt into the guard where
  the relative path is untrusted. `AtomicWrite` keeps existing board/muxpoc call sites one-line.
- **Applies to:** all batches

### Decision: behaviour-preserving moves

- **Decision:** `AtomicWriteBytes` preserves `board.AtomicWrite`'s exact mechanics (MkdirAll 0o755,
  `os.CreateTemp(dir, ".tmp-")`, write, close, rename, `defer os.Remove` on the temp path). Keep the
  rename-is-the-atomic-swap comment. `PathGuard` is moved verbatim from `internal/board/git.go`.
- **Rationale:** board's existing `store_test.go` / `render_test.go` and muxpoc's suite are the
  regression guardrail — the disk behaviour must not change.
- **Applies to:** fsx-package, board-migration, muxpoc-swap

### Decision: internal/state API

- **Decision:** `internal/state` exposes generic locked typed JSON I/O:
  - `func WriteJSON[T any](path string, v T) error` — `os.MkdirAll(filepath.Dir(path), 0o755)`,
    acquire an exclusive write lock on `path + ".lock"` via `internal/lock.AcquireWriteLock`
    (`defer Release`), `json.MarshalIndent(v, "", "  ")`, then `fsx.AtomicWriteBytes(path, data)`.
  - `func ReadJSON[T any](path string) (value T, found bool, err error)` — `os.MkdirAll` the parent,
    acquire a shared read lock on `path + ".lock"` via `internal/lock.AcquireReadLock`
    (`defer Release`), `os.ReadFile(path)`; on `os.IsNotExist` return `(zero, false, nil)`; on other
    read error return `(zero, false, err)`; `json.Unmarshal` into a `T`, returning a non-nil `err`
    (corruption is never swallowed) on parse failure, else `(value, true, nil)`.
- **Rationale:** type-safe call sites (Go 1.26 generics); the `found` bool serves the dominant
  "load existing state or start fresh" pattern without callers writing `errors.Is`. Lock lives
  beside the data file (`path + ".lock"`, suffix-append). Mirrors the proven `muxpoc.LoadState`/
  `SaveState` shape; does NOT import muxpoc.
- **Applies to:** state-package, docs

### Decision: Go test conventions

- **Decision:** tests are `package <pkg>_test` external test packages using the standard library
  `testing` only (matching the existing board/muxpoc suites). Per-batch `verify:` runs the native
  `go test ./internal/<pkg>/...` for that package only — no global build. No `PYTHONPATH=` prefix
  (this is a Go project, not Python).
- **Rationale:** consistency with the existing suites; focused verify keeps each round fast.
- **Applies to:** all batches with a runnable surface

## All Files Touched

- `docs/roadmap.md`
- `docs/shared-libs/README.md`
- `docs/shared-libs/fsx.md`
- `docs/shared-libs/state.md`
- `internal/board/git.go`
- `internal/board/git_test.go`
- `internal/board/render.go`
- `internal/board/store.go`
- `internal/fsx/fsx.go`
- `internal/fsx/fsx_test.go`
- `internal/muxpoc/state.go`
- `internal/state/state.go`
- `internal/state/state_test.go`
