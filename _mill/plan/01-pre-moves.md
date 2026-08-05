# Batch: pre-moves

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
batch: pre-moves
number: 1
cards: 4
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

This batch does everything that can leave the tree fully green **before** the package rename: it extracts `internal/weftname`, moves `LyxDirName`/`ConfigDir`/`ConfigFile` to `internal/configengine` and `DotEnv` to `internal/envsource`, moves `worktreelist.go`/`Prime`/`PrimeName`/`WeftRepoRoot` into `internal/fabricengine`, and removes the two `Layout` methods the coming reshape displaces.

It exists as its own batch for a reason the discussion's five-batch shape did not have to weigh: batch 1 as originally drawn was 19 cards and roughly 200 file-edits in one implementer session, which is a turn-exhaustion risk rather than a context one. These four cards are independent of the rename, each leaves `go test ./...` green on its own, and pulling them out shrinks the atomic rename batch to what genuinely cannot be split.

Two of the four moves are **forced early**, not merely convenient. `(*Layout).WorktreePath(slug)` must go now because its name collides with the no-arg `WorktreePath()` accessor batch 2 introduces on the same type — a duplicate-method compile error. `worktreelist.go`/`Prime`/`PrimeName`/`WeftRepoRoot` must go now because batch 2 drops `Prime` from the struct, so from that point on every consumer needs a replacement source.

