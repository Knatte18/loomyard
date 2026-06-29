**You are a READ-ONLY reviewer. You MUST NOT call Edit, Write, Bash, or any
tool that modifies files or runs commands. You MUST NOT make git commits.
Your sole output is the review file in the format below. If you find issues,
REPORT them — do NOT fix them.**

You are an independent code reviewer for **Rename Cobra modules to <module>cli, extract kernels as <module>engine**. You evaluate the complete implementation (every batch) against the approved plan and produce a structured review.

Reviewer model: **sonnethigh**. Round **1**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: Do NOT use Write, Edit, or run git/bash. Return review as text.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Prior non-blocking items

The following items were judged non-blocking in a prior round. Do NOT escalate any of them to BLOCKING unless NEW information justifies it -- a new diff, a real reproducible failure, or a concrete in-repo convention. If you escalate, you MUST state the new information explicitly.

Prefer the convention already used by analogous code in the provided source files over a stricter alternative.

(none)

## Constraints
# Constraints

## Path Invariant

All worktree and hub geometry must be resolved through `internal/paths`, not raw primitives. This invariant is enforced at build time.

### Rule

- All cwd and worktree root queries MUST go through `internal/paths.Getwd()` and `internal/paths.Resolve()`.
- Raw `os.Getwd` is forbidden outside `internal/paths` and `cmd/lyx/main.go`.
- Raw `git rev-parse --show-toplevel` is forbidden outside `internal/paths` and `cmd/lyx/main.go`.
- The ban is enforced at `go test` / CI time by `internal/paths/enforcement_test.go`, which scans the entire source tree and fails the build if either primitive is found.

### `_lyx` and config-file paths

- The `_lyx` directory name, its `config/` subdirectory, and any `<module>.yaml` config file MUST be resolved through `internal/paths` helpers — never built from string literals like `filepath.Join(base, "_lyx", "config")` or `"board.yaml"`.
  - `paths.LyxDirName` — the `_lyx` directory name constant (use `filepath.Join(base, paths.LyxDirName)` for a bare `_lyx` dir).
  - `paths.ConfigDir(base)` — the `<base>/_lyx/config` directory.
  - `paths.ConfigFile(base, module)` — the `<base>/_lyx/config/<module>.yaml` file (e.g. `module` = `"board"`, `"worktree"`, `"weft"`). For a relative path, pass `"."` as `base`.
