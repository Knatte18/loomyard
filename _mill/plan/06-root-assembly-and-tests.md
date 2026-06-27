# Batch: root-assembly-and-tests

```yaml
task: 'Built-in CLI help: lyx self-documents modules & commands'
batch: root-assembly-and-tests
number: 6
cards: 6
verify: go test ./cmd/lyx/...
depends-on: [2, 3, 4, 5]
```

## Batch Scope

The capstone: rewrite `cmd/lyx/main.go` to assemble a single cobra root from every module's
`Command()`, wire the persistent `--json` flag + `clihelp.InstallJSONHelp`, seed the
exit-state holder, and split stdout/stderr for production. Then add the tree-level tests that
encode the task's guarantees: the drift-guard, help-tree completeness, `--json` schema, and
exit-code contract. Depends on all four module batches (it imports their `Command()`s). This
batch is what makes `lyx` self-documenting end to end.

Batch-local decision: keep a testable `func run(args []string, out io.Writer) int` seam (used
by `main_test.go`) that builds the root via a shared `newRoot()` builder with **merged**
out/err (so tests capture cobra text), while `main()` uses `newRoot()` with **split**
`os.Stdout`/`os.Stderr`. New test files are unit (no build tag).

## Cards

### Card 18: main.go cobra root assembly + --json + exit wiring

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/clihelp/jsonhelp.go`
  - `internal/initcli/initcli.go`
  - `internal/board/cli.go`
  - `internal/configcli/configcli.go`
  - `internal/update/update.go`
  - `internal/ide/cli.go`
  - `internal/muxpoc/cli.go`
  - `internal/weft/cli.go`
  - `internal/warp/warp.go`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace the `switch`-based `run()` with cobra assembly. Add `func
  newRoot() *cobra.Command` — root `Use: "lyx"`, `Short`/`Long`, `SilenceUsage: true`,
  `SilenceErrors: false`; add each module's `Command()` as a child (`init`, `board`,
  `config`, `update`, `ide`, `muxpoc`, `weft`, `warp`); add a persistent bool flag `--json`
  (captured in a local `var jsonFlag bool`) and call `clihelp.InstallJSONHelp(root,
  &jsonFlag)`. Do NOT set `CompletionOptions.DisableDefaultCmd` (keep `lyx completion`).
  Reimplement `func run(args []string, out io.Writer) int`: build `root := newRoot()`,
  `root.SetOut(out); root.SetErr(out)` (merged seam), `ctx, es := clihelp.NewExitContext(...)`,
  `root.SetArgs(args)`, `if err := root.ExecuteContext(ctx); err != nil { return 1 }`, return
  `es.code`. `main()` uses `newRoot()` with `SetOut(os.Stdout)`/`SetErr(os.Stderr)` (split),
  its own `NewExitContext`, and `os.Exit` mapping (`err != nil → 1`, else `es.code`). Remove
  the obsolete doc-comment module table (old lines ~11–20) and update the package doc to
  describe the cobra root.
- **Commit:** `feat(lyx): assemble cobra root with --json help and exit wiring`

### Card 19: main_test no-arg/unknown assertions

- **Context:**
  - `cmd/lyx/main.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update the surface assertions while keeping the dispatch tests. The no-arg
  test (`TestRunNoArgs` or equivalent) currently asserts exit 1 AND empty output — change BOTH:
  assert exit **0** AND non-empty output that names the modules (e.g. contains `board` and
  `warp`). `TestRunUnknownModule` keeps exit 1 but its output is now cobra's `unknown command`
  text (substring assertion), not empty. The `run(...)` dispatch tests for board/warp/ide/weft/
  config/update keep asserting their existing JSON/behaviour. Do not over-pin the cobra
  qualifier string.
- **Commit:** `test(lyx): update no-arg/unknown assertions for cobra root`

### Card 20: drift-guard test

- **Context:**
  - `cmd/lyx/main.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/drift_test.go`
- **Deletes:** none
- **Requirements:** Add a test that builds the root via `newRoot()` and walks the command tree
  recursively asserting every command has a non-empty `Short` — the structural
  self-documentation invariant. Tolerate cobra's auto-added `help` and `completion` subtrees:
  either skip commands whose `Name()` is `help`/`completion` (and descendants), or rely on
  cobra populating their `Short` — but do NOT assert an exact tree shape that breaks when
  cobra changes its auto-commands.
- **Commit:** `test(lyx): drift-guard asserting every command has a Short`

### Card 21: help-tree completeness test

- **Context:**
  - `cmd/lyx/main.go`
  - `internal/clihelp/exec.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/helptree_test.go`
- **Deletes:** none
- **Requirements:** Add a test asserting `lyx --help` output names every module
  (`init`/`board`/`config`/`update`/`ide`/`muxpoc`/`weft`/`warp`), and for each verb-module
  (`board`/`warp`/`weft`/`ide`/`muxpoc`) that `lyx <module> --help` names every one of its
  subcommands. Drive via the merged `run()` seam (capture output in a buffer). Use **superset**
  assertions (pinned set ⊆ output) so cobra's auto `help`/`completion` don't make it brittle.
- **Commit:** `test(lyx): help-tree completeness for modules and subcommands`

### Card 22: --json help schema test

- **Context:**
  - `cmd/lyx/main.go`
  - `internal/clihelp/jsonhelp.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/jsonhelp_test.go`
- **Deletes:** none
- **Requirements:** Add a test that drives `lyx --json`, `lyx <module> --json` (a verb
  module), and a leaf with a local flag — use `lyx warp remove --help --json` (`remove` owns
  `--force`) so the populated-`flags` assertion is not vacuous; do NOT pick a flagless leaf
  like `board upsert`. Each via the `run()` seam; assert the captured output is valid JSON
  matching the schema (`name`, `short`, `commands`, `flags`). Assert: the root JSON lists the
  modules under `commands`; the `warp remove` leaf has a populated `flags` (containing
  `--force`) and empty `commands`; hidden flags (`--board-path`, `--weft-path`) and the
  `--json`/`--help` meta flags are absent from any `flags` array.
- **Commit:** `test(lyx): --json help schema across tree levels`

### Card 23: exit-code contract test

- **Context:**
  - `cmd/lyx/main.go`
  - `internal/clihelp/exec.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/exitcode_test.go`
- **Deletes:** none
- **Requirements:** Add a test asserting the exit-code contract via the `run()` seam: bare
  `lyx` → exit 0; `lyx <verb-module>` with no subcommand → exit 0 with a subcommand listing;
  unknown module and unknown subcommand → exit 1 with `unknown command` in the buffer; a real
  handler failure still → exit 1 with a JSON `{"ok":false}` envelope on stdout (drive a
  representative valid-but-failing command that does not need external state, e.g. a board
  subcommand missing its required JSON payload → `json payload required`). Confirm help paths
  do not emit a JSON error envelope.
- **Commit:** `test(lyx): exit-code contract across help, unknown, and failure paths`

## Batch Tests

`verify: go test ./cmd/lyx/...` — unit, no tag. Runs `main_test.go` (dispatch + updated
no-arg/unknown) plus the four new tree-level tests (`drift_test.go`, `helptree_test.go`,
`jsonhelp_test.go`, `exitcode_test.go`). These build the real assembled root importing every
module's `Command()`, so they are the end-to-end proof that `lyx` self-documents and the
exit-code/JSON contracts hold. The module batches already verified their own packages; this
batch verifies the composition.
