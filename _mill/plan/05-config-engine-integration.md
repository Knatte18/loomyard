# Batch: config-engine-integration

```yaml
task: "Extract yamlengine and migrate config via lyx update"
batch: config-engine-integration
number: 5
cards: 8
verify: go test ./internal/config/ ./internal/board/... ./internal/worktree/ ./internal/weft/ ./internal/ide/
depends-on: [1, 2, 3, 4]
```

## Batch Scope

Rewire the config layer onto the new engine: make `internal/config.Load` strict and
backed by `yamlengine` + `envsource`, delete the old inline grammar
(`expandEnv`/`envOptRe`/`envReqRe`/`loadDotEnv`/`loadYAMLLayer`), remove every
`DefaultConfig()`/`DefaultOutputs()`, and switch the three typed wrappers to pass
their embedded template and unmarshal the resolved YAML into their structs. The
public wrapper APIs (`board.LoadConfig(baseDir, module)` etc.) are unchanged — only
the internal `config.Load` signature changes — so callers outside this batch keep
compiling. The error-ignoring `ide/menu.go` consumer is fixed to handle the strict
error instead of HealthChecking an empty board path. All `DefaultConfig`-dependent
tests in the `board` package are updated. This batch is atomic: the `config.Load`
signature change and all three wrappers land together so the build never breaks.

Batch-local decisions: `config.Load` returns the resolved YAML as `[]byte` (the
wrapper unmarshals into its own struct); the strict missing-key check uses
`yamlengine.MissingKeys` (presence-based on key-paths). Strict errors must name the
config file path, the missing key-paths, and direct the user to `lyx update`.

## Cards

### Card 8: strict config.Load backed by yamlengine + envsource

- **Context:**
  - `internal/yamlengine/resolve.go`
  - `internal/yamlengine/reconcile.go`
  - `internal/envsource/envsource.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/config/config.go`
  - `internal/config/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Rewrite `func Load` in `internal/config/config.go` to the new signature `func Load(baseDir, module string, template []byte) ([]byte, error)`. Flow: (1) call `FindBaseDir(baseDir)` and propagate its error; (2) compute `cfgPath := paths.ConfigFile(baseDir, module)` and read it — if the file is absent, return an error like `config file <cfgPath> not found; run "lyx update"`; (3) `missing, err := yamlengine.MissingKeys(template, fileBytes)` — if `len(missing) > 0`, return an error naming `cfgPath` and the missing key-paths and instructing `run "lyx update"`; (4) `env, err := envsource.Build(baseDir)`; (5) `resolved, err := yamlengine.Resolve(fileBytes, env)`; return `resolved`. Errors from steps 3–5 wrap the underlying error with the config key/file context.
  - Keep `FindBaseDir(cwd string) (string, error)` but change its `_lyx` literal to `filepath.Join(cwd, paths.LyxDirName)`; keep its existing "not initialized: _lyx/ directory not found" message.
  - DELETE `expandEnv`, `envOptRe`, `envReqRe`, `loadDotEnv`, and `loadYAMLLayer` — their responsibilities now live in `yamlengine` (substitution) and `envsource` (.env/OS). Remove now-unused imports (`bufio`, `regexp`; keep `os`/`path/filepath`/`fmt` as needed).
  - Update the file's package godoc to describe the strict, engine-backed loader.
  - Rewrite `internal/config/config_test.go` for the new contract (the old grammar tests now live in `yamlengine`): happy path — write `_lyx/config/<module>.yaml` to a temp dir with all template keys, call `Load`, unmarshal the returned bytes and assert resolved values (including a `${env:...}` value resolved via a `t.Setenv` or `.env` fixture); missing-key — a file missing a template key returns an error mentioning the key-path and `lyx update`; absent-file — returns an error mentioning the path and `lyx update`; extra/stale key in the file is tolerated (no error); `_lyx` absent — returns the "not initialized" error; a nested-key template round-trips into the resolved bytes.
- **Commit:** `refactor(config): make Load strict and engine-backed (yamlengine + envsource)`

### Card 9: centralize edit.go config paths

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/config/edit.go`
  - `internal/config/edit_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `internal/config/edit.go`, replace the hardcoded `filepath.Join(baseDir, "_lyx", "config", module+".yaml")` with `paths.ConfigFile(baseDir, module)` and the `filepath.Join(baseDir, "_lyx", "config")` directory path with `paths.ConfigDir(baseDir)`. Behavior unchanged; `Edit`'s signature, scaffold/validate/abort contract, and YAML-syntax-only validation are untouched.
  - If `internal/config/edit_test.go` constructs the config path from `"_lyx"/"config"` literals, update it to use `paths.ConfigFile`/`paths.ConfigDir` (or the same joined path) so expectations match; otherwise leave it unchanged.
- **Commit:** `refactor(config): resolve edit paths via internal/paths helpers`

### Card 10: board wrapper on strict Load, remove DefaultConfig

- **Context:**
  - `internal/config/config.go`
  - `internal/yamlengine/resolve.go`
  - `internal/board/template.go`
  - `go.mod`
- **Edits:**
  - `internal/board/config.go`
  - `internal/board/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `internal/board/config.go`: DELETE `DefaultConfig()` and `DefaultOutputs()`. Rewrite `LoadConfig(baseDir, module string) (Config, error)` to call `config.Load(baseDir, module, []byte(ConfigTemplate()))`, then `yaml.Unmarshal` the returned bytes into a `Config` (struct tags already map `path`/`home`/`sidebar`/`proposal_prefix`). Preserve the existing not-initialized rewrap (`not initialized here; run "lyx init"`) by checking the error text as today. Preserve the relative-`Path` resolution (`if !filepath.IsAbs(cfg.Path) { cfg.Path = filepath.Join(baseDir, cfg.Path) }`). Keep the `Config`/`Outputs` types and the `Outputs()` method.
  - Rewrite `internal/board/config_test.go`: remove `TestDefaultConfig`/`TestDefaultOutputs` (or equivalents). Test `LoadConfig` by writing a temp `_lyx/config/board.yaml` (use `ConfigTemplate()` content or an equivalent live-YAML fixture), asserting the resolved struct fields, relative-path resolution against `baseDir`, env override via `t.Setenv`, and the not-initialized error when `_lyx` is absent.
