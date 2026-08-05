# Batch: core-reshape

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
batch: core-reshape
number: 1
cards: 19
verify: go vet -tags "integration smoke scout" ./... && go test ./...
depends-on: []
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package declaration, import lines, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch is the reshape plus full consumer rewrite every later batch depends on. It renames `internal/hubgeometry` to `internal/lyxcwd`, replaces `Layout` with `Location{RepoName, HubPath, WorktreeName, AnchorRel}`, installs the strict cwd gate, renames the anchor marker, extracts `internal/weftname`, and moves `LyxDirName`/`ConfigDir`/`ConfigFile` to `internal/configengine` and `DotEnv` to `internal/envsource`. Three moves the discussion schedules for later batches are pulled forward because they are forced, not preferred: `WorktreePath(slug)` (a duplicate-method compile error against the new no-arg accessor), and `worktreelist.go`/`Prime`/`PrimeName`/`WeftRepoRoot` (every consumer loses its source the moment `Prime` leaves the struct).

The moment the struct changes shape, every reader of a removed field stops compiling, so this batch rewrites all of them where they stand. Batches 2, 3 and 4 are then pure *relocation*: they move functions between packages without changing a signature. That is the property that makes them reviewable.

Batch-local decision — the ~190-file consumer sweep is split into **ten** package-family cards (8-12 production, 13-17 test) rather than one production card and one test card. Each is a single coherent commit an implementer can hold and a reviewer can read; a 100-file card would be neither. The split is by package family, not by file count, so each card's diff is one subsystem's cutover.

Batch-local decision — card 5 lands the package rename **before** cards 6 and 7, so the gate, the marker rename and the clone simplification are all authored against the post-rename `internal/lyxcwd` paths. The gate helper is pure string math over `(cwd, anchorRel, worktreePath)` and does not depend on the reshaped struct, so it could sit on either side; putting it after the rename keeps every card in this batch naming exactly one set of paths.

External interface batch 2 consumes: `configengine.LyxDirName`, `lyxcwd.Location`, `(*Location).AnchorPath()`, `(*Location).WorktreePath()`, `Location.HubPath`.

## Cards

### Card 1: extract `internal/weftname`

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/lyxtest/leaf_enforcement_test.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/branchname_test.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/hubgeometry/geometry_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/audit_test.go`
- **Creates:**
  - `internal/weftname/weftname.go`
  - `internal/weftname/weftname_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `weftname`, stdlib-only (`path/filepath` and `strings` only). Declare `const Suffix = "-weft"` as the single declarer of that token; `func SiblingPath(container, base string) string` returning `filepath.Join(container, base+Suffix)`; `func BareSiblingPath(container, base string) string` returning `filepath.Join(container, base+Suffix+"-bare")`. Delete `WeftSuffix` and `WeftSiblingPath` from `internal/hubgeometry/hubgeometry.go`; the in-module callers that remain (`WeftHostSlug`, `WeftRepoRoot`, `WeftWorktreePath`, `WeftWorktree`) call `weftname.Suffix`/`weftname.SiblingPath` instead, so `internal/hubgeometry` gains a temporary import of `internal/weftname` that disappears in batch 3 when those methods relocate. Retarget `fabricengine/branchname.go` and `add.go` to `weftname.Suffix`, and `clone.go` to `weftname.SiblingPath`. Retarget `fabriccli/fabric.go`'s three cobra `Long` help-text uses at lines 45, 67 and 78 to `weftname.Suffix` — user-facing prose, not geometry, and the rendered string must not change. Retarget `websterengine/audit.go:89`'s `regexp.QuoteMeta(hubgeometry.WeftSuffix)` to `weftname.Suffix`; its comment at `:85` insisting the value come from the constant "never from string literals" stays true and is updated to name the new source. In `lyxtest/lyxtest.go`, retarget the three `hubgeometry.WeftSiblingPath` calls to `weftname.SiblingPath` and replace the two hardcoded `base+"-weft-bare"` literals at lines 194 and 458 with `weftname.BareSiblingPath(tmpDir, base)` / `weftname.BareSiblingPath(tempContainer, base)`. `lyxtest` does not consume weft geometry — it *produces* the on-disk shape production expects, and the fixture is valid only if its directory names match what production derives, which is why one shared leaf rather than two constants. The new test covers `SiblingPath`/`BareSiblingPath` over container/base combinations plus an assertion that the name `lyxtest` builds and the name `fabricengine` derives agree for the same input; untagged, pure string math.
- **Commit:** `refactor(weftname): extract the -weft naming convention into a stdlib-only leaf`

### Card 2: move `LyxDirName`/`ConfigDir`/`ConfigFile` to `configengine` and `DotEnv` to `envsource`

- **Context:**
  - `CONSTRAINTS.md`
  - `docs/shared-libs/configengine.md`
  - `docs/shared-libs/envsource.md`
