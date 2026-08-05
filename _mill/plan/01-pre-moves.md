# Batch: pre-moves

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
batch: pre-moves
number: 1
cards: 4
verify: go vet -tags "integration smoke scout" ./... && go test ./... && go test -tags integration ./...
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
  - `internal/hubgeometry/enforcement_test.go`
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
- **Requirements:** Create package `weftname`, stdlib-only (`path/filepath` and `strings` only). Declare `const Suffix = "-weft"` as the single declarer of that token; `func SiblingPath(container, base string) string` returning `filepath.Join(container, base+Suffix)`; `func BareSiblingPath(container, base string) string` returning `filepath.Join(container, base+Suffix+"-bare")`. Delete `WeftSuffix` and `WeftSiblingPath` from `internal/hubgeometry/hubgeometry.go`; the in-module callers that remain (`WeftHostSlug`, `WeftRepoRoot`, `WeftWorktreePath`, `WeftWorktree`) call `weftname.Suffix`/`weftname.SiblingPath` instead, so `internal/hubgeometry` gains a temporary import of `internal/weftname` that disappears in batch 6 when those methods relocate. Retarget `fabricengine/branchname.go` and `add.go` to `weftname.Suffix`, and `clone.go` to `weftname.SiblingPath`. Retarget `fabriccli/fabric.go`'s three cobra `Long` help-text uses at lines 45, 67 and 78 to `weftname.Suffix` — user-facing prose, not geometry, and the rendered string must not change. Retarget `websterengine/audit.go:89`'s `regexp.QuoteMeta(hubgeometry.WeftSuffix)` to `weftname.Suffix`; its comment at `:85` insisting the value come from the constant "never from string literals" stays true and is updated to name the new source. In `lyxtest/lyxtest.go`, retarget the three `hubgeometry.WeftSiblingPath` calls to `weftname.SiblingPath` and replace the two hardcoded `base+"-weft-bare"` literals at lines 194 and 458 with `weftname.BareSiblingPath(tmpDir, base)` / `weftname.BareSiblingPath(tempContainer, base)`. `lyxtest` does not consume weft geometry — it *produces* the on-disk shape production expects, and the fixture is valid only if its directory names match what production derives, which is why one shared leaf rather than two constants. The new test covers `SiblingPath`/`BareSiblingPath` over container/base combinations plus an assertion that the name `lyxtest` builds and the name `fabricengine` derives agree for the same input; untagged, pure string math. The module's own `geometry_test.go` (external `hubgeometry_test` package) carries ~17 `hubgeometry.WeftSiblingPath`/`hubgeometry.WeftSuffix` references — the slug-parsing table and `TestWeftLayoutMethodParity`'s want-expressions; retarget every one onto `weftname.SiblingPath`/`weftname.Suffix` (an external-package test imports `weftname` with no cycle), which is why the file is in this card's `Edits:`. **This card also registers its own ownership row in the guard, and cannot be green without it.** `TestEnforcement_GeometryLiterals` (`internal/hubgeometry/enforcement_test.go`) flags a production string const declaration of any geometry token — `-weft` among them, context (c) at `:281-302` — anywhere outside the single allowlisted directory at `:420`, and `const Suffix = "-weft"` in the new package is exactly that. So this card performs the allowlist-to-map conversion the guard rewrite needs: replace the single-directory check with a per-token ownership map keyed by token value, giving `-weft` the owner `internal/weftname` and every other token (`_board`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_lyx`, `_pattern`) the owner `internal/hubgeometry`, which reproduces today's behaviour for all of them. Note the path is the pre-rename one: this batch runs before batch 2's `git mv`. Keep the existing `predicate` sub-test shape and the `scanned_non_empty` sanity sub-test unchanged.
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
  - `internal/hubgeometry/enforcement_test.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_unit_test.go`
  - `internal/envsource/envsource_test.go`
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
- **Requirements:** Declare `LyxDirName = "_lyx"`, `configDirName = "config"`, `func ConfigDir(baseDir string) string` and `func ConfigFile(baseDir, module string) string` in `internal/configengine/config.go`, with the bodies they have at `hubgeometry.go:24-31,187-195`. `configengine` becomes the single declarer of `_lyx`; the direction is forced, not chosen — `configengine/config.go:23` already consumes the constant and `fabricengine/config.go:16` already imports `configengine`, so declaring `_lyx` in `fabricengine` would be a compile-time cycle. Declare `dotEnvName = ".env"` and `func DotEnv(baseDir string) string` in `internal/envsource/envsource.go`; `envsource.Build` calls its own `DotEnv` and drops the `internal/hubgeometry` import entirely, becoming a stdlib-only leaf. `DotEnv` goes here and **not** to `configengine` because `configengine/config.go:15,66` already imports `envsource`, so the obvious move is a cycle — and `envsource` already owns the `.env` format end to end. Delete `LyxDirName`, `configDirName`, `dotEnvName`, `ConfigDir`, `ConfigFile` and `DotEnv` from `internal/hubgeometry/hubgeometry.go`. Its own remaining `_lyx` users (`PerchRunsDir`, `PlanDir`, `PlanDirRel`, `BuilderDir`, `WebsterDir`, `LyxDir`, `LoomStatusFile`, `LoomStatusLock`, `DiscussionDir`, `PortalTarget`, `HostLyxLink`, `HostLyxLinkHere`, `WeftLyxDir`, `WeftLyxDirFor`) keep working via a **private** `lyxDirName = "_lyx"` const declared in `hubgeometry.go` — a transitional second declarer, registered as such in this card's own guard row and removed in batch 8 once every one of those functions has left. **This card also updates the guard, and cannot be green without it**, for the same reason card 1 could not: `configengine.LyxDirName = "_lyx"` is a production const declaration of a policed token outside the owning directory. In the ownership map card 1 introduced, change `_lyx`'s owner from `internal/hubgeometry` to `internal/configengine` **and**, transitionally, `internal/hubgeometry` for the private `lyxDirName` const this card leaves behind — a two-owner row, which is precisely why the map has to hold a set of directories per token rather than a single string. Leave every other token's row untouched. Every other listed file swaps the qualifier and fixes its import block. Verify `go list -deps` shows no `configengine` → `hubgeometry` edge afterwards. `modelspec/leaf_enforcement_test.go` and `scoutengine/leaf_enforcement_test.go` add `github.com/Knatte18/loomyard/internal/configengine` to their allowlist maps, or their leaf tests fail the moment this card lands. `modelspec/modelspec.go:27` and `scoutengine/doc.go:112` are godoc references naming `hubgeometry.ConfigFile`; correct them. `modelspec/modelspec.go:34`'s import-ceiling enumeration ("standard library (including embed), internal/hubgeometry, and gopkg.in/yaml.v3") also goes stale in this card: after the move `modelspec`'s only geometry-adjacent import is `internal/configengine`, so rewrite that enumeration to name `internal/configengine` in place of `internal/hubgeometry` — this is the card that changes the import set, so the doc moves with it. For the same reason `modelspec`'s allowlist map **drops** its `internal/hubgeometry` entry in this card, not just gains `configengine`: `load.go:23`'s `ConfigFile` call was modelspec's only `hubgeometry` use, so after this card the entry would allowlist an import the package no longer has, and an over-wide allowlist stops enforcing as quietly as a stale one — correct `leaf_enforcement_test.go:3`'s doc comment in the same pass. (`scoutengine` keeps its `hubgeometry` entry here: `ensureserver.go:300` still builds a synthetic `Layout` until batch 5 card 24.)

  The module's own in-package unit test must be cut in this card, or batch 1's own `go test ./...` stops compiling package `hubgeometry`: `hubgeometry_unit_test.go` calls every deleted symbol unqualified — `ConfigDir` (`:23`), `ConfigFile` (`:36`), `DotEnv` (`:96`), and the exported `LyxDirName` both in `TestLyxDirNameConstant` (`:109`) and in the want-expressions of the `PerchRunsDir`/`PlanDir`/`BuilderDir`/`BuilderReportsDir` sub-tests (`:49,61,73,85`) that survive until batch 5. The coverage decision is per sub-test, checked against the destination packages' existing tests: `configengine/config_test.go` exercises the config path only indirectly through `Load` (no direct path-math assertion anywhere), so the `ConfigDir` (`:19-29`) and `ConfigFile` (`:31-42`) sub-tests and `TestLyxDirNameConstant` (`:106-112`) **move** into `configengine/config_test.go`, rewritten onto `configengine.ConfigDir`/`ConfigFile`/`LyxDirName` — not deleted, because nothing else pins the `<base>/_lyx/config/<module>.yaml` shape or the `"_lyx"` value at its new single declarer. Likewise `envsource/envsource_test.go` covers `.env` parsing and OS overlay via locally-joined paths but never pins `DotEnv(baseDir) == filepath.Join(baseDir, ".env")`, so the `DotEnv` sub-test (`:92-102`) **moves** there, onto the new exported `envsource.DotEnv` — which is why both destination test files are in this card's `Edits:`. The four surviving sub-tests' want-expressions retarget from the deleted exported `LyxDirName` onto the **private** transitional `lyxDirName` this card declares — the file is in-package, so no import is needed and no cycle question arises; cards 20, 21 and 23 delete those sub-tests with their subjects.
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
  - `internal/fabricengine/launchers.go`
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
  - `internal/hubgeometry/pattern_test.go`
  - `internal/hubgeometry/weft_test.go`
  - `internal/idecli/cli.go`
  - `internal/ideengine/menu.go`
  - `internal/ideengine/menu_test.go`
  - `internal/ideengine/spawn.go`
  - `internal/ideengine/spawn_test.go`
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/reedengine/mouse_boot_integration_test.go`
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/tokenvocab/tokenvocab_test.go`
  - `internal/vscode/color.go`
  - `internal/vscode/color_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/hubgeometry/worktreelist.go` -> `internal/fabricengine/worktreelist.go`
  - `internal/hubgeometry/worktreelist_test.go` -> `internal/fabricengine/worktreelist_test.go`