- **Commit:** `refactor(board): load config via strict engine, drop DefaultConfig`

### Card 11: board --board-path child uses Config literal

- **Context:**
  - `internal/board/sync.go`
  - `internal/board/config.go`
- **Edits:**
  - `internal/board/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `internal/board/cli.go`, replace the `--board-path` branch `cfg = DefaultConfig(); cfg.Path = *boardPathFlag` with `cfg = Config{Path: *boardPathFlag}`. This is correct because the detached `lyx board sync` child only consumes `cfg.Path` (see `sync.go`); `SkipGit`/`SkipPush` stay zero-valued exactly as before (`DefaultConfig` never set them), and `applySkipEnv(cfg)` still runs afterward. Do not reintroduce a defaults helper.
- **Commit:** `refactor(board): build detached-sync cfg from Path literal (no DefaultConfig)`

### Card 12: update board-package tests for DefaultConfig removal

- **Context:**
  - `internal/board/config.go`
- **Edits:**
  - `internal/board/render_test.go`
  - `internal/board/board_test.go`
  - `internal/board/boardtest/sync_test.go`
  - `internal/board/boardtest/bench_test.go`
  - `internal/board/boardtest/concurrency_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Replace every `board.DefaultConfig()` call in these test files with an explicit `board.Config{Path: <whatever the test used>, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}` literal (the former default values), keeping each test's existing `Path` override. Replace every `board.DefaultOutputs()` with `board.Config{Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}.Outputs()` (or an equivalent `Outputs{...}` literal matching the former defaults). Where a test only needed `Path`, the other fields may be omitted if the test does not read them — preserve each test's original intent. Make no behavioral changes to the tests beyond removing the deleted symbols.
- **Commit:** `test(board): replace DefaultConfig/DefaultOutputs with explicit literals`

### Card 13: worktree wrapper on strict Load, remove DefaultConfig

- **Context:**
  - `internal/config/config.go`
  - `internal/worktree/template.go`
  - `internal/yamlengine/resolve.go`
  - `go.mod`
