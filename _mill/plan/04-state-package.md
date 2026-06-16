# Batch: state-package

```yaml
task: "Extract internal/fsx and build internal/state"
batch: "state-package"
number: 4
cards: 2
verify: go test ./internal/state/...
depends-on: [1]
```

## Batch Scope

Build the new `internal/state` package — generic locked typed JSON I/O, built test-first. It composes
`fsx.AtomicWriteBytes` (batch 1) with `internal/lock` (already exists). No live consumer ships in this
task (`mux` is a future milestone), so the test suite is the only caller and the primary deliverable.
The external interface is `WriteJSON[T]` / `ReadJSON[T]` per `## Shared Decisions → internal/state API`.
Independent of batches 2/3 (disjoint files); depends on batch 1.

## Cards

### Card 8: Create the state package

- **Context:**
  - `internal/fsx/fsx.go`
  - `internal/lock/lock.go`
  - `internal/muxpoc/state.go`
- **Edits:** none
- **Creates:**
  - `internal/state/state.go`
- **Deletes:** none
- **Requirements:** New file `internal/state/state.go` in `package state`. Implement
  `func WriteJSON[T any](path string, v T) error`: `os.MkdirAll(filepath.Dir(path), 0o755)`; acquire
  an exclusive lock via `lock.AcquireWriteLock(path + ".lock")` with `defer l.Release()`;
  `data, err := json.MarshalIndent(v, "", "  ")`; `return fsx.AtomicWriteBytes(path, data)`. Implement
  `func ReadJSON[T any](path string) (T, bool, error)`: declare `var zero T`; `os.MkdirAll(filepath.Dir(path), 0o755)`;
  acquire `lock.AcquireReadLock(path + ".lock")` with `defer l.Release()`; `data, err := os.ReadFile(path)`;
  on `os.IsNotExist(err)` return `(zero, false, nil)`; on any other read error return `(zero, false, err)`;
  `var v T`; on `json.Unmarshal(data, &v)` error return `(zero, false, fmt.Errorf("unmarshal state: %w", err))`;
  else return `(v, true, nil)`. Use the `lock` package as `"github.com/Knatte18/loomyard/internal/lock"`
  and fsx as `"github.com/Knatte18/loomyard/internal/fsx"`. Model the lock-then-atomic-write shape on
  `muxpoc.SaveState`/`LoadState` but do NOT import muxpoc. Imports: `encoding/json`, `fmt`, `os`,
  `path/filepath`, plus the two internal packages.
- **Commit:** `feat(state): add generic locked JSON read/write`

### Card 9: state unit tests (test-first coverage)

- **Context:**
  - `internal/state/state.go`
- **Edits:** none
- **Creates:**
  - `internal/state/state_test.go`
- **Deletes:** none
- **Requirements:** New file `internal/state/state_test.go` in `package state_test`. Define a small
  local struct type (e.g. `type sample struct { Name string; N int }`) to instantiate the generics.
  Cover: (1) round-trip — `WriteJSON(path, sample{...})` then `ReadJSON[sample](path)` returns
  `found=true`, `err=nil`, equal value; (2) missing file — `ReadJSON[sample]` on a never-written path
  under `t.TempDir()` returns `found=false`, `err=nil`, and the parent dir plus a `<path>.lock` file
  now exist; (3) corrupt file — `os.WriteFile(path, []byte("{not json"), 0o644)` then `ReadJSON[sample]`
  returns a non-nil `err` (corruption surfaced, not swallowed); (4) no temp leak — after `WriteJSON`,
  the directory contains only the data file and `<path>.lock`, no `.tmp-` entry; (5) overwrite — a
  second `WriteJSON` with a different value, then `ReadJSON` returns the new value; (6) lock-file
  location — assert the lock path is exactly `path + ".lock"` beside the data file. Use stdlib
  `testing`/`os`/`path/filepath` only.
- **Commit:** `test(state): cover round-trip, missing, corrupt, overwrite`

## Batch Tests

`verify: go test ./internal/state/...` runs the new `internal/state/state_test.go`. Because nothing
in production consumes `internal/state` yet, this suite is the sole verification surface — it must
exercise the full read/write/missing/corrupt/overwrite matrix and the lock-file placement. Scope is
the single state package.
