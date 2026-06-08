# Batch: config-driven-render-facade

```yaml
task: "board-modul (rename fra wiki) + _mhgo-konfigurasjon"
batch: "config-driven-render-facade"
number: 3
cards: 5
verify: go build ./... && go vet -tags integration ./... && go test ./...
depends-on: [2]
```

## Batch Scope

This batch reshapes the facade and renderer to be config-driven while keeping
the CLI's externally observable behavior identical (it still resolves the board
dir via `--wiki-path`/`MHGO_WIKI_PATH`/`../gowiki` and uses the default output
names). It threads the configurable home/sidebar filenames and proposal prefix
through `Render`/`RenderToDisk` and the four render sites, changes the `Board`
struct to carry an `Outputs`, and changes `New` to take a `Config`. The CLI is
updated to build a `Config` from the still-existing path resolution plus
`DefaultConfig()` outputs, so the build stays green and tests pass — the cwd/
config activation and flag removal happen in batch 4. All facade/render test
call sites are updated to the new signatures, and new render tests cover the
configurable filenames and prefix. The external interface the next batch
consumes is `New(cfg Config)` and `Render`/`RenderToDisk` taking `Outputs`, per
the overview's `facade-and-render-signatures` Shared Decision.

## Cards

### Card 7: board.go — Board carries Outputs, New takes Config

- **Context:**
  - `internal/board/config.go`
  - `internal/board/store.go`
- **Edits:**
  - `internal/board/board.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Change `type Board struct` to `{ boardPath string; out
  Outputs }`. Change `func New(boardPath string) *Board` to `func New(cfg
  Config) *Board` returning `&Board{boardPath: cfg.Path, out: cfg.Outputs()}`.
  In `writeOp`, change the `RenderToDisk(b.boardPath, store.Tasks())` call to
  `RenderToDisk(b.boardPath, store.Tasks(), b.out)` (the new render signature
  from card 8). Leave `spawnSync(b.boardPath)` and the `BOARD_SKIP_GIT` guard as
  they are. Read methods (`GetTask`, `ListTasksBrief`, `ListTasksFull`) and
  `writeOp`'s lock/load/save sequence are otherwise unchanged in this batch.
- **Commit:** `refactor(board): Board carries Outputs, New takes Config`

### Card 8: render.go — thread Outputs through Render and the four prefix sites

- **Context:**
  - `internal/board/config.go`
  - `internal/board/layer.go`
  - `internal/board/git.go`
- **Edits:**
  - `internal/board/render.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Change `func Render(tasks []Task) (map[string]string,
  error)` to `func Render(tasks []Task, out Outputs) (map[string]string,
  error)`, building the result map with keys `out.Home` and `out.Sidebar`
  instead of the literals `"Home.md"`/`"_Sidebar.md"`. Change `func
  RenderToDisk(boardPath string, tasks []Task)` to `func RenderToDisk(boardPath
  string, tasks []Task, out Outputs)` and pass the prefix/names down. Thread the
  proposal prefix to all four sites named in the `facade-and-render-signatures`
  Shared Decision: `renderProposals` (filename `out.ProposalPrefix + slug +
  ".md"`), `removeOrphanProposals` (glob `out.ProposalPrefix + "*.md"`), the
  in-content proposal link in `renderHome` (currently `fmt.Sprintf("[%s](proposal-%s.md)", ...)`),
  and the in-content link in `renderSidebar` (currently
  `fmt.Sprintf("- [%s](proposal-%s.md)", ...)`). Update the helper signatures as
  needed (e.g. `renderHome(ordered, taskMap, prefix string)`,
  `renderSidebar(ordered, prefix string)`, `renderProposals(tasks, prefix
  string)`, `removeOrphanProposals(boardPath string, rendered map[string]string,
  prefix string)`). Keep `RenderOrder`/`ExtendedTitle` (in `layer.go`)
  unchanged. Update doc comments that hardcode `Home.md`/`_Sidebar.md`/`proposal-`
  to describe them as the configured names.
- **Commit:** `refactor(board): thread configurable output names through render`

### Card 9: cli.go — build Config and call New(cfg), behavior unchanged

- **Context:**
  - `internal/board/config.go`
  - `internal/board/board.go`
