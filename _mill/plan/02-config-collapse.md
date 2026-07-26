# Batch: B -- config collapse

```yaml
task: 'fabric: cutover -- rewire consumers onto fabric, delete warp/weft'
batch: B -- config collapse
number: 2
cards: 4
verify: go test -tags integration ./internal/configreg/... ./internal/configcli/...
depends-on: []
```

## Batch Scope

Collapse the config-module registry from twelve modules to ten by removing the separate
`warp` and `weft` config modules (the already-registered `fabric` module covers both), and
switch `configcli`'s weft-sync dispatch from `weftcli.RunCLI` to `fabriccli.RunCLI`. Also
rewrite the two tests that pin the module list / build warp-based fixtures. Independent of A
and C (the old modules still exist and compile). No alias, no on-disk config migration -- a
live hub regenerates via `lyx init`. Batch D1 depends on this batch having removed the last
`configreg`/`configcli` production import of the old engines.

## Cards

### Card 9: collapse configreg to fabric-only

- **Context:**
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configsync/configsync_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Modules()`, delete the two registry rows that register module
  `"warp"` (`warpengine.ConfigTemplate`) and module `"weft"` (`weftengine.ConfigTemplate`) --
  taking the row count from 12 to 10 -- and delete the `warpengine` and `weftengine` imports.
  Keep the already-present `"fabric"` row backed by `fabricengine.ConfigTemplate()` (it embeds
  the merged `branch_prefix` from warp + `pathspec` from weft, so no config field is lost).
  Also sweep BOTH bare-word comment references so neither names a removed module: the
  package-comment `// ... a neutral registry of available config modules (board, warp, weft)`
  and the `Module.Name` field doc-comment `// Name is the module identifier (e.g., "board",
  "warp", "weft").` -- reword each example to a still-valid pair (e.g. `"board", "fabric"`).
  (Card 27's grep gate targets `warpengine|weftengine` words and import paths, so these bare
  `"warp"`/`"weft"` string examples would otherwise survive.) `configreg.Names()`
  now returns the list without `warp`/`weft`; consumers that surface the list (`configcli`)
  update automatically. `internal/configsync/configsync_test.go` was discovered during
  implementation as a downstream consumer of `configreg.Modules()` NOT anticipated by this
  batch's "no other package is affected" scope note: `TestReconcileAll_ApplyCreatesFiles`
  looks up a `"weft"` entry in `ReconcileAll`'s per-module results and asserts
  `hubgeometry.ConfigFile(tmpDir, "weft")` was created on disk -- both go stale the moment
  `Modules()` drops the `"weft"` row (ReconcileAll iterates `configreg.Modules()`, so no
  `"weft"` result is ever produced). Re-point both the result lookup and the on-disk path
  assertion from `"weft"` to `"fabric"` (the row `Modules()` now carries in weft's place).
- **Commit:** `refactor(configreg): drop warp/weft modules, keep fabric`

### Card 10: switch configcli weft-sync to fabriccli

- **Context:**
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `internal/configcli/configcli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the `weftcli.RunCLI(w, []string{"sync"})` dispatch with
  `fabriccli.RunCLI(w, []string{"sync"})` (identical `RunCLI(out io.Writer, args []string) int`
  signature). Swap the `weftcli` import for `fabriccli`. Do not change the surrounding
  config-write logic. The `Known modules:` help text and `ValidArgs` derive from
  `configreg.Names()`, so they need no manual edit here.
- **Commit:** `refactor(configcli): dispatch weft sync via fabriccli.RunCLI`

### Card 11: rewrite configreg_test.go

- **Context:**
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/configreg/configreg_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The test pins the module-name list (currently including `"warp"` and
  `"weft"`) and asserts against `weftengine.ConfigTemplate()`. Remove `"warp"` and `"weft"`
  from the expected-names assertion (drop the module count by two), and re-point the
  `want := weftengine.ConfigTemplate()` assertion to the `fabric` module's template
  (`fabricengine.ConfigTemplate()`). Import-wise this file imports ONLY `weftengine` (not
  `warpengine`): drop the `weftengine` import and add `fabricengine`.
- **Commit:** `test(configreg): pin fabric-only module list`

### Card 12: rewrite configcli_integration_test.go onto fabric

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/add.go`
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `internal/configcli/configcli_integration_test.go`
  - `internal/configcli/configcli_test.go`
  - `internal/configcli/reconcile_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/configcli/configcli_test.go` (untagged unit tests) and
  `internal/configcli/reconcile_integration_test.go` (the reconcile scenarios) are additional
  same-package files discovered during implementation that also pin `"warp"`/`"weft"` module
  names and go red once card 9 drops those two rows from the registry -- `go test -tags
  integration ./internal/configcli/...` builds the whole package (both files) alongside
  `configcli_integration_test.go`, so they are in-scope for this batch's `verify:` regardless
  of the original card list. In `configcli_test.go`: every `editOne`/`dispatch`/
  `seedModuleConfig` call and JSON-envelope assertion using `"warp"` moves to `"fabric"`
  (module now seeded/dispatched/asserted as `"fabric"`); the one `"weft"` case in
  `TestEditOneSyncFails` (sync-failure message uses the literal string "weft sync failed",
  which is generic per-module wording, not a module-name reference, and stays unchanged) moves
  its seeded/dispatched module to `"fabric"` too; `TestMenuStatus` seeds `board` + `fabric`
  configured, asserts `fabric (configured)` and picks a still-unconfigured registry module
  (e.g. `builder`) to assert `(default)` instead of the removed `weft`; the file-header comment
  updates `weft.RunCLI` to `fabriccli.RunCLI`. In `reconcile_integration_test.go`:
  `TestReconcile_Apply`'s final assertion swaps `hubgeometry.ConfigFile(tmpDir, "weft")` for
  `hubgeometry.ConfigFile(tmpDir, "fabric")` (fabric.yaml is what --apply now materializes in
  its place). This card's Commit: message covers all three files in one commit.
- **Requirements (original):** This file has MORE than one test that uses the removed `"warp"` module --
  rewrite EVERY warp usage in the file, not just the first fixture:
  - `TestE2ESyncIntegration`: the fixture builds via `warpengine.New().Add()` +
    `warpengine.WireJunctions` + `weftcli.RunCLI` and dispatches `[]string{"warp"}` (and reads
    `hubgeometry.ConfigFile(".", "warp")`, asserts `module == "warp"`). Rewrite onto
    `fabricengine.NewTopology(cfg).Add()` (note `New` -> `NewTopology`, `*Worktree` ->
    `*Topology`) + `fabricengine.WireJunctions` + `fabriccli.RunCLI`, dispatching
    `[]string{"fabric"}` and asserting `module == "fabric"` / `ConfigFile(".", "fabric")`.
  - `TestDispatchSet_PreservedKeyDetectedByReconcile`: it `seedModuleConfig(t, tmpDir,
    "warp", ...)`, `dispatch(..., []string{"warp"}, ...)` with `--set`, and asserts
    `mod["module"] == "warp"`. Rewrite all three onto the `"fabric"` module (seed `fabric`,
    dispatch `[]string{"fabric"}`, assert `module == "fabric"`). Since card 9 removes `warp`
    from the registry, any lingering `"warp"` dispatch would return not-found -> exit 1 ->
    `t.Fatalf`, reddening the batch-B verify; every `"warp"` string in this file must move to
    `"fabric"`.
  Drop the `warpengine`/`weftcli` imports for `fabricengine`/`fabriccli`. Preserve each
  assertion's intent against the `fabric` module.
- **Commit:** `test(configcli): rewrite integration fixture onto fabric`

## Batch Tests

`verify` runs the integration suites for the two touched modules: `internal/configreg`
(configreg.go, configreg_test.go) and `internal/configcli` (configcli.go,
configcli_integration_test.go). `configcli_integration_test.go` exercises the real
config-write + weft-sync dispatch path, so `-tags integration` is required. The old modules
still exist, so no other package is affected.
