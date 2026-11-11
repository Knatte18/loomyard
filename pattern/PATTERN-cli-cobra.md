# PATTERN-cli-cobra

Adding or wiring a CLI module: a cobra subtree under one root in `cmd/lym/main.go`; standalone wiring only through `internal/cliwire`.

## Cobra subtree

Rule: Adding CLI module: a cobra subtree under one root in `cmd/lym/main.go`. (test)

Every lym CLI module is a cobra subtree assembled under one root in `cmd/lym/main.go`.

- Each module exposes `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int`.
  Every module but `internal/selfreportcli` also carries `RunCLIIn(cwd, out, args) int`.
- An alias command may delegate into another module's subtree with no seam function of its own.
- Every command has a non-empty `Short`.
- Every invocable command carries an `audience` annotation from the closed set in `internal/clihelp`;
  a hidden one is `internal`, and a non-invocable group carries none.
  `clihelp.Invocable` decides which commands are invocable.
  A command with subcommands is invocable when it is runnable and its `Use` carries a token after its name;
  it is also invocable when it has a visible local flag.
  Any other group carries no audience, prints help on a bare call and refuses a mistyped subcommand.
  Behavior meant for the group name belongs on a leaf such as `show`.
- `Use` is the command name, then argument tokens: `<arg>`, `[<arg>]`, `<arg>...` or `[<arg>...]`.
  It may also carry `--flag <value>` for a flag cobra marks required, or a brace-balanced `{...}` payload span.
  `Short` is one lower-case line with no final period,
  and `Long` stays under the byte cap in `cmd/lym/clitree_test.go`.
- One walk of `newRoot()` in `cmd/lym/clitree_test.go` checks every command.
  It enforces the audience, `Use`, `Short` and `Long` form, bare-group listing and unknown-subcommand refusal.
  `cmd/lym/registration_test.go` is the oracle that every `Command()` package is mounted.
  A module needs no copy of these checks in its own tests.
- Errors are JSON via `internal/output`, one object per line, and every `RunE` checks `clihelp.ShouldAbort` first.
- The global `--json` flag, declared by `clihelp.InstallJSONHelp`, prints a command's help as JSON.
  It never runs the command, so an agent can walk the tree with `lym <path> --json`.
  A command whose own local `--json` flag means something else shadows the global one and runs.
  `lym shed status --json` is one.
- The global `--envelope` flag, declared by `clihelp.InstallReadableOutput`, keeps the full JSON envelope.
  Without it, an operator verb on a terminal prints its readable summary.
  A command's own local `--envelope` flag, such as `lym creel read`, shadows it.

### Interactive-handoff exception

Narrow and per-command, covering:

- `reedengine` `attach` and `watchdog`.
- `lym loom status --watch`, `lym shed status --watch` and `lym batten status --watch`.
- The `status` verbs' terminal rendering.

### Package naming

`<module>cli` imports `<module>engine`; an engine never imports cli or cobra.
Deviations:
- `stencilcli` uses `internal/stencilstore`.
- `quarrycli` uses `internal/planglyph`.
- `battencli` uses `internal/battenshed` and `internal/battenrecipe`, with no engine package of its own.
- `shedcli` uses `internal/shedverbs`, `internal/loomcli` and `internal/battencli`, with no engine package of its own.
- `tendercli` uses `tenderengine`, and mounts one root command, `tender`.
- `shortcutcli` mounts one root command per shortcut from its embedded file, with no `Command()` or `RunCLI`;
  a shortcut is an alias stretched to a sequence of argvs, so the module guards skip its group.
- `cleanupcli` has no engine package of its own.
  It calls the cleanup functions of `battencli`, `boardengine`, `orchengine`, `logger` and `lymtemp`.
- `adoptcli` calls the functions of `fabricengine`, `pairteardown`, `boardengine`, `tenderengine`, `tendercli`, `battencli`, `battenshed`, `buildvcs` and `landingshed`, with no engine package of its own.
- `beamcli` calls the functions of `fabricengine`, `boardengine`, `hubgeom` and `battencli`, with no engine package of its own.
- `machinecli` calls the functions of `machinename`, `boardengine`, `fabricengine` and `hubregistry`, with no engine package of its own.

## Sole wiring

Rule: Wiring standalone CLI: `internal/cliwire` alone resolves target dir, repo root and state. (test)

`internal/cliwire` is the sole owner of standalone and hub CLI wiring resolution for the standalone-capable CLIs.

- A `<module>cli` never re-implements `--target-dir` resolution, the repository-root lift or mode-derived state.
  Nor does it re-implement plan and stencils resolution, the nested-geometry guard or the durable-sink redirect.
  It declares its own `cliwire.Module` descriptor and calls in.
- `internal/cliwire` is the only production caller of `standalonestate.Derive`;
  test files may call it to build fixtures and to assert the real derivation.
- Both halves are enforced by tests in `internal/cliwire`.
