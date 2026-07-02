**You are a READ-ONLY reviewer. You MUST NOT call Edit, Write, Bash, or any
tool that modifies files or runs commands. You MUST NOT make git commits.
Your sole output is the review file in the format below. If you find issues,
REPORT them — do NOT fix them.**

You are an independent code reviewer for **Build internal/mux: the window to the world (overlay + strands + render)**. You evaluate the complete implementation (every batch) against the approved plan and produce a structured review.

Reviewer model: **opushigh**. Round **2**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: Do NOT use Write, Edit, or run git/bash. Return review as text.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Prior non-blocking items

The following items were judged non-blocking in a prior round. Do NOT escalate any of them to BLOCKING unless NEW information justifies it -- a new diff, a real reproducible failure, or a concrete in-repo convention. If you escalate, you MUST state the new information explicitly.

Prefer the convention already used by analogous code in the provided source files over a stricter alternative.

- `status` mutates the live layout as a side effect: **Location:** `internal/muxengine/lifecycle.go:263-295`


## Constraints
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
  on the test's `excludedModules` allowlist with a one-line reason: `muxpoc` (PoC,
  slated for replacement by the mux module), `ide` (side-effect heavy: `spawn`
  opens a real VS Code window, `menu` is an interactive stdin picker),
  `selfreport` (`create` files a real GitHub issue).
- **Exists ⇒ covered or excluded.** Adding a new registered module requires either
  a scenario tagged with that module's `**Covers:**` or a new allowlist entry with
  a reason — the same "exists ⇒ registered" discipline as the CLI/Cobra Invariant's
  registration guard.
- **Enforced by** `cmd/lyx/sandbox_coverage_test.go`
  (`TestSandboxCoverage_AllModulesCoveredOrExcluded`) on every `go test`.

## Documentation Lifecycle

Which docs are kept vs deleted (mechanical per-module docs vs durable design docs):
see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Files included (N=97)

- C:\Code\loomyard\wts\internal-mux\_mill\plan\00-overview.md
- C:\Code\loomyard\wts\internal-mux\_mill\plan\01-hubgeometry-dotlyx.md
- C:\Code\loomyard\wts\internal-mux\_mill\plan\02-logger.md
- C:\Code\loomyard\wts\internal-mux\_mill\plan\03-render.md
- C:\Code\loomyard\wts\internal-mux\_mill\plan\04-muxengine-carrier.md
- C:\Code\loomyard\wts\internal-mux\_mill\plan\05-muxengine-operations.md
- C:\Code\loomyard\wts\internal-mux\_mill\plan\06-muxcli.md
- C:\Code\loomyard\wts\internal-mux\_mill\plan\07-cmd-lyx-integration.md
- C:\Code\loomyard\wts\internal-mux\_mill\plan\08-docs.md
- C:\Code\loomyard\wts\internal-mux\internal\hubgeometry\hubgeometry_test.go
- C:\Code\loomyard\wts\internal-mux\internal\hubgeometry\hubgeometry.go
- C:\Code\loomyard\wts\internal-mux\internal\hubgeometry\hubgeometry_unit_test.go
- C:\Code\loomyard\wts\internal-mux\internal\output\output.go
- C:\Code\loomyard\wts\internal-mux\internal\logger\logger.go
- C:\Code\loomyard\wts\internal-mux\internal\logger\logger_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\cmd.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\types.go
- C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\cmd_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\checksum.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\checksum_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\layout.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\policy.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\policy_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\focus.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\height.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\height_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\rules.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\rules_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\doc.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\parse.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\parse_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\cli.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\overlay.go
- C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\state.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\env.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\env_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\server.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\server_test.go
- C:\Code\loomyard\wts\internal-mux\internal\state\state.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\state.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\state_test.go
- C:\Code\loomyard\wts\internal-mux\internal\warpengine\config.go
- C:\Code\loomyard\wts\internal-mux\internal\warpengine\template.go
- C:\Code\loomyard\wts\internal-mux\internal\warpengine\template.yaml
- C:\Code\loomyard\wts\internal-mux\internal\configengine\config.go
- C:\Code\loomyard\wts\internal-mux\internal\lyxtest\lyxtest.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\config.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\config_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\template.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\template.yaml
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\name.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\name_test.go
- C:\Code\loomyard\wts\internal-mux\internal\lock\lock.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\lock.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\lock_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\reconcile.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\reconcile_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\apply.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\apply_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\up.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\spawn.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\spawn_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\strand.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\strand_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\down.go
- C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\status.go
- C:\Code\loomyard\wts\internal-mux\internal\proc\proc_windows.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\lifecycle.go
- C:\Code\loomyard\wts\internal-mux\internal\muxengine\lifecycle_test.go
- C:\Code\loomyard\wts\internal-mux\internal\weftcli\cli.go
- C:\Code\loomyard\wts\internal-mux\internal\clihelp\exec.go
- C:\Code\loomyard\wts\internal-mux\internal\muxcli\cli.go
- C:\Code\loomyard\wts\internal-mux\internal\muxcli\up.go
- C:\Code\loomyard\wts\internal-mux\internal\muxcli\add.go
- C:\Code\loomyard\wts\internal-mux\internal\muxcli\remove.go
- C:\Code\loomyard\wts\internal-mux\internal\muxcli\status.go
- C:\Code\loomyard\wts\internal-mux\internal\muxcli\resume.go
- C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\attach.go
- C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\spawnattach_windows.go
- C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\spawnattach_other.go
- C:\Code\loomyard\wts\internal-mux\internal\muxcli\attach.go
- C:\Code\loomyard\wts\internal-mux\internal\muxcli\cli_test.go
- C:\Code\loomyard\wts\internal-mux\internal\muxcli\smoke_test.go
- C:\Code\loomyard\wts\internal-mux\cmd\lyx\main.go
- C:\Code\loomyard\wts\internal-mux\cmd\lyx\helptree_test.go
- C:\Code\loomyard\wts\internal-mux\cmd\lyx\jsonhelp_test.go
- C:\Code\loomyard\wts\internal-mux\cmd\lyx\unknown_subcommand_test.go
- C:\Code\loomyard\wts\internal-mux\cmd\lyx\registration_test.go
- C:\Code\loomyard\wts\internal-mux\cmd\lyx\sandbox_coverage_test.go
- C:\Code\loomyard\wts\internal-mux\internal\configreg\configreg.go
- C:\Code\loomyard\wts\internal-mux\internal\configreg\configreg_test.go
- C:\Code\loomyard\wts\internal-mux\tools\sandbox\SANDBOX-SUITE.md
- C:\Code\loomyard\wts\internal-mux\_mill\discussion.md
- C:\Code\loomyard\wts\internal-mux\docs\modules\mux.md
- C:\Code\loomyard\wts\internal-mux\docs\overview.md
- C:\Code\loomyard\wts\internal-mux\docs\roadmap.md
- C:\Code\loomyard\wts\internal-mux\CONSTRAINTS.md

