# Plan: board-modul (rename fra wiki) + _mhgo-konfigurasjon

```yaml
task: "board-modul (rename fra wiki) + _mhgo-konfigurasjon"
slug: "config-layer"
approved: false
started: "20260608-115311"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: rename-wiki-to-board
    file: 01-rename-wiki-to-board.md
    depends-on: []
    verify: go build ./... && go vet -tags integration ./... && go test ./...
  - number: 2
    name: config-system
    file: 02-config-system.md
    depends-on: [1]
    verify: go test ./internal/board/ && go build ./...
  - number: 3
    name: config-driven-render-facade
    file: 03-config-driven-render-facade.md
    depends-on: [2]
    verify: go build ./... && go vet -tags integration ./... && go test ./...
  - number: 4
    name: cwd-activation-and-board-path
    file: 04-cwd-activation-and-board-path.md
    depends-on: [3]
    verify: go build ./... && go vet -tags integration ./... && go test ./...
  - number: 5
    name: mhgo-init-command
    file: 05-mhgo-init-command.md
    depends-on: [2]
    verify: go build ./... && go test ./...
  - number: 6
    name: docs-and-roadmap
    file: 06-docs-and-roadmap.md
    depends-on: [4, 5]
    verify: null
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions,
error-handling posture, test frameworks, style/lint constraints. One
subsection per decision. Batch-local decisions live in each batch file._

### Decision: rename-surface

- **Decision:** The `wiki` name is erased from every code- and developer-facing
  surface. Concretely: directory `internal/wiki/` → `internal/board/`;
  `internal/wiki/wikitest/` → `internal/board/boardtest/`; `package wiki` →
  `package board`; `package wikitest` → `package boardtest`; the external test
  package `package wiki_test` → `package board_test`; import path
  `github.com/Knatte18/mhgo/internal/wiki` → `.../internal/board`; the `Wiki`
  struct → `Board` (receiver `w *Wiki` → `b *Board`); struct field `wikiPath` →
  `boardPath`; exported error types `WikiPushError` → `BoardPushError` and
  `WikiPathError` → `BoardPathError`; control env vars `WIKI_SKIP_GIT` →
  `BOARD_SKIP_GIT` and `WIKI_SKIP_PUSH` → `BOARD_SKIP_PUSH`; the background
  commit message `"wiki sync"` → `"board sync"`; the spawned module argument
  `"wiki"` → `"board"`; CLI `mhgo wiki ...` → `mhgo board ...`. **Kept
  unchanged:** the on-disk filenames `tasks.json`, `tasks.json.lock`,
  `tasks.json.push.lock`, `*.swaplock`. The env var `MHGO_WIKI_PATH` and the
  `--wiki-path` flag are NOT renamed — they are deleted in batch 4 and replaced
  by the config system.
- **Rationale:** matches discussion decision `rename-depth` (full rename,
  on-disk data names kept) and `config-location` (config replaces the old path
  knobs rather than renaming them).
- **Applies to:** all batches

### Decision: file-renames-via-git-mv

- **Decision:** Move files with `git mv` (never delete-and-recreate) so history
  is preserved: `git mv internal/wiki internal/board`, then
  `git mv internal/board/wikitest internal/board/boardtest`, then
  `git mv internal/board/wiki.go internal/board/board.go` and
  `git mv internal/board/wiki_test.go internal/board/board_test.go`. The
  docs file move `git mv docs/wiki.md docs/board.md` happens in batch 6.
- **Rationale:** preserves blame/history across the rename.
- **Applies to:** rename-wiki-to-board, docs-and-roadmap

### Decision: config-api

- **Decision:** The board module's configuration is defined in
  `internal/board/config.go`:

  ```go
  type Config struct {
      Path           string // board dir; relative to baseDir or absolute; may contain $env:...
      Home           string
      Sidebar        string
      ProposalPrefix string
  }
  type Outputs struct { Home, Sidebar, ProposalPrefix string }
  func (c Config) Outputs() Outputs
  func DefaultConfig() Config   // {Path:"../_board", Home:"Home.md", Sidebar:"_Sidebar.md", ProposalPrefix:"proposal-"}
  func DefaultOutputs() Outputs // == DefaultConfig().Outputs()
  func LoadConfig(baseDir, module string) (Config, error)
  ```

  `LoadConfig` semantics (discussion `config-location`, `merge-and-defaults`,
  `env-interpolation`): if `<baseDir>/_mhgo/` does not exist → return an error
  whose message contains `not initialized here; run "mhgo init"`. Otherwise
  start from `DefaultConfig()`, then deep-merge per key from
  `<baseDir>/_mhgo/<module>.yaml` (optional — absent file is no error), then
  from `<baseDir>/.mhgo/<module>.yaml` (optional). A key absent from a higher
  layer falls through. After merge, expand `$env:NAME` tokens in every string
  value from the process environment; a referenced-but-unset variable is a hard
  error. A relative `Path` (after expansion) is resolved against `baseDir` via
  `filepath.Join`; an absolute `Path` is used as-is. Malformed YAML → error.
- **Rationale:** matches the discussion's config decisions verbatim; one flat
  YAML file per module keyed by filename.
- **Applies to:** config-system, config-driven-render-facade,
  cwd-activation-and-board-path, mhgo-init-command

### Decision: env-token-expansion

- **Decision:** The `$env:NAME` expander replaces every occurrence of
  `$env:NAME` (where `NAME` matches `[A-Za-z_][A-Za-z0-9_]*`) anywhere within a
  string value with `os.Getenv("NAME")`. Implement with a single
  `regexp.MustCompile(` + "`" + `\$env:([A-Za-z_][A-Za-z0-9_]*)` + "`" + `)` and a
  replace callback that records the first unset variable and surfaces it as an
  error (`referenced env var %q is not set`). The token may appear mid-value
  (e.g. `$env:MHGO_BOARD_PATH/sub`).
- **Rationale:** discussion `env-interpolation` — opt-in, session-local, fails
  loud on unset.
- **Applies to:** config-system

### Decision: facade-and-render-signatures

- **Decision:** After batch 3 the facade and renderer are config-driven:

  ```go
  type Board struct { boardPath string; out Outputs }
  func New(cfg Config) *Board                                  // boardPath=cfg.Path, out=cfg.Outputs()
  func Render(tasks []Task, out Outputs) (map[string]string, error)
  func RenderToDisk(boardPath string, tasks []Task, out Outputs) error
  ```

  `renderHome`, `renderSidebar`, `renderProposals`, and `removeOrphanProposals`
  take the proposal prefix (and `Render` uses `out.Home`/`out.Sidebar` as the
  output map keys). The proposal prefix is applied at all four sites:
  `renderProposals` filename, `removeOrphanProposals` glob (`out.ProposalPrefix
  + "*.md"`), the in-content link in `renderHome`, and the in-content link in
  `renderSidebar`.
- **Rationale:** discussion `config-schema` + Technical context (`render.go`)
  + `testability` (facade constructor bypasses config resolution in tests).
- **Applies to:** config-driven-render-facade, cwd-activation-and-board-path

### Decision: go-verify-no-pythonpath

- **Decision:** This is a Go project; `verify:` commands use the native Go test
  runner with NO `PYTHONPATH=` prefix. Batches that touch the `boardtest`
  package (which contains `//go:build integration` files) include
  `go vet -tags integration ./...` in `verify:` so the integration-gated files
  are compile-checked without running their network tests.