- **This rule applies to test code too.** A migration of the config layout (PR #20 moved configs from `_lyx/<module>.yaml` to `_lyx/config/<module>.yaml`) silently broke a hardcoded test fixture (`internal/worktree/cli_test.go`) because its literal write path drifted from the loader's read path. Routing every path through the helpers makes such migrations track automatically. The two genuine exceptions are `internal/paths/*_test.go` (those literals *are* the spec under test) and `_lyx` used as link-target geometry or string-content assertions — neither resolves a config path.
- This case is **not** caught by `internal/paths/enforcement_test.go` (which only bans `os.Getwd` / `git rev-parse`); it is a code-review and planning-discipline rule.

### For New Code

If you need a cwd or worktree root:
- Call `paths.Getwd()` to get the current working directory.
- Call `paths.Resolve(cwd)` to obtain a `Layout` with all geometry fields (root, hub, relative path, etc.).
- Use the `Layout` methods to derive paths: `LyxDir()`, `WorktreePath(slug)`, `PortalsDir()`, `PortalLink(slug)`, `PortalTarget(slug)`, `LaunchersDir()`, `LauncherDir(slug)`, `MenuLauncherPath()`, `LauncherSpawnRel(slug)`, `MenuLauncherRel()`, `PrimeName()`, `WeftRepoRoot()`, `WeftWorktreePath(slug)`, `WeftWorktree()`, `WeftLyxDir()`, `WeftLyxDirFor(slug)`, `WeftCodeguideDir()`, `HostLyxLink(slug)`, `HostLyxLinkHere()`, `HostJunctions(slug)`.

If you need an `_lyx` / config path (in production or test code), use `paths.LyxDirName`, `paths.ConfigDir(base)`, and `paths.ConfigFile(base, module)` as above.

## lyxtest Leaf Invariant

`internal/lyxtest` must remain a leaf package importing only the standard library and `internal/paths`. It must not import `internal/configreg` or any feature package (`boardengine`/`boardcli`, `warpengine`/`warpcli`, `weftengine`/`weftcli`, etc.).

### Rule

- `internal/lyxtest` must not import `internal/configreg` or any feature package.
- Tests that need real configuration must seed it themselves via `SeedConfig`, passing a configreg-free `map[string]string` (module name to YAML content).
- The `configreg.Modules()` to map conversion happens at the test site, in a package that may legally import `configreg`.
- Feature packages' internal tests import `lyxtest`; a `lyxtest → configreg → feature` import would close a test-build cycle (the trap that motivated this task).

### Rationale

The cycle closes silently when `lyxtest` imports `configreg` and `configreg` imports feature packages, but only under `-tags integration` (feature-internal tests are integration-tagged). An untagged import-scan test (`internal/lyxtest/leaf_enforcement_test.go`) catches a reintroduced edge on every `go test ./...` with a clear message, instead of waiting for an integration suite run.

### For New Tests

If a test needs real config:
- Obtain each module's template from the module's own `ConfigTemplate()` function (e.g., `weftengine.ConfigTemplate()`).
- Use the unqualified name (`ConfigTemplate()`) when calling from a file in that same package.
- Use the qualified form (e.g., `weftengine.ConfigTemplate()`) from a different package, adding the feature import as needed to the test file.
- Pass the templates to `lyxtest.SeedConfig(tb, repoDir, map[string]string{...})` **never** pass `configreg.Module` types or call `configreg` from inside lyxtest.

The enforcement test (`internal/lyxtest/leaf_enforcement_test.go`) is run on every `go test ./...` and fails the build if any of the banned imports appear in lyxtest source files.

## CLI / Cobra Invariant

Every lyx CLI module is a cobra command tree assembled under one root in
`cmd/lyx/main.go`. The seam, the registration, and the self-documentation are all
load-bearing and partly enforced at `go test` time.

### Rule

- **Module seam.** Every CLI module exposes `Command() *cobra.Command` (builds that
  module's command subtree) and a thin `RunCLI(out io.Writer, args []string) int` seam
  that is exactly `return clihelp.Execute(Command(), out, args)`. Tests and the root
  both drive the module through this seam — never re-implement argument parsing.
- **Registration.** A new module MUST be wired into `cmd/lyx/main.go` `newRoot()`:
  (1) import the package, (2) `root.AddCommand(<module>.Command())`, and (3) append the
  module name to the root command's `Long` module-list string. A module that is not
  registered is invisible to `lyx --help`.
- **Every command has a `Short`.** Both the parent module command and every subcommand
  MUST carry a non-empty `Short`. Enforced by `cmd/lyx/drift_test.go`
  (`TestDriftGuard_AllCommandsHaveShort`), which walks the whole tree and fails the
  build on any blank `Short`. Commands whose `--help` is the discovery path (anything an
  agent or operator must learn from the binary alone) SHOULD also carry a `Long` with
  concrete usage examples.
- **Help is co-located, never a central table.** Help text lives on each command
  (`Short`/`Long`), so it cannot drift from behaviour. Do not add a hand-maintained
  command listing anywhere else.
- **Help tree is pinned by test.** `cmd/lyx/helptree_test.go` asserts the root names
  every module and each module names every subcommand. When you add a module or a
  subcommand, update the pinned sets in that test (root `requiredModules`, and the
  module's `wantSubs`).
- **Registration and Long-list enforced by guards.** `cmd/lyx/registration_test.go`
  (source/AST scan: every `internal/*` package with `func Command() *cobra.Command`
  must be registered in `newRoot()` — "exists ⇒ registered") and
  `cmd/lyx/longlist_test.go` (live tree: every registered child must appear in
  `root.Long` — "registered ⇒ in --help prose") enforce these automatically on every
  `go test ./cmd/lyx/...` run.
- **Handlers and output.** Bridge a `func(out io.Writer, args []string) int` handler
  into cobra via `clihelp.WrapRun`; use `clihelp` exit handling (`ShouldAbort` /
  `SetExit` / `Abort`) rather than ad-hoc `os.Exit`. Emit results through the
  `internal/output` JSON envelope (`output.Ok` / `output.Err`) — one JSON object per
  line. A persistent `--json` flag on the root exposes machine-readable help
  (`internal/clihelp/jsonhelp.go`).
- **Errors are JSON.** Cobra-level errors (unknown command/flag, arg validation) are
  wrapped in the `internal/output` JSON envelope (`{"ok":false,"error":"..."}`) on
  stdout at the `clihelp.Execute` / `RunRoot` seam and at the `cmd/lyx` root, both of
  which set `SilenceErrors = true`. `output.Err` trims the message with
  `strings.TrimSpace`. Do not reintroduce bare plain-text error paths — config's were
  harmonized in the CLI ergonomics pass (2026-06-28).
- **Parent groups reject unknown subcommands.** Every parent module group (`board`,
  `warp`, `weft`, `ide`, `muxpoc`) sets `RunE = clihelp.GroupRunE`, which errors
  `unknown subcommand %q for %q` on extra args and otherwise shows help. Groups with a
  layout-resolving `PersistentPreRunE` (`weft`, `board`, `ide`, `muxpoc`) guard it with
  an early return at the top of that hook when `cmd.Name()` equals the group name,
  preserving the "list subcommands without a git repo" property for bare-group invocations.

### For New Code

When adding a CLI module or subcommand:
- Follow the **warp variant** (`internal/warpcli/warp.go`) for a module with positional
  args and per-subcommand local flags: no `PersistentPreRunE`, `clihelp.WrapRun`
  handlers, flags read via a closure over the `*cobra.Command`. Follow the **boardcli/weftcli
  variant** when you need a `PersistentPreRunE` to resolve shared state once.
- Set `Short` on the new command immediately (the drift guard will fail otherwise), and
  a `Long` with examples when the command is meant to be self-discoverable.
- Update `cmd/lyx/helptree_test.go` pinned sets in the **same commit** (this is also the
  Task-completion docs discipline from `CLAUDE.md`).

### Package naming

Every package registered in `newRoot()` (i.e. anything that lands in Cobra) is named
`<module>cli`; the domain kernel a non-CLI consumer needs is extracted as `<module>engine`.
This is the **inverted** convention from the earlier bare-name rule. Precedent:
`internal/yamlengine`, `internal/configengine`, and now `internal/boardcli` /
`internal/boardengine`, `internal/warpcli` / `internal/warpengine`, etc.

**Litmus test.** Ask of every function or file: does it return `(T, error)` with no Cobra,
no `io.Writer`-for-output, and no exit codes? → it belongs in the engine. Does it exist
only because of the command line (flags, subcommand wiring, `Short`/`Long`, exit-code
handling)? → it belongs in the cli package.

**cli/engine boundary:**
- **cli** owns `Command() *cobra.Command`, the `RunCLI` seam, Cobra subcommands, flags,
  `Short`/`Long`, `PersistentPreRunE`, and exit-code handling.
- **engine** owns the domain kernel: types and operations returning `(T, error)` with no
  Cobra, no `io.Writer`-for-output, and no exit codes.

**Dependency direction:** cli imports engine. engine → engine is allowed (e.g. `ideengine`
imports `boardengine`). Engine must never import a `cli` package or cobra; doing so would
close an import cycle and lock the kernel out of loom consumption.

**Skip clause.** Create an engine unless:
- The logic is trivial or incidental and no real kernel exists — `initcli` and `configcli`
  are thin command wrappers with no domain kernel worth extracting.
- The module is throwaway — `muxpoccli` is a proof-of-concept slated for replacement by the
  full `mux` module.

"No external consumer today" is **not** a skip reason. Loom is the designed future consumer
of every engine; the absence of a caller today does not justify merging cli and engine into
one package.

## Documentation Lifecycle

For the convention governing which docs are kept and which are deleted (mechanical per-module docs vs. durable design docs), see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Files included (N=154)

- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\00-overview.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\01-board-split.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\02-weft-split.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\03-warp-split.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\04-ide-split.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\05-ghissues-split.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\06-muxpoc-rename.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\07-update-fold.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\08-constraints-docs-guards.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configengine\config.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\board.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\store.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\task.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\layer.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\render.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\git.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\sync.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\config.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\template.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\spawn.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\template.yaml
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\board_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\store_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\task_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\layer_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\render_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\config_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\template_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\boardtest\bench_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\boardtest\concurrency_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\boardtest\git_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\boardtest\sync_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\boardtest\doc.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\clihelp\exec.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardcli\cli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardcli\cli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardcli\help_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardcli\skipenv_internal_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\main.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configreg\configreg.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\initcli\initcli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\weft.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\config.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\sync.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\status.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\template.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\template.yaml
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\config_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\sync_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\status_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\template_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\weft_integration_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftcli\cli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftcli\spawn.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftcli\cli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configreg\configreg_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configcli\configcli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configcli\configcli_integration_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\fslink\fslink.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\add.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\checkout.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\remove.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\prune.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\cleanup.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\status.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\list.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\reconcile.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\worktreelifecycle.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\drift.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\junction.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\hook.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\weftwiring.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\launchers.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\portals.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\ancestors.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\config.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\template.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\clone.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\template.yaml
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\post-checkout.sh
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\add_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\checkout_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\cleanup_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\config_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\drift_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\hook_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\launchers_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\list_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\portals_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\prune_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\reconcile_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\remove_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\status_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\template_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\weftwiring_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\ancestors_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\clone_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\clone_integration_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpcli\warp.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpcli\clone.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpcli\warp_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpcli\clone_cli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\initcli\initcli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\vscode\launch_windows.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\vscode\config.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ideengine\spawn.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ideengine\menu.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ideengine\spawn_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ideengine\menu_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\idecli\cli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\idecli\cli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissuesengine\ghissues.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissuescli\cli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissuescli\cli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\cli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\up.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\down.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\status.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\review.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\attach.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\daemon.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\cmd.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\state.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\spawnattach_other.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\spawnattach_windows.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\cli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\cmd_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\state_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\muxpoc_smoke_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configsync\configsync.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\paths\paths.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\gitexec\gitexec.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configcli\reconcile_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\output\output.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\helptree_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\jsonhelp_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\main_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\unknown_subcommand_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configengine\config_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\shared-libs\configengine.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\CONSTRAINTS.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\lyxtest\lyxtest.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\lyxtest\doc.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\lyxtest\leaf_enforcement_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\overview.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\benchmarks\test-suite-timing.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\modules\README.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\modules\mux.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\sandbox-hub.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\roadmap.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\benchmarks\board-performance.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\benchmarks\running-tests.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\tools\sandbox\main.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\testtiming\main.go

## Plan + source files to review
- Overview: `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\00-overview.md`
- Batch file(s):
  - `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\01-board-split.md`
  - `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\02-weft-split.md`
  - `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\03-warp-split.md`
  - `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\04-ide-split.md`
  - `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\05-ghissues-split.md`
  - `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\06-muxpoc-rename.md`
  - `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\07-update-fold.md`
  - `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\08-constraints-docs-guards.md`

Read the overview and every batch file above. Then read every source file listed below for full context (includes cross-batch ancestor creates already on disk):
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configengine\config.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\board.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\store.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\task.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\layer.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\render.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\git.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\sync.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\config.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\template.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\spawn.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\template.yaml`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\board_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\store_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\task_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\layer_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\render_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\config_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\template_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\boardtest\bench_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\boardtest\concurrency_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\boardtest\git_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\boardtest\sync_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardengine\boardtest\doc.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\clihelp\exec.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardcli\cli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardcli\cli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardcli\help_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\boardcli\skipenv_internal_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\main.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configreg\configreg.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\initcli\initcli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\weft.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\config.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\sync.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\status.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\template.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\template.yaml`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\config_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\sync_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\status_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\template_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftengine\weft_integration_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftcli\cli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftcli\spawn.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weftcli\cli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configreg\configreg_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configcli\configcli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configcli\configcli_integration_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\fslink\fslink.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\add.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\checkout.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\remove.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\prune.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\cleanup.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\status.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\list.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\reconcile.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\worktreelifecycle.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\drift.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\junction.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\hook.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\weftwiring.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\launchers.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\portals.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\ancestors.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\config.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\template.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\clone.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\template.yaml`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\post-checkout.sh`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\add_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\checkout_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\cleanup_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\config_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\drift_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\hook_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\launchers_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\list_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\portals_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\prune_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\reconcile_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\remove_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\status_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\template_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\weftwiring_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\ancestors_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\clone_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpengine\clone_integration_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpcli\warp.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpcli\clone.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpcli\warp_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warpcli\clone_cli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\initcli\initcli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\vscode\launch_windows.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\vscode\config.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ideengine\spawn.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ideengine\menu.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ideengine\spawn_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ideengine\menu_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\idecli\cli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\idecli\cli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissuesengine\ghissues.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissuescli\cli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissuescli\cli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\cli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\up.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\down.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\status.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\review.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\attach.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\daemon.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\cmd.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\state.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\spawnattach_other.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\spawnattach_windows.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\cli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\cmd_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\state_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoccli\muxpoc_smoke_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configsync\configsync.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\paths\paths.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\gitexec\gitexec.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configcli\reconcile_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\output\output.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\helptree_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\jsonhelp_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\main_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\unknown_subcommand_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configengine\config_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\shared-libs\configengine.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\CONSTRAINTS.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\lyxtest\lyxtest.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\lyxtest\doc.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\lyxtest\leaf_enforcement_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\overview.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\benchmarks\test-suite-timing.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\modules\README.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\modules\mux.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\sandbox-hub.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\roadmap.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\benchmarks\board-performance.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\docs\benchmarks\running-tests.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\tools\sandbox\main.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\testtiming\main.go`

## Intentionally deleted (N=108)

- internal/board/board.go
- internal/board/board_test.go
- internal/board/boardtest/bench_test.go
- internal/board/boardtest/concurrency_test.go
- internal/board/boardtest/doc.go
- internal/board/boardtest/git_test.go
- internal/board/boardtest/sync_test.go
- internal/board/cli.go
- internal/board/cli_test.go
- internal/board/config.go
- internal/board/config_test.go
- internal/board/git.go
- internal/board/help_test.go
- internal/board/layer.go
- internal/board/layer_test.go
- internal/board/render.go
- internal/board/render_test.go
- internal/board/skipenv_internal_test.go
- internal/board/spawn.go
- internal/board/store.go
- internal/board/store_test.go
- internal/board/sync.go
- internal/board/task.go
- internal/board/task_test.go
- internal/board/template.go
- internal/board/template.yaml
- internal/board/template_test.go
- internal/ghissues/cli.go
- internal/ghissues/cli_test.go
- internal/ghissues/ghissues.go
- internal/ide/cli.go
- internal/ide/cli_test.go
- internal/ide/menu.go
- internal/ide/menu_test.go
- internal/ide/spawn.go
- internal/ide/spawn_test.go
- internal/muxpoc/attach.go
- internal/muxpoc/cli.go
- internal/muxpoc/cli_test.go
- internal/muxpoc/cmd.go
- internal/muxpoc/cmd_test.go
- internal/muxpoc/daemon.go
- internal/muxpoc/down.go
- internal/muxpoc/muxpoc_smoke_test.go
- internal/muxpoc/review.go
- internal/muxpoc/spawnattach_other.go
- internal/muxpoc/spawnattach_windows.go
- internal/muxpoc/state.go
- internal/muxpoc/state_test.go
- internal/muxpoc/status.go
- internal/muxpoc/up.go
- internal/update/update.go
- internal/update/update_test.go
- internal/warp/add.go
- internal/warp/add_test.go
- internal/warp/ancestors.go
- internal/warp/ancestors_test.go
- internal/warp/checkout.go
- internal/warp/checkout_test.go
- internal/warp/cleanup.go
- internal/warp/cleanup_test.go
- internal/warp/clone.go
- internal/warp/clone_integration_test.go
- internal/warp/clone_test.go
- internal/warp/config.go
- internal/warp/config_test.go
- internal/warp/drift.go
- internal/warp/drift_test.go
- internal/warp/hook.go
- internal/warp/hook_test.go
- internal/warp/junction.go
- internal/warp/launchers.go
- internal/warp/launchers_test.go
- internal/warp/list.go
- internal/warp/list_test.go
- internal/warp/portals.go
- internal/warp/portals_test.go
- internal/warp/post-checkout.sh
- internal/warp/prune.go
- internal/warp/prune_test.go
- internal/warp/reconcile.go
- internal/warp/reconcile_test.go
- internal/warp/remove.go
- internal/warp/remove_test.go
- internal/warp/status.go
- internal/warp/status_test.go
- internal/warp/template.go
- internal/warp/template.yaml
- internal/warp/template_test.go
- internal/warp/warp.go
- internal/warp/warp_test.go
- internal/warp/weftwiring.go
- internal/warp/weftwiring_test.go
- internal/warp/worktreelifecycle.go
- internal/weft/cli.go
- internal/weft/cli_test.go
- internal/weft/config.go
- internal/weft/config_test.go
- internal/weft/spawn.go
- internal/weft/status.go
- internal/weft/status_test.go
- internal/weft/sync.go
- internal/weft/sync_test.go
- internal/weft/template.go
- internal/weft/template.yaml
- internal/weft/template_test.go
- internal/weft/weft.go
- internal/weft/weft_integration_test.go

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
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: sonnethigh
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
