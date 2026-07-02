# Batch: cmd-lyx-integration

```yaml
task: 'Build internal/mux: the window to the world (overlay + strands + render)'
batch: 'cmd-lyx-integration'
number: 7
cards: 5
verify: go test ./cmd/lyx/... ./internal/configreg/... ./tools/sandbox/...
depends-on: [6]
```

## Batch Scope

The atomic "register mux + park muxpoc + go green" batch. Registration is inherently
atomic: the CLI/Cobra registration guards, the sandbox-coverage guard, and the configreg
guard must all flip together with the `main.go` wiring, the sandbox scenario, and the
configreg entry — so they live in one batch. Also wires the root `-v/--verbose` flag to
`logger.SetVerbosity`. No new module behavior; this is integration + guard-test upkeep +
parking `muxpoc` (kept on disk, unwired). No renames.

Batch-local decision: `muxpoc` is parked by removing it from `main.go` (import, AddCommand,
`root.Long`), adding it to `registration_test.go`'s allowlist (it still exports `Command()`),
and removing it from `sandbox_coverage_test.go`'s `excludedModules` (a stale exclude entry
for a now-unregistered module would itself break that test).

## Cards

### Card 28: main.go — register muxcli, park muxpoc, wire root -v flag

- **Context:**
  - `internal/muxcli/cli.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/main.go` `newRoot()`: (1) in the import block, remove
  `"github.com/Knatte18/loomyard/internal/muxpoccli"` and add
  `"github.com/Knatte18/loomyard/internal/muxcli"` (alphabetical, after `initcli`); (2) in
  the `root.AddCommand(...)` block, remove `muxpoccli.Command(),` and add `muxcli.Command(),`;
  (3) in `root.Long`, change the "Available modules:" line to replace `muxpoc` with `mux`
  (final: `init, board, config, ide, mux, weft, warp, selfreport.`); (4) add a **persistent
  count flag** on root: `var verbosity int` + `root.PersistentFlags().CountVarP(&verbosity,
  "verbose", "v", "increase log verbosity (-v info, -vv debug)")`, and call
  `logger.SetVerbosity(verbosity)` in root's `PersistentPreRun`/`PersistentPreRunE` (or
  immediately after flag parse, before subcommands run) so 0->Warn / 1->Info / >=2->Debug.
  Do not delete `internal/muxpoccli` — it stays on disk.
- **Commit:** `feat(cmd/lyx): register mux, park muxpoc, add -v verbosity flag`

### Card 29: cmd/lyx help-tree/json-help/unknown-subcommand guards

- **Context:**
  - `cmd/lyx/main.go`
- **Edits:**
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/jsonhelp_test.go`
  - `cmd/lyx/unknown_subcommand_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the pinned lists for `mux` (add) and `muxpoc` (remove). In
  `helptree_test.go`: in `requiredModules`, replace `"muxpoc"` with `"mux"`; in the
  per-module subcommand table, remove the `muxpoc` case and add a `mux` case with `wantSubs:
  []string{"up", "add", "remove", "status", "attach", "resume", "down"}` (the seven mux verbs;
  helptree is a set/superset `strings.Contains` check, so the listed order need not match the
  `AddCommand` order in `internal/muxcli/cli.go`). In `jsonhelp_test.go`: in the
  `requiredModules` slice inside `TestJSONHelp_RootSchema`, replace `"muxpoc"` with `"mux"`.
  In `unknown_subcommand_test.go`: in `TestMountedUnknownSubcommand`, replace `{"muxpoc"}`
  with `{"mux"}`; in `TestMountedBareGroupListing_NoGitRepo`, replace `{"muxpoc", "up"}` with
  `{"mux", "up"}`; update the prose doc comments that name "muxpoc"/"five module groups" for
  accuracy.
- **Commit:** `test(cmd/lyx): update help-tree/json-help/unknown-subcommand guards for mux`

### Card 30: registration allowlist + sandbox-coverage exclusion

- **Context:**
  - `cmd/lyx/main.go`
  - `internal/muxpoccli/cmd.go`
- **Edits:**
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/registration_test.go`, change the empty `allowlist :=
  map[string]bool{}` to `map[string]bool{"muxpoccli": true}` with a comment noting muxpoc is
  parked (on disk, still exports `Command()`, intentionally unwired pending the mux
  replacement) — the existing `allowlist[pkg]` truthiness lookup needs no change. In
  `cmd/lyx/sandbox_coverage_test.go`, remove the `"muxpoc": "PoC, slated for replacement by
  the mux module",` entry from `excludedModules` (leaving `ide` and `selfreport`) — the now
  newly-registered `mux` module is covered by the sandbox scenario added in card 32, so it
  needs no exclude entry.
- **Commit:** `test(cmd/lyx): allowlist parked muxpoc, drop stale sandbox exclusion`

### Card 31: configreg — register mux config module

- **Context:**
  - `internal/muxengine/template.go`
  - `internal/configreg/configreg.go`
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/configreg/configreg.go`, add the import
  `"github.com/Knatte18/loomyard/internal/muxengine"` and insert `{"mux",
  muxengine.ConfigTemplate}` into the `Modules()` slice literal at the alphabetical position
  (order: `board, mux, warp, weft`). In `internal/configreg/configreg_test.go`, update the
  `TestNames` `want` slice to match exactly: `[]string{"board", "mux", "warp", "weft"}` (the
  test compares index-by-index, so `Modules()` order and `want` order must agree).
- **Commit:** `feat(configreg): register mux config module`

### Card 32: sandbox scenario — Covers: mux

- **Context:**
  - `tools/sandbox/SANDBOX-SUITE.md`
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `tools/sandbox/SANDBOX-SUITE.md`, add a scenario tagged `**Covers:**
  mux` (same bold-label style as existing scenarios' `**Goal:**`/`**Watch:**`/`**Verdict:**`
  lines) that drives the deployed binary through the mux lifecycle `up` -> `add` -> `status`
  -> `attach` -> `resume` -> `down`. Note in the scenario prose that overlay steps needing a
  live psmux server carry the same live-psmux caveat as other environment-dependent
  scenarios; the coverage guard (`sandbox_coverage_test.go`) is satisfied by the
  `**Covers:** mux` tag regardless of runtime availability.
- **Commit:** `docs(sandbox): add mux lifecycle coverage scenario`

## Batch Tests

`verify: go test ./cmd/lyx/... ./internal/configreg/... ./tools/sandbox/...` exercises every
guard that must flip atomically with registration: `registration_test` (mux registered,
muxpoc allowlisted), `helptree_test`/`jsonhelp_test`/`unknown_subcommand_test` (mux named,
muxpoc gone), `longlist_test` + `drift_test` (auto-derived — pass once `root.Long` names mux
and every mux command has a `Short`), `sandbox_coverage_test` (mux covered, no stale muxpoc
exclusion), and `configreg_test` (mux in `Names()`). The module-wide overview `verify: go
build ./...` additionally confirms nothing else fails to compile.
