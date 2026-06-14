# Batch: board-paths-healthcheck

```yaml
task: 'Extend worktree module: portals and launchers'
batch: 'board-paths-healthcheck'
number: 4
cards: 3
verify: go test ./internal/board/...
depends-on: [1, 2]
```

## Batch Scope

This batch routes the `board` module's cwd acquisition through `paths.Getwd`
(behavior-preserving — config resolution stays cwd-authoritative via
`internal/config`), refactors `board init`'s gitignore block onto the shared
`internal/gitignore` lib, and adds the one sanctioned new board method,
`HealthCheck`, that `ide` will call before reading titles. **Interface consumed
by batch 6:** `func (b *Board) HealthCheck() error`.

## Cards

### Card 12: board cwd via paths.Getwd

- **Context:**
  - `internal/paths/paths.go`
  - `internal/config/config.go`
- **Edits:**
  - `internal/board/cli.go`
  - `internal/board/init.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace `os.Getwd()` in `internal/board/cli.go`'s `RunCLI`
  (the cwd branch, around line 74) and in `internal/board/init.go`'s `RunInit`
  (around line 25) with `paths.Getwd()`. Behavior is unchanged: still
  cwd-authoritative, still `LoadConfig(cwd, "board")`, still `_mhgo/`-at-cwd
  enforced by `internal/config.FindBaseDir`. Drop the direct `os` import from a
  file only if it becomes unused after the swap (both files still use `os`
  elsewhere — keep the import where needed).
- **Commit:** `refactor(board): obtain cwd via internal/paths`

### Card 13: board init uses the shared gitignore lib

- **Context:**
  - `internal/gitignore/gitignore.go`
  - `internal/board/init_test.go`
- **Edits:**
  - `internal/board/init.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/init.go`, replace the Step-4
  `updateGitignoreBlock(gitignorePath)` call with `gitignore.Ensure(cwd,
  ".mhgo/")`, mapping the returned `changed` bool to the existing status string
  (`true` → `"updated"`, `false` → `"unchanged"`). Delete the now-dead
  `updateGitignoreBlock` function. Preserve the rest of `RunInit` (the
  `_mhgo/` + `board.yaml` + `worktree.yaml` scaffolding and the JSON summary)
  unchanged. The existing `internal/board/init_test.go` integration assertions
  (markers present, `.mhgo/` entry present, idempotent `gitignore: "unchanged"`
  on re-run) must still pass through the new code path without edits; do not
  modify the test unless a status-string value genuinely changes.
- **Commit:** `refactor(board): use internal/gitignore for the managed block`

### Card 14: board HealthCheck

- **Context:**
  - `internal/board/git.go`
  - `internal/board/config.go`
- **Edits:**
  - `internal/board/board.go`
  - `internal/board/board_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func (b *Board) HealthCheck() error` to
  `internal/board/board.go`. Per its pinned contract: `os.Stat(b.boardPath)` and
  return an error if the board dir is absent; then open and read
  `filepath.Join(b.boardPath, "tasks.json")` (e.g. `os.ReadFile`) and return an
  error if it cannot be opened/read; do NOT `json.Unmarshal` — a
  syntactically-corrupt-but-readable file passes health-check (parse errors stay
  the readers' concern). Return nil on success. `board_test.go`: nil for a
  present board whose `tasks.json` is readable (including a corrupt-but-readable
  one); non-nil when the board dir is absent; non-nil when `tasks.json` is absent
  or unreadable. Reuse the package's existing board-construction test helpers.
- **Commit:** `feat(board): add stat-level HealthCheck`

## Batch Tests

`verify: go test ./internal/board/...` runs the full board suite. The migration
cards (12, 13) are behavior-preserving and covered by the existing
`cli_test.go`/`init_test.go`; card 14 adds `HealthCheck` cases to `board_test.go`.
Scope is the single `board` package this batch touches.
