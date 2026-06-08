# Batch: config-system

```yaml
task: "board-modul (rename fra wiki) + _mhgo-konfigurasjon"
batch: "config-system"
number: 2
cards: 2
verify: go build ./... && go test ./internal/board/
depends-on: [1]
```

## Batch Scope

This batch adds the layered configuration system as new, **unwired** code in
`internal/board/config.go`, plus its table-driven unit tests, and adds the
`gopkg.in/yaml.v3` dependency. Nothing in the existing CLI/facade calls it yet
(that wiring is batch 3 onward), so the rest of the tree is unchanged and green.
The external interface this batch publishes — consumed by every later batch — is
the `Config`/`Outputs` types, `DefaultConfig()`, `DefaultOutputs()`,
`(Config).Outputs()`, and `LoadConfig(baseDir, module string) (Config, error)`,
exactly as specified in the overview's `## Shared Decisions` (`config-api`,
`env-token-expansion`). Built as TDD: card 6 writes the tests against the card-5
API.

## Cards

### Card 5: config.go — Config/Outputs types, defaults, LoadConfig, env expander

- **Context:**
  - `internal/board/task.go`
  - `go.sum`
- **Edits:**
  - `go.mod`
  - `go.sum`
- **Creates:**
  - `internal/board/config.go`
- **Deletes:** none
- **Requirements:** Add the dependency by running
  `go get gopkg.in/yaml.v3@v3.0.1` then `go mod tidy` (do not hand-edit `go.mod`
  / `go.sum`; the network proxy `GOPROXY=direct` is reachable and `check.v1` —
  yaml.v3's only test dep — is already present in `go.sum`). Create
  `internal/board/config.go` in `package board` implementing the `config-api`
  and `env-token-expansion` Shared Decisions: define `type Config struct { Path,
  Home, Sidebar, ProposalPrefix string }` and `type Outputs struct { Home,
  Sidebar, ProposalPrefix string }`; `func (c Config) Outputs() Outputs`;
  `func DefaultConfig() Config` returning `{Path: "../_board", Home: "Home.md",
  Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}`; `func DefaultOutputs()
  Outputs` returning `DefaultConfig().Outputs()`; and `func LoadConfig(baseDir,
  module string) (Config, error)`. `LoadConfig`: if `<baseDir>/_mhgo/` does not
  exist (`os.Stat`), return an error whose message contains `not initialized
  here; run "mhgo init"`. Otherwise start from `DefaultConfig()`, then for each
  layer file in order `<baseDir>/_mhgo/<module>.yaml` then
  `<baseDir>/.mhgo/<module>.yaml`: if the file is absent, skip (no error); if
  present, `yaml.Unmarshal` it into a struct with `omitempty`-style optional
  fields (use `*string` fields or a `map`-then-overlay so an absent key does not
  clobber a lower layer — deep-merge per key) and overlay only the keys present.
  After merging all layers, run the `$env:` expander over `Path`, `Home`,
  `Sidebar`, and `ProposalPrefix`. Then resolve `Path`: if
  `filepath.IsAbs(Path)` use as-is, else `filepath.Join(baseDir, Path)`.
  Malformed YAML must surface as an error (do not swallow). Implement the
  expander as an unexported helper, e.g. `expandEnv(s string) (string, error)`,
  using `regexp.MustCompile` of the pattern matching `$env:` followed by
  `[A-Za-z_][A-Za-z0-9_]*`; replace each match with `os.Getenv(name)`, and
  return an error `referenced env var %q is not set` for the first name where
  `os.LookupEnv` reports unset. Follow the package's existing doc-comment style
  (see `task.go`).
- **Commit:** `feat(board): add layered config system (config.go)`

### Card 6: config_test.go — table-driven LoadConfig + expander tests

- **Context:**
  - `internal/board/config.go`
  - `internal/board/store_test.go`
- **Edits:** none
- **Creates:**
  - `internal/board/config_test.go`
- **Deletes:** none
- **Requirements:** Create `internal/board/config_test.go` in
  `package board_test`, table-driven where natural, each case passing an
  explicit temp base-dir (`t.TempDir()`) — never `os.Chdir`. Cover: (1) defaults
  returned when `_mhgo/` exists but `board.yaml` is absent; (2) error (message
  contains `not initialized`) when `_mhgo/` is absent; (3) per-key deep-merge
  across the three layers — a key set only in `_mhgo/board.yaml` overrides the
  default, a key set in `.mhgo/board.yaml` overrides `_mhgo/board.yaml`, and a
  key absent from both falls through to the default; (4) `$env:NAME` expansion of
  a whole value and of an embedded `$env:NAME/sub` form (use `t.Setenv`), plus
  the hard error when a referenced variable is unset; (5) a relative `Path`
  resolved against the base-dir vs an absolute `Path` passed through unchanged;
  (6) malformed YAML surfaces an error. Match the existing test style (see
  `store_test.go`).
- **Commit:** `test(board): table-driven config tests`

## Batch Tests

`verify: go build ./... && go test ./internal/board/`. The new behavior is
isolated to `config.go`, fully exercised by `config_test.go`; `go test
./internal/board/` runs the whole board unit suite (the renamed tests from batch
1 still pass, plus the new config tests). `go build ./...` confirms the yaml
dependency resolves and the module still builds. The `boardtest` package is not
touched, so no integration vet step is needed this batch.
