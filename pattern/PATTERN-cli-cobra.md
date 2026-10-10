# PATTERN-cli-cobra

Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.

- Each module exposes `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int`.
  Every module but `internal/selfreportcli` also carries `RunCLIIn(cwd, out, args) int`.
- An alias command may delegate into another module's subtree with no seam function of its own.
- Every command has a non-empty `Short`.
- Every invocable command carries an `audience` annotation from the closed set in `internal/clihelp`, a hidden one is `internal`, and a non-invocable group carries none.
- `Use` is the command name, then `<arg>`, `[<arg>]`, `<arg>...`, `[<arg>...]`, `--flag <value>` for a flag cobra marks required, or a brace-balanced `{...}` payload span.
  `Short` is one lower-case line with no final period, and `Long` stays under the byte cap in `cmd/lyx/clitree_test.go`.
- One walk of `newRoot()` in `cmd/lyx/clitree_test.go` enforces the audience, `Use`, `Short` and `Long` form, bare-group listing and unknown-subcommand refusal for every command.
  `cmd/lyx/registration_test.go` is the oracle that every `Command()` package is mounted.
  Per-CLI copies are no longer an obligation.
- Errors are JSON via `internal/output`, one object per line, and every `RunE` checks `clihelp.ShouldAbort` first.
- The global `--json` flag, declared by `clihelp.InstallJSONHelp`, prints a command's help as JSON and never runs the command, so an agent can walk the tree with `lyx <path> --json`.
  A command whose own local `--json` flag means something else, such as `lyx shed status --json`, shadows the global one and runs.

## Interactive-handoff exception

Narrow and per-command: `reedengine` `attach` and `watchdog`, `lyx loom status --watch`, `lyx shed status --watch`, `lyx batten status --watch`, and the `status` verbs' terminal rendering.

## Package naming

`<module>cli` imports `<module>engine`; an engine never imports cli or cobra.
Deviations:
- `stencilcli` uses `internal/stencilstore`.
- `quarrycli` uses `internal/planglyph`.
- `battencli` uses `internal/battenshed` and `internal/battenrecipe`, with no engine package of its own.
- `shedcli` uses `internal/shedverbs`, `internal/loomcli` and `internal/battencli`, with no engine package of its own.