- **Edits:**
  - `internal/worktree/config.go`
  - `internal/worktree/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `internal/worktree/config.go`: DELETE `DefaultConfig()`. Rewrite `LoadConfig(baseDir, module string) (Config, error)` to call `config.Load(baseDir, module, []byte(ConfigTemplate()))` and `yaml.Unmarshal` into `Config` (the `branch_prefix` tag already exists). Preserve the not-initialized rewrap. Keep the `Config` type.
  - Rewrite `internal/worktree/config_test.go`: remove DefaultConfig tests; test `LoadConfig` against a temp `_lyx/config/worktree.yaml` fixture, asserting `branch_prefix` (including the empty-default case) and the not-initialized error.
- **Commit:** `refactor(worktree): load config via strict engine, drop DefaultConfig`

### Card 14: weft wrapper on strict Load, remove DefaultConfig

- **Context:**
  - `internal/config/config.go`
  - `internal/weft/template.go`
  - `internal/yamlengine/resolve.go`
  - `go.mod`
- **Edits:**
  - `internal/weft/config.go`
  - `internal/weft/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `internal/weft/config.go`: DELETE `DefaultConfig()`. Rewrite `LoadConfig(weftBaseDir string) (Config, error)` to call `config.Load(weftBaseDir, "weft", []byte(ConfigTemplate()))` and `yaml.Unmarshal` into `Config` (the `pathspec` tag exists). Preserve the weft-specific rewrap (`weft worktree or its _lyx is missing at <weftBaseDir>`). Keep the `Config` type and the `Dirs()` method. `weft.LoadConfig` keeps its single `weftBaseDir` argument — the caller (`weft/cli.go:95`) still builds it as `filepath.Join(l.WeftWorktree(), l.RelPath)`; do not change the call site.
  - Rewrite `internal/weft/config_test.go`: remove DefaultConfig tests; test `LoadConfig` against a temp `_lyx/config/weft.yaml` fixture, asserting `pathspec`, `Dirs()` splitting, and the missing-`_lyx` rewrap.
- **Commit:** `refactor(weft): load config via strict engine, drop DefaultConfig`

### Card 15: ide menu handles board config load error

- **Context:**
  - `internal/board/config.go`
- **Edits:**
  - `internal/ide/menu.go`
  - `internal/ide/menu_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `internal/ide/menu.go` `Menu`, change `cfg, _ := board.LoadConfig(l.Cwd, "board")` to capture and handle the error: `cfg, err := board.LoadConfig(l.Cwd, "board"); if err != nil { return fmt.Errorf("load board config: %w", err) }`. This prevents `b.HealthCheck()` from running against a zero `Config{}` (empty `cfg.Path`) under strict Load. Leave the rest of `Menu` unchanged.
  - Update `internal/ide/menu_test.go` if it relied on `LoadConfig` silently succeeding without a config file: ensure the test sets up a valid `_lyx/config/board.yaml` (via the template) for the success path, and add/adjust a case asserting the HARD error when the board config cannot be loaded. Keep the zero-worktree and picker tests intact.
  - NOTE: `internal/ide/menu_test.go` carries a `//go:build integration` tag, so the batch verify (`go test ./internal/ide/` with no `-tags integration`) does NOT execute these tests — it only confirms the package compiles. The updated menu assertions are validated by review and by the integration tier (`-tags integration`), not by this batch's verify. Ensure the file still COMPILES (and that the existing integration tests, which use an uninitialized `Cwd`, are adjusted to expect the new hard load-error rather than silent success).
- **Commit:** `fix(ide): handle board config load error in menu (no empty-path HealthCheck)`

## Batch Tests

`verify: go test ./internal/config/ ./internal/board/... ./internal/worktree/ ./internal/weft/ ./internal/ide/`
covers the rewritten loader, all three wrappers, the board-package test updates
(including `boardtest/...`), and the ide menu fix. Scope is bounded to the packages
this batch edits. The grammar matrix is NOT re-tested here (it lives in
`yamlengine`); these tests assert the integration — strict errors, struct
round-trips, env resolution through the full `Load` path.
