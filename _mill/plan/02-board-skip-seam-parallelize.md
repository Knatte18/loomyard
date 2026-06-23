# Batch: board-skip-seam-parallelize

```yaml
task: "Speed up and stabilize the integration test tier"
batch: board-skip-seam-parallelize
number: 2
cards: 8
verify: go build ./... && go test -tags integration ./internal/board/... -count=1
depends-on: [1]
```

## Batch Scope

Replace the process-global `BOARD_SKIP_GIT` / `BOARD_SKIP_PUSH` env seams with explicit
config fields / function params, resolved from env exactly once at the `RunCLI` entry
point (see overview Shared Decisions). This lets every local git test in `boardtest`
(`git_test.go`, `sync_test.go`) drop its `t.Setenv` calls and run `t.Parallel()`,
collapsing the package's ~26s serial floor toward its slowest single test (~9s) — the
"speed up" half of the task. `concurrency_test.go` (a Tier-1 test) is converted too for
consistency. Production behaviour is preserved: the detached `lyx board sync` child still
honors an inherited `BOARD_SKIP_PUSH=1` because `RunCLI` folds env into `cfg`.

External interface this batch establishes (consumed by its own tests): `board.Config`
gains `SkipGit`/`SkipPush`; package `Sync` and `CommitPush` gain explicit skip params.
Batch-local decision: package `Sync`/`CommitPush` take plain bool params, not functional
options (overview Shared Decisions explains why).

## Cards

### Card 3: Add SkipGit/SkipPush to Config and Board

- **Context:**
  - `internal/board/sync.go`
- **Edits:**
  - `internal/board/config.go`
  - `internal/board/board.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `config.go`, add `SkipGit bool` and `SkipPush bool` fields to the
  `Config` struct (no yaml tag — these are set programmatically, not from config files; add
  a short comment that they are populated from `BOARD_SKIP_*` env at the CLI entry). In
  `board.go`: add `skipGit bool` and `skipPush bool` fields to the `Board` struct; in
  `New(cfg Config)` set `skipGit: cfg.SkipGit, skipPush: cfg.SkipPush`. In `writeOp`,
  replace the spawn gate `if os.Getenv("BOARD_SKIP_GIT") != "1"` (board.go:83) with
  `if !b.skipGit`. Keep the `os` import (still used by `os.MkdirAll`/`os.Stat` elsewhere in
  board.go). Do not change `(b *Board) Sync()` yet — that is card 5.
- **Commit:** `feat(board): add SkipGit/SkipPush config and Board fields`

### Card 4: Resolve BOARD_SKIP_* env into cfg once in RunCLI

- **Context:**
  - `internal/board/config.go`
  - `internal/board/board.go`
- **Edits:**
  - `internal/board/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `RunCLI` (`cli.go`), immediately after `cfg` is fully resolved (after
  the `if *boardPathFlag != "" { … } else { … LoadConfig … }` block, ~cli.go:83, before
  `fs.Args()`), fold env into cfg: `if os.Getenv("BOARD_SKIP_GIT") == "1" { cfg.SkipGit = true }`
  and `if os.Getenv("BOARD_SKIP_PUSH") == "1" { cfg.SkipPush = true }`. This is the single
  production env read; it covers both the normal cwd path and the `--board-path` detached
  sync child (which inherits the env). `os` is already imported in cli.go. Extract this into
  a small helper `applySkipEnv(cfg Config) Config` (returns cfg with the two bools OR-ed from
  env) so it is unit-testable; call it from `RunCLI`. Place `applySkipEnv` in `cli.go`.
- **Commit:** `feat(board): fold BOARD_SKIP_* env into cfg at the CLI entry`

### Card 5: Thread skip flags through package Sync

- **Context:**
  - `internal/board/config.go`
- **Edits:**
  - `internal/board/sync.go`
  - `internal/board/board.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `sync.go`, change `func Sync(boardPath string) error` to
  `func Sync(boardPath string, skipGit, skipPush bool) error`; replace
  `if os.Getenv("BOARD_SKIP_GIT") == "1"` (sync.go:32) with `if skipGit`. Change
  `func pushUnpushed(boardPath string) error` to `func pushUnpushed(boardPath string, skipPush bool) error`;
  replace `if os.Getenv("BOARD_SKIP_PUSH") == "1"` (sync.go:103) with `if skipPush`; update
  the call inside `Sync`'s loop to `pushUnpushed(boardPath, skipPush)`. Keep the `os` import
  (still used by `os.ReadFile`/`os.OpenFile` in `ensureLockfilesIgnored`). In `board.go`,
  change `(b *Board) Sync()` (board.go:172-173) to `return Sync(b.boardPath, b.skipGit, b.skipPush)`.
- **Commit:** `refactor(board): pass skip flags to Sync instead of reading env`

### Card 6: Thread skipPush param through CommitPush

- **Context:**
  - `internal/board/sync.go`
- **Edits:**
  - `internal/board/git.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `git.go`, change `func CommitPush(boardPath string, relPaths []string, message string) error`
  to `func CommitPush(boardPath string, relPaths []string, message string, skipPush bool) error`;
  replace `if os.Getenv("BOARD_SKIP_PUSH") == "1"` (git.go:69) with `if skipPush`. Remove the
  now-unused `os` import from git.go (line 69 is its only `os.` reference). `Pull` is
  unchanged. Note: `CommitPush` has no production callers (only `boardtest`), so this param
  is consumed solely by tests — that is expected.
