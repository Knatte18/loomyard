# Batch: weft-module

```yaml
task: 'weft engine: paths geometry, paired worktrees, lyx weft'
batch: weft-module
number: 2
cards: 9
verify: go test ./internal/weft/ ./cmd/lyx/
depends-on: [1]
```

## Batch Scope

This batch delivers the new `internal/weft` package and the `lyx weft <status|commit|push|pull|sync>` command, wired into `cmd/lyx/main.go`. It owns all git into the paired weft worktree (`git -C <weft>`), porting the board pusher model (push-lock-coalesced detached push) into weft. It depends on batch 1's geometry methods. Batch-local decisions: (1) `sync` commits locally (synchronous, no network) then spawns a **push-only** detached worker, so the worker needs only `--weft-path` and never re-derives cwd geometry — the commit's RelPath-scoped pathspec is applied before the spawn; (2) the manual `push` verb commits dirty then runs the same push loop; the detached worker runs push-only. Lock files live in `<weft>/.weft/`, outside the committed pathspec.

## Cards

### Card 5: weft package skeleton + pathspec helper

- **Context:**
  - `internal/paths/paths.go`
  - `internal/board/board.go`
- **Edits:** none
- **Creates:**
  - `internal/weft/weft.go`
- **Deletes:** none
- **Requirements:** New `package weft`. Add a package doc comment explaining weft is the only owner of git into the paired weft worktree (`git -C <weft>`), one-shot and daemonless, mirroring board's git-ownership contract. Add unexported constants `commitMessage = "weft sync"`, `lockDirName = ".weft"`, `writeLockFile = "weft.write.lock"`, `pushLockFile = "weft.push.lock"`. Add `func scopedPathspec(relPath string, dirs []string) []string` returning `filepath.Join(relPath, dir)` for each dir in `dirs` (so at `relPath == "."` `["_lyx"]` → `["_lyx"]`; at `relPath == "sub"` → `["sub/_lyx"]`). No git, no I/O in this file.
- **Commit:** `feat(weft): add package skeleton and pathspec helper`

### Card 6: weft config (pathspec, junction-independent baseDir)

- **Context:**
  - `internal/config/config.go`
  - `internal/board/config.go`
  - `internal/worktree/config.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/weft/config.go`
  - `internal/weft/config_test.go`
- **Deletes:** none
- **Requirements:** Add `type Config struct { Pathspec string \`yaml:"pathspec"\` }`, `func DefaultConfig() Config { return Config{Pathspec: "_lyx"} }`, and `func (c Config) Dirs() []string { return strings.Fields(c.Pathspec) }`. Add `func LoadConfig(weftBaseDir string) (Config, error)` that calls `config.Load(weftBaseDir, "weft", map[string]string{"pathspec": DefaultConfig().Pathspec})` and maps `raw["pathspec"]` into `Config`. The caller passes `weftBaseDir = filepath.Join(layout.WeftWorktree(), layout.RelPath)` — junction-independent (reads the real `_lyx/config/weft.yaml` in the weft worktree, never via the host junction). Tests (`config_test.go`, white-box): default pathspec when no `weft.yaml`; override from a written `<base>/_lyx/config/weft.yaml` containing `pathspec: "_lyx _codeguide"`; `Dirs()` splits on whitespace into `["_lyx","_codeguide"]`; `LoadConfig` errors when `<base>/_lyx` is absent (the missing-weft-worktree case).
- **Commit:** `feat(weft): add weft config with pathspec knob`

### Card 7: weft git core — commit, push loop, pull, locks

- **Context:**
  - `internal/board/sync.go`
  - `internal/board/git.go`
  - `internal/git/git.go`
  - `internal/lock/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/weft/sync.go`
  - `internal/weft/sync_test.go`
