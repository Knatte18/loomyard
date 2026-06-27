# Batch: clihelp-foundation

```yaml
task: 'Built-in CLI help: lyx self-documents modules & commands'
batch: clihelp-foundation
number: 1
cards: 4
verify: go test ./internal/clihelp/...
depends-on: []
```

## Batch Scope

This batch adds the `github.com/spf13/cobra` dependency and a new shared `internal/clihelp`
package that every module and `cmd/lyx` will build on. It delivers three reusable pieces:
the per-invocation exit-state holder + the `RunCLI`-seam adapter + the legacy-handler→`RunE`
wrapper (`exec.go`), and the `--json` help renderer + `HelpFunc` installer (`jsonhelp.go`).
Nothing outside `internal/clihelp`, `go.mod`, `go.sum` is touched — modules and main consume
this API in later batches. The external interface the next batches consume:
`clihelp.Execute(cmd, out, args) int`, `clihelp.WrapRun(fn) func(*cobra.Command,[]string)
error`, `clihelp.SetExit`/`Abort`/`ShouldAbort` helpers, `clihelp.NewExitContext`, and
`clihelp.InstallJSONHelp(root, &jsonFlag)`.

Batch-local decision: the exact exported names above are introduced here and are the
contract for all later batches; keep them stable. Implement the holder strictly as a
context value (no package-level mutable state).

## Cards

### Card 1: Add cobra dependency

- **Context:**
  - `go.mod`
- **Edits:**
  - `go.mod`
  - `go.sum`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `github.com/spf13/cobra` to the module. Run `go get
  github.com/spf13/cobra@latest` then `go mod tidy` so `go.mod` gains the `require` entry
  (cobra + its transitive `github.com/spf13/pflag`) and `go.sum` is updated. Do not add any
  other dependency. Verify the module still builds with `go build ./...`.
- **Commit:** `build(lyx): add spf13/cobra dependency`

### Card 2: clihelp exit-state holder, seam adapter, and RunE wrapper

- **Context:**
  - `internal/output/output.go`
  - `go.mod`
- **Edits:** none
- **Creates:**
  - `internal/clihelp/exec.go`
- **Deletes:** none
- **Requirements:** Create package `clihelp`. Define `type exitState struct { code int;
  abort bool }` and an unexported context key type `ctxKey struct{}` with `var exitKey =
  ctxKey{}`. Expose:
  - `func NewExitContext(parent context.Context) (context.Context, *exitState)` — allocates a
    fresh `*exitState`, returns it plus a context carrying it. **Per-invocation only — no
    package-level state.**
  - `func SetExit(ctx context.Context, code int)` — if a non-zero `code` and the context
    carries an `*exitState`, records `code`.
  - `func Abort(ctx context.Context, code int)` — sets `code` (non-zero) and `abort = true`;
    used by a failing `PersistentPreRunE`.
  - `func ShouldAbort(ctx context.Context) bool` — reports the holder's `abort` flag.
  - `func WrapRun(fn func(out io.Writer, args []string) int) func(*cobra.Command, []string)
    error` — returns a cobra `RunE` that: first `if ShouldAbort(cmd.Context()) { return nil
    }`; else calls `SetExit(cmd.Context(), fn(cmd.OutOrStdout(), args))` and returns `nil`.
  - `func Execute(cmd *cobra.Command, out io.Writer, args []string) int` — the seam body:
    `cmd.SilenceUsage = true` (so a cobra-error path does not dump the full usage block into
    the merged buffer — parity with the production root), `cmd.SetOut(out); cmd.SetErr(out)`
    (merged), `ctx, es := NewExitContext(context.Background())`, `cmd.SetArgs(args)`, `if err
    := cmd.ExecuteContext(ctx); err != nil { return 1 }`, return `es.code`. (Leave
    `SilenceErrors` at its default false so cobra still writes the `unknown command`/`unknown
    flag` message into the merged buffer.)
  Use `github.com/spf13/cobra`. Godoc each exported symbol per `golang-comments`.
- **Commit:** `feat(clihelp): exit-state holder, seam adapter, and RunE wrapper`

### Card 3: clihelp --json help renderer and HelpFunc installer

- **Context:**
  - `internal/clihelp/exec.go`
  - `go.mod`
- **Edits:** none
- **Creates:**
  - `internal/clihelp/jsonhelp.go`
- **Deletes:** none
- **Requirements:** In package `clihelp`, define the JSON help schema structs:
  `type flagJSON struct { Name, Shorthand, Usage, Default, Type string }` and
  `type cmdJSON struct { Name, Short, Long string; Commands []cmdChild; Flags []flagJSON }`
  (with `cmdChild` = `{Name, Short, Usage string}`), all with lowercase `json:"..."` tags
  matching the discussion schema (`name`, `short`, `long`, `commands`, `flags`,
  `shorthand`, `default`, `type`, `usage`). Expose:
  - `func renderCmdJSON(cmd *cobra.Command) cmdJSON` — `Name` = `cmd.CommandPath()`, `Short`/
    `Long` from the command; `Commands` = non-hidden immediate subcommands (skip the cobra
    auto `help`/`completion`) as `{Name: child.Name(), Short, Usage: child.UseLine()}`;
    `Flags` = `cmd.LocalFlags()` visited, **excluding hidden flags and the `--json`/`--help`
    meta flags**, each `{Name: "--"+f.Name, Shorthand, Usage, Default: f.DefValue, Type:
    f.Value.Type()}`.
  - `func InstallJSONHelp(root *cobra.Command, jsonFlag *bool)` — calls `root.SetHelpFunc`
    with a func that, when `*jsonFlag` is true, writes `json.MarshalIndent(renderCmdJSON(cmd))`
    + newline to `cmd.OutOrStdout()`; otherwise calls the previously-captured default help
    func (capture `root.HelpFunc()` before overriding). The help func is inherited by all
    children, so it covers `lyx --json`, `lyx <module> --json`, and `lyx <module> <cmd>
    --help --json`.
  Godoc each exported symbol.
- **Commit:** `feat(clihelp): --json help renderer and HelpFunc installer`

### Card 4: clihelp unit tests

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/clihelp/jsonhelp.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/clihelp/exec_test.go`
  - `internal/clihelp/jsonhelp_test.go`
- **Deletes:** none
- **Requirements:** `exec_test.go`: build a synthetic `*cobra.Command` tree in-test and
  assert: `Execute` returns 0 on a success `RunE` (handler returns 0) and the recorded code
  on a failing one (handler returns 1); `WrapRun` short-circuits when `Abort` was called in a
  `PersistentPreRunE` (the leaf `RunE` body does not run); an unknown subcommand makes
  `Execute` return 1 and writes `unknown command` into the merged `out` buffer. Use
  `t.Parallel()` and confirm two concurrent `Execute` calls do not cross exit codes (guards
  the per-invocation-holder decision). `jsonhelp_test.go`: install `InstallJSONHelp` on a
  synthetic root with a child + a local flag + a hidden flag; with the `--json` flag set,
  assert the help output is valid JSON matching the schema, lists the child, includes the
  local flag, and omits the hidden flag and the `--json`/`--help` meta flags.
- **Commit:** `test(clihelp): cover exit-state, seam adapter, and --json help`

## Batch Tests

`verify: go test ./internal/clihelp/...` runs the two new unit test files (`exec_test.go`,
`jsonhelp_test.go`). The package is new and self-contained — no integration tag, no external
tools. These tests exercise the full clihelp API that batches 2–6 depend on, so a regression
here surfaces before any module consumes it.
