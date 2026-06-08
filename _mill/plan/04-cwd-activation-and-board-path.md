# Batch: cwd-activation-and-board-path

```yaml
task: "board-modul (rename fra wiki) + _mhgo-konfigurasjon"
batch: "cwd-activation-and-board-path"
number: 4
cards: 6
verify: go build ./... && go vet -tags integration ./... && go test ./...
depends-on: [3]
```

## Batch Scope

This batch flips the CLI to the cwd-authoritative config model and removes the
old path knobs. `RunCLI` resolves config once at the top via `os.Getwd()` +
`LoadConfig(cwd, "board")`, erroring when `<cwd>/_mhgo/` is absent; the
`--wiki-path` flag, `resolveWikiPath`, `defaultWikiPath`, and `MHGO_WIKI_PATH`
are deleted. A new internal `--board-path` flag lets the detached `sync` child
bypass `LoadConfig` and the `_mhgo/` check entirely (the path is injected, not
resolved). `writeOp` gains an up-front `MkdirAll(boardPath)` before the write
lock (the lock file lives inside the board dir), and the read methods
short-circuit when the board dir is absent without taking the swap lock. The
spawner passes the resolved absolute board path via `--board-path`. All affected
tests (`cli_test.go`, `main_test.go`, and the CLI-driven benchmarks) are
re-architected to the cwd model. See the overview Shared Decisions for the exact
contracts (the discussion decisions `config-location`, `spawn-sync-path`,
`board-dir-autocreate`, and the read short-circuit under Gotchas).

Batch-local decisions: (1) tests drive the CLI by `t.Chdir`/`b.Chdir` into a
temp cwd seeded with `_mhgo/board.yaml` (Go 1.24+ `testing.TB.Chdir`; this
forces those tests/benchmarks to run non-parallel, which is acceptable). The
seed `board.yaml` sets `path: board` so the board dir is `<cwd>/board`, kept
inside the temp tree. (2) When `--board-path` is set, `RunCLI` builds the
`Board` from `DefaultConfig()` with `Path` overridden — output names are
irrelevant because the only command the child runs is `sync`, which never
renders.

## Cards

### Card 12: cli.go — cwd/config activation, --board-path bypass, remove flag

- **Context:**
  - `internal/board/config.go`
  - `internal/board/board.go`
