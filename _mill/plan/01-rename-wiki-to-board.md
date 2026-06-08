# Batch: rename-wiki-to-board

```yaml
task: "board-modul (rename fra wiki) + _mhgo-konfigurasjon"
batch: "rename-wiki-to-board"
number: 1
cards: 4
verify: go build ./... && go vet -tags integration ./... && go test ./...
depends-on: []
```

## Batch Scope

This batch performs the full, **behavior-preserving** rename of the `wiki`
module to `board`: directory, packages, the `Wiki` facade type and its
identifiers, the exported error types, the control env vars, the background
commit message, and the spawned module argument. It deliberately KEEPS the
soon-to-be-deleted path knobs (`--wiki-path` flag, `resolveWikiPath`,
`defaultWikiPath = "../gowiki"`, the `MHGO_WIKI_PATH` env var, the local
`wikiPath` variable inside `RunCLI`) and KEEPS all on-disk filenames
(`tasks.json`, `*.lock`, `*.push.lock`, `*.swaplock`) and the external test-repo
URL `github.com/Knatte18/mhgo-wiki-test`. After this batch the tree compiles and
every test passes; `mhgo board --wiki-path <dir> <subcommand>` behaves exactly
as `mhgo wiki ...` did. The renamed `internal/board` package and the `Board`
facade are the surface every later batch builds on. See the overview's
`## Shared Decisions` (`rename-surface`, `file-renames-via-git-mv`) for the
exhaustive rename mapping — this batch implements it.

Batch-local decision: the moved files are modeled in the cards as `Creates:`
(new `internal/board/*` paths) + `Deletes:` (old `internal/wiki/*` paths)
because that is how the plan validator represents a `git mv`. The implementer
performs real `git mv` operations (card 1) so git history is preserved, then
edits the moved files in place (cards 2–4).

## Cards

### Card 1: git mv the tree, rename packages, fix imports and the dispatcher

- **Context:**
  - `go.mod`
- **Edits:**
  - `cmd/mhgo/main.go`
  - `cmd/mhgo/main_test.go`
- **Creates:**
  - `internal/board/board.go`
  - `internal/board/board_test.go`
  - `internal/board/cli.go`
  - `internal/board/cli_test.go`
  - `internal/board/git.go`
  - `internal/board/git_test.go`
  - `internal/board/layer.go`
  - `internal/board/layer_test.go`
  - `internal/board/lock.go`
  - `internal/board/lock_test.go`
  - `internal/board/render.go`
  - `internal/board/render_test.go`
  - `internal/board/spawn_other.go`
  - `internal/board/spawn_windows.go`
  - `internal/board/store.go`
  - `internal/board/store_test.go`
  - `internal/board/sync.go`
  - `internal/board/sync_test.go`
  - `internal/board/task.go`
  - `internal/board/task_test.go`
  - `internal/board/boardtest/bench_git_test.go`
  - `internal/board/boardtest/bench_test.go`
  - `internal/board/boardtest/concurrency_test.go`
  - `internal/board/boardtest/doc.go`
  - `internal/board/boardtest/integration_test.go`
- **Deletes:**
  - `internal/wiki/cli.go`
  - `internal/wiki/cli_test.go`
  - `internal/wiki/git.go`
  - `internal/wiki/git_test.go`
  - `internal/wiki/layer.go`
  - `internal/wiki/layer_test.go`
  - `internal/wiki/lock.go`
  - `internal/wiki/lock_test.go`
  - `internal/wiki/render.go`
  - `internal/wiki/render_test.go`
  - `internal/wiki/spawn_other.go`
  - `internal/wiki/spawn_windows.go`
  - `internal/wiki/store.go`
  - `internal/wiki/store_test.go`
  - `internal/wiki/sync.go`
  - `internal/wiki/sync_test.go`
  - `internal/wiki/task.go`
  - `internal/wiki/task_test.go`
  - `internal/wiki/wiki.go`
  - `internal/wiki/wiki_test.go`
  - `internal/wiki/wikitest/bench_git_test.go`
  - `internal/wiki/wikitest/bench_test.go`
  - `internal/wiki/wikitest/concurrency_test.go`
  - `internal/wiki/wikitest/doc.go`
  - `internal/wiki/wikitest/integration_test.go`