External interface batch 2 consumes: `configengine.LyxDirName`, `configengine.ConfigDir`/`ConfigFile`, `envsource.DotEnv`, `weftname.Suffix`/`SiblingPath`, `fabricengine.List`/`PrimeName`/`WeftRepoRoot`/`WorktreePath`.

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
- **Requirements:** Create package `weftname`, stdlib-only (`path/filepath` and `strings` only). Declare `const Suffix = "-weft"` as the single declarer of that token; `func SiblingPath(container, base string) string` returning `filepath.Join(container, base+Suffix)`; `func BareSiblingPath(container, base string) string` returning `filepath.Join(container, base+Suffix+"-bare")`. Delete `WeftSuffix` and `WeftSiblingPath` from `internal/hubgeometry/hubgeometry.go`; the in-module callers that remain (`WeftHostSlug`, `WeftRepoRoot`, `WeftWorktreePath`, `WeftWorktree`) call `weftname.Suffix`/`weftname.SiblingPath` instead, so `internal/hubgeometry` gains a temporary import of `internal/weftname` that disappears in batch 6 when those methods relocate. Retarget `fabricengine/branchname.go` and `add.go` to `weftname.Suffix`, and `clone.go` to `weftname.SiblingPath`. Retarget `fabriccli/fabric.go`'s three cobra `Long` help-text uses at lines 45, 67 and 78 to `weftname.Suffix` — user-facing prose, not geometry, and the rendered string must not change. Retarget `websterengine/audit.go:89`'s `regexp.QuoteMeta(hubgeometry.WeftSuffix)` to `weftname.Suffix`; its comment at `:85` insisting the value come from the constant "never from string literals" stays true and is updated to name the new source. In `lyxtest/lyxtest.go`, retarget the three `hubgeometry.WeftSiblingPath` calls to `weftname.SiblingPath` and replace the two hardcoded `base+"-weft-bare"` literals at lines 194 and 458 with `weftname.BareSiblingPath(tmpDir, base)` / `weftname.BareSiblingPath(tempContainer, base)`. `lyxtest` does not consume weft geometry — it *produces* the on-disk shape production expects, and the fixture is valid only if its directory names match what production derives, which is why one shared leaf rather than two constants. The new test covers `SiblingPath`/`BareSiblingPath` over container/base combinations plus an assertion that the name `lyxtest` builds and the name `fabricengine` derives agree for the same input; untagged, pure string math.
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
- **Requirements:** Declare `LyxDirName = "_lyx"`, `configDirName = "config"`, `func ConfigDir(baseDir string) string` and `func ConfigFile(baseDir, module string) string` in `internal/configengine/config.go`, with the bodies they have at `hubgeometry.go:24-31,187-195`. `configengine` becomes the single declarer of `_lyx`; the direction is forced, not chosen — `configengine/config.go:23` already consumes the constant and `fabricengine/config.go:16` already imports `configengine`, so declaring `_lyx` in `fabricengine` would be a compile-time cycle. Declare `dotEnvName = ".env"` and `func DotEnv(baseDir string) string` in `internal/envsource/envsource.go`; `envsource.Build` calls its own `DotEnv` and drops the `internal/hubgeometry` import entirely, becoming a stdlib-only leaf. `DotEnv` goes here and **not** to `configengine` because `configengine/config.go:15,66` already imports `envsource`, so the obvious move is a cycle — and `envsource` already owns the `.env` format end to end. Delete `LyxDirName`, `configDirName`, `dotEnvName`, `ConfigDir`, `ConfigFile` and `DotEnv` from `internal/hubgeometry/hubgeometry.go`. Its own remaining `_lyx` users (`PerchRunsDir`, `PlanDir`, `PlanDirRel`, `BuilderDir`, `WebsterDir`, `LyxDir`, `LoomStatusFile`, `LoomStatusLock`, `DiscussionDir`, `PortalTarget`, `HostLyxLink`, `HostLyxLinkHere`, `WeftLyxDir`, `WeftLyxDirFor`) keep working via a **private** `lyxDirName = "_lyx"` const declared in `hubgeometry.go` — a transitional second declarer, registered as such in card 19's guard slice and removed in batch 8 once every one of those functions has left. Every other listed file swaps the qualifier and fixes its import block. Verify `go list -deps` shows no `configengine` → `hubgeometry` edge afterwards. `modelspec/leaf_enforcement_test.go` and `scoutengine/leaf_enforcement_test.go` add `github.com/Knatte18/loomyard/internal/configengine` to their allowlist maps, or their leaf tests fail the moment this card lands. `modelspec/modelspec.go:27` and `scoutengine/doc.go:112` are godoc references naming `hubgeometry.ConfigFile`; correct them.
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
- **Requirements:** `List`, `WorktreeEntry` and `parseWorktreePorcelain` become `fabricengine` declarations; `internal/fabricengine/list.go` drops its `WorktreeEntry` type alias and its `hubgeometry.List` delegation, keeping `(*Topology).List` as a one-line call to the now-local `List`. Delete the `Prime` and `Repo` fields, the `PrimeName()` and `WeftRepoRoot()` methods and the `deriveRepo` function from `internal/hubgeometry/hubgeometry.go`, and delete the `List(cwd)` call and prime-scan loop from `resolveCore` (`hubgeometry.go:130-143`) — that subprocess ran on **every** `Resolve` purely to find the `Main` entry, even though the result is identical for every worktree under one hub. `deriveRepo` is deleted, not moved: card 7 redefines the repo name as `TrimSuffix(Base(HubPath), "-HUB")`, which leaves it with no caller on either side. Add to `fabricengine`: `func PrimeName(l *hubgeometry.Layout) (string, error)` (resolves via the local `List`, returns `filepath.Base` of the `Main` entry) and `func WeftRepoRoot(l *hubgeometry.Layout) (string, error)` (`weftname.SiblingPath(l.Hub, primeName)`); retarget all 13 in-package `l.WeftRepoRoot()` call sites (`add.go:108,149`, `cleanup.go:176,208`, `weftwiring.go:36,55,69,126`, `reconcile.go:172,204,227`, `prune.go:137,152`) plus their test callers. `WeftRepoRoot` is pulled forward from batch 6 for exactly this reason: threading a `primeName` argument through 13 in-package call sites now and relocating all 13 again later is strictly more work. `MenuLauncherRel()` takes a `primeName string` argument instead of reading `l.Prime`, and stays in the module until batch 6. Change `vscode.PickColor` to `func PickColor(l *hubgeometry.Layout, primeName string) string`, using `primeName` where it used `filepath.Base(l.Prime)` and skipping the prime-skip step when it is empty — sourcing the name from `fabricengine` inside `vscode` would pull the entire fabric engine into a colour picker and reintroduce a `git worktree list` subprocess per call. `internal/ideengine` gains the `internal/fabricengine` import (it has none today — `spawn.go:6-11` imports only `filepath`, `hubgeometry` and `vscode`): `ideengine/spawn.go` sources the prime name via `fabricengine.PrimeName(l)` and passes it to `PickColor`, degrading to the empty string on error rather than failing the spawn. `loomengine/preflight.go:67`'s `l.Prime == ""` check becomes a `fabricengine.PrimeName(l)` call producing the same `Report`; `loomengine` already imports `fabricengine`. `ideengine/menu.go:38`'s `hubgeometry.List(l.Cwd)` becomes `fabricengine.List(l.Cwd)`. Leave `tokenvocab`'s `c.Layout.Repo` expression alone — card 5 renames the field.
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
- **Requirements:** Two deletions from `internal/hubgeometry/hubgeometry.go`, both forced by the new struct. First, delete `(*Layout).WorktreePath(slug string)` (`:410-413`) — its name collides with the no-arg `WorktreePath()` accessor card 5 introduces on the same type, which is a duplicate-method compile error, so it cannot wait for its scheduled batch-3 move. Add `func WorktreePath(l *hubgeometry.Layout, slug string) string` to `internal/fabricengine/junction.go` with the identical body (`filepath.Join(l.Hub, slug)`) and retarget the `fabricengine` and `ideengine/spawn.go:20` production callers plus every listed test caller. The four **in-module** callers that outlive the method and cannot import `fabricengine` — `LauncherSpawnRel` (`:459`), `HostLyxLink` (`:524`), `HostPatternLink` (`:537`) and `HostJunctions` (`:570`) — each get the method's entire body inlined as `filepath.Join(l.Hub, slug)`, carried until batch 6 relocates them. Second, delete `(*Layout).SiblingLayout` (`:164-185`) and its test file: it exists to recompute `Hub`, `RelPath`, `Prime` and `Repo` for a hub sibling, and under the new struct a hub sibling differs in exactly one field, so there is nothing left to compute. It is also the last constructor setting a dishonest `Cwd` (`:179`). `fabricengine/hostlayout.go`'s `hostLayoutFor` constructs the sibling inline instead — same `Hub`, same `RelPath`, `WorktreeRoot`/`Cwd` set to `filepath.Clean(worktreeRoot)`, reading the anchor the same way; card 5 rewrites that literal into `&lyxcwd.Location{HubPath: l.HubPath, WorktreeName: filepath.Base(worktreeRoot), AnchorRel: l.AnchorRel}`. The non-sibling `ResolveWorktree` fallback at `hostlayout.go:22` is unchanged, and its guard already rejects the non-sibling case before the inline construction is reached.
- **Commit:** `refactor(hubgeometry): drop WorktreePath(slug) and SiblingLayout`


## Batch Tests

`verify` runs the repo-wide tagged type-check (`go vet -tags "integration smoke scout" ./...`, ~7 s) followed by the full untagged suite. Both halves are required: `go test` without tags does not compile `integration`/`smoke`/`scout`-tagged test files at all, and this batch edits roughly 25 of them, so a broken tagged file would otherwise pass unnoticed until a later tagged run.

The unbounded `go test ./...` is justified here because the config-path move alone touches 34 packages' test files; no narrower scope is meaningful. New coverage lands in `internal/weftname/weftname_test.go` — untagged `SiblingPath`/`BareSiblingPath` round-trips plus the assertion that the name `lyxtest` builds for a fixture and the name `fabricengine` derives in production agree for the same input, which is the drift this leaf exists to prevent.
