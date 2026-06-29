# Batch: board split

```yaml
task: "Rename Cobra modules to `<module>cli`, extract kernels as `<module>engine`"
batch: "board split"
number: 1
cards: 4
verify: "go build ./... && go test ./... && go test -tags integration ./..."
depends-on: []
```

## Batch Scope

Split `internal/board` into `internal/boardengine` (domain kernel) and
`internal/boardcli` (cobra command), relocate the `boardtest` support subpackage to
`internal/boardengine/boardtest`, delete `internal/board`, and retarget every importer
(`cmd/lyx/main.go`, `internal/configreg`, `internal/ide/menu.go`,
`internal/initcli/initcli_test.go`). This is the first batch and establishes the
move-then-retarget pattern every later split batch follows. `boardengine` carries the
one engine→engine dependency target for batch 4 (`ideengine` will import `boardengine`);
this batch retargets `internal/ide/menu.go` to `boardengine` even though `ide` itself is
not split until batch 4. No behaviour changes: all moved test assertions are preserved.

## Cards

### Card 1: Create `internal/boardengine` domain package

- **Context:**
  - `internal/board/board.go`
  - `internal/board/store.go`
  - `internal/board/task.go`
  - `internal/board/layer.go`
  - `internal/board/render.go`
  - `internal/board/git.go`
  - `internal/board/sync.go`
  - `internal/board/config.go`
  - `internal/board/template.go`
  - `internal/board/spawn.go`
  - `internal/board/template.yaml`
  - `internal/board/board_test.go`
  - `internal/board/store_test.go`
  - `internal/board/task_test.go`
  - `internal/board/layer_test.go`
  - `internal/board/render_test.go`
  - `internal/board/config_test.go`
  - `internal/board/template_test.go`
  - `internal/configengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/boardengine/board.go`
  - `internal/boardengine/store.go`
  - `internal/boardengine/task.go`
  - `internal/boardengine/layer.go`
  - `internal/boardengine/render.go`
  - `internal/boardengine/git.go`
  - `internal/boardengine/sync.go`
  - `internal/boardengine/config.go`
  - `internal/boardengine/template.go`
  - `internal/boardengine/spawn.go`
  - `internal/boardengine/template.yaml`
  - `internal/boardengine/board_test.go`
  - `internal/boardengine/store_test.go`
  - `internal/boardengine/task_test.go`
  - `internal/boardengine/layer_test.go`
  - `internal/boardengine/render_test.go`
  - `internal/boardengine/config_test.go`
  - `internal/boardengine/template_test.go`
- **Deletes:** none
- **Requirements:** Move the listed domain files (the Board facade `board.go` plus
  `store.go`, `task.go`, `layer.go`, `render.go`, `git.go`, `sync.go`, `config.go`,
  `template.go`, `spawn.go` containing `spawnSync`, the `template.yaml` asset, and their
  domain `*_test.go` files) into `internal/boardengine` with their content byte-identical
  except the package clause `package board` → `package boardengine`. `config.go` keeps its
  `internal/configengine` import. `spawn.go`'s `spawnSync` is called engine-internally by
  `Board.Sync()` so it stays unexported. The already-exported engine surface (`Board`,
  `New`, `Store`, `Task`, `NewTask`, `ApplyPatch`, `ComputeLayers`, `RenderOrder`,
  `ExtendedTitle`, `Render`, `RenderToDisk`, `Pull`, `CommitPush`, `Sync`, `Config`,
  `Outputs`, `LoadConfig`, `ConfigTemplate`, `BriefTask`, `MergeStatusUpdate`) moves
  unchanged. Do not move `cli.go`, `cli_test.go`, `help_test.go`,
  `skipenv_internal_test.go` (those are card 3) and do not delete `internal/board` yet
  (card 4). Keep the `internal/board` originals in place for now so the tree still
  compiles between cards within the implementer session.
- **Commit:** `refactor(board): extract boardengine domain package`

### Card 2: Relocate `boardtest` to `internal/boardengine/boardtest`

- **Context:**
  - `internal/board/boardtest/bench_test.go`
  - `internal/board/boardtest/concurrency_test.go`
  - `internal/board/boardtest/git_test.go`
  - `internal/board/boardtest/sync_test.go`
  - `internal/board/boardtest/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/boardengine/boardtest/bench_test.go`
  - `internal/boardengine/boardtest/concurrency_test.go`
  - `internal/boardengine/boardtest/git_test.go`
  - `internal/boardengine/boardtest/sync_test.go`
  - `internal/boardengine/boardtest/doc.go`
- **Deletes:** none
- **Requirements:** Move the `boardtest` support subpackage to
  `internal/boardengine/boardtest`. In every moved file change the
  `github.com/Knatte18/loomyard/internal/board` import to
  `github.com/Knatte18/loomyard/internal/boardengine` and update each `board.<Symbol>`
  selector to `boardengine.<Symbol>`. The package clause stays `package boardtest`. Update
  the `doc.go` comment so any reference to `internal/board` reads
  `internal/boardengine/boardtest`. `git_test.go` and `sync_test.go` are
  `//go:build integration` files — preserve the build tag verbatim. No external package
  imports `boardtest`, so no importer retarget is needed.
