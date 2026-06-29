**You are a READ-ONLY reviewer. You MUST NOT call Edit, Write, Bash, or any
tool that modifies files or runs commands. You MUST NOT make git commits.
Your sole output is the review file in the format below. If you find issues,
REPORT them — do NOT fix them.**

You are an independent plan reviewer for **Rename Cobra modules to <module>cli, extract kernels as <module>engine**. You evaluate the complete plan (all batches) and produce a structured review.

Reviewer model: **opushigh**. Round **1**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: Do NOT use Write, Edit, or run git/bash. Return review as text.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

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

`internal/lyxtest` must remain a leaf package importing only the standard library and `internal/paths`. It must not import `internal/configreg` or any feature package (`board`, `worktree`, `weft`).

### Rule

- `internal/lyxtest` must not import `internal/configreg` or any feature package.
- Tests that need real configuration must seed it themselves via `SeedConfig`, passing a configreg-free `map[string]string` (module name to YAML content).
- The `configreg.Modules()` to map conversion happens at the test site, in a package that may legally import `configreg`.
- Feature packages' internal tests import `lyxtest`; a `lyxtest → configreg → feature` import would close a test-build cycle (the trap that motivated this task).

### Rationale

The cycle closes silently when `lyxtest` imports `configreg` and `configreg` imports feature packages, but only under `-tags integration` (feature-internal tests are integration-tagged). An untagged import-scan test (`internal/lyxtest/leaf_enforcement_test.go`) catches a reintroduced edge on every `go test ./...` with a clear message, instead of waiting for an integration suite run.

### For New Tests

If a test needs real config:
- Obtain each module's template from the module's own `ConfigTemplate()` function (e.g., `weft.ConfigTemplate()`).
- Use the unqualified name (`ConfigTemplate()`) when calling from a file in that same package.
- Use the qualified form (e.g., `weft.ConfigTemplate()`) from a different package, adding the feature import as needed to the test file.
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
- Follow the **warp variant** (`internal/warp/warp.go`) for a module with positional
  args and per-subcommand local flags: no `PersistentPreRunE`, `clihelp.WrapRun`
  handlers, flags read via a closure over the `*cobra.Command`. Follow the **board/weft
  variant** when you need a `PersistentPreRunE` to resolve shared state once.
- Set `Short` on the new command immediately (the drift guard will fail otherwise), and
  a `Long` with examples when the command is meant to be self-discoverable.
- Update `cmd/lyx/helptree_test.go` pinned sets in the **same commit** (this is also the
  Task-completion docs discipline from `CLAUDE.md`).

### Package naming

A command-owning package takes the command's bare name: `internal/warp` owns `lyx warp`,
`internal/weft` owns `lyx weft`, `internal/board` owns `lyx board`. A `cli` suffix is used
**only** when the bare name is unavailable — either taken by a sibling package or reserved
by Go itself:
- `config` is the config **engine** (`internal/configengine`), so the `lyx config` command
  lives in `internal/configcli`.
- `init` is the Go reserved identifier `func init()`, so the `lyx init` command lives in
  `internal/initcli`.

Therefore `configcli` and `initcli` are principled, deliberate exceptions to the
bare-name rule, not inconsistency. Reach for a `cli` suffix only when the bare name is
genuinely blocked; otherwise use the bare command name.

## Documentation Lifecycle

For the convention governing which docs are kept and which are deleted (mechanical per-module docs vs. durable design docs), see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Files included (N=152)

- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\00-overview.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\01-board-split.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\02-weft-split.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\03-warp-split.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\04-ide-split.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\05-ghissues-split.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\06-muxpoc-rename.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\07-update-fold.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\08-constraints-docs-guards.md
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\board.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\store.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\task.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\layer.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\render.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\git.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\sync.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\config.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\template.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\spawn.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\template.yaml
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\board_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\store_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\task_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\layer_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\render_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\config_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\template_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configengine\config.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\boardtest\bench_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\boardtest\concurrency_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\boardtest\git_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\boardtest\sync_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\boardtest\doc.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\cli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\cli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\help_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\skipenv_internal_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\clihelp\exec.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\main.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configreg\configreg.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\menu.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\initcli\initcli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\weft.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\config.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\sync.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\status.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\template.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\template.yaml
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\config_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\sync_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\status_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\template_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\weft_integration_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\cli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\spawn.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\cli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configreg\configreg_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configcli\configcli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configcli\configcli_integration_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\add.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\checkout.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\remove.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\prune.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\cleanup.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\status.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\list.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\reconcile.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\worktreelifecycle.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\drift.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\junction.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\hook.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\weftwiring.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\launchers.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\portals.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\ancestors.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\config.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\template.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\clone.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\warp.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\template.yaml
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\post-checkout.sh
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\fslink\fslink.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\add_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\checkout_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\cleanup_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\config_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\drift_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\hook_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\launchers_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\list_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\portals_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\prune_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\reconcile_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\remove_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\status_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\template_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\weftwiring_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\ancestors_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\clone_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\clone_integration_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\warp_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\initcli\initcli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\spawn.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\spawn_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\menu_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\cli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\vscode\launch_windows.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\vscode\config.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\cli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissues\ghissues.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissues\cli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissues\cli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\cli.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\up.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\down.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\status.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\review.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\attach.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\daemon.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\cmd.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\state.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\spawnattach_other.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\spawnattach_windows.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\cli_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\cmd_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\state_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\muxpoc_smoke_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\update\update_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configsync\configsync.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\paths\paths.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\gitexec\gitexec.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\update\update.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\output\output.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\helptree_test.go
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
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\main_test.go
- C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\testtiming\main.go

