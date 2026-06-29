# Discussion: Rename Cobra modules to `<module>cli`, extract kernels as `<module>engine`

```yaml
task: Rename Cobra modules to <module>cli, extract kernels as <module>engine
slug: cobra-cli-engine-sweep
status: discussing
parent: main
```

## Problem

The `internal/` module names are inconsistent. The recent `config → configengine`
work introduced the convention "a `cli` suffix only when the bare name is taken"
(so `configcli`/`configengine`, `initcli`), while every other Cobra module keeps a
bare name (`board`, `warp`, `weft`, …). Those bare names are also domain nouns that
collide with the real things they name (`weft`/`warp`/`board` are both packages and
domain concepts), and they mix two responsibilities in one package: the Cobra
command layer and the pure domain logic.

This task **inverts** the convention written into `CONSTRAINTS.md` by the
`configengine` PR. The new, uniform rule: **anything registered in `newRoot()` (i.e.
anything that lands in Cobra) is named `<module>cli`; the domain kernel a non-CLI
consumer needs is extracted as `<module>engine`.** Precedent already exists
(`yamlengine`, `configengine`). The designed future consumer of every engine is
**loom** (does not exist yet) — the orchestrator that will drive these operations
programmatically instead of through the binary.

Why now: it must land when no other large CLI change is in flight (high conflict
surface), and its precursor — the `config → configengine` PR — is already merged.

## Scope

**In:**

- Split these modules into two packages each (`<module>cli` + `<module>engine`), in
  **new directories**, deleting the old `internal/<module>` directory:
  - `internal/board`   → `internal/boardcli` + `internal/boardengine`
  - `internal/weft`    → `internal/weftcli` + `internal/weftengine`
  - `internal/warp`    → `internal/warpcli` + `internal/warpengine`
  - `internal/ide`     → `internal/idecli` + `internal/ideengine`
  - `internal/ghissues`→ `internal/ghissuescli` + `internal/ghissuesengine`
- **Rename-only** (no split): `internal/muxpoc` → `internal/muxpoccli` (package +
  directory rename; all files stay inside `muxpoccli` unchanged). Deliberate
  exception: muxpoc is a throwaway POC slated for replacement by the real `mux`
  module, so a clean cli/engine boundary is wasted polish. The `cli` suffix alone
  achieves the disambiguation goal.
- **Fold `update` away**: remove `internal/update`; re-home its behaviour as a
  `reconcile` subcommand on `lyx config` (`lyx update` → `lyx config reconcile`).
- Update every cross-module importer, `cmd/lyx/main.go`, `internal/configreg`, and
  the affected guard tests to the new package names.
- Rewrite the `CONSTRAINTS.md` CLI/Cobra invariant: invert the package-naming
  convention **and** codify the cli/engine split rules as first-class repo rules
  (litmus, boundary, dependency direction, skip clause) — see Decisions.
- Update docs (`docs/overview.md` module table; affected `docs/modules/*`,
  `docs/shared-libs/*`). Also:
  - `docs/benchmarks/*` — fix **runnable command paths and clickable links** that
    name renamed packages: `board-performance.md` (the link
    `[internal/board/boardtest](../../internal/board/boardtest)` and the
    `go test ... ./internal/board/boardtest` command → `internal/boardengine/boardtest`),
    `running-tests.md` (`go test ./internal/weft`, `go test ./internal/board/boardtest`
    examples → new package paths). The **historical timing tables / narrative in
    `test-suite-timing.md`** are point-in-time measurement records; leave the
    historical package-name cells as-is (rewriting past measurements would falsify the
    record) — fix only any runnable command or clickable link there.
  - `docs/sandbox-hub.md` — the clickable link
    `[internal/warp/clone.go](../internal/warp/clone.go)` will 404 post-rename; retarget
    it to `internal/warpengine/clone.go` (the `deriveHostName`/`cloneHub` domain half
    lands in `warpengine`), and fix the `deriveHostName` attribution path.
  - `docs/roadmap.md` — correct the stale path refs (`internal/warp`,
    `internal/ghissues`) for accuracy. This is a **path correction**, not a roadmap
    content change or milestone append, so it does not violate the "roadmap =
    planned milestones only" rule.