- **Edits:**
  - `cmd/lyx/exitcode_test.go`
  - `cmd/lyx/main_integration_test.go`
  - `internal/boardcli/cli_test.go`
  - `internal/boardengine/boardtest/bench_test.go`
  - `internal/boardengine/config_test.go`
  - `internal/buildercli/smoke_test.go`
  - `internal/buildercli/weft.go`
  - `internal/buildercli/weft_integration_test.go`
  - `internal/burlerengine/config.go`
  - `internal/burlerengine/config_test.go`
  - `internal/configcli/configcli.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/configcli/configcli_test.go`
  - `internal/configcli/menu.go`
  - `internal/configcli/reconcile_integration_test.go`
  - `internal/configengine/config.go`
  - `internal/configengine/config_test.go`
  - `internal/configengine/edit.go`
  - `internal/configengine/edit_test.go`
  - `internal/configengine/set.go`
  - `internal/configengine/set_test.go`
  - `internal/configsync/configsync.go`
  - `internal/configsync/configsync_test.go`
  - `internal/envsource/envsource.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/config_test.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/ideengine/menu.go`
  - `internal/ideengine/menu_test.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/lyxtest_test.go`
  - `internal/modelspec/leaf_enforcement_test.go`
  - `internal/modelspec/load.go`
  - `internal/modelspec/load_test.go`
  - `internal/modelspec/modelspec.go`
  - `internal/modelspec/template_test.go`
  - `internal/perchcli/run.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/perchengine/config_test.go`
  - `internal/reedengine/config_test.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/scoutengine/doc.go`
  - `internal/scoutengine/leaf_enforcement_test.go`
  - `internal/scoutengine/load.go`
  - `internal/scoutengine/load_test.go`
  - `internal/shuttleengine/config_test.go`
  - `internal/webstercli/weft.go`
  - `internal/webstercli/weft_integration_test.go`
  - `internal/websterengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare `LyxDirName = "_lyx"`, `configDirName = "config"`, `func ConfigDir(baseDir string) string` and `func ConfigFile(baseDir, module string) string` in `internal/configengine/config.go`, with the bodies they have at `hubgeometry.go:24-31,187-195`. `configengine` becomes the single declarer of `_lyx`; the direction is forced, not chosen — `configengine/config.go:23` already consumes the constant and `fabricengine/config.go:16` already imports `configengine`, so declaring `_lyx` in `fabricengine` would be a compile-time cycle. Declare `dotEnvName = ".env"` and `func DotEnv(baseDir string) string` in `internal/envsource/envsource.go`; `envsource.Build` calls its own `DotEnv` and drops the `internal/hubgeometry` import entirely, becoming a stdlib-only leaf. `DotEnv` goes here and **not** to `configengine` because `configengine/config.go:15,66` already imports `envsource`, so the obvious move is a cycle — and `envsource` already owns the `.env` format end to end. Delete `LyxDirName`, `configDirName`, `dotEnvName`, `ConfigDir`, `ConfigFile` and `DotEnv` from `internal/hubgeometry/hubgeometry.go`. Its own remaining `_lyx` users (`PerchRunsDir`, `PlanDir`, `PlanDirRel`, `BuilderDir`, `WebsterDir`, `LyxDir`, `LoomStatusFile`, `LoomStatusLock`, `DiscussionDir`, `PortalTarget`, `HostLyxLink`, `HostLyxLinkHere`, `WeftLyxDir`, `WeftLyxDirFor`) keep working via a **private** `lyxDirName = "_lyx"` const declared in `hubgeometry.go` — a transitional second declarer, registered as such in card 19's guard slice and removed in batch 5 once every one of those functions has left. Every other listed file swaps the qualifier and fixes its import block. Verify `go list -deps` shows no `configengine` → `hubgeometry` edge afterwards. `modelspec/leaf_enforcement_test.go` and `scoutengine/leaf_enforcement_test.go` add `github.com/Knatte18/loomyard/internal/configengine` to their allowlist maps, or their leaf tests fail the moment this card lands. `modelspec/modelspec.go:27` and `scoutengine/doc.go:112` are godoc references naming `hubgeometry.ConfigFile`; correct them.
- **Commit:** `refactor(configengine): own _lyx, ConfigDir and ConfigFile; envsource owns DotEnv`

### Card 3: move `worktreelist.go`, `Prime`, `PrimeName` and `WeftRepoRoot` to `fabricengine`

- **Context:**
  - `internal/hubgeometry/worktreelist.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/topology.go`
  - `internal/weftname/weftname.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/checkout_index_refresh_test.go`
  - `internal/fabricengine/checkout_rollback_test.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/list.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/hubgeometry/geometry_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
  - `internal/hubgeometry/hubgeometry_unit_test.go`
  - `internal/idecli/cli.go`
  - `internal/ideengine/menu.go`
  - `internal/ideengine/menu_test.go`
  - `internal/ideengine/spawn.go`
  - `internal/ideengine/spawn_test.go`
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/tokenvocab/tokenvocab_test.go`
  - `internal/vscode/color.go`
  - `internal/vscode/color_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/hubgeometry/worktreelist.go` -> `internal/fabricengine/worktreelist.go`
  - `internal/hubgeometry/worktreelist_test.go` -> `internal/fabricengine/worktreelist_test.go`
- **Requirements:** `List`, `WorktreeEntry` and `parseWorktreePorcelain` become `fabricengine` declarations; `internal/fabricengine/list.go` drops its `WorktreeEntry` type alias and its `hubgeometry.List` delegation, keeping `(*Topology).List` as a one-line call to the now-local `List`. Delete the `Prime` and `Repo` fields, the `PrimeName()` and `WeftRepoRoot()` methods and the `deriveRepo` function from `internal/hubgeometry/hubgeometry.go`, and delete the `List(cwd)` call and prime-scan loop from `resolveCore` (`hubgeometry.go:130-143`) — that subprocess ran on **every** `Resolve` purely to find the `Main` entry, even though the result is identical for every worktree under one hub. `deriveRepo` is deleted, not moved: card 7 redefines the repo name as `TrimSuffix(Base(HubPath), "-HUB")`, which leaves it with no caller on either side. Add to `fabricengine`: `func PrimeName(l *hubgeometry.Layout) (string, error)` (resolves via the local `List`, returns `filepath.Base` of the `Main` entry) and `func WeftRepoRoot(l *hubgeometry.Layout) (string, error)` (`weftname.SiblingPath(l.Hub, primeName)`); retarget all 13 in-package `l.WeftRepoRoot()` call sites (`add.go:108,149`, `cleanup.go:176,208`, `weftwiring.go:36,55,69,126`, `reconcile.go:172,204,227`, `prune.go:137,152`) plus their test callers. `WeftRepoRoot` is pulled forward from batch 3 for exactly this reason: threading a `primeName` argument through 13 in-package call sites now and relocating all 13 again later is strictly more work. `MenuLauncherRel()` takes a `primeName string` argument instead of reading `l.Prime`, and stays in the module until batch 3. Change `vscode.PickColor` to `func PickColor(l *hubgeometry.Layout, primeName string) string`, using `primeName` where it used `filepath.Base(l.Prime)` and skipping the prime-skip step when it is empty — sourcing the name from `fabricengine` inside `vscode` would pull the entire fabric engine into a colour picker and reintroduce a `git worktree list` subprocess per call. `internal/ideengine` gains the `internal/fabricengine` import (it has none today — `spawn.go:6-11` imports only `filepath`, `hubgeometry` and `vscode`): `ideengine/spawn.go` sources the prime name via `fabricengine.PrimeName(l)` and passes it to `PickColor`, degrading to the empty string on error rather than failing the spawn. `loomengine/preflight.go:67`'s `l.Prime == ""` check becomes a `fabricengine.PrimeName(l)` call producing the same `Report`; `loomengine` already imports `fabricengine`. `ideengine/menu.go:38`'s `hubgeometry.List(l.Cwd)` becomes `fabricengine.List(l.Cwd)`. Leave `tokenvocab`'s `c.Layout.Repo` expression alone — card 5 renames the field.
- **Commit:** `refactor(fabricengine): own the worktree list, Prime and WeftRepoRoot`

### Card 4: remove the two methods the reshape displaces

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/reconcile.go`
- **Edits:**
  - `internal/configcli/configcli_integration_test.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/hook_test.go`
  - `internal/fabricengine/hostlayout.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/ideengine/spawn.go`
  - `internal/ideengine/spawn_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/hubgeometry/siblinglayout_test.go`