- **Requirements:** `List`, `WorktreeEntry` and `parseWorktreePorcelain` become `fabricengine` declarations; `internal/fabricengine/list.go` drops its `WorktreeEntry` type alias and its `hubgeometry.List` delegation, keeping `(*Topology).List` as a one-line call to the now-local `List`. Delete the `Prime` field, the `PrimeName()` and `WeftRepoRoot()` methods and the `deriveRepo` function from `internal/hubgeometry/hubgeometry.go`. **Keep the `Repo` field**, and switch its derivation to its final form in this card: `resolveCore` sets `Repo` to `strings.TrimSuffix(filepath.Base(hub), HubSuffix)`. Pulling the `reponame-derivation` change forward to here is what lets `Prime` leave without breaking the batch's own green-tree gate — `tokenvocab/tokenvocab.go:25` and six struct literals in `tokenvocab_test.go` read `.Repo` directly, and both files are in this card's `Edits:` list, so deleting the field here would stop the tree compiling. Behaviour note for the commit message: for a non-hub layout (an ordinary clone, or a `lyxtest` synthetic hub) `Repo` now yields the parent directory's name rather than the prime worktree's; both are heuristics and the new one costs no subprocess. Also delete the `List(cwd)` call and prime-scan loop from `resolveCore` (`hubgeometry.go:130-143`) — that subprocess ran on **every** `Resolve` purely to find the `Main` entry, even though the result is identical for every worktree under one hub. `deriveRepo` is deleted, not moved: its `filepath.Base(prime)` branch has no source once `Prime` is gone, and its `worktreeRoot` branch is superseded by the `-HUB`-trimming derivation above, so it has no caller on either side. Add to `fabricengine`: `func PrimeName(l *hubgeometry.Layout) (string, error)` (resolves via the local `List`, returns `filepath.Base` of the `Main` entry) and `func WeftRepoRoot(l *hubgeometry.Layout) (string, error)` (`weftname.SiblingPath(l.Hub, primeName)`); retarget all 13 in-package `l.WeftRepoRoot()` call sites (`add.go:108,149`, `cleanup.go:176,208`, `weftwiring.go:36,55,69,126`, `reconcile.go:172,204,227`, `prune.go:137,152`) plus their test callers. `WeftRepoRoot` is pulled forward from batch 6 for exactly this reason: threading a `primeName` argument through 13 in-package call sites now and relocating all 13 again later is strictly more work. `MenuLauncherRel()` takes a `primeName string` argument instead of reading `l.Prime`, and stays in the module until batch 6. Its only production caller is `fabricengine/writeLaunchers` (`launchers.go:63`): that call becomes `l.MenuLauncherRel(primeName)` with the name sourced via the new in-package `PrimeName(l)`, propagating the error — `writeLaunchers` already returns one, and a launcher pointing at an unresolved prime is worse than a failed wire. The in-module test callers change with it: `hubgeometry_test.go`'s two `MenuLauncherRel` sub-tests (`:469`, `:494`) pass an explicit prime name derived from the fixture's main-worktree basename — the value they formerly read off `layout.Prime` — and their `targetPath` recomputation (`:473`, `:498`) uses the same name. Change `vscode.PickColor` to `func PickColor(l *hubgeometry.Layout, primeName string) string`, using `primeName` where it used `filepath.Base(l.Prime)` and skipping the prime-skip step when it is empty — sourcing the name from `fabricengine` inside `vscode` would pull the entire fabric engine into a colour picker and reintroduce a `git worktree list` subprocess per call. `internal/ideengine` gains the `internal/fabricengine` import (it has none today — `spawn.go:6-11` imports only `filepath`, `hubgeometry` and `vscode`): `ideengine/spawn.go` sources the prime name via `fabricengine.PrimeName(l)` and passes it to `PickColor`, degrading to the empty string on error rather than failing the spawn. `loomengine/preflight.go:67`'s `l.Prime == ""` check becomes a `fabricengine.PrimeName(l)` call producing the same `Report` — a `PrimeName` error maps to the same `CheckGeometry` failure entry, never a hard error, preserving preflight's report-not-error contract; `loomengine` already imports `fabricengine`. That rewrite kills the injection mechanism of `TestPreflight_EmptyPrime` (`preflight_integration_test.go:217-230`), which simulates the no-prime case by setting `l.Prime = ""` on a copied Layout at `:223` — a field that no longer exists. Delete that sub-test with the field: the no-main-worktree condition is no longer injectable through a struct field, and the surviving behaviour (a prime-resolution failure reports `CheckGeometry` rather than erroring) is stated above as the rewrite's contract. `ideengine/menu.go:38`'s `hubgeometry.List(l.Cwd)` becomes `fabricengine.List(l.Cwd)`. Leave `tokenvocab`'s `c.Layout.Repo` expression alone: the field survives this card under its old name, so nothing there changes. Card 5 renames it to `RepoName`, and cards 11 and 17 update the read and its test literals. The `Prime`/`WeftRepoRoot`/`deriveRepo` deletions also reach six test files whose fallout this card must absorb, or the batch's own `go test ./...`/tagged gates fail to compile: `hubgeometry/weft_test.go` drops the `Prime:` field from its synthetic literals (`:86,166,181,199,281,318`) and deletes its `WeftRepoRoot()` assertions — the table sub-test at `:90` (with the `wantWeftRepoRoot` column feeding it) and `TestWeftGeometryAtMainWorktree` (`:189-206`) — because the method's replacement lives in `fabricengine`, which an in-package `hubgeometry` test cannot import; the equivalent property (`WeftRepoRoot(l)` == `weftname.SiblingPath(l.Hub, primeName)`) is held by the `fabricengine` test callers this card retargets. `hubgeometry/pattern_test.go:24` drops `Prime:` from `newTestLayout`'s literal. `hubgeometry_test.go` rewrites its `Prime`/`Repo` assertions at `:63-70` to the new `-HUB`-trim derivation and deletes the `PrimeName` sub-test at `:158-161` — its subject is the deleted method, and `fabricengine.PrimeName` is covered by that package's own retargeted callers. `reedengine/mouse_boot_integration_test.go:50-55` drops `Prime:` from its synthetic `Layout` literal; the remaining fields survive until batch 4's sweep. `hubgeometry/geometry_test.go`'s `TestWeftLayoutMethodParity` drops the `Prime:` field from its literal (`:180`) and deletes its `WeftRepoRoot()` parity assertion (`:191-197`) — same reasoning as `weft_test.go`'s, the replacement lives in `fabricengine` and the property is held by that package's retargeted test callers — while its `WeftWorktreePath`/`WeftWorktree` assertions stay, those methods surviving until card 31, whose lift covers this file. And `hubgeometry/hubgeometry_unit_test.go`'s `TestDeriveRepo` (`:151-192`) is deleted whole with its subject: `deriveRepo` is deleted, not moved, and the new `-HUB`-trim derivation it gives way to is asserted by `hubgeometry_test.go`'s rewritten `:63-70` block above.
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
  - `internal/hubgeometry/hubgeometry_test.go`
  - `internal/hubgeometry/pattern_test.go`
  - `internal/hubgeometry/weft_test.go`
  - `internal/ideengine/spawn.go`
  - `internal/ideengine/spawn_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/hubgeometry/siblinglayout_test.go`