- Update stale **comment-only** references to the renamed packages for accuracy
  (non-functional but they drift on rename): `internal/lyxtest/doc.go` (mentions
  `internal/warp`, `internal/weft`), `tools/sandbox/main.go` (mentions
  `internal/warp/clone.go`), `cmd/lyx/main_test.go` (mentions `internal/board`),
  `internal/paths/paths.go:344` (mentions "seeders in `internal/warp`"), and
  `cmd/testtiming/main.go:36,180` (illustrative `internal/board` in a doc-comment /
  `shortPkg` example — will name a nonexistent package post-rename; update the
  illustrative path or note it is purely illustrative), and
  `internal/configreg/configreg.go:3-4` (doc comment names modules `board, warp, weft`
  and says "used by init, **update**, and config CLI commands" — drop/replace the
  `update` mention, since `update` is folded into `config reconcile`; configreg.go is
  already edited for the importer retargets, so fold the comment fix into that edit).

**Out:**

- **No behaviour changes** other than the single observable CLI change
  (`lyx update` → `lyx config reconcile`). This is a behaviour-preserving sweep.
- **`Use:` command names do not change** — only Go package (and directory) names.
  `lyx board`, `lyx warp`, etc. invoke exactly as before.
- **No backward-compat alias** for `update` (no hidden forwarding command).
- **muxpoc is not split** — do not extract a `muxpocengine`.
- **No opportunistic refactors / dead-code cleanup / behaviour tweaks** beyond what
  the rename mechanically requires.
- No new module functionality; `loom` is not created here.

## Decisions

### package-layout

- Decision: each split module becomes **two new directories**, `internal/<module>cli`
  (package `<module>cli`) and `internal/<module>engine` (package `<module>engine`);
  the old `internal/<module>` directory is deleted. Directory name == package name.
- Rationale: matches existing precedent (`internal/configengine`, `internal/yamlengine`).
  Go requires one package per directory.
- Rejected: keeping the engine in `internal/<module>` with only a renamed package
  declaration (dir ≠ package; inconsistent with precedent).

### cli-engine-boundary

- Decision: the **cli** package owns everything that exists because of the command
  line — `Command() *cobra.Command`, the `RunCLI(out io.Writer, args []string) int`
  seam, Cobra subcommands, flags, `Short`/`Long`, `clihelp`/`output` envelope
  handlers, exit-code handling, `PersistentPreRunE`. The **engine** package owns the
  domain kernel — types and operations that return `(T, error)`, with no Cobra, no
  `io.Writer`, no exit codes.
- Litmus: returns `(T, error)`, no cobra / `io.Writer` / exit codes → engine; exists
  only because of the command line → cli.
- Dependency direction: **cli imports engine**; **engine → engine is allowed**
  (e.g. `ideengine` imports `boardengine`); **engine must never import a `cli`
  package or cobra.**
- Rationale: this is the convention the whole task establishes; it makes the kernel
  loom-consumable and keeps the import graph acyclic and one-directional.
- Rejected: leaving domain logic co-located with Cobra (the status quo this task
  removes).

### split-is-earned

- Decision: create an engine **unless** the logic is **(a) trivial/incidental** (as
  with `initcli`, `configcli` — thin wrappers with no real domain kernel) or
  **(b) throwaway** (as with `muxpoccli` — a POC slated for replacement). "No
  external consumer today" is **not** a reason to skip: loom is the designed future
  consumer for all of these, and `board`/`warp`/`weft`/`ide` are split on exactly
  that ground, not because a caller exists now.
- Rationale: an empty/near-empty `<module>engine` is the indirection we are trying to
  avoid; but permanent, non-trivial domain logic earns its engine even with no
  current caller.
- Applied results:
  - board, warp, weft → split (rich domain kernels).
  - ide → split (`Spawn` = open editor for a worktree, `Menu` = pick a task; both
    permanent, non-trivial domain loom will drive).
  - ghissues → split (`createIssue` + the gh argv/stdin seams are real domain).
  - muxpoc → **no split** (throwaway POC).
- Rejected: (1) splitting all modules uniformly regardless of kernel size; (2)
  keeping ide cli-only because it has no caller today (inconsistent with the same
  reasoning that splits board/warp/weft).

### ambiguous-file-placement

