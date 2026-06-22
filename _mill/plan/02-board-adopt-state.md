# Batch: board-adopt-state

```yaml
task: "Adopt internal/state in board and muxpoc"
batch: "board-adopt-state"
number: 2
cards: 2
verify: go test ./internal/board/...
depends-on: [1]
```

## Batch Scope

Route `board.Store`'s persistence through `internal/state`, passing board's existing
swap lock so the coarse-lock / swap-lock design is preserved. `Store.Save` loses its
redundant `(boardPath, relPath)` parameters (it already re-derives the same path the
constructor stored in `s.filePath`). `Store.Load` adopts `state.ReadJSON` and now
surfaces a corrupt `tasks.json` as an error instead of silently producing an empty
board — the read consumers (`GetTask`, `ListTasksBrief`, `ListTasksFull`, used by
`internal/ide/menu.go` and the board CLI) propagate that error. The DependsOn nil→`[]`
normalization and the empty-`filePath` short-circuit are retained. Depends on batch 1
for the explicit-lock-path signature. `board.HealthCheck` is intentionally NOT touched:
it reads via raw `os.ReadFile`, not `store.Load`, and deliberately tolerates corrupt
files.

## Cards

### Card 2: Adopt state in Store.Save and Store.Load

- **Context:**
  - `internal/state/state.go`
  - `internal/board/task.go`
- **Edits:**
  - `internal/board/store.go`
  - `internal/board/board.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `internal/board/store.go`, change `Save` from
    `func (s *Store) Save(boardPath, relPath string) error` to `func (s *Store) Save() error`.
    Implement it as `return state.WriteJSON(s.filePath, s.filePath+swapLockSuffix, s.tasks)`.
    Remove the manual `json.MarshalIndent`, the manual `flock.AcquireWriteLock(... + swapLockSuffix)`,
    and the manual `fsx.AtomicWriteBytes`. Keep the `swapLockSuffix` constant.
  - Change `Load` to call `state.ReadJSON[[]Task](s.filePath, s.filePath+swapLockSuffix)`.
    Preserve: the empty-`s.filePath` short-circuit at the top (set `s.tasks = []Task{}`,
    return nil, do not touch disk); on returned error, return it wrapped
    (`fmt.Errorf("load store: %w", err)` or similar — a corrupt file now surfaces, not
    a silent empty board); when the boolean "found" result is false (file absent), set
    `s.tasks = []Task{}` and return nil; on success, assign the returned slice and then
    run the existing loop that sets each `tasks[i].DependsOn` to `[]string{}` when nil.
  - Remove now-unused imports from `store.go` (`encoding/json`, `os` if no longer used,
    `path/filepath` if no longer used, `github.com/Knatte18/loomyard/internal/fsx`, and the
    `flock` alias) and add the `internal/state` import. Verify against the final file which
    imports remain referenced (e.g. `fmt` is still used).
  - In `internal/board/board.go`, update the single caller in `writeOp` from
    `store.Save(b.boardPath, "tasks.json")` to `store.Save()`. The coarse
    `flock.AcquireWriteLock(filepath.Join(b.boardPath, writeLockFile))` in `writeOp` is
    unchanged.
  - **Do not** change `board.HealthCheck` or any read op's locking posture: reads still
    acquire only the swap lock (now via `state.ReadJSON`), never the coarse lock.
- **Commit:** `refactor(board): persist tasks.json via internal/state`

### Card 3: Update board store tests for corruption-as-error

- **Context:**
  - `internal/board/store.go`
  - `internal/board/board.go`
  - `internal/board/board_test.go`
  - `internal/board/boardtest/concurrency_test.go`
- **Edits:**
  - `internal/board/store_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Add a test in `store_test.go` that writes a syntactically corrupt `tasks.json`, calls
    `Store.Load`, and asserts it returns a non-nil error (previously Load silently set an
    empty task list). If an existing test asserted the old silent-empty-on-corrupt
    behaviour, update it to expect the error.
  - Keep coverage that `Load` normalizes a nil `DependsOn` to an empty slice and that a
    missing file yields an empty store with no error.
  - Update any test call of `Store.Save` to the new no-argument signature `Save()`.
  - Do NOT modify `internal/board/board_test.go` — in particular leave
    `TestHealthCheckPassesCorruptFile` unchanged; `HealthCheck` bypasses `Load` and still
    tolerates corrupt-but-readable files. Do NOT modify the `boardtest` concurrency tests;
    they are the guardrail proving the swap-lock / coarse-lock split survived.
- **Commit:** `test(board): assert Load surfaces corrupt tasks.json`

## Batch Tests

`verify: go test ./internal/board/...` covers the `board` package (`store_test.go`,
`board_test.go`, etc.) and the `board/boardtest` subpackage (the concurrency and sync
guardrail tests). Running the whole `board/...` tree is the right scope here because the
change touches the store's locking path, which the `boardtest` concurrency tests exercise
directly; a narrower scope would miss the deadlock-regression signal.