- **Moves:** none
- **Requirements:** Two deletions from `internal/hubgeometry/hubgeometry.go`, both forced by the new struct. First, delete `(*Layout).WorktreePath(slug string)` (`:410-413`) — its name collides with the no-arg `WorktreePath()` accessor card 5 introduces on the same type, which is a duplicate-method compile error, so it cannot wait for its scheduled batch-3 move. Add `func WorktreePath(l *hubgeometry.Layout, slug string) string` to `internal/fabricengine/junction.go` with the identical body (`filepath.Join(l.Hub, slug)`) and retarget the `fabricengine` and `ideengine/spawn.go:20` production callers plus every listed test caller. The four **in-module** callers that outlive the method and cannot import `fabricengine` — `LauncherSpawnRel` (`:459`), `HostLyxLink` (`:524`), `HostPatternLink` (`:537`) and `HostJunctions` (`:570`) — each get the method's entire body inlined as `filepath.Join(l.Hub, slug)`, carried until batch 3 relocates them. Second, delete `(*Layout).SiblingLayout` (`:164-185`) and its test file: it exists to recompute `Hub`, `RelPath`, `Prime` and `Repo` for a hub sibling, and under the new struct a hub sibling differs in exactly one field, so there is nothing left to compute. It is also the last constructor setting a dishonest `Cwd` (`:179`). `fabricengine/hostlayout.go`'s `hostLayoutFor` constructs the sibling inline instead — same `Hub`, same `RelPath`, `WorktreeRoot`/`Cwd` set to `filepath.Clean(worktreeRoot)`, reading the anchor the same way; card 5 rewrites that literal into `&lyxcwd.Location{HubPath: l.HubPath, WorktreeName: filepath.Base(worktreeRoot), AnchorRel: l.AnchorRel}`. The non-sibling `ResolveWorktree` fallback at `hostlayout.go:22` is unchanged, and its guard already rejects the non-sibling case before the inline construction is reached.
- **Commit:** `refactor(hubgeometry): drop WorktreePath(slug) and SiblingLayout`

### Card 5: rename the package to `internal/lyxcwd` and reshape `Layout` into `Location`

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/hostlayout.go`
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/lyxcwd/anchor.go` -> `internal/lyxcwd/anchor.go`
  - `internal/hubgeometry/anchor_test.go` -> `internal/lyxcwd/anchor_test.go`
  - `internal/hubgeometry/discussionpath_test.go` -> `internal/lyxcwd/discussionpath_test.go`
  - `internal/hubgeometry/enforcement_test.go` -> `internal/lyxcwd/enforcement_test.go`
  - `internal/hubgeometry/geometry_test.go` -> `internal/lyxcwd/geometry_test.go`
  - `internal/hubgeometry/hubgeometry.go` -> `internal/lyxcwd/lyxcwd.go`
  - `internal/hubgeometry/hubgeometry_test.go` -> `internal/lyxcwd/lyxcwd_test.go`
  - `internal/hubgeometry/hubgeometry_unit_test.go` -> `internal/lyxcwd/lyxcwd_unit_test.go`
  - `internal/hubgeometry/loomstatus_test.go` -> `internal/lyxcwd/loomstatus_test.go`
  - `internal/hubgeometry/pattern_test.go` -> `internal/lyxcwd/pattern_test.go`
  - `internal/hubgeometry/planpath_test.go` -> `internal/lyxcwd/planpath_test.go`
  - `internal/hubgeometry/raddle_guard_test.go` -> `internal/lyxcwd/raddle_guard_test.go`
  - `internal/hubgeometry/scoutdaemon_test.go` -> `internal/lyxcwd/scoutdaemon_test.go`
  - `internal/hubgeometry/testmain_test.go` -> `internal/lyxcwd/testmain_test.go`
  - `internal/hubgeometry/webstergeom_test.go` -> `internal/lyxcwd/webstergeom_test.go`
  - `internal/hubgeometry/weft_test.go` -> `internal/lyxcwd/weft_test.go`
  - `internal/hubgeometry/worktreelogs_test.go` -> `internal/lyxcwd/worktreelogs_test.go`
- **Requirements:** `git mv` the package directory first, then rename the package clause to `lyxcwd` in every moved file and rewrite the package godoc: the module is no longer a geometry owner — constructing paths from structural tokens is precisely what it stops doing — it is the entry gate that converts "the process started somewhere" into "these are the coordinates of a legal lyx worktree, or here is why this is not one". Rename the type `Layout` to `Location` and replace its fields with, in this order, `RepoName string`, `HubPath string`, `WorktreeName string`, `AnchorRel string`. The order is outermost identity first: a repo has hubs, a hub has worktrees, a worktree has an anchored subpath. `Cwd` stops being a field — it remains the parameter to `Resolve(cwd)`, because it is the only thing the process knows at startup, but under the strict gate it is provably equal to `AnchorPath()` after a successful resolve and storing it would duplicate a derivable value. The worktree path is likewise not stored: the worktree is a direct child of the hub by construction (`hub := filepath.Dir(worktreeRoot)` today), so `WorktreeName` suffices. Add `func (l *Location) WorktreePath() string` returning `filepath.Join(l.HubPath, l.WorktreeName)` and `func (l *Location) AnchorPath() string` returning `filepath.Join(l.WorktreePath(), l.AnchorRel)`. `resolveCore` sets `HubPath` from `filepath.Dir(workTreeRoot)`, `WorktreeName` from `filepath.Base(workTreeRoot)`, `AnchorRel` from the recorded marker falling back to `"."` — **not** to the cwd-derived relative path, which would make the new name a lie and makes `_lyx` resolve to a different place depending on where the user happened to stand — and `RepoName` from `strings.TrimSuffix(filepath.Base(HubPath), HubSuffix)`. Note the behaviour change: for a non-hub layout (an ordinary clone, or a `lyxtest` synthetic hub) `RepoName` now yields the parent directory's name rather than the prime worktree's; both are heuristics and the new one costs no subprocess. Rewrite every in-module reference to a removed field: `l.WorktreeRoot` becomes `l.WorktreePath()`, `l.Hub` becomes `l.HubPath`, `l.RelPath` becomes `l.AnchorRel`, and the `_lyx`-durable constructors (`PlanDir`, `LoomStatusFile`, `LoomStatusLock`, `DiscussionDir`, `LyxDir`, `DotLyxDir`) rebase from `l.Cwd`/`l.WorktreeRoot` onto `l.AnchorPath()`, while `WorktreeLogsDir`, `ScoutDaemonStateFile` and `ScoutDaemonLock` stay on `l.WorktreePath()` and `HubLogsDir` stays on `l.HubPath` — see the anchoring table in the overview's Shared Decisions, and do not collapse the three groups onto one base. Rewrite `fabricengine/hostlayout.go`'s card-4 inline construction and `clone.go`'s step-5 hook literal into `&lyxcwd.Location{...}` form. Cards 6 and 7 then land the gate and the marker rename against the renamed paths, and cards 8-17 sweep the consumers.
- **Commit:** `refactor(lyxcwd)!: rename hubgeometry to lyxcwd and reshape Layout into Location`