- **Moves:** none
- **Requirements:** Two deletions from `internal/hubgeometry/hubgeometry.go`, both forced by the new struct. First, delete `(*Layout).WorktreePath(slug string)` (`:410-413`) — its name collides with the no-arg `WorktreePath()` accessor card 5 introduces on the same type, which is a duplicate-method compile error, so it cannot wait for its scheduled batch-3 move. Add `func WorktreePath(l *hubgeometry.Layout, slug string) string` to `internal/fabricengine/junction.go` with the identical body (`filepath.Join(l.Hub, slug)`) and retarget the `fabricengine` and `ideengine/spawn.go:20` production callers plus every listed test caller. The four **in-module** callers that outlive the method and cannot import `fabricengine` — `LauncherSpawnRel` (`:459`), `HostLyxLink` (`:524`), `HostPatternLink` (`:537`) and `HostJunctions` (`:570`) — each get the method's entire body inlined as `filepath.Join(l.Hub, slug)`, carried until batch 6 relocates them. Seven **in-package test** callers need the same inlining and for the same reason — package `hubgeometry`'s own tests cannot import `fabricengine`, which imports `hubgeometry`: `hubgeometry_test.go:130,423,449,572`, `pattern_test.go:106` and `weft_test.go:131,299`. Each becomes `filepath.Join(layout.Hub, slug)` (`l.Hub` where the local is named `l`), preserving the surrounding `filepath.Join(..., l.RelPath, ...)` wrapper where one is present. `hubgeometry_test.go:130` is the direct-assertion case — `if got := layout.WorktreePath(slug); got != expectedWtPath` becomes the same comparison against the inlined join, and it is the one call site whose *subject* is the deleted method rather than a convenience, so consider whether the assertion still earns its place once card 5 introduces the no-arg accessor it will be renamed onto. Second, delete `(*Layout).SiblingLayout` (`:164-185`) and its test file: it exists to recompute `Hub`, `RelPath`, `Prime` and `Repo` for a hub sibling, and under the new struct a hub sibling differs in exactly one field, so there is nothing left to compute. It is also the last constructor setting a dishonest `Cwd` (`:179`). `fabricengine/hostlayout.go`'s `hostLayoutFor` constructs the sibling inline instead — same `Hub`, same `RelPath`, `WorktreeRoot`/`Cwd` set to `filepath.Clean(worktreeRoot)`, reading the anchor the same way; card 5 rewrites that literal into `&lyxcwd.Location{HubPath: l.HubPath, WorktreeName: filepath.Base(worktreeRoot), AnchorRel: l.AnchorRel}`. The non-sibling `ResolveWorktree` fallback at `hostlayout.go:22` is unchanged, and its guard already rejects the non-sibling case before the inline construction is reached.
- **Commit:** `refactor(hubgeometry): drop WorktreePath(slug) and SiblingLayout`


## Batch Tests

`verify` runs the repo-wide tagged type-check (`go vet -tags "integration smoke scout" ./...`, ~7 s) followed by the full untagged suite. Both halves are required: `go test` without tags does not compile `integration`/`smoke`/`scout`-tagged test files at all, and this batch edits roughly 25 of them, so a broken tagged file would otherwise pass unnoticed until a later tagged run.

The unbounded `go test ./...` is justified here because the config-path move alone touches 34 packages' test files; no narrower scope is meaningful. New coverage lands in `internal/weftname/weftname_test.go` — untagged `SiblingPath`/`BareSiblingPath` round-trips plus the assertion that the name `lyxtest` builds for a fixture and the name `fabricengine` derives in production agree for the same input, which is the drift this leaf exists to prevent.
