# Plan: weft engine: paths geometry, paired worktrees, lyx weft

```yaml
task: 'weft engine: paths geometry, paired worktrees, lyx weft'
slug: weft-engine
approved: false
started: '20260619-153153'
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: paths-weft-geometry
    file: 01-paths-weft-geometry.md
    depends-on: []
    verify: go test ./internal/paths/
  - number: 2
    name: weft-module
    file: 02-weft-module.md
    depends-on: [1]
    verify: go test ./internal/weft/ ./cmd/lyx/
  - number: 3
    name: worktree-paired-spawn
    file: 03-worktree-paired-spawn.md
    depends-on: [1]
    verify: go test ./internal/worktree/
  - number: 4
    name: ide-watcher-exclude
    file: 04-ide-watcher-exclude.md
    depends-on: []
    verify: go test ./internal/ide/
```

## Shared Decisions

### Decision: geometry-only-in-paths

- **Decision:** Every host↔weft path (both junction ends, weft worktree, weft repo root, config baseDir) is derived through a `Layout` method in `internal/paths`. `internal/weft` and `internal/worktree` NEVER assemble these paths with raw `filepath.Join` on `Hub`/`WorktreeRoot`; they call the `Layout` accessors added in batch 1.
- **Rationale:** `CONSTRAINTS.md` makes `internal/paths` the sole owner of geometry; `enforcement_test.go` scans the tree for raw `os.Getwd`/`git rev-parse --show-toplevel`. The user's explicit rule: "all paths in one place."
- **Applies to:** all batches

### Decision: git-via-RunGit-with-cwd

- **Decision:** All git invocations go through `git.RunGit(args, cwd)` with an explicit `cwd`; no process ever `cd`s. Weft git targets the weft worktree (`git -C <weft>` ≙ `RunGit(args, weftPath)`); spawn git targets the weft repo root (`RunGit(args, WeftRepoRoot)`). Non-zero exit is `err==nil` with a non-zero `exitCode` — check `exitCode`, not `err`.
- **Rationale:** matches the board/worktree prototype of the git-ownership contract.
- **Applies to:** weft-module, worktree-paired-spawn

### Decision: env-skip-guards

- **Decision:** `WEFT_SKIP_GIT=1` disables the entire weft git/sync path; `WEFT_SKIP_PUSH=1` commits locally but skips push + the detached spawn. Read via `os.Getenv("WEFT_SKIP_GIT")=="1"` etc., inline (no shared constant), exactly like board's `BOARD_SKIP_GIT`/`BOARD_SKIP_PUSH`. Both guards are honored in BOTH `internal/weft` and the spawn-time weft `push -u` in `internal/worktree`.
- **Rationale:** lets unit tests exercise file/junction/commit logic offline; integration tests use a local bare remote.
- **Applies to:** weft-module, worktree-paired-spawn

### Decision: go-test-conventions

- **Decision:** White-box (`package x`) `_test.go` next to source. Build synthetic git repos in `t.TempDir()` via the existing `mustRun(t, dir, "git", ...)` pattern (see `internal/worktree/testhelpers_test.go`). Each batch's `verify:` runs only that package's tests (`go test ./internal/<pkg>/`). No `PYTHONPATH` prefix — this is a Go project.
- **Rationale:** mirrors `worktree`/`board` test style; per-package verify keeps runs fast.
- **Applies to:** all batches

### Decision: json-output-envelope

- **Decision:** Every `RunCLI` path emits JSON via `internal/output`: `output.Ok(out, map[string]any{...})` (exit 0) / `output.Err(out, msg)` (exit 1). New `lyx weft` follows the board/worktree CLI shape.
- **Rationale:** consistent CLI surface; `main.go` stays a thin dispatcher.
- **Applies to:** weft-module

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/main.go`
- `cmd/lyx/main_test.go`
- `docs/overview.md`
- `internal/ide/vscode.go`
- `internal/ide/vscode_test.go`
- `internal/paths/paths.go`
- `internal/paths/weft_test.go`
- `internal/weft/cli.go`
- `internal/weft/cli_test.go`
- `internal/weft/config.go`
- `internal/weft/config_test.go`
- `internal/weft/spawn_other.go`
- `internal/weft/spawn_windows.go`
- `internal/weft/status.go`
- `internal/weft/status_test.go`
- `internal/weft/sync.go`
- `internal/weft/sync_test.go`
- `internal/weft/weft.go`
- `internal/weft/weft_integration_test.go`
- `internal/worktree/add.go`
- `internal/worktree/add_test.go`
- `internal/worktree/remove.go`
- `internal/worktree/remove_test.go`
- `internal/worktree/testhelpers_test.go`
- `internal/worktree/weft.go`
- `internal/worktree/weft_test.go`
