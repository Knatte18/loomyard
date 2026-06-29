# Batch: update fold to config reconcile

```yaml
task: "Rename Cobra modules to `<module>cli`, extract kernels as `<module>engine`"
batch: "update fold to config reconcile"
number: 7
cards: 4
verify: "go build ./... && go test ./... && go test -tags integration ./..."
depends-on: [6]
```

## Rename mechanic — `git mv` where code moves, not rewrite

This batch is mostly behavioural (new `reconcile` subcommand) but it **re-homes** logic
out of `internal/update`. Where a file or its body moves to `internal/configcli`, move it
rather than rewriting:

1. `git mv <old-path> <new-path>` first for any file that relocates, then apply surgical
   edits to the package declaration, import paths, and identifier retargeting.
2. Genuinely new code (the test-first `reconcile` test, the new subcommand wiring) is
   authored normally — full-file creation is correct only where there is no predecessor
   file.
3. Never write a file from scratch and then delete its old twin.

## Batch Scope

The one behavioural change in the whole task: remove `internal/update` and re-home its
behaviour as a `reconcile` subcommand on `lyx config` (`lyx update` → `lyx config
reconcile`), with no backward-compat alias. This is the TDD batch: write the migrated
reconcile test first (card 20, red), then add the subcommand (card 21, green), then remove
`lyx update` everywhere and update the coupled guards (card 22). `lyx config` keeps its
existing `[module]` edit/menu `RunE`; cobra resolves `lyx config reconcile` to the new
subcommand and `lyx config <module>` to the `RunE` arg, so both coexist.

## Cards

### Card 20: Write the migrated reconcile test (TDD)

- **Context:**
  - `internal/update/update_test.go`
  - `internal/configcli/configcli.go`
  - `internal/configsync/configsync.go`
  - `internal/paths/paths.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/configcli/reconcile_test.go`