- Decision:
  - board `spawn.go` (`spawnSync`) → **`boardengine`**. Its caller is
    `Board.Sync()` (`board.go:88`) — engine-internal, so this is a clean
    engine→engine placement; no export needed.
  - weft `spawn.go` (`spawnPush`) → **`weftcli`** (NOT engine). Its only caller is
    the sync `RunE` in `cli.go:230`; weft has **no engine `Sync()`** (only `Commit`/
    `Push`/`Pull`). It is the CLI's mechanism for backgrounding the push it just ran,
    so it stays with the CLI and needs no export. (This is the asymmetry vs. board,
    whose spawn is engine-called.)
  - warp `clone.go` → **split the file**: handler half (`runClone`,
    `runCloneWithReset`) → `warpcli`; domain half (`cloneHub`, `cloneRepo`,
    `teardownHub`, `deriveHostName`, `deriveBoardURL`) → `warpengine`. Because the
    cli half calls into the engine half, **export the surface it needs**:
    `cloneHub` → `warpengine.CloneHub` (called by `runClone`, clone.go:95; returns
    `(hubPath, resolvedBoardURL, err)`), `deriveHostName` → `warpengine.DeriveHostName`
    and the `hubSuffix` const → `warpengine.HubSuffix` (both called by
    `runCloneWithReset`), and the `removeAll` test seam → a single exported settable
    seam `warpengine.RemoveAll` (`var RemoveAll = os.RemoveAll`) used by **both**
    `runCloneWithReset` (cli) and `teardownHub` (engine). The reset-swap test
    (`clone_integration_test.go:309–353`) tests `runCloneWithReset`'s reset path, so it
    goes to `warpcli` and swaps `warpengine.RemoveAll` (cross-package, the ghissues
    seam pattern). `deriveBoardURL`/`cloneRepo` stay engine-internal (only `cloneHub`
    calls them).
  - ide `menu.go` (interactive `Menu(l, in io.Reader, out io.Writer) error`) →
    **engine** (`ideengine`); it returns `error`, has no cobra, and its `board`
    usage retargets to `boardengine`.
- Rationale: follow the litmus AND the actual call direction. A symbol moves to the
  package its caller can reach; when a cli-half caller needs an engine symbol, export
  it (never the reverse — engine never imports cli). `Menu` takes a reader/writer but
  is interactive domain (no cobra, returns error), the kind of picker loom would drive.
- Rejected: blindly placing both `spawn.go` files in engine (weft's would orphan its
  cli caller); leaving warp's cli-half calls pointing at unexported engine helpers.

### update-fold

- Decision: remove `internal/update` entirely. Add a `reconcile` subcommand to
  `configcli.Command()` carrying the `--apply` flag (dry-run default), whose handler
  is the current `runUpdate` body (resolve `Layout`, `configsync.ReconcileAll(baseDir,
  apply)`, emit the JSON envelope). Remove `update` from `cmd/lyx/main.go`
  (`import`, `root.AddCommand`) and from the root command's `Long` module list. No
  alias.
- Rationale: `update` is vague and misleading (it does not update the binary; it
  reconciles module configs against templates) and is a thin CLI shell over
  `configsync.ReconcileAll`. `config reconcile` names it correctly and lives next to
  the other config operations. Behaviour is unchanged; only the invocation path moves.
- Rejected: keeping `lyx update`; adding a hidden deprecated alias.
- Note: `configcli` must add the `internal/configsync` import. `lyx config` keeps its
  existing `[module]` edit/menu `RunE`; Cobra resolves `lyx config reconcile` to the
  new subcommand and `lyx config <module>` to the `RunE` arg, so both coexist.

### constraints-as-deliverable

- Decision: the cli/engine split rules established here are codified in
  `CONSTRAINTS.md` as repo rules in the **same task** (final batch): (1) invert the
  package-naming convention (anything in `newRoot()`/cobra → `<module>cli`; domain
  kernel → `<module>engine`; cite `yamlengine`/`configengine`); (2) record the
  litmus, the cli/engine boundary, the dependency direction (cli→engine, engine→engine
  allowed, engine never imports cli/cobra), and the skip clause (trivial/incidental or
  throwaway). Per `CLAUDE.md`, a new cross-cutting invariant is recorded in
  `CONSTRAINTS.md` in the same commit as the behaviour.
- Rationale: the convention only holds if it is written down and review-enforced; the
  current `CONSTRAINTS.md` "Package naming" section says the opposite and must be
  replaced.
- Rejected: leaving the rule implicit / documenting it only in a module doc.

## Technical context

**Module map (from codebase exploration). Cobra is concentrated in exactly one file
per module** (`cli.go`, `warp.go`, or `update.go`); each module exposes the same seam
pair `Command() *cobra.Command` + `RunCLI(out io.Writer, args []string) int`
(delegating to `clihelp.Execute`); only `cmd/lyx/main.go` wires these into the root.

Per-module file placement:

