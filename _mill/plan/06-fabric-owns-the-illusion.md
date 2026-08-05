# Batch: fabric-owns-the-illusion

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
batch: fabric-owns-the-illusion
number: 6
cards: 7
verify: go vet -tags "integration smoke scout" ./... && go test ./internal/lyxcwd/... ./internal/fabricengine/... ./internal/fabriccli/... ./internal/lyxtest/... ./internal/pattern/... ./internal/loomengine/... ./internal/websterengine/... ./internal/webstercli/... ./internal/builderengine/... ./internal/buildercli/... ./internal/perchcli/... ./internal/boardcli/... ./internal/boardengine/... ./internal/configcli/... ./internal/configsync/... ./internal/ideengine/... ./cmd/lyx/... && go test -tags integration ./internal/lyxcwd/... ./internal/fabricengine/... ./internal/fabriccli/... ./internal/lyxtest/... ./internal/pattern/... ./internal/loomengine/... ./internal/websterengine/... ./internal/webstercli/... ./internal/builderengine/... ./internal/buildercli/... ./internal/perchcli/... ./internal/boardcli/... ./internal/boardengine/... ./internal/configcli/... ./internal/configsync/... ./internal/ideengine/... ./cmd/lyx/...
depends-on: [5]
```

## Batch Scope

This batch moves Fabric's own weft/junction/portal/launcher plumbing into `internal/fabricengine`, private unless an in-scope caller needs otherwise, and closes the weft-visibility leak that made seven non-`fabric*` packages query a shared module for weft paths. After it, `internal/lyxcwd` never mentions weft, and no consumer outside `fabricengine` can tell there are two repos — which is the illusion the whole `fabric` design promises.

It has the widest blast radius of any batch here and is deliberately isolated. Card 30 lands first and is what makes the rest safe: it adds the `fabricengine` accessors the seven leak sites need and cuts them over **while the old methods still exist**, so cards 31-34 are pure privatization against a tree that already compiles without external callers. All seven leak uses are read-only — reporting text, audit text, and one regex built to scrub the weft path *out* of logs — so none needs an independent weft git operation, and slice 8 is left owning only its open policy question about CLI wording.

Batch-local decision — the four pattern-specific accessors are **deleted, not moved** (card 35). They are Fabric's own illusion-maintenance plumbing, and PATTERN least of all needs to know weft exists. They are also redundant: `_pattern` is already wired by the generic config-driven junction machinery, and `lyxcwd` deliberately excludes `PatternDirName` from the reserved-name set precisely so it flows through `pathspec` like any other name. Every reference to the four is a test; those tests are rewritten against the generic junction list, which is what they should have asserted against all along.

External interface batch 7 consumes: `fabricengine.BoardDir(hubPath)`, and the now-private junction primitives it wires `_board` alongside.

## Cards

### Card 30: fabricengine accessors for the seven weft-visibility leak sites

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/weftname/weftname.go`
- **Edits:**
  - `internal/buildercli/weft.go`
  - `internal/buildercli/weft_integration_test.go`
  - `internal/builderengine/spawn.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/fabricengine/fabric.go`
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/perchcli/run.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/webstercli/weft.go`
  - `internal/webstercli/weft_integration_test.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/audit_test.go`
  - `internal/websterengine/runlevel.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add to `internal/fabricengine/fabric.go` the exported read-only accessors the leak sites need: `func WeftWorktree(l *lyxcwd.Location) string` (the weft worktree paired with `l`) and `func WeftLyxDir(l *lyxcwd.Location) string`. Retarget each leak site: `buildercli/weft.go:26` and `webstercli/weft.go:23` call `fabricengine.WeftWorktree(layout)`; `builderengine/spawn.go:376` and `websterengine/runlevel.go:826` do the same inside their `fabricengine.New(...)` calls; `perchcli/run.go:322` likewise; `loomengine/preflight.go:104` stats `fabricengine.WeftWorktree(l)`. `websterengine/audit.go:87` is the one exception that does **not** route through a `fabricengine` accessor for its suffix: `weftReferencePattern` keeps `fabricengine.WeftWorktree(layout)` for the path half but takes its suffix from `weftname.Suffix`, already wired in batch 1 — the comment at `audit.go:85` insisting the value come from the constant "never from string literals" stays true and must be updated to name `weftname.Suffix` rather than `hubgeometry.WeftSuffix`. `lyxtest` must **not** import `fabricengine`: that is a compile-time cycle, since 25 in-package `fabricengine` test files import `lyxtest` and 19 of them need unexported access, so they cannot be converted to `package fabricengine_test`. `lyxtest` keeps building weft names through `weftname` alone. The three out-of-package test files that break when cards 31-32 privatize (`websterengine/audit_test.go`, `loomengine/preflight_integration_test.go`, `configcli/configcli_integration_test.go`) adopt the same accessor their production sibling adopts here. User-visible output must be byte-identical after this card — these are reporting and audit strings, and a changed one is a regression, not an improvement.
