# Plan: Built-in CLI help: lyx self-documents modules & commands

```yaml
task: 'Built-in CLI help: lyx self-documents modules & commands'
slug: builtin-cli-help
approved: true
started: '20260627-150156'
parent: main
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: clihelp-foundation
    file: 01-clihelp-foundation.md
    depends-on: []
    verify: go test ./internal/clihelp/...
  - number: 2
    name: flat-and-config-modules
    file: 02-flat-and-config-modules.md
    depends-on: [1]
    verify: go test -tags integration ./internal/initcli/... ./internal/update/... ./internal/configcli/...
  - number: 3
    name: ide-and-weft
    file: 03-ide-and-weft.md
    depends-on: [1]
    verify: go test -tags integration ./internal/ide/... ./internal/weft/...
  - number: 4
    name: board
    file: 04-board.md
    depends-on: [1]
    verify: go test ./internal/board/...
  - number: 5
    name: muxpoc-and-warp
    file: 05-muxpoc-and-warp.md
    depends-on: [1]
    verify: go test -tags integration ./internal/muxpoc/... ./internal/warp/...
  - number: 6
    name: root-assembly-and-tests
    file: 06-root-assembly-and-tests.md
    depends-on: [2, 3, 4, 5]
    verify: go test ./cmd/lyx/...
```

## Shared Decisions

### Decision: cobra-style-c-seam

- **Decision:** Build the `lyx` command tree with `github.com/spf13/cobra`. Each module
  exposes `func Command() *cobra.Command` carrying `Use`/`Short`/`Long`. A thin
  `RunCLI(out io.Writer, args []string) int` (initcli: `RunInit`) adapter is **preserved**
  per module as the public seam; it delegates to `clihelp.Execute(Command(), out, args)`.
  `cmd/lyx/main.go` assembles a single root from every module's `Command()`. Handler logic
  moves from `switch` cases into each subcommand's `RunE` verbatim (except shared
  pre-dispatch — see `prerune-shared-resolution`). **Arg-index shift (applies to every
  converted handler):** under `clihelp.WrapRun(fn(out, args))`, `args` is the post-subcommand
  argument list — cobra has already stripped the subcommand token. A body that did `rest :=
  fs.Args(); subcommand := rest[0]; payload := rest[1]` must rebind: the subcommand is gone,
  and the JSON payload / positional slug is now `args[0]` (`fs.Arg(0)` → `args[0]`). Do not
  copy a `rest[1]`/`fs.Arg(0)` reference verbatim.
- **Rationale:** cobra gives `--help`/completion/suggestions for free and makes anti-drift
  a framework invariant; the preserved seam keeps the ~51 existing in-process tests
  compiling and passing. Full rationale in `_mill/discussion.md`.
- **Applies to:** all batches.

### Decision: exit-and-error-contract

