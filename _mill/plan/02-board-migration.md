# Batch: board-migration

```yaml
task: "Extract internal/fsx and build internal/state"
batch: "board-migration"
number: 2
cards: 4
verify: go test ./internal/board/...
depends-on: [1]
```

## Batch Scope

Migrate `internal/board` off its own filesystem primitives and onto `internal/fsx` —
behaviour-preserving. `git.go` loses the moved functions; the fixed-path write in `store.go` switches
to the trusted-absolute `fsx.AtomicWriteBytes`; the dynamic-relPath write in `render.go` keeps the
guarded `fsx.AtomicWrite`; `git_test.go` loses the two moved tests. board's existing `store_test.go`
and `render_test.go` are the regression guardrail. Depends on batch 1 (the fsx symbols must exist).

## Cards

### Card 3: Strip primitives from git.go

- **Context:**
  - `internal/fsx/fsx.go`
- **Edits:**
  - `internal/board/git.go`
  - `internal/board/boardtest/integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/git.go` delete `PathGuard` (lines 33-59), `AtomicWrite`
  (lines 61-96), and `type BoardPathError` with its `Error()` method (lines 25-30). Keep `Pull`,
  `CommitPush`, and `type BoardPushError`. Remove the now-unused `"path/filepath"` import — the
  remaining functions use only `fmt`, `os`, `strings`, and `github.com/Knatte18/loomyard/internal/git`.
  Do NOT add an `internal/fsx` import here: git.go references no fsx symbol after the deletion. Update
  the file's top doc comment so it no longer claims to own `PathGuard`/`AtomicWrite`. In
  `internal/board/boardtest/integration_test.go`, replace all calls to `board.AtomicWrite` with
  `fsx.AtomicWrite` and add the import `"github.com/Knatte18/loomyard/internal/fsx"`.
- **Commit:** `refactor(board): drop AtomicWrite/PathGuard from git.go`

### Card 4: Point render.go at fsx.AtomicWrite

- **Context:**
  - `internal/fsx/fsx.go`
- **Edits:**
  - `internal/board/render.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/render.go`, `RenderToDisk` calls `AtomicWrite(boardPath, relPath, content)`
  at line 27 (unqualified, same-package). Change it to `fsx.AtomicWrite(boardPath, relPath, content)`
  and add `"github.com/Knatte18/loomyard/internal/fsx"` to the import block. The guard is retained
  here because `relPath` comes from `Render`'s output map (dynamic filenames). Leave the existing
  `fmt`/`os`/`path/filepath`/`strings` imports as-is (still used elsewhere in the file).
- **Commit:** `refactor(board): use fsx.AtomicWrite in render.go`

### Card 5: Point store.go at fsx.AtomicWriteBytes

- **Context:**
  - `internal/fsx/fsx.go`
- **Edits:**
  - `internal/board/store.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/store.go` the `Save` path holds the swap lock (line 107) and
  then calls `AtomicWrite(boardPath, relPath, string(content))` at line 113, where `content` is the
  `[]byte` from `json.Marshal`. Change that call to `fsx.AtomicWriteBytes(filepath.Join(boardPath, relPath), content)`
  — dropping the `string(content)` conversion and the redundant guard on the fixed `tasks.json` path.
  Add `"github.com/Knatte18/loomyard/internal/fsx"` to the import block. Keep the swap-lock
  acquire/release (lines 107-111) and the `path/filepath` import (still used for the swap-lock path
  and the new `filepath.Join`). Leave the existing `flock` import alias untouched.
- **Commit:** `refactor(board): use fsx.AtomicWriteBytes in store.go`

### Card 6: Remove moved tests from git_test.go

- **Context:**
  - `internal/fsx/fsx_test.go`
- **Edits:**
  - `internal/board/git_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/git_test.go` delete `TestPathGuard` (lines 17-41) and
  `TestAtomicWrite` (lines 43-97) — they now live in `internal/fsx/fsx_test.go`. Keep `TestPull` and
  `TestCommitPush` unchanged. Verify the remaining imports (`os`, `os/exec`, `path/filepath`,
  `strings`, `testing`, and the `board` package) are all still referenced by the kept tests and
  remove any that became unused (all are still used by `TestPull`/`TestCommitPush`, so none should
  need removal). Update the file's top doc comment to drop the `PathGuard`/`AtomicWrite` mention.
- **Commit:** `test(board): drop moved PathGuard/AtomicWrite tests`

## Batch Tests

`verify: go test ./internal/board/...` runs board's full suite, including `store_test.go`,
`render_test.go`, and the trimmed `git_test.go` (`TestPull`, `TestCommitPush`). This is the
behaviour-preserving guardrail: tasks.json writes (store), rendered-file writes (render), and git
plumbing must all still pass after the migration to fsx. Scope is the single board package.