### Card 6: strict cwd gate, path comparator and `ResolveWithAnchor`

- **Context:**
  - `internal/configengine/config.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxcwd/anchor.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:**
  - `internal/lyxcwd/gate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extract two named helpers, both callable without spawning git so the tests stay untagged — the gate must be its own function rather than inline logic inside `Resolve`, which spawns `git rev-parse --show-toplevel` and could therefore only be exercised by a tagged test. `func samePath(a, b string) bool` normalizes each side through `filepath.EvalSymlinks` then `filepath.Clean`, falling back to `Clean`-only on whichever side `EvalSymlinks` fails for (the path may not exist yet), and compares byte-exact on Linux/macOS, `strings.EqualFold` on Windows. Normalization is not optional: the worktree side comes from `git rev-parse --show-toplevel` while cwd comes from `os.Getwd`, and the two disagree routinely — macOS `/tmp` is a symlink to `/private/tmp`, `lyxtest` fixtures live under symlinked temp dirs, and Windows/macOS filesystems are case-insensitive while Go string compare is not. `func checkCwdAnchorGate(cwd, anchorRel, worktreePath string) error` returns `nil` when `samePath(cwd, filepath.Join(worktreePath, anchorRel))` and otherwise wraps `ErrCwdOutsideAnchor` in a message naming both sides and the marker file. Replace the at-or-below gate at `hubgeometry.go:118-127` with a call to it, and hoist the call out of the `if anchor, found := readRecordedAnchor(hub); found` block so it runs for unanchored repos too. Today `filepath.Rel(anchorAbs, cwd)` returning `internal/foo` passes, so cwd may sit arbitrarily deeper than the anchor and lyx then dies further downstream at `configengine.FindBaseDir` with `not initialized: _lyx/ directory not found`; strict equality turns a confusing late failure into an immediate accurate one. With the `"."` fallback from card 5 this also means lyx is accepted only at the worktree root for an unanchored repo, never in a subdirectory — a user-visible behaviour change that must be called out in the commit message. Add `func ResolveWithAnchor(cwd, anchor string) (*Layout, error)`, resolving exactly as `Resolve` does but taking the anchor as a parameter and applying **no** gate; document it as a **bypass** — not a general-purpose resolver, and a caller reaching for it to escape a gate failure is misusing it, the correct fix being to stand in the anchored directory. It must stay ungated because both its callers stand somewhere the gate would reject: clone passes the freshly-cloned worktree root while the anchor may be a non-`"."` subpath, and `lyxtest` injects anchors into synthetic hubs. `ResolveWorktree` keeps applying no gate, unchanged. `gate_test.go` is untagged: a table over `(cwd, anchorRel, worktreePath)` triples asserting exact match resolves, a subdirectory errors, a parent errors and a sibling errors; a `samePath` table covering trailing separator, `.`/`..` segments, mixed separators, a symlinked path resolving to its target (temp dir) and a case-differing path that must match on Windows and must not on Linux; and one row per entry point asserting the same triple that makes the gate return `ErrCwdOutsideAnchor` is accepted without error by `ResolveWithAnchor`, so a later "consistency" change cannot quietly gate the bypass and break clone.
- **Commit:** `feat(hubgeometry): require cwd to equal the anchored directory exactly`

### Card 7: rename the anchor marker and simplify clone's step-5 resolve

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/hook.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/buildercli/weft_integration_test.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/lyxcwd/anchor.go`
  - `internal/hubgeometry/anchor_test.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/webstercli/weft_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two changes to the clone path, both small and both landing before the rename. First, rename the constant `FabricAnchorName` to `AnchorFileName` in `internal/lyxcwd/anchor.go` and its value from `.fabric-anchor` to `.lyx-anchor`, with **no** compatibility fallback read — the marker anchors the whole weft repo, not the fabric module, and `fabricengine/doc.go:110` already calls the value "the lyx-anchor subpath". In `internal/fabricengine/clone.go`, the step-8 marker path uses the new constant, and a leftover `.fabric-anchor` in `_board` with no `.lyx-anchor` beside it is a hard error naming re-clone as the remedy; without that detection the break would be silent, because an old clone would fall back to `AnchorRel = "."` and resolve `_lyx` to the wrong place for a subpath-anchored repo. Add that case to `clone_adopt_test.go`. Every listed test file writes or asserts the marker filename and takes the new constant. Second, at `clone.go:112` replace `hubgeometry.Resolve(hostWorktreePath)` with a direct struct construction from `hubPath` and `name`, already in scope at `:103`, because `InstallPostCheckoutHook` reads exactly one field (`hook.go:59`) — it needs a path, not a resolution. This is a **simplification, not a correctness fix**: step 3 aborts the clone if the hub already exists and step 4 creates it fresh, so at `:112` the hub is provably empty, `<hubPath>/_board` cannot exist, the marker read returns not-found, and the call succeeds today and would keep succeeding under the strict gate. Delete the now-unreachable non-fatal `else` branch at `:116-118`, which reads as a real failure path and is not one. `fabricengine/doc.go:110` names `.fabric-anchor`, `hubgeometry.BoardDir(Hub)` and `SiblingLayout` in one sentence: correct the marker name and drop the `SiblingLayout` clause, leaving the `BoardDir` wording for batch 3.