- **board** (`internal/board` → `boardcli` + `boardengine`):
  - `boardcli`: `cli.go` (+ `cli_test.go`, `help_test.go`, `skipenv_internal_test.go` —
    verify which package each `_test.go` belongs to by what it exercises).
  - `boardengine`: `board.go` (Board facade), `store.go`, `task.go`, `layer.go`,
    `render.go`, `git.go`, `sync.go`, `config.go` (imports `configengine`),
    `template.go` (`ConfigTemplate`), `spawn.go`. Engine tests: `board_test.go`,
    `store_test.go`, `task_test.go`, `layer_test.go`, `render_test.go`,
    `config_test.go`, `template_test.go`, `template.yaml` asset.
  - `internal/board/boardtest/` is a test-support subpackage — its new home is
    **pinned to `internal/boardengine/boardtest`** (it supports the board domain
    tests). Move it there, update its `doc.go` comment, fix every importer's path, and
    update the doc command/link references (see docs sweep:
    `internal/board/boardtest` → `internal/boardengine/boardtest`). Inspect its
    importers during the board batch to confirm the retarget set.
  - Engine exports already present: `Board`, `New`, `Store`, `Task`, `NewTask`,
    `ApplyPatch`, `ComputeLayers`, `RenderOrder`, `ExtendedTitle`, `Render`,
    `RenderToDisk`, `Pull`, `CommitPush`, `Sync`, `Config`, `Outputs`, `LoadConfig`,
    `ConfigTemplate`, `BriefTask`, `MergeStatusUpdate` (the last two are referenced by
    `boardcli`'s `cli.go` — already exported, they move to `boardengine` with the rest).
  - Importers to retarget: `internal/configreg` (`board.ConfigTemplate` →
    `boardengine.ConfigTemplate`), `internal/ide` `menu.go` (`board.LoadConfig`,
    `board.New` → `boardengine`), `cmd/lyx/main.go` (`board.Command` →
    `boardcli.Command`), and **`internal/initcli/initcli_test.go`** (`board.LoadConfig`
    → `boardengine.LoadConfig`).

- **weft** (`internal/weft` → `weftcli` + `weftengine`):
  - `weftcli`: `cli.go` (rich `PersistentPreRunE`, hidden `--weft-path` bypass),
    **`spawn.go`** (`spawnPush` — CLI-only caller; stays unexported in weftcli) +
    `cli_test.go`.
  - `weftengine`: `weft.go` (package doc + domain constants `commitMessage`/
    `lockDirName`/`writeLockFile`/`pushLockFile` — these are used by `sync.go` and
    stay engine-internal — plus `scopedPathspec`, which is called by `weftcli`'s
    `PersistentPreRunE` (`cli.go:104`) and must be **exported as
    `ScopedPathspec`**), `config.go`, `sync.go` (`Commit`/`Push`/`Pull`/
    `SyncOptions`), `status.go` (`Status`), `template.go` (`ConfigTemplate`). Engine
    tests: `config_test.go`, `sync_test.go`, `status_test.go`, `template_test.go`,
    `weft_integration_test.go`, `template.yaml`.
  - Importers: `configreg` (`weft.ConfigTemplate` → `weftengine`), `configcli`
    (`weft.RunCLI` → `weftcli.RunCLI`), `cmd/lyx/main.go` (`weft.Command` →
    `weftcli.Command`), **`internal/initcli/initcli_test.go`** (`weft.LoadConfig` →
    `weftengine.LoadConfig`), and **`internal/configreg/configreg_test.go`**
    (`weft.ConfigTemplate` → `weftengine.ConfigTemplate`).

- **warp** (`internal/warp` → `warpcli` + `warpengine`):
  - `warpcli`: `warp.go` (cobra tree + handlers), the **handler half of `clone.go`**
    (`runClone`, `runCloneWithReset`). cli tests: `warp_test.go`, plus the handler
    portions of `clone_test.go`/`clone_integration_test.go` — including the
    reset-swap test (`clone_integration_test.go:309–353`), which swaps the exported
    `warpengine.RemoveAll` seam.
  - `warpengine`: `add.go`, `checkout.go`, `remove.go`, `prune.go`, `cleanup.go`,
    `status.go`, `list.go`, `reconcile.go`, `worktreelifecycle.go` (`Worktree`,
    `New`), `drift.go` (`PairInSync`), `junction.go` (`WireJunctions`), `hook.go`
    (`InstallPostCheckoutHook`), `weftwiring.go`, `launchers.go`, `portals.go`,
    `ancestors.go`, `config.go`, `template.go` (`ConfigTemplate`), the **domain half
    of `clone.go`**, `post-checkout.sh` asset. Engine tests: all the corresponding
    `*_test.go` files.
  - Exports the cli half needs (because `runCloneWithReset` stays in `warpcli` but
    calls into the clone domain half): `deriveHostName` → **`DeriveHostName`**, the
    `hubSuffix` const → **`HubSuffix`**, and the `removeAll` seam → a single exported
    settable seam **`var RemoveAll = os.RemoveAll`** owned by `warpengine` and used by
    both `runCloneWithReset` (cli) and `teardownHub` (engine).
  - warp's package doc (`warp.go`) already states the dependency discipline: warp must
    NOT import `initcli`/`configsync`; `initcli` imports warp; `configreg` imports warp.
    After the split those land on `warpengine`.
  - Importers: `configreg` (`warp.ConfigTemplate` → `warpengine`), `initcli`
    (`warp.WireJunctions` → `warpengine`), `cmd/lyx/main.go` (`warp.Command` →
    `warpcli.Command`), and **`internal/configcli/configcli_integration_test.go`**
    (uses `warp.New`, `warp.Config`, `warp.AddOptions`, `w.Add`, `warp.WireJunctions`
    → all `warpengine`; its `weft.RunCLI` usage retargets to `weftcli.RunCLI` in the
    weft batch), and **`internal/initcli/initcli_test.go`** (`warp.LoadConfig` →
    `warpengine.LoadConfig`). The importer list is exhaustive for production + test
    consumers; the build-green gate is the backstop.

- **ide** (`internal/ide` → `idecli` + `ideengine`):
  - `idecli`: `cli.go` + `cli_test.go`.
  - `ideengine`: `spawn.go` (`Spawn(l *paths.Layout, slug string) error`), `menu.go`
    (`Menu(...)`; retarget its `board.*` usage — `LoadConfig`/`New`/`HealthCheck`/
    `GetTask` — to `boardengine`). Engine tests: `spawn_test.go`, `menu_test.go`.
  - **`codeLauncher` seam**: `spawn.go:15` defines `var codeLauncher = vscode.Launch`,
    swapped by `cli_test.go` (→ `idecli`), `menu_test.go` and `spawn_test.go` (both →
    `ideengine`). Since `cli_test.go` leaves the package, **export it** as a settable
    seam `ideengine.CodeLauncher = vscode.Launch`; `spawn.go`/`menu.go` reference
    `CodeLauncher`; the in-package engine tests swap it directly; `idecli`'s
    `cli_test.go` swaps `ideengine.CodeLauncher` cross-package (the warp `RemoveAll` /
    ghissues `RunGH` pattern). (Note the shared global keeps these tests serial.)
  - `ideengine` imports `boardengine` (the one engine→engine edge). Ordering: board
    must be split before ide, or ide's retarget done in board's batch — see Testing.
  - Importers: `cmd/lyx/main.go` (`ide.Command` → `idecli.Command`).

- **ghissues** (`internal/ghissues` → `ghissuescli` + `ghissuesengine`): the current
  `ghissues.go` mixes a **cli-side seam** (`var stdin io.Reader`, read by `runCreate`
  in `cli.go:94`) with **engine seams** (`var runGH`, `createIssue`). A wholesale
  `ghissues.go → ghissuesengine` move would break `runCreate` and the white-box
  `cli_test.go`. Resolve the seam placement explicitly:
  - `ghissuescli`: `cli.go` (cobra + `runCreate`) **plus the `stdin` seam** (move
    `var stdin io.Reader = os.Stdin` here — it is a CLI-input concern that only
    `runCreate` reads). `runCreate` calls `ghissuesengine.CreateIssue`.
  - `ghissuesengine`: the rest of `ghissues.go`. **Export exactly two symbols**:
    `CreateIssue` (returns `(url, number, error)`; called by `runCreate`) and a
    **settable seam** `var RunGH = realRunGH` (so tests can swap the gh invocation).
    Keep `targetRepo`, `realRunGH`, `buildCreateArgs`, `lastNonEmptyLine`
    engine-internal (unexported — only `createIssue`/`CreateIssue` uses them).
  - Importers: `cmd/lyx/main.go` (`ghissues.Command` → `ghissuescli.Command`).

- **muxpoc** (`internal/muxpoc` → `internal/muxpoccli`, **rename only**):
  - Rename directory and package declaration in every file to `muxpoccli`; no file
    moves, no engine. `Config` (defined in `cli.go`) stays put. All of `up.go`,
    `down.go`, `status.go`, `review.go`, `attach.go`, `daemon.go`, `cmd.go`,
    `state.go`, `spawnattach_*.go` stay inside `muxpoccli`.
  - Importers: `cmd/lyx/main.go` (`muxpoc.Command` → `muxpoccli.Command`).

- **update fold** (`internal/update` → deleted; `lyx config reconcile`):
  - `runUpdate` body: `paths.Getwd` → `paths.Resolve` → `baseDir =
    filepath.Join(l.WorktreeRoot, l.RelPath)` → `configsync.ReconcileAll(baseDir,
    apply)` → map each result (`module`/`added`/`removed`/`applied`) into the JSON
    envelope via `output.Ok`. Dry-run unless `--apply`.
  - Move this into `configcli` as the `reconcile` subcommand handler; add the
    `internal/configsync` import to `configcli`.
  - Delete `internal/update` (incl. `update_test.go`); migrate its assertions to a
    new `configcli` reconcile test.

**No `loom` / `internal/loom` package exists yet** — confirmed by exploration. The
engine extraction is for the convention and loom's designed future use.

**`cmd/testtiming/main.go`** references the string `"github.com/Knatte18/loomyard/
internal/board"` only in a comment / `shortPkg` trimming example — verify it has no
functional hardcoded package list that breaks; update if it does.

## Constraints

From `CONSTRAINTS.md` (hub root) — all remain in force:

- **Path Invariant** — all cwd/geometry and `_lyx`/config paths go through
  `internal/paths` (`paths.Getwd`, `paths.Resolve`, `paths.ConfigDir`,
  `paths.ConfigFile`, `paths.LyxDirName`). Enforced by
  `internal/paths/enforcement_test.go`. Moving files between packages must not
  introduce raw `os.Getwd` / `git rev-parse` or literal `_lyx`/config path strings.
- **lyxtest Leaf Invariant** — `internal/lyxtest` imports only stdlib + `internal/paths`;
  never `configreg` or a feature package. Tests needing config seed via
  `lyxtest.SeedConfig` with templates obtained from the (now `*engine`)
  `ConfigTemplate()` functions. Update those call sites to the new engine import paths.
- **CLI / Cobra Invariant** — module seam (`Command()` + `RunCLI` = exactly
  `clihelp.Execute(Command(), out, args)`), registration in `newRoot()`, `Short` on
  every command (`drift_test.go`), help co-located, help tree pinned
  (`helptree_test.go`), registration + Long-list guards
  (`registration_test.go`/`longlist_test.go`), JSON error envelope, parent groups
  reject unknown subcommands via `clihelp.GroupRunE`. **This invariant's "Package
  naming" section is rewritten by this task** (see constraints-as-deliverable).
- **Documentation Lifecycle** + `CLAUDE.md` task-completion docs discipline — module
  docs / `docs/overview.md` / `CONSTRAINTS.md` updated in the same commit as the
  behaviour.
- **fslink** — cross-OS links go through `internal/fslink` (directory junctions on
  Windows). Relevant if any moved warp file touches linking; keep using `fslink`.

Discovered during discussion:

- The whole sweep is **behaviour-preserving** except `lyx update` → `lyx config
  reconcile`. Build (`go build ./...`) and tests (`go test ./...`) must be green
  **after each module's batch**, not only at the end.
- `registration_test.go` assumes the selector identifier equals the package name
  (e.g. `warpcli.Command()`, package `warpcli`) — that assumption still holds after
  the rename; update the test's allowlist to the new `*cli` package names. Only the
  `*cli` packages have a `Command()`, so "exists ⇒ registered" continues to hold; the
  `*engine` packages have no `Command()`.

## Testing

Behaviour-preserving sweep, so the dominant test work is **relocating existing test
files to the correct new package and fixing import paths**, not writing new behaviour
tests. Per unit:

- **board / weft / warp / ide / ghissues split**: move each `_test.go` into whichever
  new package it exercises (cli-handler tests → `<module>cli`; domain tests →
  `<module>engine`). Keep every existing assertion; the tests are the
  behaviour-preservation guard. Confirm `go test ./...` is green after each module.
- **warp `clone_integration_test.go` must be physically split** (a single `_test.go`
  cannot belong to two packages): the `cloneHub`-driving domain scenarios → a
  `warpengine` test file (calling `warpengine.CloneHub`); the reset-swap test
  (lines 309–353, exercising `runCloneWithReset`) → a `warpcli` test file (swapping
  `warpengine.RemoveAll`). `clone_test.go` likewise splits by what each test drives.
- **ghissues tests specifically**: `cli_test.go` is white-box (package `ghissues`)
  and drives the full `cobra → flag → createIssue → runGH` pipeline through `RunCLI`,
  swapping **both** `runGH` (L24) and `stdin` (L161). After the split it becomes
  package `ghissuescli`: it sets the exported `ghissuesengine.RunGH` seam (instead of
  the unexported `runGH`) and the local `stdin` seam. Engine-only assertions (e.g.
  `buildCreateArgs` argv, `lastNonEmptyLine`, URL/number parsing in isolation) may be
  left driving through `ghissuescli`'s `RunCLI` as today, or factored into a
  `ghissuesengine` white-box test — either is acceptable so long as the existing
  scenarios (happy path, custom labels, body via flag/stdin/omitted, wrong arg count,
  gh-not-found, gh non-zero exit, unparseable URL, number parsing) all still run.
- **board/boardtest support package**: inspect and relocate; fix all importers'
  paths.
- **muxpoc rename**: package-decl + import-path change only; `muxpoc_smoke_test.go`,
  `cli_test.go`, `cmd_test.go`, `state_test.go` move with the package. No new tests.
- **update → config reconcile** (the one behavioural change worth a focused test):
  migrate `update_test.go`'s scenarios into a `configcli` reconcile test — dry-run
  default (no writes), `--apply` writes, JSON envelope shape (`applied`, `modules[]`
  with `module`/`added`/`removed`/`applied`). Verify `lyx update` no longer resolves.
- **Guard tests (final batch)**:
  - `cmd/lyx/registration_test.go` — update allowlist to new `*cli` package names.
  - `cmd/lyx/longlist_test.go` — remove `update` from the root `Long` expectation.
  - `cmd/lyx/helptree_test.go` — drop `update` from root `requiredModules`; add
    `reconcile` to `configcli`'s `wantSubs`.
  - `cmd/lyx/drift_test.go` — `reconcile` subcommand must carry a non-empty `Short`.
  - `internal/lyxtest/leaf_enforcement_test.go` — its `bannedImports` slice
    (lines ~31–35) hardcodes `internal/board`, `internal/warp`, `internal/weft`.
    After the rename those import paths vanish, so the leaf-invariant guard would
    silently stop guarding the feature packages while still passing. Update
    `bannedImports` to the new feature paths (`boardengine`/`boardcli`,
    `warpengine`/`warpcli`, `weftengine`/`weftcli`, and the other `*engine`/`*cli`
    packages) so the lyxtest Leaf Invariant keeps protecting against a
    `lyxtest → configreg → feature` cycle.
- TDD candidate: the `config reconcile` subcommand (write the migrated reconcile test
  first, then wire the subcommand). Everything else is mechanical relocation verified
  by the moved suites + `go build ./... && go test ./...`.

Sequencing (one unit per batch, build+test green between each; shared files
`main.go`/`configreg.go` force serialization):
`board → weft → warp → ide → ghissues → muxpoc(rename) → update→config-reconcile fold
→ CONSTRAINTS + docs rewrite`.

## Q&A log

- **Q:** New dirs per module or rename package in place? **A:** New dirs
  `<module>cli` + `<module>engine`, delete old `internal/<module>` (matches
  `configengine`/`yamlengine`).
- **Q:** Backward-compat alias for `update`? **A:** No — hard-remove; only
  `lyx config reconcile`.
- **Q:** When is an engine warranted? **A:** Split is *earned*: create an engine
  unless logic is (a) trivial/incidental (initcli, configcli) or (b) throwaway (POC).
  "No external consumer today" is NOT a skip reason — loom is the designed future
  consumer for all of them.
- **Q:** muxpoc — split? **A:** No. Rename-only to `muxpoccli`; it's a throwaway POC
  to be replaced by the real `mux` module. daemon/attach/state stay inside
  `muxpoccli`.
- **Q:** ide — split or cli-only (no caller today)? **A:** Split (`idecli` +
  `ideengine`). `Spawn`/`Menu` are permanent, non-trivial domain loom will drive;
  cli-only would be inconsistent with board/warp/weft split on the same grounds.
- **Q:** ghissues engine symbols are unexported — split anyway? **A:** Yes, split and
  export; `createIssue` + gh seams are real domain loom will call.
- **Q:** Where do the ambiguous files go? **A:** board/weft `spawn.go` → engine;
  warp `clone.go` → split (handlers→cli, domain→engine); ide `menu.go` → engine.
- **Q:** Scope beyond the rename? **A:** Strictly behaviour-preserving; only
  observable change is `lyx update` → `lyx config reconcile`. No opportunistic
  refactors.
- **Q:** Is engine→engine allowed (ideengine→boardengine)? **A:** Yes. Only forbidden
  direction is engine → cli/cobra.
- **Q:** Do the new split rules go anywhere durable? **A:** Yes — codify them in
  `CONSTRAINTS.md` as repo rules (invert package-naming convention + record litmus,
  boundary, dependency direction, skip clause) in the final batch.
- **Q:** Batch order? **A:** Sequential, one unit per batch, green between each:
  board → weft → warp → ide → ghissues → muxpoc → update-fold → CONSTRAINTS/docs.
- **Q:** (review r1 GAP) Which guard tests beyond cmd/lyx are affected? **A:** Also
  `internal/lyxtest/leaf_enforcement_test.go` — its hardcoded `bannedImports`
  (`internal/board|warp|weft`) must be retargeted to the new `*engine`/`*cli` paths,
  else the lyxtest Leaf Invariant silently stops guarding. Added to the final guard
  batch.
- **Q:** (review r1 NOTEs) Any omitted files / stale references? **A:** `weft.go` is
  pure domain (constants used by `sync.go`) → placed in `weftengine`. Comment-only
  refs to old paths in `internal/lyxtest/doc.go`, `tools/sandbox/main.go`,
  `cmd/lyx/main_test.go` to be updated for accuracy.
- **Q:** (review r2 GAP) How is the ghissues cli/engine seam split, given `ghissues.go`
  mixes the `stdin` (cli) and `runGH` (engine) seams and the white-box `cli_test.go`
  swaps `runGH`? **A:** `stdin` → `ghissuescli`; export only `CreateIssue` and a
  settable `RunGH` seam in `ghissuesengine`; `cli_test.go` → `ghissuescli`, swapping
  `ghissuesengine.RunGH`. `buildCreateArgs`/`lastNonEmptyLine`/`realRunGH`/`targetRepo`
  stay engine-internal.
- **Q:** (review r2 NOTE) Comment sweep complete? **A:** Added
  `internal/paths/paths.go:344` and `cmd/testtiming/main.go:36,180` (illustrative) to
  the comment-accuracy sweep.
- **Q:** (review r3 GAP) Docs scope beyond overview/modules/shared-libs? **A:** Add
  `docs/benchmarks/*` (runnable command paths + clickable links only; leave historical
  timing tables as point-in-time records), `docs/sandbox-hub.md` (fix the
  `internal/warp/clone.go` 404 link → `internal/warpengine/clone.go`), and
  `docs/roadmap.md` (correct stale path refs — a path correction, not a milestone
  append, so allowed).
- **Q:** (review r3 NOTE) boardtest's new home? **A:** Pinned to
  `internal/boardengine/boardtest`.
- **Q:** (review r3 NOTE) Any unlisted warp importer? **A:** Yes —
  `internal/configcli/configcli_integration_test.go` (warp engine symbols →
  `warpengine`); added to warp's retarget set.
- **Q:** (review r4 GAP) weft `spawnPush`/`scopedPathspec` placement — they're called
  by cli, not an engine `Sync()`. **A:** weft has no engine `Sync()`. `spawn.go`
  (`spawnPush`) → **weftcli** (CLI-only caller, no export). `scopedPathspec` (in
  `weft.go`, called by `cli.go:104`) → **weftengine, exported as `ScopedPathspec`**.
  Corrected the earlier "invoked by engine Sync/Push" rationale. (board's `spawnSync`
  IS engine-called via `Board.Sync()`, hence the asymmetry.)
- **Q:** (review r4 GAP) warp clone split leaves cli calling unexported engine
  helpers. **A:** Export `DeriveHostName`, `HubSuffix`, and a single settable
  `warpengine.RemoveAll` seam (used by both `runCloneWithReset` cli + `teardownHub`
  engine); reset-swap test → warpcli, swaps `warpengine.RemoveAll`.
- **Q:** (review r4 NOTE) board exports omitted? **A:** Added `BriefTask`,
  `MergeStatusUpdate` (referenced by `boardcli/cli.go`) to the boardengine export list.
- **Q:** (review r5 GAP) warp clone export set incomplete — `runClone` calls
  `cloneHub`. **A:** Add `cloneHub` → `warpengine.CloneHub` (returns
  `(hubPath, resolvedBoardURL, err)`); `cloneRepo`/`deriveBoardURL` stay
  engine-internal (only `cloneHub` calls them).
- **Q:** (review r5 NOTE) Can `clone_integration_test.go` just "move"? **A:** No — it
  must be **physically split**: `cloneHub` scenarios → `warpengine` test, reset-swap
  test (309–353) → `warpcli` test.
- **Q:** (review r6 GAP) ide `codeLauncher` seam across the split? **A:** Export it as
  settable `ideengine.CodeLauncher = vscode.Launch`. `cli_test.go` (→ idecli) swaps it
  cross-package; `menu_test.go`/`spawn_test.go` (→ ideengine) swap it in-package. Same
  seam class as warp `RemoveAll` / ghissues `RunGH`.
- **Q:** (review r6 NOTE) configreg doc comment stale? **A:** `configreg.go:3-4` names
  `update` — drop the `update` mention (folded into `config reconcile`); folded into
  the configreg importer-retarget edit.