- **Decision:** Exit codes flow through a **per-invocation** `*exitState{code int; abort
  bool}` carried in the command **context** (`clihelp`), never a package-level var. Root is
  configured `SilenceUsage=true`, `SilenceErrors=false`. Each subcommand `RunE` is a wrapper
  that runs the legacy handler, calls `setExit(cmd.Context(), handlerExitCode)`, and returns
  `nil` to cobra (so cobra never double-prints over a handler's JSON). cobra-level errors
  (unknown command, bad flag) are left un-silenced → cobra prints human text to stderr and
  `Execute()` returns an error → mapped to exit 1. `SilenceUsage=true` is set on BOTH the
  production root AND the module seam (`clihelp.Execute` sets it on the executed `Command()`),
  so neither dumps the full usage block on an error path. The holder is allocated
  fresh per `main`/`RunCLI` call and seeded via `ExecuteContext`; this is load-bearing for
  parallel tests (122 `t.Parallel()` sites).
- **Rationale:** Separates the two failure surfaces cleanly while preserving every existing
  JSON-error assertion. See `_mill/discussion.md` → Decisions → exit-and-error-contract.
- **Applies to:** all batches.

### Decision: help-and-unknown-surfaces

- **Decision:** Help is human plain text to stdout, exit 0 (`lyx`, `lyx <verb-module>`,
  `--help`). cobra-level errors go to stderr, exit 1 (NOT a JSON envelope — verified no
  test/caller depends on JSON there). **Two distinct cobra messages, do not conflate them:**
  an unknown module/subcommand prints `unknown command "x" for "…"` (+ "did you mean…?"
  suggestions); a bad/unknown flag prints `unknown flag: --x` (no suggestions). Real command
  results/errors stay JSON via `internal/output` on stdout. The `RunCLI`/`run` seam wires
  `SetOut(out)` **and** `SetErr(out)` (merged) so in-process tests capture cobra's text from
  one buffer; production `main` keeps `SetOut(os.Stdout)` / `SetErr(os.Stderr)` split. Tests
  assert the matching substring + exit code — `unknown command` for an unknown subcommand,
  `unknown flag` (or just exit 1) for a bad flag — never the exact parent qualifier.
- **Rationale:** Discoverability for typos; JSON contract preserved where it matters.
- **Applies to:** all batches.

### Decision: flags-to-pflag

- **Decision:** Migrate each command's flags from stdlib `flag` to cobra/pflag
  (`cmd.Flags()` / `cmd.PersistentFlags()`). Internal injected flags `--board-path` (board)
  and `--weft-path` (weft) become **hidden persistent** flags on their parent. muxpoc's
  tuning flags become persistent flags on the `muxpoc` parent. warp's per-verb flags
  (`remove --force`, `prune --apply`, `cleanup --apply/--force`) and update's `--apply`
  become local flags. Remove `update.go`'s `fs.Usage = func(){}` hack. All injected
  call-sites already use `--long` form, so pflag breaks nothing.
- **Rationale:** Auto-generated co-located flag help. See discussion → flags-to-pflag.
- **Applies to:** clihelp-foundation (none), flat-and-config-modules, ide-and-weft, board,
  muxpoc-and-warp.

### Decision: prerune-shared-resolution

- **Decision:** Modules that resolve cwd-dependent config/layout once before their old
  `switch` lift that resolution into a `PersistentPreRunE` on the parent command (cobra
  skips it on the no-`Run` parent's no-arg/`--help` listing path, so listing never requires
  a git repo). Affected: **muxpoc, board, weft, ide**. The resolved value(s) live in
  `Command()`-closure variables the PreRunE populates and each `RunE` closes over
  (module-local; not the context). board/weft preserve their `--board-path`/`--weft-path`
  bypass inside the PreRunE; weft's push `RunE` additionally branches on `--weft-path`
  (bypass `Push`-only vs normal `Commit`+`Push`). On PreRunE failure: write the existing
  JSON error, set `code`+`abort` on the holder, return `nil`; each `RunE` guards
  `if abort { return nil }`.
- **Rationale:** See discussion → Technical context → shared-pre-dispatch gotcha.
- **Applies to:** ide-and-weft, board, muxpoc-and-warp.

### Decision: json-help-and-completion

- **Decision:** A persistent `--json` flag on the root renders any help node as structured
  JSON via a custom `HelpFunc` installed with `root.SetHelpFunc` (inherited by children),
  not a `RunE`. Schema per node: `{name, short, long, commands:[{name,short,usage}],
  flags:[{name,shorthand,usage,default,type}]}`; `flags` is **local flags only**
  (`cmd.LocalFlags()`) minus hidden + the `--json`/`--help` meta. `--json` is inert on real
  command paths (NOT a second output-mode switch). cobra's built-in `lyx completion …`
  command is kept enabled (do NOT set `DisableDefaultCmd`).
- **Rationale:** Operator opted into `--json`; completion is free cobra payoff. See
  discussion → json-help-form.
- **Applies to:** clihelp-foundation, root-assembly-and-tests.

### Decision: testing-and-tags

- **Decision:** Preserve the seam so existing tests pass; update only the no-arg /
  unknown-subcommand / unknown-module / usage assertions (whether they check plain text OR
  `json.Unmarshal` a now-non-JSON buffer). New tree-level tests live in `cmd/lyx`
  (drift-guard, help-tree completeness, `--json` schema, exit-code) and must tolerate
  cobra's auto `help`/`completion` commands (superset assertions). Go-native `go test`; the
  CLI tests for initcli/ide/weft/warp are `//go:build integration` (so their batches verify
  with `-tags integration`); update/configcli/board/muxpoc/cmd-lyx are unit.
- **Rationale:** See discussion → Testing.
- **Applies to:** all batches.

## All Files Touched

- `cmd/lyx/drift_test.go`
- `cmd/lyx/exitcode_test.go`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/jsonhelp_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/main_test.go`
- `go.mod`
- `go.sum`
- `internal/board/cli.go`
- `internal/board/cli_test.go`
- `internal/clihelp/exec.go`
- `internal/clihelp/exec_test.go`
- `internal/clihelp/jsonhelp.go`
- `internal/clihelp/jsonhelp_test.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_test.go`
- `internal/ide/cli.go`
- `internal/ide/cli_test.go`
- `internal/initcli/initcli.go`
- `internal/initcli/initcli_test.go`
- `internal/muxpoc/cli.go`
- `internal/muxpoc/cli_test.go`
- `internal/update/update.go`
- `internal/update/update_test.go`
- `internal/warp/warp.go`
- `internal/warp/warp_test.go`
- `internal/weft/cli.go`
- `internal/weft/cli_test.go`
