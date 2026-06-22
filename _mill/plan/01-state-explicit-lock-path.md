# Batch: state-explicit-lock-path

```yaml
task: "Adopt internal/state in board and muxpoc"
batch: "state-explicit-lock-path"
number: 1
cards: 1
verify: go test ./internal/state/
depends-on: []
```

## Batch Scope

Make `internal/state` accept an explicit lock-file path instead of deriving it from the
data path. This is the enabling change the board and muxpoc batches depend on: board
needs a lock file distinct from `<path>.lock`, which the current hardcoded derivation
cannot express. `state` has no production callers yet — only `state_test.go` — so the
signature change is self-contained to this package. The external interface batches 2 and
3 consume is the new two-string signature: `WriteJSON[T any](path, lockPath string, v T) error`
and `ReadJSON[T any](path, lockPath string) (T, bool, error)`. Corruption-as-error
behaviour in `ReadJSON` is already correct and is left unchanged.

## Cards

### Card 1: Thread an explicit lock path through state.WriteJSON / state.ReadJSON

- **Context:**
  - `internal/lock/lock.go`
  - `internal/fsx/fsx.go`
- **Edits:**
  - `internal/state/state.go`
  - `internal/state/state_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Change `WriteJSON` from `func WriteJSON[T any](path string, v T) error` to
    `func WriteJSON[T any](path, lockPath string, v T) error`. Replace the
    `lock.AcquireWriteLock(path + ".lock")` call with `lock.AcquireWriteLock(lockPath)`.
    Everything else (MkdirAll on `filepath.Dir(path)`, `json.MarshalIndent(v, "", "  ")`,
    `fsx.AtomicWriteBytes(path, data)`, deferred release) is unchanged.
  - Change `ReadJSON` from `func ReadJSON[T any](path string) (T, bool, error)` to
    `func ReadJSON[T any](path, lockPath string) (T, bool, error)`. Replace the
    `lock.AcquireReadLock(path + ".lock")` call with `lock.AcquireReadLock(lockPath)`.
    Keep the existing semantics: MkdirAll on `filepath.Dir(path)`; NotExist → `(zero, false, nil)`;
    other read errors → wrapped error; unmarshal error → wrapped error (corruption is
    NOT swallowed); success → `(v, true, nil)`; deferred release.
  - Update the godoc on both functions so they describe the caller-supplied lock path
    rather than `path + ".lock"`.
  - Update `internal/state/state_test.go` for the new two-argument signatures: every
    `WriteJSON(path, v)` becomes `WriteJSON(path, path+".lock", v)` and every
    `ReadJSON[...](path)` becomes `ReadJSON[...](path, path+".lock")`. In
    `TestLockFileLocation` and the "exactly two files in the directory" test, pass
    `path + ".lock"` as the lock path and keep asserting the lock lands at that location.
    The corruption test (if present) keeps asserting an error.
- **Commit:** `refactor(state): take explicit lock path in WriteJSON/ReadJSON`

## Batch Tests

`verify: go test ./internal/state/` runs the `internal/state` package tests
(`state_test.go`), covering the round-trip, NotExist, lock-file-location, and corruption
cases against the new signatures. Scope is the single package this batch touches.
