# Plan: Extract shared infrastructure (config, git, lock)

```yaml
task: "Extract shared infrastructure (config, git, lock)"
slug: "extract-shared-infra"
approved: true
started: "20260609T130000Z"
parent: "main"
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: "internal/lock — lift lock primitives"
    file: 01-internal-lock.md
    depends-on: []
    verify: go test ./internal/lock/...
  - number: 2
    name: "internal/git — extract RunGit"
    file: 02-internal-git.md
    depends-on: []
    verify: go test ./internal/git/...
  - number: 3
    name: "internal/config — generic config loader"
    file: 03-internal-config.md
    depends-on: []
    verify: go test ./internal/config/...
  - number: 4
    name: "board migration — adopt all three packages"
    file: 04-board-migration.md
    depends-on: [1, 2, 3]
    verify: go test ./...
```

## Shared Decisions

### Decision: config.Load returns map[string]string

- **Decision:** `internal/config.Load` returns `map[string]string`. Board's `LoadConfig` wraps it and maps to the typed `Config` struct.
- **Rationale:** `internal/config` must be generic — it has no knowledge of board's `Config` shape. The flat map is the natural intermediate for a flat YAML file with env-expanded string values. Board does the mapping in four lines.
- **Applies to:** batch 3, batch 4

### Decision: no new external dependencies

- **Decision:** All three new packages use only existing module dependencies: `gofrs/flock` (lock), `gopkg.in/yaml.v3` (config), stdlib only (git). No `go.mod` changes.
- **Rationale:** All required deps are already present in the module. Adding deps for trivial functionality would be over-engineering.
- **Applies to:** all batches

### Decision: build constraint conventions

- **Decision:** Windows-only files use the `_windows.go` filename suffix as the build constraint (no explicit `//go:build windows`). Non-Windows files use explicit `//go:build !windows` on line 1. This mirrors the existing board convention in `spawn_windows.go` / `spawn_other.go`.
- **Rationale:** Preserves consistency with existing board code. Do NOT add a redundant `//go:build windows` to `_windows.go` files.
- **Applies to:** batch 2 (git_windows.go, git_other.go), batch 4 (spawn_windows.go unchanged)

### Decision: test package and helper conventions

- **Decision:** Tests use the external test package (`package foo_test`). Temp directories use `t.TempDir()`. Env var manipulation uses `t.Setenv()`. No global state or `os.Setenv` in tests.
- **Rationale:** `t.TempDir()` and `t.Setenv()` auto-clean on test exit, preventing leakage between parallel tests. External test packages verify the public API surface only.
- **Applies to:** all batches

### Decision: batch 04 card ordering preserves compilation

- **Decision:** Within batch 04, cards apply changes in this order: (1) update lock import sites (cards 9–11), (2) remove RunGit from board/git.go and update callers (card 12), (3) remove hideProcWindow from spawn files (cards 13–14), (4) delete board/lock.go and board/lock_test.go (card 15), (5) rewrite config files (card 16). Lock.go deletion happens AFTER all call sites have been updated to `lock.*` prefix, so the board package compiles at every intermediate step.
- **Rationale:** Prevents a mid-batch compilation break that would confuse the implementer's incremental verify.
- **Applies to:** batch 4

## All Files Touched

- `internal/board/board.go`
- `internal/board/config.go`
- `internal/board/config_test.go`
- `internal/board/git.go`
- `internal/board/init.go`
- `internal/board/lock.go` _(deleted)_
- `internal/board/lock_test.go` _(deleted)_
- `internal/board/spawn_other.go`
- `internal/board/spawn_windows.go`
- `internal/board/store.go`
- `internal/board/sync.go`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/git/git.go`
- `internal/git/git_other.go`
- `internal/git/git_windows.go`
- `internal/git/git_test.go`
- `internal/lock/lock.go`
- `internal/lock/lock_test.go`