- **Deletes:** none
- **Requirements:** Port board's `sync.go` into weft, parameterized by the weft worktree path. Add `func ensureLockDir(weftPath string) (string, error)` that `MkdirAll`s `filepath.Join(weftPath, lockDirName)` and returns it. Add:
  - `func Commit(weftPath string, pathspec []string) (committed bool, err error)` — return `(false, nil)` immediately if `os.Getenv("WEFT_SKIP_GIT") == "1"`; acquire write lock under the lock dir; `git add --` + `pathspec` via `git.RunGit`; `git diff --cached --quiet` (exit 0 → nothing staged → `(false, nil)`; exit 1 → staged); `git commit -m commitMessage`; return `(true, nil)`. Staging is scoped to `pathspec` only — never `git add .` / `add -A`.
  - `func Push(weftPath string) error` — return nil if `WEFT_SKIP_GIT` or `WEFT_SKIP_PUSH` is `"1"`; ensure lock dir; acquire push lock; loop: `unpushed, _ := hasUnpushed(weftPath); if !unpushed { break }; pushUnpushed(weftPath)`. Push-only (no commit) — coalescing comes from the push lock + the loop catching commits that land during a push.
  - `func Pull(weftPath string) error` — return nil if `WEFT_SKIP_GIT`; `git pull --ff-only`.
  - unexported `pushUnpushed(weftPath string) error` (rebase-retry on `non-fast-forward`/`rejected`/`fetch first`, mirroring board) and `hasUnpushed(weftPath string) (bool, error)` (`rev-list --count @{u}..HEAD`; true when no upstream).

  Tests (`sync_test.go`, white-box, offline): build a temp git repo with a committed `_lyx/`; `Commit` stages only the pathspec (write a stray file at repo root → assert it is NOT staged/committed); `Commit` returns `(false,nil)` on a clean tree; `WEFT_SKIP_GIT=1` makes `Commit` a no-op; `scopedPathspec(".", ["_lyx"])` round-trips into a real `add --`. Use `t.Setenv`.
- **Commit:** `feat(weft): port board commit/push/pull git core`

### Card 8: detached push worker spawn

- **Context:**
  - `internal/board/spawn_windows.go`
  - `internal/board/spawn_other.go`
  - `internal/board/board.go`
- **Edits:** none
- **Creates:**
  - `internal/weft/spawn_windows.go`
  - `internal/weft/spawn_other.go`
- **Deletes:** none
- **Requirements:** Mirror board's `spawnSync` as `func spawnPush(weftPath string) error`, building `exec.Command(exe, "weft", "--weft-path", abs, "push")` where `exe, _ = os.Executable()` and `abs, _ = filepath.Abs(weftPath)`. `spawn_windows.go` (`//go:build windows` implicit via filename) uses `SysProcAttr{HideWindow:true, CreationFlags: createNoWindow | createNewProcessGroup}` with the same two const values as board; `spawn_other.go` (`//go:build !windows`) uses `SysProcAttr{Setsid:true}`. Leave stdio nil; `cmd.Start()` without `Wait()`. No test file (the spawn path is exercised end-to-end in the integration test, card 12).
- **Commit:** `feat(weft): add detached push-worker spawn`

### Card 9: weft status (drift + junction integrity)

- **Context:**
  - `internal/paths/paths.go`
  - `internal/git/git.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/weft/status.go`
  - `internal/weft/status_test.go`
- **Deletes:** none
- **Requirements:** Add `func Status(weftWorktree, hostLink, weftLyxDir string, pathspec []string) (map[string]any, error)` returning a map with keys: `weft_worktree` (the path), `branch` (`git rev-parse --abbrev-ref HEAD` in `weftWorktree`), `dirty` (bool — `git status --porcelain --` + pathspec is non-empty), `ahead`/`behind` (ints from `rev-list --count @{u}..HEAD` and `rev-list --count HEAD..@{u}`; both null when no upstream), and junction integrity `junction_ok` (bool) + `junction_reason` (string, empty when ok). Junction check: `os.Lstat(hostLink)` — if missing → `junction_ok=false, reason="host _lyx junction missing"`; if present, resolve its target (`os.Readlink`; on Windows a junction reports `ModeSymlink|ModeIrregular`, so accept either) and compare the cleaned target against `weftLyxDir` → ok when equal, else `junction_ok=false, reason="host _lyx junction points elsewhere"`. Status must complete and return even when `junction_ok=false` (config + git already resolved from the weft worktree). Tests (`status_test.go`): `junction_ok=false` with a missing `hostLink`; `dirty` true/false; `branch` reported; and (guarded for the platform) a positive junction case using `os.Symlink` (skip via `t.Skip` if symlink creation is unprivileged-unavailable). Build the weft worktree as a temp git repo.
- **Commit:** `feat(weft): add status with junction-integrity drift report`

### Card 10: weft CLI router

- **Context:**
  - `internal/board/cli.go`
  - `internal/worktree/cli.go`
  - `internal/output/output.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/weft/cli.go`
  - `internal/weft/cli_test.go`