- **Commit:** `refactor(fabricengine): expose read-only weft accessors and close the leak`

### Card 31: move the `Weft*` surface into fabricengine

- **Context:**
  - `internal/weftname/weftname.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/branchname_test.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/checkout_index_refresh_test.go`
  - `internal/fabricengine/checkout_rollback_test.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/hostclean.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/warpforward_integration_test.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/lyxcwd/geometry_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/lyxcwd_test.go`
  - `internal/lyxcwd/weft_test.go`
- **Creates:**
  - `internal/fabricengine/weftpaths_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `WeftWorktree`, `WeftWorktreePath`, `WeftLyxDir`, `WeftLyxDirFor`, `WeftRaddleDir` and `WeftHostSlug` from `internal/lyxcwd/lyxcwd.go`. **The module's own tests go with them**, or package `lyxcwd` stops compiling the moment the methods leave: `weft_test.go`, `lyxcwd_test.go` and `geometry_test.go` between them hold ~67 references to this surface. Lift those sub-tests into the new `internal/fabricengine/weftpaths_test.go` — untagged, same table shapes, rewritten against the relocated functions (`weftWorktreePath(l, slug)` in place of `layout.WeftWorktreePath(slug)`) over a `&lyxcwd.Location{...}` literal instead of a `*Layout` one. Do not delete the coverage: these tables pin the `-weft` sibling naming across the container/base/subpath combinations that the illusion depends on, and they are the reason a wrong `HubPath`-vs-`WorktreePath()` base would be caught here rather than at runtime. `TestHostLyxLinkHereDivergesFromLyxDir` (`weft_test.go:154-186`) belongs to card 32's surface, not this one — leave it in place for that card to move. Re-declare each in `internal/fabricengine/weftwiring.go` as a function taking `*lyxcwd.Location`, unexported unless card 30 already exported it: `weftWorktreePath(l, slug)`, `weftLyxDirFor(l, slug)`, `WeftHostSlug(name)` (exported — `cleanup.go` and `prune.go` are in-package but `branchname.go`'s parsing is part of the package's public naming contract; keep it exported only if an out-of-package caller exists, otherwise unexport it). `WeftWorktreePath` has **9 production call sites, all inside `fabricengine`** (`add.go:111,144`, `weftwiring.go:66,86,125`, `reconcile.go:104`, `remove.go:54`, `status.go:86`, `prune.go:61`), so it is a pure in-package privatization with no external fallout. `WeftRaddleDir` has zero callers anywhere; delete it outright rather than relocating dead surface — which is why it is absent from the re-declare list above, not an omission. `fabriccli/weft_verbs.go` is the one out-of-package caller and takes the accessor card 30 adds. Every relocated body keeps `weftname.SiblingPath` as its name source — the `-weft` token stays owned by `internal/weftname`, and `fabricengine` must not re-declare it.
- **Commit:** `refactor(fabricengine): own the weft path surface`

### Card 32: move the host-junction surface into fabricengine

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/fabricengine/checkout_rollback_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/junction_repoint_test.go`
  - `internal/fabricengine/junction_test.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/weftwiring_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/lyxcwd_test.go`
  - `internal/lyxcwd/weft_test.go`