## Plan + source files to review
- Overview: `C:\Code\loomyard\wts\internal-mux\_mill\plan\00-overview.md`
- Batch file(s):
  - `C:\Code\loomyard\wts\internal-mux\_mill\plan\01-hubgeometry-dotlyx.md`
  - `C:\Code\loomyard\wts\internal-mux\_mill\plan\02-logger.md`
  - `C:\Code\loomyard\wts\internal-mux\_mill\plan\03-render.md`
  - `C:\Code\loomyard\wts\internal-mux\_mill\plan\04-muxengine-carrier.md`
  - `C:\Code\loomyard\wts\internal-mux\_mill\plan\05-muxengine-operations.md`
  - `C:\Code\loomyard\wts\internal-mux\_mill\plan\06-muxcli.md`
  - `C:\Code\loomyard\wts\internal-mux\_mill\plan\07-cmd-lyx-integration.md`
  - `C:\Code\loomyard\wts\internal-mux\_mill\plan\08-docs.md`

Read the overview and every batch file above. Then read every source file listed below for full context (includes cross-batch ancestor creates already on disk):
- `C:\Code\loomyard\wts\internal-mux\internal\hubgeometry\hubgeometry_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\hubgeometry\hubgeometry.go`
- `C:\Code\loomyard\wts\internal-mux\internal\hubgeometry\hubgeometry_unit_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\output\output.go`
- `C:\Code\loomyard\wts\internal-mux\internal\logger\logger.go`
- `C:\Code\loomyard\wts\internal-mux\internal\logger\logger_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\cmd.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\types.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\cmd_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\checksum.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\checksum_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\layout.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\policy.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\policy_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\focus.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\height.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\height_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\rules.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\render\rules_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\doc.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\parse.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\parse_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\cli.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\overlay.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\state.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\env.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\env_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\server.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\server_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\state\state.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\state.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\state_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\warpengine\config.go`
- `C:\Code\loomyard\wts\internal-mux\internal\warpengine\template.go`
- `C:\Code\loomyard\wts\internal-mux\internal\warpengine\template.yaml`
- `C:\Code\loomyard\wts\internal-mux\internal\configengine\config.go`
- `C:\Code\loomyard\wts\internal-mux\internal\lyxtest\lyxtest.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\config.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\config_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\template.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\template.yaml`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\name.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\name_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\lock\lock.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\lock.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\lock_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\reconcile.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\reconcile_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\apply.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\apply_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\up.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\spawn.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\spawn_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\strand.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\strand_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\down.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\status.go`
- `C:\Code\loomyard\wts\internal-mux\internal\proc\proc_windows.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\lifecycle.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxengine\lifecycle_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\weftcli\cli.go`
- `C:\Code\loomyard\wts\internal-mux\internal\clihelp\exec.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxcli\cli.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxcli\up.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxcli\add.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxcli\remove.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxcli\status.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxcli\resume.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\attach.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\spawnattach_windows.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxpoccli\spawnattach_other.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxcli\attach.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxcli\cli_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\muxcli\smoke_test.go`
- `C:\Code\loomyard\wts\internal-mux\cmd\lyx\main.go`
- `C:\Code\loomyard\wts\internal-mux\cmd\lyx\helptree_test.go`
- `C:\Code\loomyard\wts\internal-mux\cmd\lyx\jsonhelp_test.go`
- `C:\Code\loomyard\wts\internal-mux\cmd\lyx\unknown_subcommand_test.go`
- `C:\Code\loomyard\wts\internal-mux\cmd\lyx\registration_test.go`
- `C:\Code\loomyard\wts\internal-mux\cmd\lyx\sandbox_coverage_test.go`
- `C:\Code\loomyard\wts\internal-mux\internal\configreg\configreg.go`
- `C:\Code\loomyard\wts\internal-mux\internal\configreg\configreg_test.go`
- `C:\Code\loomyard\wts\internal-mux\tools\sandbox\SANDBOX-SUITE.md`
- `C:\Code\loomyard\wts\internal-mux\_mill\discussion.md`
- `C:\Code\loomyard\wts\internal-mux\docs\modules\mux.md`
- `C:\Code\loomyard\wts\internal-mux\docs\overview.md`
- `C:\Code\loomyard\wts\internal-mux\docs\roadmap.md`
- `C:\Code\loomyard\wts\internal-mux\CONSTRAINTS.md`

## Source-grounding rule

**Never guess.** A `## Files included` manifest at the top of the artefact section above lists every file delivered to you in this prompt. Before emitting `verdict: NEED_CONTEXT`, scan the manifest and confirm the file you claim is missing is genuinely absent from the list. If a file IS in the manifest but you cannot find its content via the `--- FILE: <path> ---` delimiter, that is a long-context recall failure on your side — re-scan; do not emit NEED_CONTEXT for files in the manifest. Only emit `verdict: NEED_CONTEXT` for paths that are NOT in the manifest, and explain under `## Missing context` why each path is needed (one line per path). The orchestrator will re-fire the review with those files added. Fabricating file contents — or inferring them from filename / position alone — is a worse failure than halting honestly.