- **Commit:** `feat(fabric): rename the anchor marker to .lyx-anchor with a stale-marker guard`

### Card 8: production sweep — fabric

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/anchor.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/unwire.go`
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/hook.go`
  - `internal/fabricengine/hostclean.go`
  - `internal/fabricengine/hostlayout.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/list.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/weftwiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In every listed file, swap the import `github.com/Knatte18/loomyard/internal/hubgeometry` for `github.com/Knatte18/loomyard/internal/lyxcwd`, the qualifier `hubgeometry.` for `lyxcwd.`, and the type `*hubgeometry.Layout` for `*lyxcwd.Location`. Rewrite field reads: `.Cwd` becomes `.AnchorPath()`, `.WorktreeRoot` becomes `.WorktreePath()`, `.Hub` becomes `.HubPath`, `.RelPath` becomes `.AnchorRel`, `.Repo` becomes `.RepoName`. The `.Cwd` sites are the ones to read carefully: nearly all pass a base directory into `configengine.Load`, which stats `<base>/_lyx`, so `AnchorPath()` is the directory they actually want — under the strict gate from card 6 the two are provably equal, and where they were not equal before (an invocation from a subdirectory) the old value was wrong. Change no behaviour beyond the field-source swap. Correct any godoc comment in these files that names `hubgeometry` or `Layout` in the same pass. `fabricengine` is the heaviest consumer and the one where `.WorktreeRoot` dominates rather than `.Cwd`; those become `.WorktreePath()` with no semantic change at all. `fabriccli/weft_verbs.go:102` passes the raw unfiltered `cfg.Dirs()` into `ScopedPathspec(l.RelPath, …)` — that argument becomes `l.AnchorRel` and the raw/filtered asymmetry stays exactly as it is; it is load-bearing and batch 4 depends on it.
- **Commit:** `refactor(fabric): point fabricengine and fabriccli at lyxcwd.Location`

### Card 9: production sweep — webster

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/webstercli/beginbatch.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/recordbatch.go`
  - `internal/webstercli/recoverbatch.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/validate.go`
  - `internal/webstercli/weft.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/render.go`
  - `internal/websterengine/runlevel.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In every listed file, swap the import `github.com/Knatte18/loomyard/internal/hubgeometry` for `github.com/Knatte18/loomyard/internal/lyxcwd`, the qualifier `hubgeometry.` for `lyxcwd.`, and the type `*hubgeometry.Layout` for `*lyxcwd.Location`. Rewrite field reads: `.Cwd` becomes `.AnchorPath()`, `.WorktreeRoot` becomes `.WorktreePath()`, `.Hub` becomes `.HubPath`, `.RelPath` becomes `.AnchorRel`, `.Repo` becomes `.RepoName`. The `.Cwd` sites are the ones to read carefully: nearly all pass a base directory into `configengine.Load`, which stats `<base>/_lyx`, so `AnchorPath()` is the directory they actually want — under the strict gate from card 6 the two are provably equal, and where they were not equal before (an invocation from a subdirectory) the old value was wrong. Change no behaviour beyond the field-source swap. Correct any godoc comment in these files that names `hubgeometry` or `Layout` in the same pass. `websterengine/runlevel.go` and `webstercli/cli.go` hold the bulk: a `Layout` field on the deps struct plus ~15 `.Cwd`/`.WorktreeRoot` reads each. The deps-struct field keeps its name and changes type.
- **Commit:** `refactor(webster): point websterengine and webstercli at lyxcwd.Location`

### Card 10: production sweep — builder, burler, loom

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/buildercli/cli.go`
  - `internal/buildercli/poll.go`
  - `internal/buildercli/run.go`
  - `internal/buildercli/spawnbatch.go`
  - `internal/buildercli/validate.go`
  - `internal/buildercli/weft.go`
  - `internal/builderengine/spawn.go`
  - `internal/burlercli/cli.go`
  - `internal/burlerengine/config.go`
  - `internal/burlerengine/engine.go`
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/plan.go`
  - `internal/loomengine/preflight.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/planparser/parse.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In every listed file, swap the import `github.com/Knatte18/loomyard/internal/hubgeometry` for `github.com/Knatte18/loomyard/internal/lyxcwd`, the qualifier `hubgeometry.` for `lyxcwd.`, and the type `*hubgeometry.Layout` for `*lyxcwd.Location`. Rewrite field reads: `.Cwd` becomes `.AnchorPath()`, `.WorktreeRoot` becomes `.WorktreePath()`, `.Hub` becomes `.HubPath`, `.RelPath` becomes `.AnchorRel`, `.Repo` becomes `.RepoName`. The `.Cwd` sites are the ones to read carefully: nearly all pass a base directory into `configengine.Load`, which stats `<base>/_lyx`, so `AnchorPath()` is the directory they actually want — under the strict gate from card 6 the two are provably equal, and where they were not equal before (an invocation from a subdirectory) the old value was wrong. Change no behaviour beyond the field-source swap. Correct any godoc comment in these files that names `hubgeometry` or `Layout` in the same pass. `lyxtest/lyxtest.go` is production code for the literals guard's purposes and is swept here: its exported `PairedFixture.Layout` field keeps its name and changes type to `*lyxcwd.Location`, which is the seam every fixture-consuming test reads through in cards 13-17.
- **Commit:** `refactor(builder,burler,loom): point callers at lyxcwd.Location`