- **Deletes:** none
- **Requirements:** Create `internal/configcli/reconcile_test.go` as an **internal** test
  file `package configcli` (matching the existing `configcli_test.go`, and matching
  `update_test.go`'s internal `package update`); because it is internal it calls the seam
  **unqualified** as `RunCLI(...)`, never `configcli.RunCLI(...)`. Migrate the two
  scenarios from `internal/update/update_test.go` to drive the new subcommand through the
  config seam: dry-run default — `RunCLI(&buf, []string{"reconcile"})` returns 0, JSON
  `ok=true`, top-level `applied=false`, the on-disk `board.yaml` is unchanged, and
  `modules` is a non-empty array whose first entry has `module`/`added`/`removed`/`applied`
  fields; and `--apply` — `RunCLI(&buf, []string{"reconcile", "--apply"})` returns 0, JSON
  `applied=true`, and `weft.yaml` is created on disk. Reuse the update_test fixture setup
  verbatim (`gitexec.RunGit(["init"], tmpDir)`, `paths.ConfigDir`, `paths.ConfigFile`,
  chdir into the temp repo). Keep the test untagged (matching `update_test.go`). This test
  will fail until card 21 adds the subcommand — that is the intended TDD red.
- **Commit:** `test(configcli): add reconcile subcommand test (TDD)`

### Card 21: Add the `reconcile` subcommand to `configcli`

- **Context:**
  - `internal/update/update.go`
  - `internal/configsync/configsync.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/configcli/configcli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add a `reconcile` subcommand to the command returned by
  `configcli.Command()`. Add the `internal/configsync` import to `configcli`. The
  subcommand carries a non-empty `Short` (e.g. "reconcile module configs against
  templates") — required by `drift_test` — and an `--apply` bool flag (default false =
  dry-run). Its `RunE` is `clihelp.WrapRun` over a handler that is the current
  `update.runUpdate` body: `paths.Getwd` → `paths.Resolve(cwd)` → `baseDir =
  filepath.Join(l.WorktreeRoot, l.RelPath)` → `configsync.ReconcileAll(baseDir, apply)` →
  map each result (`module`/`added`/`removed`/`applied`) into the `output.Ok` envelope
  `{"applied": apply, "modules": [...]}`. Register the subcommand on the config command
  via `configCmd.AddCommand(...)` inside `Command()` before returning. Do not change the
  config command's existing `RunE`, `--print` flag, `Args` (`cobra.MaximumNArgs(1)`), or
  `ValidArgs`. After this card card 20's test must pass.
- **Commit:** `feat(configcli): add reconcile subcommand (lyx config reconcile)`

### Card 22: Remove `lyx update` and update coupled guards

- **Context:**
  - `internal/update/update.go`
  - `internal/configcli/configcli.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/jsonhelp_test.go`
  - `cmd/lyx/main_test.go`
  - `cmd/lyx/unknown_subcommand_test.go`
  - `internal/configreg/configreg.go`
- **Creates:** none
- **Deletes:**
  - `internal/update/update.go`
  - `internal/update/update_test.go`
- **Requirements:** In `cmd/lyx/main.go` remove the `internal/update` import, remove
  `update.Command()` from the `root.AddCommand(...)` call in `newRoot()`, and remove the
  word `update` from the root command's `Long` "Available modules: …" list. In
  `cmd/lyx/helptree_test.go` remove `"update"` from the `requiredModules` slice in
  `TestHelpTree_RootNamesAllModules`, and add a focused assertion that `lyx config
  reconcile` is discoverable: invoke `run([]string{"config", "--help"}, &out)` (which
  short-circuits config's menu `RunE` and exits 0) and assert the output contains
  `reconcile`. Do NOT add a bare-`config` entry to `TestHelpTree_VerbModuleSubcommands`
  (bare `lyx config` runs the interactive menu `RunE`, which would block the test). In
  `cmd/lyx/unknown_subcommand_test.go` add a test asserting `run([]string{"update"}, &out)`
  no longer resolves: exit code 1 with a JSON error envelope (`ok=false`). In
  `internal/configreg/configreg.go` update the package doc comment to drop the now-removed
  `update` mention ("used by init, update, and config CLI commands" → "used by init and
  config CLI commands"). Then delete `internal/update/update.go` and
  `internal/update/update_test.go` (the whole `internal/update` directory).
- **Commit:** `refactor(lyx): remove update command, fold into config reconcile`

### Card 23: Repoint configengine "lyx update" guidance to "lyx config reconcile"

- **Context:**
  - `internal/configcli/configcli.go`
- **Edits:**
  - `internal/configengine/config.go`
  - `internal/configengine/config_test.go`
  - `docs/shared-libs/configengine.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `internal/configengine/config.go` emits two error strings that
  instruct the user to run the now-removed command — line 59
  `config file %s not found; run "lyx update"` and line 78
  `config file %s: missing keys: %s; run "lyx update"`. Change both `lyx update`
  occurrences to `lyx config reconcile` so the guidance points at the live command, and
  update the two matching doc comments (lines 40 and 42 that paraphrase the instruction).
  In `internal/configengine/config_test.go` update the two assertions that check the error
  contains `"lyx update"` (≈ lines 97 and 127) to assert `"lyx config reconcile"` instead.
  In `docs/shared-libs/configengine.md` replace every `lyx update` / `lyx update --apply`
  reference (the error-guidance and reconciliation prose at lines ≈37, 38, 47, 82, 110,
  111) with `lyx config reconcile` / `lyx config reconcile --apply`. This keeps the fold
  consistent end-to-end: no live error message or shared-lib doc references a command that
  no longer exists.
- **Commit:** `refactor(configengine): repoint config guidance to lyx config reconcile`

## Batch Tests

`verify` is repo-wide (Tier 1 + Tier 2). The new `internal/configcli/reconcile_test.go` is
untagged (Tier 1) and is the behaviour guard for the fold: dry-run writes nothing, `--apply`
writes, and the JSON envelope shape (`applied`, `modules[]` with
`module`/`added`/`removed`/`applied`) matches the old `lyx update`. The cmd/lyx guards
re-validate the removal: `helptree_test` no longer requires `update` and now asserts
`reconcile` discoverability; `unknown_subcommand_test` asserts `lyx update` is unknown;
`longlist_test` and `registration_test` self-derive (update is no longer a registered
child, so neither requires it). `drift_test` confirms the new `reconcile` subcommand
carries a `Short`.