- **Commit:** `refactor(board): relocate boardtest to boardengine/boardtest`

### Card 3: Create `internal/boardcli` command package

- **Context:**
  - `internal/board/cli.go`
  - `internal/board/cli_test.go`
  - `internal/board/help_test.go`
  - `internal/board/skipenv_internal_test.go`
  - `internal/clihelp/exec.go`
- **Edits:** none
- **Creates:**
  - `internal/boardcli/cli.go`
  - `internal/boardcli/cli_test.go`
  - `internal/boardcli/help_test.go`
  - `internal/boardcli/skipenv_internal_test.go`
- **Deletes:** none
- **Requirements:** Move `cli.go` (carrying `Command()` and the `RunCLI` seam) and its
  tests `cli_test.go`, `help_test.go`, `skipenv_internal_test.go` into `internal/boardcli`
  with package clause `package board` → `package boardcli`. Retarget every reference to a
  board domain symbol that `cli.go`/its tests use (e.g. `New`, `LoadConfig`, `Board`,
  `BriefTask`, `MergeStatusUpdate`, `Sync`, and any others) to the `boardengine` package:
  add the `internal/boardengine` import and qualify those selectors as
  `boardengine.<Symbol>`. Preserve all `Short`/`Long` strings and the `clihelp.Execute`
  delegation exactly. Preserve any `//go:build` tags on the test files. The `RunCLI` seam
  body must remain exactly `clihelp.Execute(Command(), out, args)`.
- **Commit:** `refactor(board): extract boardcli command package`

### Card 4: Retarget importers and delete `internal/board`

- **Context:**
  - `internal/board/board.go`
  - `internal/board/cli.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `internal/configreg/configreg.go`
  - `internal/ide/menu.go`
  - `internal/initcli/initcli_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/board/board.go`
  - `internal/board/store.go`
  - `internal/board/task.go`
  - `internal/board/layer.go`
  - `internal/board/render.go`
  - `internal/board/git.go`
  - `internal/board/sync.go`
  - `internal/board/config.go`
  - `internal/board/template.go`
  - `internal/board/spawn.go`
  - `internal/board/template.yaml`
  - `internal/board/cli.go`
  - `internal/board/board_test.go`
  - `internal/board/store_test.go`
  - `internal/board/task_test.go`
  - `internal/board/layer_test.go`
  - `internal/board/render_test.go`
  - `internal/board/config_test.go`
  - `internal/board/template_test.go`
  - `internal/board/cli_test.go`
  - `internal/board/help_test.go`
  - `internal/board/skipenv_internal_test.go`
  - `internal/board/boardtest/bench_test.go`
  - `internal/board/boardtest/concurrency_test.go`
  - `internal/board/boardtest/git_test.go`
  - `internal/board/boardtest/sync_test.go`
  - `internal/board/boardtest/doc.go`
- **Requirements:** In `cmd/lyx/main.go` replace the `internal/board` import with
  `internal/boardcli` and change `board.Command()` to `boardcli.Command()` in
  `newRoot()`. In `internal/configreg/configreg.go` replace the `internal/board` import
  with `internal/boardengine` and change the `{"board", board.ConfigTemplate}` entry to
  `{"board", boardengine.ConfigTemplate}` (the module name string stays `"board"`). In
  `internal/ide/menu.go` replace the `internal/board` import with `internal/boardengine`
  and change `board.LoadConfig`/`board.New` to `boardengine.LoadConfig`/`boardengine.New`
  (package stays `ide`). In `internal/initcli/initcli_test.go` replace the
  `internal/board` import with `internal/boardengine` and change `board.LoadConfig` to
  `boardengine.LoadConfig`. Then delete the entire `internal/board` directory (all listed
  files, including `boardtest`). After this card `go build ./...` and both test tiers must
  be green.
- **Commit:** `refactor(board): retarget importers and remove internal/board`

## Batch Tests

The batch is verified by the relocated suites running in their new packages plus the
repo-wide compile guarantee. `verify` is repo-wide (`go build ./...` + Tier 1
`go test ./...` + Tier 2 `go test -tags integration ./...`) because (a) `cmd/lyx/main.go`
imports every module, so a rename compile error surfaces only repo-wide, and (b) the
relocated `boardengine/boardtest/git_test.go` and `sync_test.go` are `integration`-tagged
and would be invisible to a plain `go test ./...`. Both tiers are cheap here (Tier 1
≈ 3.5 s, Tier 2 ≈ 65 s). Moved coverage: the boardengine domain suites
(`board_test`, `store_test`, `task_test`, `layer_test`, `render_test`, `config_test`,
`template_test`, and the `boardtest` git/sync/bench/concurrency tests), the boardcli
`cli_test`/`help_test`/`skipenv_internal_test`, and the cmd/lyx guard tests
(`registration_test`, `helptree_test`, `drift_test`, `longlist_test`) which self-derive
from the live tree / AST and therefore re-validate the renamed `boardcli` registration
automatically.