- **Deletes:** none
- **Requirements:** Add `func RunCLI(out io.Writer, args []string) int`. Parse a `--weft-path` string flag (internal, for the detached push child), mirroring board's `--board-path`. Then:
  - **If `--weft-path` is set:** use it directly; the only valid subcommand is `push` → `Push(weftPath)` → `output.Ok`. Any other subcommand → `output.Err` "subcommand requires a worktree context".
  - **Else (cwd path):** `cwd, _ := paths.Getwd()`; `l, err := paths.Resolve(cwd)` (Err on failure); `weftWorktree := l.WeftWorktree()`; `weftBaseDir := filepath.Join(l.WeftWorktree(), l.RelPath)`; `cfg, err := LoadConfig(weftBaseDir)` (Err on failure); `pathspec := scopedPathspec(l.RelPath, cfg.Dirs())`. Dispatch on subcommand:
    - `status` → `Status(weftWorktree, l.HostLyxLinkHere(), l.WeftLyxDir(), pathspec)` → `output.Ok` with the returned map.
    - `commit` → `Commit(weftWorktree, pathspec)` → `output.Ok{"committed": committed}`.
    - `push` → `Commit(weftWorktree, pathspec)` then `Push(weftWorktree)` → `output.Ok`.
    - `pull` → `Pull(weftWorktree)` → `output.Ok`.
    - `sync` → `Commit(weftWorktree, pathspec)` then `spawnPush(weftWorktree)` → `output.Ok`.
    - unknown → print `unknown subcommand` to stderr, return 1 (mirror board/worktree).
  All errors via `output.Err` (exit 1). Tests (`cli_test.go`): unknown subcommand → exit 1; `status` against a temp host+weft fixture (use `t.Chdir` into a host worktree whose `_lyx` is a symlink to the weft `_lyx`, `WEFT_SKIP_GIT=1`) returns `ok:true` with a `junction_ok` field; `--weft-path` with a non-`push` subcommand → error. Keep fixtures minimal; lean on `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`.
- **Commit:** `feat(weft): add lyx weft CLI router`

### Card 11: wire weft into the module dispatcher

- **Context:**
  - `internal/worktree/cli.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `cmd/lyx/main.go` `run()`, add `case "weft": return weft.RunCLI(out, moduleArgs)` and the `github.com/Knatte18/loomyard/internal/weft` import; add `weft` to the Modules list in the package doc comment. In `cmd/lyx/main_test.go`, add `TestRunDispatchesToWeft` mirroring `TestRunDispatchesToWorktree`: a temp cwd with no `_lyx/`, `run([]string{"weft","status"}, &out)` → exit 1 and `"ok":false` in the output (config/layout resolution fails cleanly through the error envelope).
- **Commit:** `feat(lyx): dispatch the weft module`

### Card 12: weft integration tests (push/pull/sync with a bare remote)

- **Context:**
  - `internal/board/sync.go`
  - `internal/board/git.go`
  - `internal/worktree/testhelpers_test.go`
  - `internal/git/git.go`
- **Edits:** none
- **Creates:**
  - `internal/weft/weft_integration_test.go`
- **Deletes:** none
- **Requirements:** White-box integration tests using a real local bare remote (mirror board's git-backed tests). Build a weft worktree temp repo with `_lyx/`, an `origin` bare remote, and an upstream-tracking branch. Cover: `Push` succeeds and the commit lands on the bare remote (clone/`git -C <bare> log` or `rev-parse` to assert); `Push` rebase-retry on a simulated non-fast-forward (push a competing commit to the bare, then assert `Push` rebases and succeeds); `Pull` fast-forwards a remote-ahead worktree; `sync` (via `Commit` + `spawnPush`) eventually lands the commit on the remote — poll the bare repo for the commit with a bounded retry loop (as board's tests do) since the push is detached. Do NOT set `WEFT_SKIP_PUSH` in these tests (they exercise the real network-to-local-bare path).
- **Commit:** `test(weft): integration tests for push/pull/sync`

### Card 13: update overview weft section + module list

- **Context:**
  - `cmd/lyx/main.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/overview.md`: (1) in the "## Weft overlay model" → "### Status" subsection, update the bullets to record that the Go core (paths geometry, `lyx weft` command) has landed for task 006 and that paired spawn lands with it; soften the "Weft has no Go code yet — portals are still the live mechanism" sentence to reflect that the engine now exists (paired spawn hard-requires a weft repo built by the downstream hub-creator). (2) In the "## Modules" list, add a `weft` entry: "**weft** — owns all git into the paired weft repo (`lyx weft status|commit|push|pull|sync`)." Do not touch the geometry method list (batch 1 owns it).
- **Commit:** `docs(overview): record weft module + status`

## Batch Tests

`verify: go test ./internal/weft/ ./cmd/lyx/` runs the new `internal/weft` package (`config_test.go`, `sync_test.go`, `status_test.go`, `cli_test.go`, `weft_integration_test.go`) and the `cmd/lyx` dispatch tests (including the new `TestRunDispatchesToWeft`). Unit tests run offline via `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`; the integration test (card 12) uses a local bare remote and a bounded poll loop for the detached `sync` push. Card 13 is doc-only.
