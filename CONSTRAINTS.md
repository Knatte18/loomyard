# Constraints

Short, authoritative list of the repo's structural invariants. Each is partly
machine-enforced (named test, fails `go test` / CI) and partly a review obligation.
Fuller design/how-to lives in godoc and `docs/`, not here — this file is the index.

## Hub Geometry Invariant

`internal/hubgeometry` owns all cwd, worktree-root, and geometry resolution.

- All cwd / worktree-root queries go through `hubgeometry.Getwd()` / `Resolve()`. Raw
  `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/hubgeometry`
  and `cmd/lyx/main.go`.
- Geometry tokens — `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_codeguide`,
  `_lyx` — are owned solely by `internal/hubgeometry`. No other package may use them in a
  path-construction context (a `filepath.Join` arg, a `+` operand, or a string `const`).
  Whole-token match; production files only; comparisons and git-pathspec slice literals
  are not path construction and stay allowed.
- `_lyx`, its `config/` subdir, and any `<module>.yaml` resolve through
  `hubgeometry.LyxDirName` / `ConfigDir(base)` / `ConfigFile(base, module)` — **in test
  code too** (a config-layout migration once broke a hardcoded test fixture).
- Geometry is structural, never config/env-overridable (the board dir is `--board-path`
  flag > `hubgeometry.BoardDir(l.Hub)`, not a config key).
- **Enforced by** `internal/hubgeometry/enforcement_test.go` (`TestEnforcement_GeometryLiterals`)
  on every `go test`. API and helpers: godoc for `internal/hubgeometry`.

## lyxtest Leaf Invariant

`internal/lyxtest` stays a leaf: it imports only the standard library and
`internal/hubgeometry` — never `internal/configreg` or any feature package
(`boardengine`/`boardcli`, `warpengine`/`warpcli`, `weftengine`/`weftcli`, …).

- A `lyxtest → configreg → feature` edge closes a test-build cycle under
  `-tags integration`. Tests needing real config call `lyxtest.SeedConfig(tb, dir,
  map[string]string{...})`; the `configreg`→map conversion happens at the test site, in a
  package that may legally import `configreg`.
- **Enforced by** `internal/lyxtest/leaf_enforcement_test.go` on every `go test`.

## CLI / Cobra Invariant

Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.

- **Seam.** Each module exposes `Command() *cobra.Command` and a thin
  `RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out, args)`.
  Tests and root both drive the module through this seam.
- **Registration.** A new module is wired into `newRoot()`: import, `root.AddCommand(...)`,
  and append the module name to the root `Long` module-list. Unregistered ⇒ invisible to
  `--help`.
- **`Short` on every command** (parent + sub), non-empty. Self-discoverable commands also
  carry a `Long` with concrete examples.
- **Help accuracy is a review obligation.** Presence of `Short` is machine-checked;
  prose-vs-behaviour is not. When a change alters observable behaviour, the reviewer must
  re-read every affected `Short`/`Long` and confirm it matches the code as changed — stale
  help is a review-blocking defect. Prefer generating mechanical help facts from source
  (e.g. configcli's `Known modules:` from `configreg.Names()`).
- **Errors are JSON.** Results and errors go through the `internal/output` envelope
  (`output.Ok` / `output.Err`), one JSON object per line, via the `clihelp.Execute` /
  root seam (`SilenceErrors = true`). No bare plain-text error paths. Parent groups set
  `RunE = clihelp.GroupRunE` to reject unknown subcommands.
- **Interactive-handoff exception (narrow, per-command).** A subcommand whose whole job is
  to hand the operator's stdio to another interactive program and block (`ide menu`'s stdin
  picker; `mux attach`'s `psmux attach`) cannot emit the JSON envelope on that terminal-handover
  tail. The exception is scoped tightly: everything that can fail runs **pre-flight and stays
  on the envelope** (`output.Err`, non-zero exit); only the post-handoff tail is exempt, and on
  success it emits no JSON. `mux attach` follows the pre-existing `ide menu` precedent; see
  [docs/modules/mux.md](docs/modules/mux.md#attach-is-a-documented-envelope-exception) for the
  full rationale.
- **Package naming.** A Cobra-registered package is `<module>cli`; its extracted domain
  kernel is `<module>engine`. cli imports engine; engine never imports cli or cobra.
  Litmus: returns `(T, error)` with no cobra/`io.Writer`/exit codes ⇒ engine. Skip the
  engine only for trivial wrappers (`configcli`) or throwaway (`muxpoccli`);
  "no consumer today" is not a skip reason. `initcli`/`initengine` follows the standard
  split (no longer exempt — `lyx init --undo` grew enough core logic that mixing it into
  the cli package was rot, not simplicity).
- **Enforced by** `cmd/lyx/drift_test.go` (every command has `Short`),
  `helptree_test.go` (root names every module, module names every subcommand),
  `registration_test.go` (exists ⇒ registered), `longlist_test.go` (registered ⇒ in
  `root.Long`). Update the pinned sets in the same commit when adding a module/subcommand.

## Sandbox Suite Coverage

Every registered lyx module must be exercised by the black-box sandbox suite or be
explicitly excluded with a reason.

- **Tagging.** A `tools/sandbox/SANDBOX-SUITE.md` scenario that drives a specific
  module declares it with a `**Covers:** <module>[, <module>...]` line, in the same
  bold-label style as the scenario's `**Goal:**`/`**Watch:**`/`**Verdict:**` lines.
  Coverage is checked at module granularity against the live cobra root
  (`newRoot().Commands()`, skipping `help`/`completion`) — the same enumeration
  `longlist_test.go` already uses, never a separately hand-maintained list.
- **Allowlist.** Modules that are intentionally never sandbox-exercised are named
  on the test's `excludedModules` allowlist with a one-line reason: `ide`
  (side-effect heavy: `spawn` opens a real VS Code window, `menu` is an
  interactive stdin picker), `selfreport` (`create` files a real GitHub issue).
- **Exists ⇒ covered or excluded.** Adding a new registered module requires either
  a scenario tagged with that module's `**Covers:**` or a new allowlist entry with
  a reason — the same "exists ⇒ registered" discipline as the CLI/Cobra Invariant's
  registration guard.
- **Enforced by** `cmd/lyx/sandbox_coverage_test.go`
  (`TestSandboxCoverage_AllModulesCoveredOrExcluded`) on every `go test`.

## Documentation Lifecycle

Which docs are kept vs deleted (mechanical per-module docs vs durable design docs):
see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).
