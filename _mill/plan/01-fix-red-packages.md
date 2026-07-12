# Batch: fix-red-packages

```yaml
task: 'Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks'
batch: fix-red-packages
number: 1
cards: 2
verify: go test -tags integration ./internal/initengine ./internal/ideengine -count=1
depends-on: []
```

## Batch Scope

Turn the two red Tier 2 packages green. Card 1 is test maintenance
(`initengine`'s stale hardcoded module count becomes registry-derived so it can
never rot again); card 2 is a one-line product-bug fix in `lyx ide menu`
(`ideengine/menu.go` never sets `boardengine.Config.Path` after the
board-dir-geometry migration), which also fixes the three red `ideengine` menu
tests without touching them. The two cards are independent of each other and
of every other batch; they are one batch because both are single-file,
red-to-green fixes verified by the same integration-tagged test run. No batch
consumes an interface from this one.

## Cards

### Card 1: initengine module-count assertion derives from configreg

- **Context:**
  - `internal/initengine/init.go`
  - `internal/configsync/configsync.go`
  - `internal/configreg/configreg.go`
- **Edits:**
  - `internal/initengine/init_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `TestInit_FirstRun`, replace the hardcoded assertion
  `if len(result.Modules) != 3` (currently `init_test.go:62-64`) with a
  registry-derived expectation: `want := len(configreg.Modules())` and compare
  `len(result.Modules)` against `want`, with a failure message that names
  `configreg.Modules()` as the source of the expected count. Add
  `github.com/Knatte18/loomyard/internal/configreg` to the import block.
  Include a short comment stating why the count is derived: `Init` →
  `configsync.ReconcileAll` iterates `configreg.Modules()`, so the registry is
  the single source of truth and a newly registered module must not stale this
  assertion. The existing loop asserting the `board`/`warp`/`weft` config
  files exist stays exactly as-is. No other test function in the file changes.
- **Commit:** `test(initengine): derive TestInit_FirstRun module count from configreg`

### Card 2: ide menu sets board path from hub geometry

- **Context:**
  - `internal/boardengine/config.go`
  - `internal/boardengine/board.go`
  - `internal/boardcli/cli.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/ideengine/menu_test.go`
- **Edits:**
  - `internal/ideengine/menu.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Menu` (`internal/ideengine/menu.go`), immediately after
  the `boardengine.LoadConfig(l.Cwd, "board")` call succeeds and before
  `boardengine.New(cfg)`, set `cfg.Path = hubgeometry.BoardDir(l.Hub)`. Add a
  short comment noting that `LoadConfig` never sets `Path` (`yaml:"-"`) — the
  board data dir is geometry owned by `hubgeometry.BoardDir`, mirroring the
  reference pattern in `boardcli`'s `PersistentPreRunE`
  (`internal/boardcli/cli.go:103`). `hubgeometry` is already imported; no
  other line of `Menu` changes. This makes `HealthCheck()` stat the real board
  dir instead of an empty path, turning `TestMenuExcludesMain`,
  `TestMenuRequiresLyxDir`, and `TestMenuNumericSelection` green with zero
  test edits. `TestMenuHardErrorOnMissingBoard` still passes: it fails at
  `LoadConfig` (missing `_lyx/config/board.yaml`), before the new line runs.
- **Commit:** `fix(ideengine): set board path from hub geometry in Menu`

## Batch Tests

`verify:` runs the two affected packages under `-tags integration` with
`-count=1`: `internal/initengine` (all `Init` tests including the fixed
`TestInit_FirstRun`) and `internal/ideengine` (the three previously-red menu
tests plus the package's remaining tests). Both packages were failing before
this batch; the verify command passing is the red-to-green proof. Scope is
exactly the two packages the cards touch — no repo-wide run needed here (the
module-wide overview verify covers cross-package fallout at the batch
boundary).