- **Commit:** `refactor(board): pass skipPush to CommitPush instead of reading env`

### Card 7: Parallelize git_test.go via explicit params

- **Context:**
  - `internal/board/git.go`
  - `internal/board/sync.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/board/boardtest/git_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update every `board.CommitPush(path, paths, msg)` call in `git_test.go`
  to pass the new `skipPush` argument: `true` where the subtest previously did
  `t.Setenv("BOARD_SKIP_PUSH", "1")`, `false` where it did `t.Setenv("BOARD_SKIP_PUSH", "")`
  (the non-FF rebase subtest). Remove all `t.Setenv("BOARD_SKIP_PUSH", …)` calls. Add
  `t.Parallel()` to `TestPull` and `TestCommitPush` (and to each subtest of `TestCommitPush`
  that no longer sets env). Each test already uses its own `lyxtest` fixture in `t.TempDir`,
  so they are isolation-safe once `t.Setenv` is gone. `board.Pull` calls are unchanged
  (no param).
- **Commit:** `test(board): parallelize git_test.go with explicit skipPush`

### Card 8: Parallelize sync_test.go and assert the skip seam

- **Context:**
  - `internal/board/board.go`
  - `internal/board/sync.go`
  - `internal/board/config.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/board/boardtest/sync_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `sync_test.go`, replace every `t.Setenv("BOARD_SKIP_GIT", …)` /
  `t.Setenv("BOARD_SKIP_PUSH", …)` with the corresponding `cfg` field set before
  `board.New(cfg)` — e.g. the `BOARD_SKIP_PUSH=1` case sets `cfg.SkipPush = true`; the
  `t.Setenv("BOARD_SKIP_GIT", "")` ambient-neutralizer is simply deleted (tests no longer
  read env). Add `t.Parallel()` to each top-level test that no longer sets env. Add a focused
  assertion (a subtest or a small new test) that the seam works: with `cfg.SkipPush = true`,
  `board.New(cfg).Sync()` commits locally but leaves an unpushed commit (assert `@{u}` is
  behind `HEAD`); with `cfg.SkipGit = true`, `Sync()` is a no-op (no commit created). Use
  `lyxtest.CopyWeft` fixtures as the existing tests do.
- **Commit:** `test(board): parallelize sync_test.go and cover the skip seam`

### Card 9: Convert concurrency_test.go to cfg.SkipGit

- **Context:**
  - `internal/board/board.go`
  - `internal/board/config.go`
- **Edits:**
  - `internal/board/boardtest/concurrency_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `concurrency_test.go`, replace `t.Setenv("BOARD_SKIP_GIT", "1")` in
  `TestConcurrentReadsDuringUpserts` and `TestConcurrentUpsertsDoNotLoseWrites` with
  `cfg.SkipGit = true` on the `board.Config` used to build the `Board` (so `writeOp` does not
  spawn a detached sync). Add `t.Parallel()` to both tests. Note: this file has no
  `//go:build integration` tag (it is a Tier-1 test); the conversion is for consistency and
  parallel-safety and does not affect Tier 2 wall-clock. Leave the benchmark helper in this
  file (if any) consistent with its existing `b.Setenv` usage unless it also builds a Board
  from cfg.
- **Commit:** `test(board): convert concurrency_test.go to cfg.SkipGit`

### Card 10: Unit-test the env→cfg resolution helper

- **Context:**
  - `internal/board/cli.go`
  - `internal/board/config.go`
- **Edits:**
  - `internal/board/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add a Tier-1 unit test for `applySkipEnv` (from card 4) in
  `cli_test.go`: with `BOARD_SKIP_GIT=1` set via `t.Setenv`, `applySkipEnv(Config{})` returns
  a Config with `SkipGit == true` and `SkipPush == false`; with `BOARD_SKIP_PUSH=1`, the
  reverse; with neither set, both false; with an already-true `cfg.SkipPush`, env-unset does
  not clear it. This test legitimately uses `t.Setenv` (it is asserting the env-resolution
  behaviour itself) and therefore stays serial — that is correct and expected. It needs no
  git, so it requires no `integration` tag.
- **Commit:** `test(board): unit-test applySkipEnv env resolution`

## Batch Tests

`verify: go build ./... && go test -tags integration ./internal/board/... -count=1` — the
`go build ./...` leg catches the production signature changes (`Sync`, `CommitPush`,
`RunCLI`, `New`) compiling across `cmd/lyx` and any caller; the `go test -tags integration
./internal/board/...` leg runs both the Tier-1 `internal/board` tests (incl. the new
`cli_test.go` case and `concurrency_test.go`) and the Tier-2 `internal/board/boardtest`
tests (`git_test.go`, `sync_test.go`) now running in parallel. Scope is the `board` module
subtree only — this batch touches nothing outside `internal/board`.
