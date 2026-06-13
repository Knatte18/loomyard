# Plan: Build mhgo worktree module

```yaml
task: Build mhgo worktree module
slug: mhgo-worktree-module
approved: false
started: 20260613-133159
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: config-and-facade
    file: 01-config-and-facade.md
    depends-on: []
    verify: go test ./internal/worktree/
  - number: 2
    name: links-teardown-helper
    file: 02-links-teardown-helper.md
    depends-on: []
    verify: go test ./internal/worktree/
  - number: 3
    name: subcommands
    file: 03-subcommands.md
    depends-on: [1, 2]
    verify: go test ./internal/worktree/
  - number: 4
    name: cli-router
    file: 04-cli-router.md
    depends-on: [3]
    verify: go test ./internal/worktree/
  - number: 5
    name: integration-and-docs
    file: 05-integration-and-docs.md
    depends-on: [4]
    verify: go test ./...
```

## Shared Decisions

### Decision: Go project, native test runner

- **Decision:** `verify:` commands use `go test` directly (e.g. `go test ./internal/worktree/`), with no `PYTHONPATH=` prefix. The `PYTHONPATH=` isolation prefix is a Python/mill convention and does not apply to Go.
- **Rationale:** This is a Go module (`github.com/Knatte18/mhgo`, go 1.26). Tests run via the standard toolchain.
- **Applies to:** all batches.

### Decision: domain methods take an explicit source directory, never read global cwd

- **Decision:** `Add`, `List`, and `Remove` take the source worktree directory as their first parameter (`sourceDir string`). `cli.go` resolves it once via `os.Getwd()` and passes it in. Domain methods never call `os.Getwd()` themselves.
- **Rationale:** keeps the domain layer pure and unit-testable without `t.Chdir` (process-global state). Mirrors the cwd-authoritative design in `discussion.md` while isolating the `os.Getwd()` call to the CLI boundary.
- **Applies to:** subcommands, cli-router.

### Decision: RunGit exit-code discipline

- **Decision:** every `git` call goes through `internal/git.RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)`. `err != nil` means the process failed to launch (treat as a hard error). A non-zero `exitCode` with `err == nil` means git ran and reported a failure — inspect `exitCode`/`stderr`, never rely on `err` for git-level failures.
- **Rationale:** `RunGit` deliberately swallows non-zero exits into `exitCode` (see `internal/git/git.go`). Code that checks only `err` would miss every git error.
- **Applies to:** subcommands (add, list, remove).

### Decision: JSON envelope via internal/output

- **Decision:** CLI output uses `internal/output.Ok(w, map[string]any{...})` (injects `"ok":true`, exit 0) and `internal/output.Err(w, msg)` (`"ok":false`, exit 1). Domain methods return typed result structs; `cli.go` builds the map literal per subcommand.
- **Rationale:** matches the established board/muxpoc envelope shape exactly.
- **Applies to:** cli-router, integration.

### Decision: test helpers shared across git-backed tests

- **Decision:** git-backed tests (`add`, `list`, `remove`, `cli`) live in `package worktree_test` (black-box) and share `helpers_test.go` (`mustRun`, `newTestRepo`, `addRemote`). `newTestRepo` returns a hub directory nested one level under a temp container (`<tempdir>/hub`) so the container (`filepath.Dir(hub)`) is controllable. `links_test.go` is white-box `package worktree` (it tests the unexported `removeLinks`) and does not use these helpers.
- **Rationale:** `Add`/`List`/`Remove` compute `container = filepath.Dir(sourceDir)` and place new worktrees as siblings of the source; a controlled nested layout lets tests assert sibling paths and pre-create collisions deterministically.
- **Applies to:** subcommands, cli-router.

## All Files Touched

- `cmd/mhgo/main.go`
- `cmd/mhgo/main_test.go`
- `docs/modules/worktree.md`
- `internal/board/init.go`
- `internal/board/init_test.go`
- `internal/worktree/add.go`
- `internal/worktree/add_test.go`
- `internal/worktree/cli.go`
- `internal/worktree/cli_test.go`
- `internal/worktree/config.go`
- `internal/worktree/config_test.go`
- `internal/worktree/helpers_test.go`
- `internal/worktree/links.go`
- `internal/worktree/links_test.go`
- `internal/worktree/list.go`
- `internal/worktree/list_test.go`
- `internal/worktree/remove.go`
- `internal/worktree/remove_test.go`
- `internal/worktree/worktree.go`