## Criteria (apply to the implementation as a whole)

- **End-to-end plan alignment** — every batch's cards are realised; every file listed across all batches' `Context:`/`Edits:`/`Creates:` is present in the source files provided.
- **Shared-decisions alignment** — the `## Shared Decisions` subsections are applied consistently across all batches; deviation is BLOCKING.
- **Out-of-plan files** — BLOCKING if any source file is present that is not accounted for in any batch's reference lists. If the implementer added it, the batch file must have been updated first; a review with surprise files means that discipline was skipped somewhere.
- **Cross-batch contracts** — interfaces produced by one batch and consumed by another are compatible. Dependency order implied by `depends-on:` is reflected in the code (consumers don't assume behaviour the producer doesn't guarantee).
- **Integration correctness** — the pieces work together, not just per-batch. Call sites match signatures; shared state is consistently managed; error surfaces compose.
- **Global utility duplication** — BLOCKING if two batches independently reimplement the same helper. Consolidate into a shared module.
- **Test coverage across the whole surface** — happy paths + errors for every batch's entry point. Integration tests reach across batch boundaries where appropriate.
- **Constraint violations** — BLOCKING.
- **Codebase consistency** — naming, error handling, imports, and style match the conventions visible in the source files provided.
- **Language pitfalls** — BLOCKING if high-risk (Python: mutable defaults, import side-effects, Windows path sep, CRLF/LF).

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line. Everything outside these markers is ignored by the backend. **No preamble inside the markers.** Per finding: 3–5 lines, short and factual. Cite file and line, state the issue, propose the fix.

Target length: ~400 tokens for APPROVE, ~800–1500 tokens for REQUEST_CHANGES across multiple batches. If you produce more than ~1800 tokens, compress.

~~~markdown
MILL_REVIEW_BEGIN
# Review: Build internal/mux: the window to the world (overlay + strands + render) — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: opushigh
reviewed_file: plan/ + source
date: <UTC YYYY-MM-DD>
```

## Findings

### [BLOCKING] <short title, <60 chars>
**Location:** `path/to/file.py:42` (or `:42-58`)
**Issue:** <one sentence>
**Fix:** <one sentence>

### [NIT] <short title>
**Location:** `path/to/file.py:N`
**Issue:** <one sentence>
**Fix:** <one sentence>

## Missing context
(include ONLY when verdict is NEED_CONTEXT — omit the section otherwise)

- `path/to/file.py` — <one-line reason the reviewer needs this file>

## Verdict

<APPROVE | REQUEST_CHANGES | NEED_CONTEXT>
<one sentence — max 20 words>
MILL_REVIEW_END
~~~

Severity / verdict rules match review-code-batch.md.

Omit `## Findings` if zero findings. Never invent findings to pad.
