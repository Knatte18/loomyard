# Batch: muxpoc-and-warp

```yaml
task: 'Built-in CLI help: lyx self-documents modules & commands'
batch: muxpoc-and-warp
number: 5
cards: 3
verify: go test -tags integration ./internal/muxpoc/... ./internal/warp/...
depends-on: [1]
```

## Batch Scope

Converts the two flag-heavy modules. `muxpoc` (subcommands `up`/`review`/`attach`/`status`/
`down`/`daemon`) has top-level tuning flags that become **persistent** flags on the parent,
plus a shared `cfg`/layout pre-dispatch that moves into a `PersistentPreRunE`. `warp`
(subcommands `clone`/`add`/`list`/`remove`/`checkout`/`status`/`reconcile`/`prune`/`cleanup`)
has no shared cwd pre-dispatch but does have **per-verb** flags (`remove --force`, `prune
--apply`, `cleanup --apply/--force`) that become local flags on those subcommands. Both keep
their `RunCLI` seam. Depends only on clihelp-foundation; parallel-safe with batches 2, 3, 4.

Batch-local decision: muxpoc's resolved `cfg` is the shared closure value populated by the
PreRunE. warp's top-level dispatcher had no FlagSet — only specific verbs own flags, so warp
needs no `PersistentPreRunE`, only per-subcommand local flags and positional parsing inside
each `RunE`.

## Cards

### Card 15: muxpoc Command() + persistent tuning flags + PreRunE

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/muxpoc/cmd.go`
  - `internal/muxpoc/up.go`
  - `internal/muxpoc/review.go`
  - `internal/muxpoc/attach.go`
  - `internal/muxpoc/down.go`
  - `internal/muxpoc/status.go`
  - `internal/muxpoc/daemon.go`
  - `internal/output/output.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/muxpoc/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func Command() *cobra.Command` — parent `Use: "muxpoc"`, `Short`
  (e.g. "proof-of-concept psmux mux"), with subcommands `up`/`review`/`attach`/`status`/
  `down`/`daemon`, each `RunE` wrapping the existing `cmd<X>` handler. Migrate the tuning
  flags to **persistent** flags on the parent via `cmd.PersistentFlags()`: `--psmux`,
  `--pwsh`, `--claude`, `--launch`, `--resume` (strings), `--width` (int, default 220),
  `--height` (int, default 50), `--interval` (duration, default 2s) — preserve current
  defaults and usage strings. Move the pre-switch `cfg` construction + `paths.Resolve` (today
  ~54–94) into a `PersistentPreRunE` that builds `cfg` from the flag values into a closure
  var, aborting with the existing `output.Err` + `clihelp.Abort(ctx, 1)` on resolve failure.
  Each subcommand `RunE` closes over `cfg`. No-arg `lyx muxpoc` lists subcommands without
  resolving. Keep `func RunCLI(out io.Writer, args []string) int { return
  clihelp.Execute(Command(), out, args) }`.
- **Commit:** `refactor(muxpoc): cobra Command() with persistent tuning flags + PreRunE`

### Card 16: warp Command() with per-verb flags

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/warp/remove.go`
  - `internal/warp/prune.go`
  - `internal/warp/cleanup.go`
  - `internal/warp/list.go`
  - `internal/warp/status.go`
  - `internal/warp/add.go`
  - `internal/warp/clone.go`
  - `internal/warp/checkout.go`
  - `internal/warp/reconcile.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/warp/warp.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func Command() *cobra.Command` — parent `Use: "warp"`, `Short` (e.g.
  "host↔weft coordination"), with one subcommand per verb (`clone`, `add <slug>`, `list`,
  `remove [--force] <slug>`, `checkout <branch>`, `status`, `reconcile`, `prune [--apply]`,
  `cleanup [--apply] [--force]`), each `RunE` wrapping the existing `run<Verb>` handler body.
  Migrate the per-verb flags to **local** flags on their subcommands: `remove` → `--force`;
  `prune` → `--apply`; `cleanup` → `--apply` + `--force` (preserve the existing usage strings
  and the "`--force` alone reports only" semantics inside the handler). clone/add/list/
  checkout/status/reconcile parse positional args directly inside their `RunE` as today. Keep
  `func RunCLI(out io.Writer, args []string) int { return clihelp.Execute(Command(), out,
  args) }`.
- **Commit:** `refactor(warp): cobra Command() with per-verb flags`

### Card 17: muxpoc + warp no-arg/unknown test assertions

- **Context:**
  - `internal/muxpoc/cli.go`
  - `internal/warp/warp.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/muxpoc/cli_test.go`
  - `internal/warp/warp_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** muxpoc (`cli_test.go`, unit): no-arg now → exit 0 + subcommand listing
  (was exit 1 + empty stdout); unknown **subcommand** → `unknown command` substring + exit 1;
  the existing `TestRunCLIUnknownFlagFails` (`{"--no-such-flag","status"}`) is a **bad-flag**
  case → assert the `unknown flag` substring (NOT `unknown command`) + exit 1 — these are two
  different cobra messages. warp (`warp_test.go`, integration): the `UnknownSubcommand` test
  currently `json.Unmarshal`s the buffer and asserts `ok=false` — switch it to assert the
  `unknown command` substring + exit 1 (warp's old unknown path emitted JSON; under cobra the
  buffer holds plain text). Leave warp's `list` success and `remove --force` flag-parsing +
  effect assertions intact.
- **Commit:** `test(cli): update no-arg/unknown assertions for muxpoc and warp`

## Batch Tests

`verify: go test -tags integration ./internal/muxpoc/... ./internal/warp/...`. `warp_test.go`
is `//go:build integration`, so the tag is required; `muxpoc/cli_test.go` is unit and runs
regardless (the `//go:build smoke` muxpoc smoke test stays excluded). Covers muxpoc
subcommand dispatch + no-arg/unknown, and warp list/remove behaviour + the converted
unknown-subcommand assertion.
