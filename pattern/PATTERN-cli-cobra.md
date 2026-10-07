# PATTERN-cli-cobra

Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.

- Each module exposes `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int`.
  Every module but `internal/selfreportcli` also carries `RunCLIIn(cwd, out, args) int`.
- An alias command may delegate into another module's subtree with no seam function of its own.
- Every command has a non-empty `Short`.
- One walk of `newRoot()` in `cmd/lyx/clitree_test.go` enforces `Short`, bare-group listing and unknown-subcommand refusal for every command.
  `cmd/lyx/registration_test.go` is the oracle that every `Command()` package is mounted.
  Per-CLI copies are no longer an obligation.
- Errors are JSON via `internal/output`, one object per line, and every `RunE` checks `clihelp.ShouldAbort` first.

## Interactive-handoff exception

Narrow and per-command: `reedengine` `attach` and `watchdog`, `lyx loom status --watch`, `lyx shed status --watch`, `lyx batten status --watch`, and the `status` verbs' terminal rendering.

## Package naming

`<module>cli` imports `<module>engine`; an engine never imports cli or cobra.
Deviations:
- `stencilcli` uses `internal/stencilstore`.
- `quarrycli` uses `internal/planglyph`.
- `battencli` uses `internal/battenshed` and `internal/battenrecipe`, with no engine package of its own.
- `shedcli` uses `internal/shedverbs`, `internal/loomcli` and `internal/battencli`, with no engine package of its own.
