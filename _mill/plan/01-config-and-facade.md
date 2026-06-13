# Batch: config-and-facade

```yaml
task: Build mhgo worktree module
batch: config-and-facade
number: 1
cards: 3
verify: go test ./internal/worktree/
depends-on: []
```

## Batch Scope

Establishes the `internal/worktree` package skeleton: the typed `Config`
(holding only `branch_prefix`), the `LoadConfig`/`DefaultConfig` pair that mirrors
`internal/board/config.go` (including the "not initialized → run mhgo init" error
re-wrap), and the `Worktree` facade struct with its `New(cfg)` constructor. This is
a root batch with no dependencies; it defines the `Config` type and `Worktree`
receiver that batches 3 and 4 build subcommand methods on. No subcommand logic
lives here.

External interface the next batches consume: the exported `Config` struct, the
`worktree.New(cfg Config) *Worktree` constructor, and the `*Worktree` receiver type.

## Cards

### Card 1: worktree Config + LoadConfig

- **Context:**
  - `internal/board/config.go`
  - `internal/config/config.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/config.go`
- **Deletes:** none
- **Requirements:** Create `package worktree` with a `Config` struct holding one
  field `BranchPrefix string` with yaml tag `branch_prefix`. Add
  `DefaultConfig() Config` returning `Config{BranchPrefix: ""}`. Add
  `LoadConfig(baseDir, module string) (Config, error)` that builds a defaults map
  `{"branch_prefix": DefaultConfig().BranchPrefix}`, calls
  `config.Load(baseDir, module, defaults)` (import
  `github.com/Knatte18/mhgo/internal/config`), and on error checks
  `strings.Contains(err.Error(), "not initialized")` — if so returns
  `fmt.Errorf("not initialized here; run \"mhgo init\"")`, otherwise returns the raw
  error. On success returns `Config{BranchPrefix: raw["branch_prefix"]}`. Mirror the
  structure and error-wrap of `internal/board/config.go`'s `LoadConfig` exactly
  (no path resolution — worktree has no path field).
- **Commit:** `feat(worktree): add Config and LoadConfig`

### Card 2: worktree config tests

- **Context:**
  - `internal/board/config_test.go`
  - `internal/worktree/config.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/config_test.go`
- **Deletes:** none
- **Requirements:** Create `package worktree_test` importing
  `github.com/Knatte18/mhgo/internal/worktree`. Cover: (1) `_mhgo/` exists but
  `worktree.yaml` absent → `LoadConfig(baseDir, "worktree")` returns
  `Config{BranchPrefix: ""}` with no error; (2) `_mhgo/worktree.yaml` containing
  `branch_prefix: "hanf/"` → `LoadConfig` returns `BranchPrefix == "hanf/"`;
  (3) `_mhgo/` directory missing entirely → error whose message contains
  `run "mhgo init"` (assert on that substring). Build temp dirs with `t.TempDir()`
  and `os.Mkdir`/`os.WriteFile` following the patterns in
  `internal/board/config_test.go`.
- **Commit:** `test(worktree): cover Config defaults and not-initialized rewrap`

### Card 3: Worktree facade

- **Context:**
  - `internal/board/board.go`
  - `internal/worktree/config.go`
- **Edits:** none
- **Creates:**
  - `internal/worktree/worktree.go`
- **Deletes:** none
- **Requirements:** In `package worktree`, define `type Worktree struct { cfg Config }`
  and `func New(cfg Config) *Worktree { return &Worktree{cfg: cfg} }`. Add a package
  doc comment describing the worktree module (lifecycle of git worktrees: add / list /
  remove). Do NOT add Add/List/Remove methods here — they arrive in later batches.
  Keep this file free of any dependency on `links.go` or `internal/git` so it compiles
  standalone alongside only `config.go`.
- **Commit:** `feat(worktree): add Worktree facade and New constructor`

## Batch Tests

`verify: go test ./internal/worktree/` compiles the new package and runs
`config_test.go`. At this point the package contains `config.go`, `config_test.go`,
and `worktree.go` only — all three compile without any later-batch file. The config
tests assert defaults, `branch_prefix` parsing, and the not-initialized error
re-wrap.