## Plan files to review
- Overview: `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\00-overview.md`
- Batches:
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\01-board-split.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\02-weft-split.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\03-warp-split.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\04-ide-split.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\05-ghissues-split.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\06-muxpoc-rename.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\07-update-fold.md`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\plan\08-constraints-docs-guards.md`

Read the overview and every batch listed above. Then read the source files referenced across all batches:
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\board.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\store.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\task.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\layer.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\render.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\git.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\sync.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\config.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\template.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\spawn.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\template.yaml`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\board_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\store_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\task_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\layer_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\render_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\config_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\template_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configengine\config.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\boardtest\bench_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\boardtest\concurrency_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\boardtest\git_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\boardtest\sync_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\boardtest\doc.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\cli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\cli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\help_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\board\skipenv_internal_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\clihelp\exec.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\main.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configreg\configreg.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\menu.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\initcli\initcli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\weft.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\config.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\sync.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\status.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\template.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\template.yaml`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\config_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\sync_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\status_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\template_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\weft_integration_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\cli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\spawn.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\weft\cli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configreg\configreg_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configcli\configcli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configcli\configcli_integration_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\add.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\checkout.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\remove.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\prune.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\cleanup.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\status.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\list.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\reconcile.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\worktreelifecycle.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\drift.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\junction.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\hook.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\weftwiring.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\launchers.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\portals.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\ancestors.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\config.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\template.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\clone.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\warp.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\template.yaml`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\post-checkout.sh`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\fslink\fslink.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\add_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\checkout_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\cleanup_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\config_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\drift_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\hook_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\launchers_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\list_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\portals_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\prune_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\reconcile_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\remove_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\status_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\template_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\weftwiring_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\ancestors_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\clone_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\clone_integration_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\warp\warp_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\initcli\initcli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\spawn.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\spawn_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\menu_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\cli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\vscode\launch_windows.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\vscode\config.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ide\cli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissues\ghissues.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissues\cli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\ghissues\cli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\cli.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\up.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\down.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\status.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\review.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\attach.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\daemon.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\cmd.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\state.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\spawnattach_other.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\spawnattach_windows.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\cli_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\cmd_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\state_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\muxpoc\muxpoc_smoke_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\update\update_test.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\configsync\configsync.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\paths\paths.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\gitexec\gitexec.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\update\update.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\internal\output\output.go`
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\helptree_test.go`
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
- `C:\Code\loomyard\wts\cobra-cli-engine-sweep\cmd\lyx\main_test.go`
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

## Criteria (apply to the plan as a whole)

- **Constraint violations** — BLOCKING.
- **Alignment** — plan covers all task requirements.
- **Decision alignment** — every `### Decision:` in `## Shared Decisions` faithfully implemented.
- **Completeness** — every card has `Creates`/`Edits`, `Context`, `Requirements`, `Commit`.
- **Sequencing + batch dependencies** — correct order within and across batches; `batch-depends` accurate; no forward deps.
- **Batch Index DAG integrity** — BLOCKING if the `batches:` block in `00-overview.md` has a cycle, references a batch name not declared, or names a `file:` not present in the plan directory.
- **Edge cases + risks** — failures, empty states, boundaries addressed.
- **Over-engineering** — unneeded abstractions or unrequested features.
- **Codebase consistency** — follows patterns in the source files provided.
- **Test coverage** — error paths + edges.
- **Language pitfalls** — BLOCKING if high-risk (Python: mutable defaults, import side-effects, Windows path sep, CRLF/LF).
- **Integration test reachability** — BLOCKING if integration tests added but `verify:` doesn't run them.
- **Explore targets** — purpose-driven; subset of `Context:`.
- **Step granularity + atomicity** — each card small and self-contained.
- **Requirements specificity** — BLOCKING if `Requirements:` uses vague prose ("refactor X", "update to use helper") without naming the specific function, class, or constant being changed. Stable identifiers are required.
- **Context field** — non-empty per card; Edits: files are implicitly read.
- **Context completeness** — BLOCKING if `Requirements:` mentions a function, class, or constant from a file not listed in `Context:` or `Edits:`. The implementer may only read files in `Context:`; a missing entry means cold-start exploration.
- **Global step numbering** — unique, sequential, no gaps across batches.

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line. Everything outside these markers is ignored by the backend. **No preamble inside the markers.** Per finding: 3–5 lines, short and factual. The consumer has full context of the plan; do NOT explain background. Cite the batch/card, state what's wrong, propose the fix.

Target length: ~300 tokens for APPROVE, ~600–1200 tokens for REQUEST_CHANGES across multiple batches. If you produce more than ~1500 tokens, compress.

```
MILL_REVIEW_BEGIN
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: opushigh
reviewed_file: plan/
date: <UTC YYYY-MM-DD>
```

## Findings

### [BLOCKING] <short title, <60 chars>
**Location:** <batch / card number>
**Issue:** <one sentence>
**Fix:** <one sentence>

### [NIT] <short title>
**Location:** <batch / card>
**Issue:** <one sentence>
**Fix:** <one sentence>

## Missing context
(include ONLY when verdict is NEED_CONTEXT — omit the section otherwise)

- `path/to/file.py` — <one-line reason the reviewer needs this file>

## Verdict

<APPROVE | REQUEST_CHANGES | NEED_CONTEXT>
<one sentence — max 20 words>
MILL_REVIEW_END
```

Severity / verdict rules match review-plan-batch.md.

Omit `## Findings` if zero findings. Never invent findings to pad.
