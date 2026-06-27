# Batch: board

```yaml
task: 'Built-in CLI help: lyx self-documents modules & commands'
batch: board
number: 4
cards: 3
verify: go test ./internal/board/...
depends-on: [1]
```

## Batch Scope

Converts the `board` module — the largest verb-switch (11 subcommands: `upsert`,
`upsert-batch`, `set-phase`, `remove`, `get`, `list`, `list-full`, `merge`, `set-deps`,
`rerender`, `sync`). board builds `cfg` + `b := New(cfg)` once before its switch, conditional
on the hidden `--board-path` bypass, so that shared resolution moves into a `PersistentPreRunE`
with the bypass preserved. Every subcommand keeps its existing JSON payload parsing and
`internal/output` responses verbatim inside its `RunE`. Keeps the `RunCLI` seam. Depends only
on clihelp-foundation; parallel-safe with batches 2, 3, 5.

Batch-local decision: the `Board` instance `b` is the shared resolved value, held in a
`Command()`-closure variable populated by the `PersistentPreRunE` and closed over by all 11
subcommand `RunE`s. `applySkipEnv` continues to fold `BOARD_SKIP_*` at the single resolution
point (now the PreRunE).

## Cards

### Card 12: board Command() with 11 subcommands

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/board/board.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/board/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `func Command() *cobra.Command` — parent `Use: "board"`, `Short`
  (e.g. "task-tracker board"). Add one subcommand per existing `case` with `Use` =
  subcommand name (e.g. `Use: "upsert [json-payload]"`), a one-line `Short` derived from the
  existing case comment, and a `RunE` (via `clihelp.WrapRun` over a per-subcommand handler
  func) containing that case's body — JSON payload extraction, `json.Unmarshal`,
  the `b.<Method>` call, and the `output.*` response. **Rebind the payload arg per the
  arg-index-shift rule (00-overview → cobra-style-c-seam): the JSON payload is `args[0]`, not
  `rest[1]` — cobra strips the subcommand before `RunE`, so "json payload required" now means
  `len(args) == 0`.** Preserve the "json payload required" errors. The `b *Board` is closed
  over (set by the PreRunE in Card 13). Keep the existing
  `outputError`/`outputSuccess*` helpers. Keep `func RunCLI(out io.Writer, args []string)
  int { return clihelp.Execute(Command(), out, args) }`.
- **Commit:** `refactor(board): cobra Command() with 11 subcommands`

### Card 13: board PersistentPreRunE + hidden --board-path

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/board/board.go`
  - `internal/board/spawn.go`
  - `internal/output/output.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/board/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add a **hidden persistent** flag `--board-path` (string, usage
  "internal: injected absolute board dir for the detached sync child") on the board parent via
  `cmd.PersistentFlags().String` + `MarkHidden`. Add a `PersistentPreRunE` that reproduces
  today's resolution (cli.go ~67–101): if `--board-path` is set, `cfg = Config{Path:
  boardPath}`; else resolve cwd via `paths.Getwd` + `LoadConfig(cwd, "board")`, aborting with
  the existing `output.Err` + `clihelp.Abort(ctx, 1)` on failure. Apply `applySkipEnv(cfg)`,
  then `b = New(cfg)` into the closure var. Confirm `internal/board/spawn.go:27`'s injected
  `lyx board --board-path <abs> sync` still parses (persistent flag accepted before the
  subcommand). The no-arg `lyx board` lists subcommands without resolving config.
- **Commit:** `refactor(board): PersistentPreRunE config resolution with hidden --board-path`

### Card 14: board no-arg/unknown test assertions

- **Context:**
  - `internal/board/cli.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/board/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `board/cli_test.go` is unit (no build tag) and uses an external
  `board_test` package with a `runCLI(t, args...)` wrapper over `board.RunCLI` — keep the
  wrapper. Update only: no-arg now → exit 0 + subcommand listing (was plain-text usage + exit
  1); unknown subcommand → `unknown command` substring + exit 1 (was plain-text `unknown
  subcommand` + exit 1). Leave the per-subcommand behaviour assertions (e.g. `list`,
  `rerender`, the JSON error envelope, `--board-path`-driven flows) intact.
- **Commit:** `test(board): update no-arg/unknown assertions`

## Batch Tests

`verify: go test ./internal/board/...` — unit, no tag, no external tools (board tests run
with `BOARD_SKIP_GIT=1` and a seeded `_lyx/config/board.yaml`). Covers the 11 subcommands'
behaviour, the JSON error envelope, the `--board-path` bypass, and the updated no-arg/unknown
assertions.
