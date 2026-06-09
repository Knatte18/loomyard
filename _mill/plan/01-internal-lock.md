# Batch: internal/lock — lift lock primitives

```yaml
task: "Extract shared infrastructure (config, git, lock)"
batch: "internal/lock — lift lock primitives"
number: 1
cards: 2
verify: go test ./internal/lock/...
depends-on: []
```

## Batch Scope

This batch creates the `internal/lock` package by lifting `FileLock`, `AcquireWriteLock`, `AcquireReadLock`, and `Release` verbatim from `internal/board/lock.go`. The existing `board/lock_test.go` is ported to the new package. No board files are touched here — board still compiles against its own `lock.go` until batch 4. The next batch consuming this package's output is batch 4 (board migration), which will import `internal/lock` and then delete `board/lock.go`.

## Cards

### Card 1: Create internal/lock/lock.go

- **Context:**
  - `internal/board/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/lock/lock.go`
- **Deletes:** none
- **Requirements:** Create `internal/lock/lock.go` with `package lock`. Exact verbatim lift of `FileLock` type, `AcquireWriteLock(path string) (FileLock, error)`, `AcquireReadLock(path string) (FileLock, error)`, and `Release(lock FileLock) error` from `internal/board/lock.go`. Keep all doc comments unchanged. Import `github.com/gofrs/flock` (same dep already in go.mod). No behavioural changes — this is a pure package move.
- **Commit:** `feat(lock): add internal/lock package`

### Card 2: Create internal/lock/lock_test.go

- **Context:**
  - `internal/board/lock_test.go`
- **Edits:** none
- **Creates:**
  - `internal/lock/lock_test.go`
- **Deletes:** none
- **Requirements:** Create `internal/lock/lock_test.go` with `package lock_test`. Port `TestAcquireWriteLock` (and any other tests present) verbatim from `internal/board/lock_test.go`, changing only: (1) the package declaration from `package board_test` to `package lock_test`; (2) the import from `github.com/Knatte18/mhgo/internal/board` to `github.com/Knatte18/mhgo/internal/lock`; (3) all unqualified call-site references updated to use the `lock.` prefix (e.g. `board.AcquireWriteLock` → `lock.AcquireWriteLock`, `board.Release` → `lock.Release`). Do not rewrite test logic.
- **Commit:** `test(lock): port lock tests to internal/lock`

## Batch Tests

`verify: go test ./internal/lock/...` runs `lock_test.go` against the new package. It covers `AcquireWriteLock`, `AcquireReadLock` (implicitly via the write-lock test), and `Release`. The board package is not touched in this batch so there is no full-suite regression risk here.