- **Rationale:** the `verify-not-isolated` validator check is conditional on
  project language; `go test ./...` does not compile `//go:build integration`
  files, so they would silently rot without the vet step.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` across every batch, sorted
alphabetically. mill-go reads this to warn if two parallel batches
touch the same file — a sign of a misplaced dependency._

_Note: batch 1 renames `internal/wiki/` → `internal/board/` via `git mv`. The
validator models a rename as Creates (new path) + Deletes (old path), so both
the `internal/board/*` and `internal/wiki/*` paths appear below, as does the
`docs/wiki.md` → `docs/board.md` move in batch 6._

- `cmd/mhgo/main.go`
- `cmd/mhgo/main_test.go`
- `docs/benchmarks.md`
- `docs/board.md`
- `docs/overview.md`
- `docs/roadmap.md`
- `docs/wiki.md`
- `go.mod`
- `go.sum`
- `internal/board/board.go`
- `internal/board/board_test.go`
- `internal/board/boardtest/bench_git_test.go`
- `internal/board/boardtest/bench_test.go`
- `internal/board/boardtest/concurrency_test.go`
- `internal/board/boardtest/doc.go`
- `internal/board/boardtest/integration_test.go`
- `internal/board/cli.go`
- `internal/board/cli_test.go`
- `internal/board/config.go`
- `internal/board/config_test.go`
- `internal/board/git.go`
- `internal/board/git_test.go`
- `internal/board/init.go`
- `internal/board/init_test.go`
- `internal/board/layer.go`
- `internal/board/layer_test.go`
- `internal/board/lock.go`
- `internal/board/lock_test.go`
- `internal/board/render.go`
- `internal/board/render_test.go`
- `internal/board/spawn_other.go`
- `internal/board/spawn_windows.go`
- `internal/board/store.go`
- `internal/board/store_test.go`
- `internal/board/sync.go`
- `internal/board/sync_test.go`
- `internal/board/task.go`
- `internal/board/task_test.go`
- `internal/wiki/cli.go`
- `internal/wiki/cli_test.go`
- `internal/wiki/git.go`
- `internal/wiki/git_test.go`
- `internal/wiki/layer.go`
- `internal/wiki/layer_test.go`
- `internal/wiki/lock.go`
- `internal/wiki/lock_test.go`
- `internal/wiki/render.go`
- `internal/wiki/render_test.go`
- `internal/wiki/spawn_other.go`
- `internal/wiki/spawn_windows.go`
- `internal/wiki/store.go`
- `internal/wiki/store_test.go`
- `internal/wiki/sync.go`
- `internal/wiki/sync_test.go`
- `internal/wiki/task.go`
- `internal/wiki/task_test.go`
- `internal/wiki/wiki.go`
- `internal/wiki/wiki_test.go`
- `internal/wiki/wikitest/bench_git_test.go`
- `internal/wiki/wikitest/bench_test.go`
- `internal/wiki/wikitest/concurrency_test.go`
- `internal/wiki/wikitest/doc.go`
- `internal/wiki/wikitest/integration_test.go`
