# Batch: muxpoc-paths-migration

```yaml
task: 'Extend worktree module: portals and launchers'
batch: 'muxpoc-paths-migration'
number: 5
cards: 4
verify: go test ./internal/muxpoc/...
depends-on: [1]
```

## Batch Scope

This batch eliminates the cwd-≠-worktree-root bug in `muxpoc` (a POC, but the
operator wants the bug class gone everywhere) by anchoring both the psmux session
identity and the `.mhgo/` state directory on the worktree root resolved via
`paths`, instead of `os.Getwd()` at each call site. The session/state no longer
silently split when muxpoc runs from a subfolder. `state.go`'s `LoadState` /
`SaveState` / `DeleteState` / `socketName` already accept a path argument and are
unchanged — only their callers change to pass the worktree root. This batch is
independent of the worktree/board/ide batches (disjoint files).

## Cards

### Card 15: Resolve worktree root once in RunCLI

- **Context:**
  - `internal/paths/paths.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/muxpoc/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add an exported `WorktreeRoot string` field to the `Config`
  struct in `internal/muxpoc/cli.go`. In `RunCLI`, after `fs.Parse`, obtain the
  cwd via `paths.Getwd()` and resolve `l, err := paths.Resolve(cwd)`; on error
  emit a JSON error via `output.Err(out, ...)` and return 1. Set
  `cfg.WorktreeRoot = l.WorktreeRoot` before dispatching to the subcommands.
  (`muxpoc` requires a git repo now — it is always meant to run inside a
  worktree.)
- **Commit:** `refactor(muxpoc): resolve worktree root via paths in RunCLI`

### Card 16: Subcommands anchor state/socket on the worktree root

- **Context:**
  - `internal/muxpoc/state.go`
  - `internal/muxpoc/cli.go`
- **Edits:**
  - `internal/muxpoc/up.go`
  - `internal/muxpoc/down.go`
  - `internal/muxpoc/status.go`
  - `internal/muxpoc/attach.go`
  - `internal/muxpoc/review.go`
  - `internal/muxpoc/daemon.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In each subcommand (`cmdUp`, `cmdDown`, `cmdStatus`,
  `cmdAttach`, `cmdReview`, `cmdDaemon`), replace the local `cwd, _ := os.Getwd()`
  / `cwd, err := os.Getwd()` with `cwd := cfg.WorktreeRoot` and drop the
  getwd-error handling that becomes dead. The downstream calls
  (`LoadState(cwd)`, `SaveState(cwd, …)`, `DeleteState(cwd)`,
  `socketName(cwd)`, `coldStart(out, cfg, cwd, mux)`, `coldRecover(out, cfg, cwd,
  …)`) stay as-is but now receive the worktree root. Remove the now-unused `os`
  import from any file where it is no longer referenced.
- **Commit:** `fix(muxpoc): anchor state and socket on the worktree root`

### Card 17: PsmuxCmd derives the socket from config, not os.Getwd

- **Context:**
  - `internal/muxpoc/cli.go`
  - `internal/muxpoc/state.go`
- **Edits:**
  - `internal/muxpoc/cmd.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Delete the `socketArg()` helper in `internal/muxpoc/cmd.go`
  (it calls `os.Getwd()`, which must not exist outside `paths`). Change
  `PsmuxCmd.run` and `PsmuxCmd.output` to build the `-L <socket>` argument from
  `socketName(p.cfg.WorktreeRoot)` instead of `socketArg()`. Remove the now-unused
  `os` import if nothing else in `cmd.go` needs it.
- **Commit:** `fix(muxpoc): build psmux socket name from configured worktree root`

### Card 18: Update smoke test (git repo) and socket-stability assertion

- **Context:**
  - `internal/paths/paths.go`
  - `internal/muxpoc/state.go`
- **Edits:**
  - `internal/muxpoc/muxpoc_smoke_test.go`
  - `internal/muxpoc/state_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `muxpoc_smoke_test.go` (`//go:build smoke`), after
  creating the temp dir and `os.Chdir`-ing into it, initialize it as a git repo
  (`git init -b main`, `git config user.email/user.name`, an initial commit) so
  the migrated `cmd*` functions — which now call `paths.Resolve` via
  `cfg.WorktreeRoot` set in `RunCLI` — succeed; where the test constructs `Config`
  directly and calls `cmdUp`/`cmdStatus`/etc., set `cfg.WorktreeRoot` to the temp
  dir (its now-git worktree root). Update the `LoadState(cwd)` assertion to use
  that same worktree root. In `state_test.go`, add a test documenting the fix:
  `socketName` of a worktree root differs from `socketName` of a subdirectory of
  it (`filepath.Join(root, "sub")`), proving why callers must pass the worktree
  root rather than the raw cwd; keep the existing `socketName`/state tests
  passing.
- **Commit:** `test(muxpoc): git-init smoke temp dir and pin socket stability`

## Batch Tests

`verify: go test ./internal/muxpoc/...` runs the default muxpoc suite
(`state_test.go`, `cmd_test.go`, `cli_test.go`). The full-lifecycle smoke test is
`//go:build smoke`-gated and excluded from this default run — it additionally
requires Windows + a real `psmux.exe`, so it is not part of CI verify; its update
here keeps it runnable under `-tags smoke` on a dev machine. Scope is the single
`muxpoc` package this batch touches.
