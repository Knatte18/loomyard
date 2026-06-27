# Batch: ide-and-weft

```yaml
task: 'Built-in CLI help: lyx self-documents modules & commands'
batch: ide-and-weft
number: 3
cards: 3
verify: go test -tags integration ./internal/ide/... ./internal/weft/...
depends-on: [1]
```

## Batch Scope

Converts `ide` (subcommands `spawn`/`menu`) and `weft` (subcommands `status`/`commit`/`push`/
`pull`/`sync`). Both resolve cwd-dependent layout before their old switch, so both lift that
into a `PersistentPreRunE` (skipped on the no-arg listing). `weft` additionally carries the
hidden persistent `--weft-path` bypass, whose logic is split: the resolution-skip + non-push
gate live in the PreRunE, while the push `RunE` branches on the flag (bypass `Push`-only vs
normal `Commit`+`Push`). Both keep their `RunCLI` seam. Depends only on clihelp-foundation;
parallel-safe with batches 2, 4, 5.

Batch-local decision: ide has no internal bypass flag — its `PersistentPreRunE` simply
resolves cwd+layout into a closure var. The shared resolved values live in `Command()`-closure
variables (module-local), populated by the PreRunE and closed over by each `RunE`.

## Cards

### Card 9: ide Command() + PersistentPreRunE

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/ide/menu.go`
  - `internal/ide/spawn.go`
  - `internal/output/output.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/ide/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func Command() *cobra.Command` — parent `Use: "ide"`, `Short` (e.g.
  "VS Code worktree launcher"), with subcommands `spawn <slug>` and `menu`, each with its own
  `Short` and `RunE` wrapping the existing handler bodies (`Spawn(l, slug)`, the menu launch).
  Declare the layout `l` as a closure variable in `Command()`; populate it in a
  `PersistentPreRunE` that resolves cwd via `paths.Getwd` + `paths.Resolve` (today's lines
  ~31–41) — on failure write the existing `output.Err` and `clihelp.Abort(cmd.Context(), 1)`,
  return `nil`. Each subcommand `RunE` begins with the abort guard (handled by
  `clihelp.WrapRun`) and closes over `l`. The no-arg `lyx ide` now lists `spawn`/`menu`
  (parent has no `Run`) without resolving layout. `spawn` with no slug keeps its existing
  usage error. **Rebind per the arg-index-shift rule (00-overview → cobra-style-c-seam): in
  the `spawn` `RunE` the slug is `args[0]`, not `args[1:][0]` — cobra strips the `spawn`
  token before `RunE`.** Keep `func RunCLI(out io.Writer, args []string) int { return
  clihelp.Execute(Command(), out, args) }`.
- **Commit:** `refactor(ide): cobra Command() with PersistentPreRunE layout resolution`

### Card 10: weft Command() + PreRunE + push --weft-path fork

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/weft/spawn.go`
  - `internal/weft/sync.go`
  - `internal/weft/status.go`
  - `internal/weft/config.go`
  - `internal/weft/weft.go`
  - `internal/output/output.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/weft/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func Command() *cobra.Command` — parent `Use: "weft"`, `Short` (e.g.
  "weft git operations"), with subcommands `status`/`commit`/`push`/`pull`/`sync`, each `RunE`
  wrapping the existing handler body. Add a **hidden persistent** flag `--weft-path` (string,
  usage "internal: injected absolute weft worktree path for the detached push child") via
  `cmd.PersistentFlags().String` + `cmd.PersistentFlags().MarkHidden("weft-path")`. Declare
  closure vars for the resolved `l`/`cfg`/`pathspec`. In `PersistentPreRunE`: if `--weft-path`
  is set, skip cwd/layout resolution and record that the run is in bypass mode (the
  non-`push` rejection: if the executing subcommand is not `push`, write the existing
  `output.Err(out, "subcommand requires a worktree context")` + `clihelp.Abort(ctx, 1)`,
  return `nil`); otherwise resolve cwd → `paths.Resolve` → `cfg` via `LoadConfig` →
  `pathspec` (today's lines ~82–104), aborting with the existing JSON error on failure. The
  **push `RunE` branches on `--weft-path`**: when set, call `Push(weftPath, SyncOptions{})`
  directly (today's lines ~76); otherwise the normal `Commit`+`Push` path (~123–132). Other
  subcommands close over the resolved `l`/`cfg`/`pathspec`. No-arg `lyx weft` lists
  subcommands without resolving. Keep `func RunCLI(out io.Writer, args []string) int { return
  clihelp.Execute(Command(), out, args) }`.
- **Commit:** `refactor(weft): cobra Command() with PreRunE + --weft-path push fork`

### Card 11: ide + weft no-arg/unknown test assertions

- **Context:**
  - `internal/ide/cli.go`
  - `internal/weft/cli.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/ide/cli_test.go`
  - `internal/weft/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update only the cobra-surface assertions. ide: no-arg now → exit 0 +
  subcommand listing (was `output.Err` JSON usage + exit 1); unknown subcommand → `unknown
  command` substring + exit 1. weft: no-arg now → exit 0 + listing (was plain-text usage +
  exit 1); unknown subcommand → `unknown command` substring + exit 1; the `--weft-path` +
  non-push test (`TestRunCLI_WeftPathPushOnly`) MUST still assert the JSON `{"ok":false,
  "error":"subcommand requires a worktree context"}` envelope (that path is preserved via the
  PreRunE abort). Leave `spawn`/`status`/push behaviour assertions intact.
- **Commit:** `test(cli): update no-arg/unknown assertions for ide and weft`

## Batch Tests

`verify: go test -tags integration ./internal/ide/... ./internal/weft/...`. Both
`ide/cli_test.go` and `weft/cli_test.go` are `//go:build integration`, so the tag is
required; this also pulls `weft_integration_test.go` (real git, ~12s). Covers ide spawn/menu
+ no-arg/unknown, and weft status/commit/push/pull/sync + the `--weft-path` bypass gate +
no-arg/unknown.