- **Creates:**
  - `internal/fabricengine/hostjunction_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `HostJunction` type and the `HostLyxLink`, `HostLyxLinkHere`, `HostJunctions` and `HostJunctionsHere` methods from `internal/lyxcwd/lyxcwd.go`. Re-declare `HostJunction` and the four constructors in `internal/fabricengine/junction.go` as functions over `*lyxcwd.Location`, unexported: `hostLyxLink(l, slug)`, `hostJunctions(l, slug, names)`, `hostJunctionsHere(l, names)`. `HostLyxLinkHere` (`lyxcwd.go:530`) does not match the `Host*Link` shape the discussion's glob describes and has zero *production* callers outside the module — but it is **not** dead, so relocate it as unexported `hostLyxLinkHere(l)` in `internal/fabricengine/junction.go` rather than deleting it. Seven live callers sit in `fabricengine`'s own tests, in four files, all four in this card's `Edits:`: `checkout_rollback_test.go:56,111`, `reconcile_stale_removal_test.go:130,269,379`, `reconcile_stale_registration_test.go:485` and `junction_pattern_integration_test.go:385`. The first six are plain `hostLayout.HostLyxLinkHere()` calls and become `hostLyxLinkHere(hostLayout)`. The seventh is a func value in a table — `linkFor: func(l *hubgeometry.Layout) string { return l.HostLyxLinkHere() }` — and becomes `func(l *lyxcwd.Location) string { return hostLyxLinkHere(l) }`, the type parameter changing with the rename. Three of those four files are also edited under card 31 for an unrelated reason; that card retargets the `Weft*` calls and this one the host-junction calls, so the two edits do not overlap within a file. Keep `HostJunction`'s three fields and their meaning unchanged — `Name`, `Link`, `Target` — because `seedGitExclude` reads only `j.Name` off the record and batch 7 depends on that shape. The relocated bodies join `configengine.LyxDirName` per segment, never a fused literal. `drift.go`'s `Healthy` and `reconcile.go`'s `checkJunctionHealth`/`junctionRepointedDetail` keep iterating `hostJunctionsHere(l, RepoWiredNames(l))` exactly as they do today — this card changes where the function lives, not what any of them checks. As in card 31, the module's own coverage moves with the surface: `lyxcwd_test.go` and `weft_test.go` hold ~86 references to these four methods and the `HostJunction` record, and package `lyxcwd` will not compile once they leave. Lift those sub-tests into the new `internal/fabricengine/hostjunction_test.go` — untagged — including `TestHostLyxLinkHereDivergesFromLyxDir` (`weft_test.go:154-186`), which is the one assertion that pins *why* the two differ: `hostLyxLinkHere` is anchored on the worktree root while the `_lyx` durable directory is anchored on `AnchorPath()`, so they coincide at the worktree root and diverge under a subpath anchor. That distinction is exactly what batch 2's rebase onto `AnchorPath()` puts at risk, so the test is load-bearing rather than incidental and must survive the move intact.
- **Commit:** `refactor(fabricengine): own the host-junction records`

### Card 33: move the portal and launcher surface into fabricengine

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/configengine/config.go`
  - `internal/ideengine/menu.go`
- **Edits:**
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `PortalsDir`, `PortalLink`, `PortalTarget`, `LaunchersDir`, `LauncherDir`, `MenuLauncherPath`, `menuLauncherName`, `LauncherSpawnRel` and `MenuLauncherRel` from `internal/lyxcwd/lyxcwd.go`, and with them the `_portals` and `_launchers` string literals. Re-declare each in `internal/fabricengine/portals.go` and `internal/fabricengine/launchers.go` as an unexported function over `*lyxcwd.Location`, with a `portalsDirName = "_portals"` and `launchersDirName = "_launchers"` const pair declared once in `fabricengine`. `menuLauncherName()` keeps its GOOS switch (`ide-menu.cmd` on Windows, `ide-menu.sh` elsewhere) verbatim. `MenuLauncherRel` already takes a `primeName string` argument from batch 1 card 3; it keeps that signature. `LauncherSpawnRel` carries the inline `filepath.Join(l.HubPath, slug)` batch 1 card 4 gave it; fold it back into `WorktreePath(l, slug)` now that both live in the same package.
- **Commit:** `refactor(fabricengine): own the portal and launcher paths`

### Card 34: move the hub-structural surface and export `BoardDir`