### Card 11: production sweep — config, board, ide and the leaf libraries

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/boardcli/cli.go`
  - `internal/configcli/configcli.go`
  - `internal/configcli/menu.go`
  - `internal/configengine/config.go`
  - `internal/configengine/edit.go`
  - `internal/configengine/set.go`
  - `internal/configsync/configsync.go`
  - `internal/envsource/envsource.go`
  - `internal/idecli/cli.go`
  - `internal/ideengine/menu.go`
  - `internal/ideengine/spawn.go`
  - `internal/logger/sink.go`
  - `internal/modelspec/load.go`
  - `internal/pattern/pattern.go`
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/vscode/color.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In every listed file, swap the import `github.com/Knatte18/loomyard/internal/hubgeometry` for `github.com/Knatte18/loomyard/internal/lyxcwd`, the qualifier `hubgeometry.` for `lyxcwd.`, and the type `*hubgeometry.Layout` for `*lyxcwd.Location`. Rewrite field reads: `.Cwd` becomes `.AnchorPath()`, `.WorktreeRoot` becomes `.WorktreePath()`, `.Hub` becomes `.HubPath`, `.RelPath` becomes `.AnchorRel`, `.Repo` becomes `.RepoName`. The `.Cwd` sites are the ones to read carefully: nearly all pass a base directory into `configengine.Load`, which stats `<base>/_lyx`, so `AnchorPath()` is the directory they actually want — under the strict gate from card 6 the two are provably equal, and where they were not equal before (an invocation from a subdirectory) the old value was wrong. Change no behaviour beyond the field-source swap. Correct any godoc comment in these files that names `hubgeometry` or `Layout` in the same pass. `tokenvocab/tokenvocab.go:25`'s `repo` token becomes `c.Layout.RepoName` — it is the single production consumer of that field, and the rendered token value changes only for a non-hub layout. `logger/sink.go:74,79` calls `Getwd` then `Resolve` to place its trace file; that call stays exactly where it is, because `fabricengine/coalesce.go:18` and `spawn.go:19` import `logger`, so moving resolution into `fabricengine` would produce `fabricengine → logger → fabricengine`. Keeping the module stdlib-plus-`gitexec` is what holds that cycle closed, and this card must not add an import that breaks it. Correct the godoc in `logger/logger.go:48,409`, `pattern/doc.go:13` and `planparser/parse.go` that names the old package or type.
- **Commit:** `refactor(config,board,ide): point callers at lyxcwd.Location`

### Card 12: production sweep — perch, scout, shuttle, reed

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/perchcli/cli.go`
  - `internal/perchcli/run.go`
  - `internal/perchengine/doc.go`
  - `internal/perchengine/engine.go`
  - `internal/reedcli/cli.go`
  - `internal/reedengine/lock.go`
  - `internal/scoutcli/cli.go`
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/load.go`
  - `internal/shuttlecli/cli.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/wait.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In every listed file, swap the import `github.com/Knatte18/loomyard/internal/hubgeometry` for `github.com/Knatte18/loomyard/internal/lyxcwd`, the qualifier `hubgeometry.` for `lyxcwd.`, and the type `*hubgeometry.Layout` for `*lyxcwd.Location`. Rewrite field reads: `.Cwd` becomes `.AnchorPath()`, `.WorktreeRoot` becomes `.WorktreePath()`, `.Hub` becomes `.HubPath`, `.RelPath` becomes `.AnchorRel`, `.Repo` becomes `.RepoName`. The `.Cwd` sites are the ones to read carefully: nearly all pass a base directory into `configengine.Load`, which stats `<base>/_lyx`, so `AnchorPath()` is the directory they actually want — under the strict gate from card 6 the two are provably equal, and where they were not equal before (an invocation from a subdirectory) the old value was wrong. Change no behaviour beyond the field-source swap. Correct any godoc comment in these files that names `hubgeometry` or `Layout` in the same pass. `scoutengine/ensureserver.go:300` builds a synthetic `&hubgeometry.Layout{WorktreeRoot: worktreeRoot}` purely to call `ScoutDaemonLock`; `Location` has no such field, so re-express it as `&lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}`. It disappears entirely in batch 2, when `ScoutDaemonLock` relocates into `scoutengine` as a plain-path function needing no `Location` at all. Correct the godoc in `scoutengine/doc.go:209,217`, `scoutengine/daemonstate.go:7`, `perchengine/doc.go` and `burlerengine/doc.go`.
- **Commit:** `refactor(perch,scout,shuttle,reed): point callers at lyxcwd.Location`

### Card 13: test sweep — fabric

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
  - `internal/fabricengine/add_branch_exists_test.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/branchname_test.go`
  - `internal/fabricengine/checkout_index_refresh_test.go`
  - `internal/fabricengine/checkout_rollback_test.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/config_test.go`
  - `internal/fabricengine/hook_test.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/junction_repoint_test.go`
  - `internal/fabricengine/junction_test.go`
  - `internal/fabricengine/junctionnames_test.go`
  - `internal/fabricengine/pull_integration_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/warpforward_integration_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/fabricengine/weftwiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same mechanical substitution as cards 8-12, applied to test files. Each synthetic `hubgeometry.Layout` struct literal becomes a `lyxcwd.Location` literal supplying `HubPath`/`WorktreeName`/`AnchorRel` in place of `Hub`/`WorktreeRoot`/`RelPath`, with `Cwd` dropped. A literal that set only `WorktreeRoot` becomes `HubPath: filepath.Dir(<old value>), WorktreeName: filepath.Base(<old value>)`. A literal that set `Cwd` to a value different from `WorktreeRoot` was exercising a subdirectory invocation; under the strict gate that case is now `ErrCwdOutsideAnchor`, so the test either sets `AnchorRel` to the intended subpath or asserts the error — decide per test from what it is actually checking, and never by loosening the gate. `fabricengine`'s 23 test files carry the largest share of synthetic literals and most set `WorktreeRoot` and `Hub` together, which map cleanly onto `HubPath`/`WorktreeName`. 19 of the 25 in-package test files reach unexported identifiers, so none may be converted to `package fabricengine_test` to dodge a problem — if a substitution seems to need that, it is the substitution that is wrong.
- **Commit:** `test(fabric): point fabricengine and fabriccli tests at lyxcwd.Location`

### Card 14: test sweep — webster

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/smoke_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/webstercli/weft_integration_test.go`
  - `internal/websterengine/audit_test.go`
  - `internal/websterengine/beginbatch_test.go`
  - `internal/websterengine/config_test.go`
  - `internal/websterengine/recordbatch_test.go`
  - `internal/websterengine/recoverbatch_test.go`
  - `internal/websterengine/runlevel_test.go`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same mechanical substitution as cards 8-12, applied to test files. Each synthetic `hubgeometry.Layout` struct literal becomes a `lyxcwd.Location` literal supplying `HubPath`/`WorktreeName`/`AnchorRel` in place of `Hub`/`WorktreeRoot`/`RelPath`, with `Cwd` dropped. A literal that set only `WorktreeRoot` becomes `HubPath: filepath.Dir(<old value>), WorktreeName: filepath.Base(<old value>)`. A literal that set `Cwd` to a value different from `WorktreeRoot` was exercising a subdirectory invocation; under the strict gate that case is now `ErrCwdOutsideAnchor`, so the test either sets `AnchorRel` to the intended subpath or asserts the error — decide per test from what it is actually checking, and never by loosening the gate.