- **Edits:**
  - `internal/board/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update `RunCLI` to construct the `Board` via the new `New`
  signature WITHOUT changing externally observable behavior: keep the
  `--wiki-path` flag, `resolveWikiPath`, `defaultWikiPath`, and `MHGO_WIKI_PATH`.
  Where it currently does `w := New(wikiPath)`, instead build `cfg :=
  DefaultConfig(); cfg.Path = wikiPath` and `b := New(cfg)` (so the board dir
  comes from the existing resolution and the output names are the defaults).
  Rename the local variable `w` → `b` for consistency with the `*Board`
  receiver. The flag/env removal and cwd activation are batch 4 — do not touch
  them here.
- **Commit:** `refactor(board): construct Board from Config in RunCLI`

### Card 10: update facade/render unit tests + new configurable-render tests

- **Context:**
  - `internal/board/config.go`
  - `internal/board/board.go`
  - `internal/board/render.go`
- **Edits:**
  - `internal/board/board_test.go`
  - `internal/board/sync_test.go`
  - `internal/board/render_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update every `board.New(<path>)` call to the new signature:
  build a config with `cfg := board.DefaultConfig(); cfg.Path = <path>;
  board.New(cfg)` (in `board_test.go` and the five `board.New(work)` sites in
  `sync_test.go`). In `render_test.go`, update every `board.Render(tasks)` call
  to `board.Render(tasks, board.DefaultOutputs())` and every
  `board.RenderToDisk(dir, tasks)` to `board.RenderToDisk(dir, tasks,
  board.DefaultOutputs())`; the existing assertions on `"Home.md"`,
  `"_Sidebar.md"`, and `"proposal-<slug>.md"` keys stay valid under the
  defaults. ADD new test cases proving configurability: (a) `Render` with an
  `Outputs{Home:"README.md", Sidebar:"_Sidebar.md", ProposalPrefix:"proposal-"}`
  produces a `"README.md"` key (not `"Home.md"`); (b) `Render`/`RenderToDisk`
  with a non-default `ProposalPrefix` (e.g. `"prop-"`) produces
  `"prop-<slug>.md"` files and the in-content links use that prefix; (c)
  `RenderToDisk` orphan cleanup removes a stale `prop-ghost.md` when the
  configured prefix is `"prop-"` (mirror `TestRenderToDiskWritesAndCleansOrphans`
  with the custom prefix).
- **Commit:** `test(board): update render/facade tests for configurable outputs`

### Card 11: update boardtest call sites for new New/Render signatures

- **Context:**
  - `internal/board/config.go`
  - `internal/board/board.go`
  - `internal/board/render.go`
- **Edits:**
  - `internal/board/boardtest/bench_test.go`
  - `internal/board/boardtest/concurrency_test.go`
  - `internal/board/boardtest/bench_git_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update the black-box benchmark/concurrency call sites for the
  new signatures. In `bench_test.go`: `board.Render(tasks)` →
  `board.Render(tasks, board.DefaultOutputs())` (BenchmarkRender) and
  `board.New(dir)` → `cfg := board.DefaultConfig(); cfg.Path = dir;
  board.New(cfg)` (BenchmarkUpsertFacade). The CLI-driven benchmarks
  (`BenchmarkUpsert`/`BenchmarkGet`/`BenchmarkList`) still call `board.RunCLI`
  with `--wiki-path` and are NOT re-architected here (that is batch 4). In
  `concurrency_test.go`: update the three `board.New(dir)` sites the same way. In
  `bench_git_test.go` (integration-gated): update the `board.New(repo)` site the
  same way. `go vet -tags integration ./...` in `verify` compile-checks
  `bench_git_test.go`.
- **Commit:** `test(board): update boardtest call sites for Config/Outputs`

## Batch Tests

`verify: go build ./... && go vet -tags integration ./... && go test ./...`.
The signature changes ripple across the whole package and its black-box suite,
so the full build + non-integration test run is the right scope; the
`go vet -tags integration` step compile-checks the integration-gated
`boardtest` files (`bench_git_test.go`, `integration_test.go`) that `go test
./...` does not build. New configurable-render assertions live in
`render_test.go`; the renamed/updated unit tests in `board_test.go`,
`sync_test.go`, and the `boardtest` benchmarks must all still compile and pass.
