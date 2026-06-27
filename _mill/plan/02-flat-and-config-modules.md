# Batch: flat-and-config-modules

```yaml
task: 'Built-in CLI help: lyx self-documents modules & commands'
batch: flat-and-config-modules
number: 2
cards: 4
verify: go test -tags integration ./internal/initcli/... ./internal/update/... ./internal/configcli/...
depends-on: [1]
```

## Batch Scope

Converts the three modules with no verb-switch — `init` and `update` (flat leaf commands)
and `config` (single command with an optional module-name positional + interactive menu).
Each gains a `Command() *cobra.Command` and keeps its existing public seam (`initcli.RunInit`,
`configcli.RunCLI`, `update.RunCLI`) delegating to `clihelp.Execute`. None of these three has
a shared multi-subcommand pre-dispatch, so no `PersistentPreRunE` is needed here (configcli
resolves inside its single Run; init/update are flat). `configcli` keeps its plain-text
output (it does NOT route through `internal/output`). This batch depends only on
clihelp-foundation and is parallel-safe with batches 3–5.

Batch-local decision: `config` becomes a leaf command (it has no subcommands of its own); the
optional positional module name is validated by the existing handler logic, optionally with
`ValidArgs` set to the known module names for completion. Do not convert the interactive menu
into subcommands.

## Cards

### Card 5: initcli Command() + RunInit seam

- **Context:**
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/initcli/initcli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func Command() *cobra.Command` returning a leaf command with
  `Use: "init"`, a one-line `Short` (e.g. "scaffold _lyx/config/ in the current directory")
  and a `Long` summarising the existing behaviour. Move the current `RunInit` body into a
  package-private handler `func runInit(out io.Writer, args []string) int` and set the
  command's `RunE: clihelp.WrapRun(runInit)`. Re-point the public seam: `func RunInit(out
  io.Writer, args []string) int { return clihelp.Execute(Command(), out, args) }`. Behaviour
  on `lyx init` (no args) is unchanged — it still scaffolds. Do not change the init logic
  itself.
- **Commit:** `refactor(initcli): expose cobra Command(), keep RunInit seam`

### Card 6: update Command() + --apply pflag

- **Context:**
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/update/update.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func Command() *cobra.Command` — a leaf command `Use: "update"`,
  `Short` (e.g. "reconcile module configs against templates"), with a local bool flag
  `--apply` (default false, usage "apply changes to disk (default: dry-run)") via
  `cmd.Flags().Bool`. Remove the stdlib `flag.NewFlagSet` and the `fs.Usage = func(){}` hack.
  Move the reconcile body into `func runUpdate(out io.Writer, apply bool) int`; the command's
  `RunE` reads the `--apply` flag value and calls it (wrap via `clihelp.WrapRun` over a small
  closure that reads the flag, or set `SetExit` directly — keep consistent with
  `clihelp.WrapRun`'s `func(out, args) int` shape by reading the flag inside the wrapped
  func). Keep `func RunCLI(out io.Writer, args []string) int { return clihelp.Execute(Command(),
  out, args) }`. Dry-run remains the default; `lyx update --apply` applies.
- **Commit:** `refactor(update): cobra Command() with --apply pflag, drop fs.Usage hack`

### Card 7: configcli Command() + optional module positional

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/configcli/menu.go`
- **Edits:**
  - `internal/configcli/configcli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func Command() *cobra.Command` — `Use: "config [module]"`, `Short`
  (e.g. "edit module configuration"). `Args: cobra.MaximumNArgs(1)`; optionally set
  `ValidArgs` to the known config module names. Move the current dispatch body into `func
  runConfig(out io.Writer, args []string) int` preserving today's behaviour exactly: no
  positional → interactive `menu`; one positional → `editOne` for that module; unknown module
  name → the existing plain-text `unknown config module: %s (known: …)` on `out` + exit 1.
  Preserve all plain-text `fmt.Fprintf` output (do NOT convert to `internal/output`). Set
  `RunE: clihelp.WrapRun(runConfig)`. Keep `func RunCLI(out io.Writer, args []string) int {
  return clihelp.Execute(Command(), out, args) }`.
- **Commit:** `refactor(configcli): cobra Command() preserving menu and plain-text output`

### Card 8: update no-arg/unknown test assertions for batch-2 modules

- **Context:**
  - `internal/initcli/initcli.go`
  - `internal/update/update.go`
  - `internal/configcli/configcli.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/initcli/initcli_test.go`
  - `internal/update/update_test.go`
  - `internal/configcli/configcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update only the assertions affected by the cobra surface change; leave
  behaviour assertions (init success/no-pairing error, update dry-run vs `--apply`, config
  edit/menu/unknown-module) intact since those still produce their existing output. Audit each
  file for: (a) any test invoking the module with an unknown/bad flag that previously expected
  a JSON `{"ok":false}` envelope or stdlib usage text — re-assert on the `unknown command`
  substring (merged into the buffer) + exit code; (b) any no-arg assertion that changed.
  initcli/configcli's real behaviours are unaffected (no-arg init still scaffolds, no-arg
  config still opens the menu, unknown config module still prints its own plain text). If a
  given file needs no change after audit, leave it untouched. Do not over-pin the exact cobra
  qualifier string. **Match the cobra message to the case:** an unknown subcommand →
  `unknown command` substring; a bad/unknown flag → `unknown flag` substring (or just exit
  1) — these are two different cobra messages; do not assert `unknown command` for a bad-flag
  case.
- **Commit:** `test(cli): update no-arg/unknown assertions for init/update/config`

## Batch Tests

`verify: go test -tags integration ./internal/initcli/... ./internal/update/...
./internal/configcli/...`. The `-tags integration` flag is required because
`initcli_test.go` and `configcli_integration_test.go` are `//go:build integration`; it also
runs the non-tagged `update_test.go` and `configcli_test.go`. These cover init scaffolding +
no-pairing error, update dry-run/apply, and config menu/edit/unknown-module behaviour, plus
the updated no-arg/unknown assertions.