- **Context:**
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/boardcli/cli.go`
  - `internal/boardcli/cli_test.go`
  - `internal/boardcli/notes_test.go`
  - `internal/boardengine/config.go`
  - `internal/boardengine/config_test.go`
  - `internal/boardengine/template_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/configcli/reconcile_integration_test.go`
  - `internal/configsync/configsync.go`
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/junctionnames_test.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/ideengine/menu.go`
  - `internal/lyxcwd/anchor.go`
  - `internal/lyxcwd/geometry_test.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move `BoardDir`, `BoardDirName`, `HubPath`, `HubSuffix`, `HubReservedNames` and `IsReservedHubName` out of `internal/lyxcwd/lyxcwd.go` into `internal/fabricengine/junctionnames.go`, keeping `BoardDir`, `HubPath`, `HubReservedNames` and `IsReservedHubName` exported — `BoardDir` alone has **13 production call sites across four packages** (`fabriccli` ×9, `fabricengine` ×2, `ideengine` ×1, `boardcli` ×1). `internal/lyxcwd` keeps a **private** `boardDir(hubPath string) string` and a private `boardDirName` const in `anchor.go`, used only by `readRecordedAnchor`; that keeps the module's public surface at exactly the resolution symbols while it still needs the name itself to find the marker. `_board` and `-HUB` are therefore dual-owned, and card 36 records both rows — the duplication is sanctioned by the ownership map, not a leak. `lyxcwd` keeps `HubSuffix`'s value as a private const because `RepoName` derives from it (`strings.TrimSuffix(filepath.Base(HubPath), "-HUB")`); `fabricengine` declares its own for `HubPath(parent, name)`. Note for the implementer: `internal/boardengine/config.go:21` and `internal/configsync/configsync.go:84` name `BoardDir` **only in comments** and are not callers — correct the comments, do not add imports. A grep that does not exclude comments overcounts and misattributes the dependency. `HubReservedNames()` keeps returning `{_board, _portals, _launchers, _raddle}` and keeps deliberately excluding `LyxDirName` and `PatternDirName`, which are folded in through `IsReservedHubName`'s `junctionNames` parameter instead; batch 7 depends on `_board` being in that set.
- **Commit:** `refactor(fabricengine): own the hub-structural names and export BoardDir`

### Card 35: delete the four pattern-specific weft accessors

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/junction_repoint_test.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/pull_integration_test.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/lyxcwd/geometry_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/weft_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `WeftPatternDir`, `WeftPatternDirFor`, `HostPatternLink` and `HostPatternLinkHere` (`lyxcwd.go:502-545`) outright — do **not** relocate them into `fabricengine`. They name both sides of the junction for one specific junction, which is exactly what the warp/weft illusion forbids, and they duplicate the generic config-driven junction machinery that already wires `_pattern` through `pathspec`. They have zero production consumers; every reference is a test. Rewrite each of those tests against the generic junction list instead: `junction_pattern_integration_test.go` asserts `_pattern` is present in `RepoWiredNames`, is wired by `WireJunctions`, is health-checked by `checkJunctionHealth` and is repaired by reconcile — through the same code path every other pathspec name takes, with no `_pattern`-specific accessor anywhere in the assertion. `unwire_test.go`, `junction_repoint_test.go`, `add_rollback_adopt_test.go` and `loomengine/preflight_integration_test.go` build their `_pattern` paths with `filepath.Join(…, pattern.DirName)` from batch 5 card 25. `fabricengine/pull.go:299` keeps needing the bare name as a git pathspec, so `fabricengine` declares its own `patternDirName = "_pattern"`; `internal/pattern` and `internal/fabricengine` are the two owners card 36 records. Delete the corresponding sub-tests from `internal/lyxcwd/weft_test.go` and `internal/lyxcwd/geometry_test.go`; the coverage they stood in for is the generic-junction assertion above, not a per-accessor one.
- **Commit:** `refactor(fabric): delete the pattern-specific weft accessors`

### Card 36: batch-3 slice of the enforcement-guard rewrite

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/pattern/pattern.go`
  - `internal/lyxcwd/anchor.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move each token's ownership row in lockstep with the owner this batch created. `_portals`, `_launchers` and `_raddle` become owned by `internal/fabricengine` alone. `_pattern` becomes dual-owned by `internal/pattern` and `internal/fabricengine`. `_board` and `-HUB` become dual-owned by `internal/lyxcwd` and `internal/fabricengine`. `-weft` stays owned by `internal/weftname` and `_lyx` keeps the `internal/configengine` plus transitional `internal/lyxcwd` pair from batch 4 — batch 8 removes the transitional entry. The guard is only a real assertion when it names an ownership that already exists: written up front it would assert a layout the tree does not have, written only at the end it would leave four batches red. Moving each entry with its owner is also what proves this batch **moved** ownership rather than copying code — a token still declared in `internal/lyxcwd` after its row points at `internal/fabricengine` fails the guard loudly. `internal/lyxtest` needs **no** entry: it gets `-weft` from `weftname` and `_lyx` from `configengine`, so it constructs no geometry literal of its own. `fabricengine`, `ideengine`, `buildercli`, `webstercli` and `perchcli` all reference `_lyx` and all stay off the map — the map records **declarers**, not users.
- **Commit:** `test(lyxcwd): move the guard's token ownership rows onto fabricengine`

## Batch Tests

`verify` runs the repo-wide tagged type-check plus the untagged suites of every package this batch touches. `internal/fabricengine`'s own suite is the load-bearing half: 25 of its test files are in-package and reach unexported identifiers, so a privatization that broke an invariant surfaces there rather than at a call site.

The coverage that must not be lost is the `_pattern` path. Card 35 deletes four accessors whose only consumers were tests, so the assertions move to `junction_pattern_integration_test.go` and now exercise `_pattern` through the generic config-driven junction machinery — wired, health-checked and repaired like any other pathspec name. That is what those tests should have asserted against all along, and it is the check that proves deleting the accessors lost nothing. Card 30's requirement that user-visible output stay byte-identical across the seven leak sites is covered by the existing reporting assertions in `websterengine/audit_test.go`, `loomengine/preflight_integration_test.go` and the `buildercli`/`webstercli` weft integration tests.