- **Commit:** `test(webster): point websterengine and webstercli tests at lyxcwd.Location`

### Card 15: test sweep — builder, burler, loom, treadle

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/buildercli/pause_test.go`
  - `internal/buildercli/poll_test.go`
  - `internal/buildercli/run_test.go`
  - `internal/buildercli/smoke_test.go`
  - `internal/buildercli/spawnbatch_test.go`
  - `internal/buildercli/status_test.go`
  - `internal/buildercli/testdata_test.go`
  - `internal/buildercli/weft_integration_test.go`
  - `internal/buildercli/weft_test.go`
  - `internal/builderengine/spawn_test.go`
  - `internal/burlerengine/config_test.go`
  - `internal/burlerengine/engine_test.go`
  - `internal/burlerengine/smoke_cluster_test.go`
  - `internal/burlerengine/smoke_round_test.go`
  - `internal/loomengine/discussion_test.go`
  - `internal/loomengine/plan_test.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/lyxtest/lyxtest_test.go`
  - `internal/treadleengine/smoke_judge_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same mechanical substitution as cards 8-12, applied to test files. Each synthetic `hubgeometry.Layout` struct literal becomes a `lyxcwd.Location` literal supplying `HubPath`/`WorktreeName`/`AnchorRel` in place of `Hub`/`WorktreeRoot`/`RelPath`, with `Cwd` dropped. A literal that set only `WorktreeRoot` becomes `HubPath: filepath.Dir(<old value>), WorktreeName: filepath.Base(<old value>)`. A literal that set `Cwd` to a value different from `WorktreeRoot` was exercising a subdirectory invocation; under the strict gate that case is now `ErrCwdOutsideAnchor`, so the test either sets `AnchorRel` to the intended subpath or asserts the error — decide per test from what it is actually checking, and never by loosening the gate. The fixture-derived reads in `burlerengine/smoke_cluster_test.go:129,133`, `burlerengine/smoke_round_test.go:298,302` and `treadleengine/smoke_judge_test.go:256,260` are `fixture.Layout.Cwd` and become `fixture.Layout.AnchorPath()`. `treadleengine` is swept here for that one file only; its seam invariant still forbids a direct `internal/lyxcwd` import in production code, and this is a test file, so the allowlist is unaffected.
- **Commit:** `test(builder,burler,loom): point tests at lyxcwd.Location`

### Card 16: test sweep — perch, scout, shuttle, reed

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/perchcli/cli_integration_test.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/perchengine/config_test.go`
  - `internal/perchengine/run_test.go`
  - `internal/reedcli/cli_integration_test.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedengine/config_test.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/reedengine/header_test.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/mouse_boot_integration_test.go`
  - `internal/scoutengine/ensureserver_integration_test.go`
  - `internal/scoutengine/ensureserver_test.go`
  - `internal/scoutengine/leaf_enforcement_test.go`
  - `internal/scoutengine/load_test.go`
  - `internal/scoutengine/refs_integration_test.go`
  - `internal/scoutengine/supervised_integration_test.go`
  - `internal/scoutengine/supervised_scout_test.go`
  - `internal/scoutengine/supervised_test.go`
  - `internal/shuttlecli/cli_test.go`
  - `internal/shuttlecli/smoke_interrupt_test.go`
  - `internal/shuttleengine/config_test.go`
  - `internal/shuttleengine/run_inject_test.go`
  - `internal/shuttleengine/run_test.go`
  - `internal/shuttleengine/rundir_test.go`
  - `internal/shuttleengine/wait_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same mechanical substitution as cards 8-12, applied to test files. Each synthetic `hubgeometry.Layout` struct literal becomes a `lyxcwd.Location` literal supplying `HubPath`/`WorktreeName`/`AnchorRel` in place of `Hub`/`WorktreeRoot`/`RelPath`, with `Cwd` dropped. A literal that set only `WorktreeRoot` becomes `HubPath: filepath.Dir(<old value>), WorktreeName: filepath.Base(<old value>)`. A literal that set `Cwd` to a value different from `WorktreeRoot` was exercising a subdirectory invocation; under the strict gate that case is now `ErrCwdOutsideAnchor`, so the test either sets `AnchorRel` to the intended subpath or asserts the error — decide per test from what it is actually checking, and never by loosening the gate.
- **Commit:** `test(perch,scout,shuttle,reed): point tests at lyxcwd.Location`

### Card 17: test sweep — config, board, ide and the leaf libraries

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `cmd/lyx/exitcode_test.go`
  - `cmd/lyx/main_integration_test.go`
  - `internal/boardcli/cli_test.go`
  - `internal/boardcli/notes_test.go`
  - `internal/boardengine/boardtest/bench_test.go`
  - `internal/boardengine/config_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/configcli/configcli_test.go`
  - `internal/configcli/reconcile_integration_test.go`
  - `internal/configengine/config_test.go`
  - `internal/configengine/edit_test.go`
  - `internal/configengine/set_test.go`
  - `internal/configsync/configsync_test.go`
  - `internal/ideengine/menu_test.go`
  - `internal/ideengine/spawn_test.go`
  - `internal/modelspec/leaf_enforcement_test.go`
  - `internal/modelspec/load_test.go`
  - `internal/modelspec/template_test.go`
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/pattern/pattern_test.go`
  - `internal/tokenvocab/leaf_enforcement_test.go`
  - `internal/tokenvocab/tokenvocab_test.go`
  - `internal/vscode/color_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same mechanical substitution as cards 8-12, applied to test files. Each synthetic `hubgeometry.Layout` struct literal becomes a `lyxcwd.Location` literal supplying `HubPath`/`WorktreeName`/`AnchorRel` in place of `Hub`/`WorktreeRoot`/`RelPath`, with `Cwd` dropped. A literal that set only `WorktreeRoot` becomes `HubPath: filepath.Dir(<old value>), WorktreeName: filepath.Base(<old value>)`. A literal that set `Cwd` to a value different from `WorktreeRoot` was exercising a subdirectory invocation; under the strict gate that case is now `ErrCwdOutsideAnchor`, so the test either sets `AnchorRel` to the intended subpath or asserts the error — decide per test from what it is actually checking, and never by loosening the gate. `cmd/lyx/main_integration_test.go` and `exitcode_test.go` exercise the CLI end to end from a fixture; a subdirectory invocation there is now `ErrCwdOutsideAnchor` and the assertion must reflect that rather than be relaxed.