- **Requirements:** Run, in order: `git mv internal/wiki internal/board`; then
  `git mv internal/board/wikitest internal/board/boardtest`; then
  `git mv internal/board/wiki.go internal/board/board.go` and
  `git mv internal/board/wiki_test.go internal/board/board_test.go`. Then change
  the package declaration `package wiki` → `package board` in every non-test
  `.go` file under `internal/board/`; `package wiki_test` → `package board_test`
  in every external test file under `internal/board/` (cli_test.go,
  board_test.go, store_test.go, render_test.go, sync_test.go, git_test.go,
  lock_test.go, layer_test.go, task_test.go); and `package wikitest` →
  `package boardtest` in every file under `internal/board/boardtest/`. Replace
  the import path `github.com/Knatte18/mhgo/internal/wiki` →
  `github.com/Knatte18/mhgo/internal/board` everywhere it appears (the test
  files and `cmd/mhgo/main.go`). In `cmd/mhgo/main.go`: change the import, the
  `case "wiki": return wiki.RunCLI(...)` to `case "board": return
  board.RunCLI(...)`, and the `wiki` references in the package doc comment (the
  `Modules:` list and usage) to `board`. In `cmd/mhgo/main_test.go`: rename the
  test functions `TestRunDispatchesToWiki` → `TestRunDispatchesToBoard` and
  `TestRunWikiErrorPropagatesExitCode` → `TestRunBoardErrorPropagatesExitCode`,
  and change the dispatched module argument from `"wiki"` to `"board"` in their
  `run([]string{...})` calls (keep `--wiki-path` and `WIKI_SKIP_GIT` here for
  now — they are renamed in cards 3–4). Do NOT yet rename identifiers, env vars,
  or `wiki.` qualifiers inside the moved files; cards 2–4 do that. The tree need
  not compile until the batch is complete.
- **Commit:** `refactor(board): git mv wiki module to board, rename packages`

### Card 2: rename Board facade type, fields, and error types in source

- **Context:**
  - `go.mod`
- **Edits:**
  - `internal/board/board.go`
  - `internal/board/cli.go`
  - `internal/board/store.go`
  - `internal/board/git.go`
  - `internal/board/sync.go`
  - `internal/board/render.go`
  - `internal/board/layer.go`
  - `internal/board/lock.go`
  - `internal/board/task.go`
  - `internal/board/spawn_other.go`
  - `internal/board/spawn_windows.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In the non-test source files, rename the facade type `Wiki`
  → `Board` and its receiver variable `w *Wiki` → `b *Board` in every method in
  `board.go` (`New`, `writeOp`, `UpsertTask`, `SetPhase`, `RemoveTask`,
  `MergeTasks`, `SetDeps`, `UpsertTasksBatch`, `Rerender`, `Sync`, `GetTask`,
  `ListTasksBrief`, `ListTasksFull`); rename the struct field `wikiPath` →
  `boardPath` — this field is defined and used only in `board.go`; `cli.go`'s
  local variable also named `wikiPath` is a different thing and stays unchanged
  (see the "Keep …" note below). Rename the
  exported error types `WikiPushError` → `BoardPushError` and `WikiPathError` →
  `BoardPathError` in `git.go` (definitions) and every use site (`git.go`,
  `sync.go`). Update doc comments that name `Wiki`/`wiki` as the type/module to
  `Board`/`board`. Keep `New(wikiPath string) *Board` taking a single path
  argument named `boardPath` (the signature does not change to a Config until
  batch 3). Keep `defaultWikiPath`, `resolveWikiPath`, the `--wiki-path` flag,
  the local `wikiPath` variable in `RunCLI`, and `MHGO_WIKI_PATH` untouched
  (deleted in batch 4).
- **Commit:** `refactor(board): rename Wiki type and error types to Board`

### Card 3: rename control env vars, commit message, and spawn argument in source

- **Context:**
  - `internal/board/cli.go`
- **Edits:**
  - `internal/board/board.go`
  - `internal/board/sync.go`
  - `internal/board/spawn_other.go`
  - `internal/board/spawn_windows.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rename the control env vars everywhere they are read:
  `WIKI_SKIP_GIT` → `BOARD_SKIP_GIT` (in `board.go` `writeOp` and `sync.go`
  `Sync`) and `WIKI_SKIP_PUSH` → `BOARD_SKIP_PUSH` (in `sync.go` `pushUnpushed`).
  Change the background commit message literal `"wiki sync"` → `"board sync"` in
  `sync.go` `commitDirty`. Change the spawned module argument from `"wiki"` to
  `"board"` in `spawnSync` in both `spawn_windows.go` and `spawn_other.go`
  (the `exec.Command(exe, "wiki", "--wiki-path", ..., "sync")` call — keep
  `--wiki-path` for now; it becomes `--board-path` in batch 4). Update the
  doc-comment references to these env vars and to `mhgo wiki sync`.