- **Edits:**
  - `internal/board/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rework `RunCLI`'s path resolution. Delete the `--wiki-path`
  flag, the `resolveWikiPath` function, the `defaultWikiPath` constant, and all
  reference to `MHGO_WIKI_PATH`. Add an internal hidden flag `boardPathFlag :=
  fs.String("board-path", "", "internal: injected absolute board dir for the
  detached sync child")`. After flag parse: if `*boardPathFlag != ""`, build
  `cfg := DefaultConfig(); cfg.Path = *boardPathFlag` and skip both `LoadConfig`
  and the `_mhgo/` existence check (the path is injected); else call `cwd, err
  := os.Getwd()` (on error → `outputError`), then `cfg, err := LoadConfig(cwd,
  "board")` and on error `return outputError(out, err.Error())` (this is how the
  "not initialized here; run \"mhgo init\"" message reaches the user, as
  single-line JSON with exit 1). Then `b := New(cfg)` and dispatch subcommands
  unchanged. Keep the `--board-path` flag out of the public usage string (the
  discussion calls it internal); the usage line becomes `mhgo board <subcommand>
  [json-payload]`. Update the package/RunCLI doc comment to describe the cwd
  model and drop the flag/env precedence text.
- **Commit:** `feat(board): cwd-authoritative config in RunCLI, drop --wiki-path`

### Card 13: board.go — mkdir-first in writeOp, read short-circuit

- **Context:**
  - `internal/board/store.go`
  - `internal/board/git.go`
- **Edits:**
  - `internal/board/board.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `writeOp`, make `os.MkdirAll(b.boardPath, 0o755)` the
  FIRST statement (before `AcquireWriteLock`), returning a wrapped error on
  failure — the write lock opens `<boardPath>/tasks.json.lock`, so the board dir
  must exist before the lock is taken (discussion `board-dir-autocreate`). In the
  three read methods `GetTask`, `ListTasksBrief`, `ListTasksFull`: before
  constructing the `Store` / taking any lock, `os.Stat(b.boardPath)`; if it does
  not exist (`os.IsNotExist`), return the empty result without side effects —
  `GetTask` → `(Task{}, false, nil)`, `ListTasksBrief` → `(nil, nil)`,
  `ListTasksFull` → `(nil, nil)`. Reads must NOT `MkdirAll` (no filesystem side
  effects on read). Other read errors continue to surface as today.
- **Commit:** `feat(board): mkdir board dir before write lock, short-circuit reads`

### Card 14: spawn — pass resolved absolute board path via --board-path

- **Context:**
  - `internal/board/board.go`
- **Edits:**
  - `internal/board/spawn_windows.go`
  - `internal/board/spawn_other.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `spawnSync(boardPath string)` in both files, resolve the
  absolute path first — `abs, err := filepath.Abs(boardPath)` (return the error
  if it fails) — and change the command to `exec.Command(exe, "board",
  "--board-path", abs, "sync")` (replacing the `"--wiki-path", boardPath`
  arguments from batch 1). Add the `path/filepath` import where needed. The
  detached child therefore receives an unambiguous absolute path and (per card
  12) skips config resolution and the `_mhgo/` check.
- **Commit:** `feat(board): spawn sync child with absolute --board-path`

### Card 15: cli_test.go — re-architect to the cwd model

- **Context:**
  - `internal/board/cli.go`
  - `internal/board/config.go`
- **Edits:**
  - `internal/board/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rewrite the `runCLI` helper and tests for the cwd model. The
  helper no longer prepends `--wiki-path`; instead each test creates `cwd :=
  t.TempDir()`, writes `<cwd>/_mhgo/board.yaml` with contents `path: board\n`
  (so the board dir is `<cwd>/board`), calls `t.Chdir(cwd)`, sets
  `BOARD_SKIP_GIT=1`, and invokes `board.RunCLI(&buf, args)` with the bare
  subcommand args. Add a new test asserting that running a subcommand from a cwd
  WITHOUT `_mhgo/` returns exit 1 and JSON whose `error` contains `not
  initialized`. Update the `Home.md` existence assertions
  (`TestCLIRerender`, and any in the upsert path) to check `<cwd>/board/Home.md`.
  Keep the JSON-contract assertions (ok/task/tasks/error) otherwise unchanged.
  Note `t.Chdir` makes these tests non-parallel — do not add `t.Parallel()`.
- **Commit:** `test(board): drive CLI tests via cwd-seeded _mhgo/board.yaml`

### Card 16: main_test.go — re-architect dispatch tests to the cwd model

- **Context:**
  - `internal/board/cli.go`
  - `internal/board/config.go`
- **Edits:**
  - `cmd/mhgo/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update `TestRunDispatchesToBoard` and
  `TestRunBoardErrorPropagatesExitCode` (renamed in batch 1) to the cwd model:
  create `cwd := t.TempDir()`, write `<cwd>/_mhgo/board.yaml` with `path:
  board\n`, `t.Chdir(cwd)`, set `BOARD_SKIP_GIT=1`, and call `run([]string{
  "board", "rerender"}, &out)` / `run([]string{"board", "remove",
  `+"`"+`{"id_or_slug":"nope"}`+"`"+`}, &out)` — i.e. drop the `--wiki-path`
  argument. Assertions (exit code, JSON `ok`) are otherwise unchanged.
  `TestRunNoArgs` and `TestRunUnknownModule` are unaffected.
- **Commit:** `test(mhgo): cwd-model dispatch tests for board`

### Card 17: boardtest/bench_test.go — re-architect CLI benchmarks

- **Context:**
  - `internal/board/cli.go`
  - `internal/board/config.go`
- **Edits:**
  - `internal/board/boardtest/bench_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Re-architect the CLI-driven benchmarks
  (`BenchmarkUpsert`, `BenchmarkGet`, `BenchmarkList`) for the cwd model, since
  `--wiki-path` no longer exists. Replace the `seedWiki` helper (or add a
  variant) so it creates a temp cwd containing `_mhgo/board.yaml` (with `path:
  board\n`) AND a seeded `<cwd>/board/tasks.json` of `n` tasks, returning the
  cwd. Each benchmark calls `b.Chdir(cwd)` once before the timed loop and invokes
  `board.RunCLI(io.Discard, args)` with bare args (no `--wiki-path`). Keep
  `BenchmarkRender` and `BenchmarkUpsertFacade` (facade-based, already updated in
  batch 3) as they are. Add a short comment that the CLI-bench numbers now
  include the added `os.Getwd()` + `LoadConfig` cost (per discussion). `b.Chdir`
  is Go 1.24+ and makes the benchmark non-parallel — acceptable.
- **Commit:** `test(board): re-architect CLI benchmarks for cwd model`

## Batch Tests

`verify: go build ./... && go vet -tags integration ./... && go test ./...`.
The change spans the CLI, facade, spawner, and their tests, so the full build +
non-integration test run is the right scope. `go vet -tags integration ./...`
compile-checks the integration-gated `boardtest` files (which reference the
package API and must still build). `cli_test.go` and `main_test.go` exercise the
new cwd activation (including the "not initialized" error path); the
re-architected `bench_test.go` must compile (benchmarks are not run by `go test`
without `-bench`, but they are type-checked).
