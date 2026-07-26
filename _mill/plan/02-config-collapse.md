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

Collapse the config-module registry from thirteen modules to eleven by removing the separate
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
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the two registry rows that register module `"warp"`
  (`warpengine.ConfigTemplate`) and module `"weft"` (`weftengine.ConfigTemplate`), and delete
  the `warpengine` and `weftengine` imports. Keep the already-present `"fabric"` row backed by
  `fabricengine.ConfigTemplate()` (it embeds the merged `branch_prefix` from warp + `pathspec`
  from weft, so no config field is lost). `configreg.Names()` now returns the list without
  `warp`/`weft`; consumers that surface the list (`configcli`) update automatically.
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
  from the expected-names assertion (drop the module count by two), and re-point or drop the
  `weftengine.ConfigTemplate()` assertion in favour of the `fabric` module's template
  (`fabricengine.ConfigTemplate()`). Drop the `warpengine`/`weftengine` imports.
- **Commit:** `test(configreg): pin fabric-only module list`

### Card 12: rewrite configcli_integration_test.go onto fabric

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabriccli/fabric.go`
- **Edits:**
  - `internal/configcli/configcli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The fixture builds via `warpengine.New().Add()` +
  `warpengine.WireJunctions` + `weftcli.RunCLI`, and dispatches the `"warp"` config module.
  Rewrite onto `fabricengine.NewTopology(cfg).Add()` (note `New` -> `NewTopology`,
  `*Worktree` -> `*Topology`) + `fabricengine.WireJunctions` + `fabriccli.RunCLI`, and
  dispatch the `"fabric"` config module instead of `"warp"`. Drop the
  `warpengine`/`weftcli` imports for `fabricengine`/`fabriccli`. Preserve every assertion.
- **Commit:** `test(configcli): rewrite integration fixture onto fabric`

## Batch Tests

`verify` runs the integration suites for the two touched modules: `internal/configreg`
(configreg.go, configreg_test.go) and `internal/configcli` (configcli.go,
configcli_integration_test.go). `configcli_integration_test.go` exercises the real
config-write + weft-sync dispatch path, so `-tags integration` is required. The old modules
still exist, so no other package is affected.