- **Commit:** `refactor(board): rename WIKI_SKIP_* env vars and sync message`

### Card 4: rename wiki. qualifiers and refs across all test files

- **Context:**
  - `internal/board/board.go`
  - `internal/board/git.go`
- **Edits:**
  - `internal/board/board_test.go`
  - `internal/board/cli_test.go`
  - `internal/board/store_test.go`
  - `internal/board/render_test.go`
  - `internal/board/sync_test.go`
  - `internal/board/git_test.go`
  - `internal/board/lock_test.go`
  - `internal/board/layer_test.go`
  - `internal/board/task_test.go`
  - `internal/board/boardtest/bench_test.go`
  - `internal/board/boardtest/bench_git_test.go`
  - `internal/board/boardtest/concurrency_test.go`
  - `internal/board/boardtest/integration_test.go`
  - `internal/board/boardtest/doc.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace every `wiki.` package qualifier with `board.` across
  all listed test files (e.g. `wiki.New` → `board.New`, `wiki.Task` →
  `board.Task`, `wiki.RunCLI` → `board.RunCLI`, `wiki.Render` → `board.Render`,
  `wiki.NewStore` → `board.NewStore`, `wiki.AtomicWrite` → `board.AtomicWrite`,
  `wiki.CommitPush` → `board.CommitPush`, `wiki.Pull` → `board.Pull`,
  `wiki.PathGuard` → `board.PathGuard`). Replace the renamed error-type
  references `wiki.WikiPushError`/`wiki.WikiPathError` →
  `board.BoardPushError`/`board.BoardPathError` if present in `git_test.go`.
  Replace `WIKI_SKIP_GIT` → `BOARD_SKIP_GIT` and `WIKI_SKIP_PUSH` →
  `BOARD_SKIP_PUSH` in all `t.Setenv`/`b.Setenv` calls and comments. Update the
  `boardtest/doc.go` package comment (it describes the `wiki` module and imports
  `internal/wiki`) to name `board`/`internal/board`. KEEP the `--wiki-path` flag
  string in `runCLI` helpers and benchmark args (the flag still exists until
  batch 4). KEEP the external test-repo URL
  `https://github.com/Knatte18/mhgo-wiki-test.git` unchanged in
  `integration_test.go` and `bench_git_test.go`. After this card the whole tree
  compiles and `go test ./...` passes.
- **Commit:** `refactor(board): update wiki. qualifiers across test suites`

## Batch Tests

`verify` runs the whole Go suite because the rename is cross-cutting and touches
every file in the package: `go build ./...` (compiles all non-test code),
`go vet -tags integration ./...` (compile-checks the `//go:build integration`
files in `boardtest/` — `integration_test.go` and `bench_git_test.go` — which
`go test ./...` does not build), and `go test ./...` (runs `cmd/mhgo`,
`internal/board`, and the non-integration `boardtest` tests). The full
non-integration suite is unit-only (no network) and runs in seconds, so the
unbounded scope is justified for a rename that must leave the entire tree green.