- **Commit:** `test(config,board,ide): point tests at lyxcwd.Location`

### Card 18: update the leaf and seam invariants

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `docs/overview.md`
  - `manifest/designs/fabric-unified-view.md`
- **Edits:**
  - `CONSTRAINTS.md`
  - `internal/modelspec/leaf_enforcement_test.go`
  - `internal/scoutengine/leaf_enforcement_test.go`
  - `internal/tokenvocab/leaf_enforcement_test.go`
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/lyxtest/leaf_enforcement_test.go`
  - `internal/treadleengine/seam_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename `internal/hubgeometry` to `internal/lyxcwd` in the allowlist maps of `modelspec`, `scoutengine`, `tokenvocab` and `pattern`, and in every doc comment naming the package across all six enforcement files. A stale package name in a leaf allowlist silently stops enforcing, and an over-wide allowlist stops enforcing just as quietly, so neither is cosmetic. `modelspec` and `scoutengine` also carry `internal/configengine` from card 2 — verify, do not duplicate. `tokenvocab` and `pattern` are **not** widened: `tokenvocab` holds only a `*Location` and reads `RepoName`, `pattern` keeps only worktree-side constructors, so `internal/lyxcwd` alone remains each one's correct non-stdlib entry. **Correction to the discussion's `leaf-invariant-updates` decision**, which called for widening `lyxtest`'s allowlist: `lyxtest/leaf_enforcement_test.go` enforces a **banned-imports list**, not an allowlist, and `weftname`/`configengine`/`lyxcwd` are not on it — so the test needs only its doc-comment wording updated, and adding those imports requires no code change there. In `CONSTRAINTS.md`, retitle the **Hub Geometry Invariant** to the **Cwd Resolution Invariant** and rewrite it to the narrow post-shrink contract: `internal/lyxcwd` owns cwd resolution and nothing else; `Resolve` exposes only `RepoName`/`HubPath`/`WorktreeName`/`AnchorRel` and the two derived accessors, never a weft path, a junction path or any per-module subdirectory; cwd must equal `AnchorPath()` exactly, with `ErrCwdOutsideAnchor` otherwise; `ResolveWithAnchor` and `ResolveWorktree` are ungated and the first is a documented bypass; the module's imports are capped at stdlib plus `internal/gitexec`, which is what keeps `fabricengine` → `logger` → `lyxcwd` acyclic; and a module's own durable subdirectory is that module's private relative-path constant joined onto `AnchorPath()` — replacing the current line-12 wording that says `cwd`, which must land in step with `manifest/designs/fabric-unified-view.md` in batch 5 so the two never disagree. Update the `internal/hubgeometry` name in the other five invariants. Leave the enforcement pointer at line 18 naming `enforcement_test.go`; card 19 moves the file path.
- **Commit:** `docs(constraints): retitle Hub Geometry to the Cwd Resolution Invariant`

### Card 19: batch-1 slice of the enforcement-guard rewrite

- **Context:**
  - `internal/weftname/weftname.go`
  - `internal/configengine/config.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Switch both allowlisted directory literals — `enforcement_test.go:138` (`TestEnforcement`'s `os.Getwd`/`--show-toplevel` ban) and `:420` (`TestEnforcement_GeometryLiterals`) — from `internal/hubgeometry` to `internal/lyxcwd`, along with every doc comment and failure message naming the old directory. The package rename alone would fail this test from card 5 onward, which is why the guard is rewritten incrementally rather than once in batch 5. Then replace the single-package allowlist in `TestEnforcement_GeometryLiterals` with a per-token ownership map keyed by token value, seeded with exactly the rows this batch earns: `-weft` owned by `internal/weftname`, and `_lyx` owned by `internal/configengine` **and**, transitionally, `internal/lyxcwd` for the private `lyxDirName` const card 2 left behind. Every other token (`_board`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_pattern`) keeps `internal/lyxcwd` as its owner for now; batch 3 moves those rows and batch 5 removes the transitional `_lyx` co-owner. A per-token map is strictly stronger than a blanket allowlist — it encodes *who* owns each token rather than "one package owns all of them", and it is what proves each batch moved ownership rather than copying code. Keep the existing `predicate` sub-test shape (synthetic positive/negative Go snippets parsed with `go/parser`, whole-token matching by exact equality after `strconv.Unquote`, so `_boardroom` and `-weft-bare` stay negatives) and keep the `scanned_non_empty` sanity sub-test — a misconfigured walk must not produce a vacuous pass. `.lyx` stays unpoliced this slice, as it is today; slice 9 is where it gets an owner, and adding it now would have to be undone one slice later.
- **Commit:** `test(lyxcwd): stage the geometry guard onto a per-token ownership map`

## Batch Tests

`verify` runs the repo-wide tagged type-check (`go vet -tags "integration smoke scout" ./...`, ~7 s) followed by the full untagged suite (`go test ./...`). Both halves are required and the scope is deliberately unbounded for this batch alone: the package rename touches every importer in the tree, so no narrower scope is meaningful. The tagged `go vet` is not redundant with `go test` — `go test` without tags does not compile `integration`/`smoke`/`scout`-tagged test files at all, and this batch edits roughly 40 of them, so a broken tagged file would otherwise pass unnoticed until a later tagged run.

New coverage lands in `internal/lyxcwd/gate_test.go` (untagged: the strict-gate table, the `samePath` normalization table including a symlinked temp dir, and the per-entry-point rows pinning `ResolveWithAnchor`/`ResolveWorktree` as ungated) and `internal/weftname/weftname_test.go` (untagged: `SiblingPath`/`BareSiblingPath` round-trips plus the lyxtest-vs-fabricengine agreement assertion). The stale-`.fabric-anchor` detection is asserted in `internal/fabricengine/clone_adopt_test.go`, which is integration-tagged because it needs a real clone. Existing test files in the renamed module that cover departing symbols move with those symbols in batches 2 and 3 rather than being deleted — losing that coverage would be the silent cost of this refactor.
