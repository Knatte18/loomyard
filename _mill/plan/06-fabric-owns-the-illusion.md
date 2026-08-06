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

It has the widest blast radius of any batch here and is deliberately isolated. Card 30 lands first and is what makes the rest safe: it adds the `fabricengine` accessors the seven leak sites need and cuts them over **while the old methods still exist**, so cards 31-34 relocate against a tree whose non-fabric *production* callers are already gone. The relocated constructors are unexported by default, but each card names the ones that stay exported and why: roughly 20 of `fabricengine`'s own test files are `package fabricengine_test` (external), and an external-package caller of an unexported identifier does not compile — the illusion boundary is a review obligation on non-fabric production callers, not a compiler check on fabric's own tests. All seven leak uses are read-only — reporting text, audit text, and one regex built to scrub the weft path *out* of logs — so none needs an independent weft git operation, and slice 8 is left owning only its open policy question about CLI wording.

Batch-local decision — the four pattern-specific accessors are **deleted, not moved** (card 35). They are Fabric's own illusion-maintenance plumbing, and PATTERN least of all needs to know weft exists. They are also redundant: `_pattern` is already wired by the generic config-driven junction machinery, and `lyxcwd` deliberately excludes `PatternDirName` from the reserved-name set precisely so it flows through `pathspec` like any other name. Every reference to the four is a test; those tests are rewritten against the generic junction list, which is what they should have asserted against all along.

External interface batch 7 consumes: `fabricengine.BoardDir(hubPath)`, and the relocated junction primitives (`HostJunctions`, `seedGitExclude`, `WorktreePath`) it wires `_board` alongside.

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
- **Requirements:** Add to `internal/fabricengine/fabric.go` the exported read-only accessors the leak sites need: `func WeftWorktree(l *lyxcwd.Location) string` (the weft worktree paired with `l`) and `func WeftLyxDir(l *lyxcwd.Location) string`. Retarget each leak site: `buildercli/weft.go:26` and `webstercli/weft.go:23` call `fabricengine.WeftWorktree(layout)`; `builderengine/spawn.go:376` and `websterengine/runlevel.go:826` do the same inside their `fabricengine.New(...)` calls; `perchcli/run.go:322` likewise; `loomengine/preflight.go:104` stats `fabricengine.WeftWorktree(l)`. `websterengine/audit.go:87` is the one exception that does **not** route through a `fabricengine` accessor for its suffix: `weftReferencePattern` keeps `fabricengine.WeftWorktree(layout)` for the path half but takes its suffix from `weftname.Suffix`, already wired in batch 1 — the comment at `audit.go:85` insisting the value come from the constant "never from string literals" stays true and must be updated to name `weftname.Suffix` rather than `hubgeometry.WeftSuffix`. `lyxtest` must **not** import `fabricengine`: that is a compile-time cycle, since roughly 30 of `fabricengine`'s ~50 test files are in-package, they import `lyxtest`, and they reach unexported identifiers, so they cannot be converted to `package fabricengine_test`. `lyxtest` keeps building weft names through `weftname` alone. Two out-of-package test files adopt the same accessor their production sibling adopts here, because their calls are the no-slug pair form this card's accessors cover: `websterengine/audit_test.go:46,96,205` and `loomengine/preflight_integration_test.go:318` each swap `layout.WeftWorktree()` for `fabricengine.WeftWorktree(layout)`. The out-of-package callers this card's two accessors do **not** cover are retargeted by the card that relocates each symbol, not here: `configcli/configcli_integration_test.go:111`'s slug-form `WeftWorktreePath` by card 31, `loomengine/preflight_integration_test.go:414`'s `HostLyxLink` by card 32, and the `BoardDir` readers by card 34. User-visible output must be byte-identical after this card — these are reporting and audit strings, and a changed one is a regression, not an improvement.
- **Commit:** `refactor(fabricengine): expose read-only weft accessors and close the leak`

### Card 31: move the `Weft*` surface into fabricengine

- **Context:**
  - `internal/weftname/weftname.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/configcli/configcli_integration_test.go`
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
  - `internal/fabricengine/hook_test.go`
  - `internal/fabricengine/hostclean.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/junction_repoint_test.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/unwire_test.go`
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
- **Requirements:** Delete `WeftWorktree`, `WeftWorktreePath`, `WeftLyxDir`, `WeftLyxDirFor`, `WeftRaddleDir` and `WeftHostSlug` from `internal/lyxcwd/lyxcwd.go`. **The module's own tests go with them**, or package `lyxcwd` stops compiling the moment the methods leave: `weft_test.go`, `lyxcwd_test.go` and `geometry_test.go` between them hold ~67 references to this surface. Lift those sub-tests into the new `internal/fabricengine/weftpaths_test.go` — `package fabricengine`, untagged, same table shapes, rewritten against the relocated functions (`WeftWorktreePath(l, slug)` in place of `layout.WeftWorktreePath(slug)`) over a `&lyxcwd.Location{...}` literal instead of a `*Layout` one. Do not delete the coverage: these tables pin the `-weft` sibling naming across the container/base/subpath combinations that the illusion depends on, and they are the reason a wrong `HubPath`-vs-`WorktreePath()` base would be caught here rather than at runtime. `TestHostLyxLinkHereDivergesFromLyxDir` (`weft_test.go:154-186`) belongs to card 32's surface, not this one — leave it in place for that card to move. Re-declare each in `internal/fabricengine/weftwiring.go` as a function taking `*lyxcwd.Location`: `WeftWorktreePath(l, slug)`, `WeftLyxDirFor(l, slug)` and `WeftHostSlug(name)`, all three **exported** — not out of habit, but because live callers sit in `package fabricengine_test` (external) files, where an unexported identifier does not compile: `branchname_test.go:41,43,47` calls `WeftHostSlug`, and the `WeftWorktreePath`/`WeftLyxDirFor` test callers below are all external-package too. This is the "unless an in-scope caller needs otherwise" branch of the batch scope's privatization rule; the illusion boundary is that no non-fabric *production* package touches these, which the guard's review obligation holds, not the compiler. `WeftWorktreePath`'s **production** call sites are all inside `fabricengine` (`add.go:111,144`, `weftwiring.go:66,86,125`, `reconcile.go:104`, `remove.go:54`, `status.go:86`, `prune.go:61`), but its **test** callers are not: this card retargets every one — `reconcile_stale_registration_test.go:59,239,271,314`, `reconcile_stale_removal_test.go:105,163,261`, `add_rollback_adopt_test.go:108,109,231,232`, `junction_pattern_integration_test.go:470` and `hook_test.go:233` become `fabricengine.WeftWorktreePath(l, slug)` (bare `WeftWorktreePath(l, slug)` in the in-package `hook_test.go`), and `configcli/configcli_integration_test.go:111`'s `f.Layout.WeftWorktreePath(slug)` becomes `fabricengine.WeftWorktreePath(f.Layout, slug)` — `configcli`'s integration test already imports `fabriccli`, so the `fabricengine` import adds no new cycle risk, and card 30 deliberately did not cover this slug-form call. The `WeftLyxDirFor` callers likewise: `unwire.go:67` (in-package production), plus the external-package tests `junction_repoint_test.go:53,155`, `junction_pattern_integration_test.go:74`, `unwire_test.go:104` and `add_rollback_adopt_test.go:142`, all becoming `fabricengine.WeftLyxDirFor(...)`. `junction_pattern_integration_test.go:386`'s Lyx-row `targetFor` func value (`return l.WeftLyxDir()`) becomes `return fabricengine.WeftLyxDir(l)`, the accessor card 30 exported, with the func's parameter type changing to `*lyxcwd.Location`. The `WeftWorktree()` callers in this card's files (`checkout_rollback_test.go:51,74,108,127`, `checkout_index_refresh_test.go:52,58,80`, `warpforward_integration_test.go:62,105,122,156`, `reconcile_stale_registration_test.go:355`, plus the in-package production sites in `checkout.go`, `hostclean.go`, `clone.go:187`, `drift.go:41`, `unwire.go:61`) take card 30's `fabricengine.WeftWorktree(l)`. `WeftRaddleDir` has zero **production** callers; its only references are the module's own `weft_test.go` sub-test (`:114-116`) and the `wantWeftRaddleDir` table column feeding it (`:28,43,58,73`), which are deleted in the lift rather than rewritten — delete the symbol outright rather than relocating dead surface, which is why it is absent from the re-declare list above, not an omission. `fabriccli/weft_verbs.go` takes the accessor card 30 adds. Every relocated body keeps `weftname.SiblingPath` as its name source — the `-weft` token stays owned by `internal/weftname`, and `fabricengine` must not re-declare it.
- **Commit:** `refactor(fabricengine): own the weft path surface`

### Card 32: move the host-junction surface into fabricengine

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/fabricengine/add_rollback_adopt_test.go`
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
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/lyxcwd_test.go`
  - `internal/lyxcwd/weft_test.go`
- **Creates:**
  - `internal/fabricengine/hostjunction_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `HostJunction` type and the `HostLyxLink`, `HostLyxLinkHere`, `HostJunctions` and `HostJunctionsHere` methods from `internal/lyxcwd/lyxcwd.go`. Re-declare `HostJunction` and the four constructors in `internal/fabricengine/junction.go` as functions over `*lyxcwd.Location`, **exported**: `HostLyxLink(l, slug)`, `HostLyxLinkHere(l)`, `HostJunctions(l, slug, names)`, `HostJunctionsHere(l, names)`. Unexported was the first-draft shape and cannot compile: most of the caller files below are `package fabricengine_test` (external), where an unexported `fabricengine` identifier is unreachable — `config_driven_junctions_integration_test.go:52` calls `HostJunctions`, and the `HostLyxLink`/`HostLyxLinkHere` callers are external-package too. This is the batch scope's "unless an in-scope caller needs otherwise" branch; the illusion holds as a review obligation on non-fabric production callers, not as a compiler check. `HostLyxLinkHere` (`lyxcwd.go:530`) does not match the `Host*Link` shape the discussion's glob describes and has zero *production* callers outside the module — but it is **not** dead, so relocate it rather than deleting it. Seven live callers sit in `fabricengine`'s own tests, in four files, all four in this card's `Edits:`: `checkout_rollback_test.go:56,111`, `reconcile_stale_removal_test.go:130,269,379`, `reconcile_stale_registration_test.go:485` and `junction_pattern_integration_test.go:385`. The first six are plain `hostLayout.HostLyxLinkHere()` calls and become `fabricengine.HostLyxLinkHere(hostLayout)`. The seventh is a func value in a table — `linkFor: func(l *hubgeometry.Layout) string { return l.HostLyxLinkHere() }` — and becomes `func(l *lyxcwd.Location) string { return fabricengine.HostLyxLinkHere(l) }`, the type parameter changing with the rename. `HostLyxLink(slug)`'s own callers are all in this card's `Edits:` and every one is retargeted here: `junction_repoint_test.go:52,154`, `junction_pattern_integration_test.go:91,126,205,586`, `remove_junctions_integration_test.go:65` and `add_rollback_adopt_test.go:142,211` become `fabricengine.HostLyxLink(l, slug)`, and the one cross-package caller — `loomengine/preflight_integration_test.go:414`'s `linkFor: func(f lyxtest.PairedFixture, slug string) string { return f.Layout.HostLyxLink(slug) }` — becomes `fabricengine.HostLyxLink(f.Layout, slug)`, `loomengine` already importing `fabricengine` in production (its `:419` `HostPatternLink` sibling row is card 35's job, untouched here). All four of the seven-caller files are also edited under card 31 (Weft* retargets); the two cards touch different call sites, so the edits do not overlap within a file. Keep `HostJunction`'s three fields and their meaning unchanged — `Name`, `Link`, `Target` — because `seedGitExclude` reads only `j.Name` off the record and batch 7 depends on that shape. The relocated bodies join `configengine.LyxDirName` per segment, never a fused literal. `drift.go`'s `Healthy` and `reconcile.go`'s `checkJunctionHealth`/`junctionRepointedDetail` keep iterating `HostJunctionsHere(l, RepoWiredNames(l))` exactly as they do today — this card changes where the function lives, not what any of them checks. As in card 31, the module's own coverage moves with the surface: `lyxcwd_test.go` and `weft_test.go` hold ~86 references to these four methods and the `HostJunction` record, and package `lyxcwd` will not compile once they leave. Lift those sub-tests into the new `internal/fabricengine/hostjunction_test.go` — `package fabricengine`, untagged — including `TestHostLyxLinkHereDivergesFromLyxDir` (`weft_test.go:154-186`), which is the one assertion that pins *why* the two differ: `HostLyxLinkHere` is anchored on the worktree root while the `_lyx` durable directory is anchored on `AnchorPath()`, so they coincide at the worktree root and diverge under a subpath anchor. That distinction is exactly what batch 2's rebase onto `AnchorPath()` puts at risk, so the test is load-bearing rather than incidental and must survive the move intact. One rewrite rule for the lift is not optional: the `HostJunctions`/`HostJunctionsHere` tables' `_pattern` rows currently assert against `HostPatternLink`/`WeftPatternDirFor`/`WeftPatternDir` (`lyxcwd_test.go:615-619`, `:663`, `:705`) — rewrite those expectations against the generic join instead, `filepath.Join(WorktreePath(l, slug), l.AnchorRel, pattern.DirName)` for the Link side and the weft-sibling equivalent for the Target side, importing `internal/pattern` for the name. Card 35 deletes those four accessors later in this batch, and `hostjunction_test.go` must not be reachable by that deletion — the generic-join form is also what the `_pattern` rows should have asserted all along, per card 35's own argument.
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
  - `internal/lyxcwd/lyxcwd_test.go`
- **Creates:**
  - `internal/fabricengine/portallauncher_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `PortalsDir`, `PortalLink`, `PortalTarget`, `LaunchersDir`, `LauncherDir`, `MenuLauncherPath`, `menuLauncherName`, `LauncherSpawnRel` and `MenuLauncherRel` from `internal/lyxcwd/lyxcwd.go`, and with them the `_portals` and `_launchers` string literals. Re-declare each in `internal/fabricengine/portals.go` and `internal/fabricengine/launchers.go` as a function over `*lyxcwd.Location`, with a `portalsDirName = "_portals"` and `launchersDirName = "_launchers"` const pair declared once in `fabricengine`. Three of the nine are **exported** — `PortalsDir(l)`, `PortalLink(l, slug)` and `LauncherDir(l, slug)` — because their live test callers sit in `package fabricengine_test` (external) files where an unexported identifier does not compile: `add_rollback_adopt_test.go:84` (`l.PortalsDir()` becomes `fabricengine.PortalsDir(l)`) and `reconcile_stale_registration_test.go:216,217` (`l.PortalLink(slug)`/`l.LauncherDir(slug)` become `fabricengine.PortalLink(l, slug)`/`fabricengine.LauncherDir(l, slug)`). The other six have only in-package callers and stay unexported. `menuLauncherName()` keeps its GOOS switch (`ide-menu.cmd` on Windows, `ide-menu.sh` elsewhere) verbatim. `MenuLauncherRel` already takes a `primeName string` argument from batch 1 card 3; it keeps that signature. `LauncherSpawnRel` carries the inline `filepath.Join(l.HubPath, slug)` batch 1 card 4 gave it; fold it back into `WorktreePath(l, slug)` now that both live in the same package. As in cards 31 and 32, the module's own coverage moves with the surface, or package `lyxcwd` stops compiling: `lyxcwd_test.go` holds the portal/launcher sub-tests — the inline `PortalsDir`/`PortalTarget`/`LaunchersDir`/`LauncherDir` assertions at `:134-155` and the `PortalLink` (`:223-290`), `LauncherDir` (`:295-363`), `MenuLauncherPath` (`:368-402`), `LauncherSpawnRel` (`:407` ff.) and `MenuLauncherRel` (`:458-499`, already reshaped by card 3) sub-tests. Lift them into the new `internal/fabricengine/portallauncher_test.go` — `package fabricengine`, untagged, same table shapes, rewritten onto the relocated functions over `&lyxcwd.Location{...}` literals — excising each block surgically so the surrounding resolution assertions in `lyxcwd_test.go` stay intact. These tables pin the `_portals`/`_launchers` layout and the launcher relative-path math, and are the reason a wrong `HubPath` base or a broken `filepath.Rel` would be caught here rather than in a live hub.
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
  - `internal/buildercli/weft_integration_test.go`
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
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/junctionnames_test.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/ideengine/menu.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/lyxcwd/anchor.go`
  - `internal/lyxcwd/anchor_test.go`
  - `internal/lyxcwd/geometry_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/lyxcwd_test.go`
  - `internal/perchcli/cli_integration_test.go`
  - `internal/perchcli/run_integration_test.go`
  - `internal/webstercli/weft_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move `BoardDir`, `BoardDirName`, `HubPath`, `HubSuffix`, `HubReservedNames` and `IsReservedHubName` out of `internal/lyxcwd/lyxcwd.go` into `internal/fabricengine/junctionnames.go`, keeping `BoardDir`, `HubPath`, `HubReservedNames` and `IsReservedHubName` exported — `BoardDir` alone has **13 production call sites across four packages** (`fabriccli` ×9, `fabricengine` ×2, `ideengine` ×1, `boardcli` ×1). Its **test** callers reach further than its production ones, and every one is in this card's `Edits:` with the same `fabricengine.BoardDir` qualifier retarget (adding the `fabricengine` import where absent — none of these packages is imported by `fabricengine`, so no cycle): `buildercli/weft_integration_test.go:43,58`, `webstercli/weft_integration_test.go:35,55`, `perchcli/run_integration_test.go:30,44`, `loomengine/preflight_integration_test.go:59`, `fabricengine/config_driven_junctions_integration_test.go:107` (external `fabricengine_test` package), plus the boardcli/configcli/fabriccli/fabricengine test files already listed. `internal/lyxcwd` keeps a **private** `boardDir(hubPath string) string` and a private `boardDirName` const in `anchor.go`, used only by `readRecordedAnchor`; that keeps the module's public surface at exactly the resolution symbols while it still needs the name itself to find the marker. `_board` and `-HUB` are therefore dual-owned, and card 36 records both rows — the duplication is sanctioned by the ownership map, not a leak. `lyxcwd` keeps `HubSuffix`'s value as a private const because `RepoName` derives from it (`strings.TrimSuffix(filepath.Base(HubPath), "-HUB")`); `fabricengine` declares its own for `HubPath(parent, name)`. As in cards 31-33, the module's own tests go with the surface, or `internal/lyxcwd` stops compiling under this batch's gates: `lyxcwd/anchor_test.go:26` (external package, integration-tagged — compiled by the tagged vet and the `go test -tags integration ./internal/lyxcwd/...` leg) calls the deleted `BoardDir(hub)` in its `writeAnchor` helper and becomes `fabricengine.BoardDir(hub)` — an external-package `lyxcwd` test may import `fabricengine` with no cycle, since only production imports are directional, and the geometry guard skips `_test.go` files so no literal is needed; this is also the honest spelling, `fabricengine` being the owner and `lyxcwd`'s retained `boardDir` being private to `anchor.go`. `lyxcwd_test.go`'s `TestIsReservedHubName_Pattern` (`:786-791`) and `geometry_test.go`'s `TestBoardDir` (`:48-76`), `TestHubPath` (`:79-109`) and `TestIsReservedHubName` (`:208-289`) move into `internal/fabricengine/junctionnames_test.go` (already in this card's `Edits:`, `package fabricengine`, untagged), dropping the qualifier — they pin the `_board`/`-HUB` join shapes and the reserved-name union, coverage that must follow the symbols rather than be deleted. Note for the implementer: `internal/boardengine/config.go:21`, `internal/boardengine/template_test.go:25`, `internal/configsync/configsync.go:84` and `internal/fabricengine/remove_junctions_integration_test.go:76` name `BoardDir` **only in comments** and are not callers — correct the comments to the `fabricengine.BoardDir` owner this card creates, do not add imports (the last file is in this card's `Edits:` solely for that comment; its `HostPatternLink` call is card 35's). The same comment pass finishes `fabricengine/doc.go`: beyond the `BoardDir` wording this card already owns, its `:64` `internal/hubgeometry` aside and the `:66`/`:68` `HubReservedNames()`/`HostJunctions` attributions take this card's `fabricengine` ownership — the residue card 18 deferred here by design. A grep that does not exclude comments overcounts and misattributes the dependency. `HubReservedNames()` keeps returning `{_board, _portals, _launchers, _raddle}` and keeps deliberately excluding `LyxDirName` and `PatternDirName`, which are folded in through `IsReservedHubName`'s `junctionNames` parameter instead; batch 7 depends on `_board` being in that set.
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
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/lyxcwd/geometry_test.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/weft_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `WeftPatternDir`, `WeftPatternDirFor`, `HostPatternLink` and `HostPatternLinkHere` (`lyxcwd.go:502-545`) outright — do **not** relocate them into `fabricengine`. They name both sides of the junction for one specific junction, which is exactly what the warp/weft illusion forbids, and they duplicate the generic config-driven junction machinery that already wires `_pattern` through `pathspec`. They have zero production consumers; every reference is a test. Rewrite each of those tests against the generic junction list instead: `junction_pattern_integration_test.go` asserts `_pattern` is present in `RepoWiredNames`, is wired by `WireJunctions`, is health-checked by `checkJunctionHealth` and is repaired by reconcile — through the same code path every other pathspec name takes, with no `_pattern`-specific accessor anywhere in the assertion. `unwire_test.go`, `junction_repoint_test.go`, `add_rollback_adopt_test.go` and `loomengine/preflight_integration_test.go` build their `_pattern` paths with `filepath.Join(…, pattern.DirName)` from batch 5 card 25. Two more external-package caller files take the same generic-join form, all four call sites named: `reconcile_stale_removal_test.go:113,273,380`'s `hostLayout.HostPatternLinkHere()` becomes `filepath.Join(hostLayout.WorktreePath(), hostLayout.AnchorRel, pattern.DirName)` (the Here-form base, mirroring card 32's rule for the slug form), and `remove_junctions_integration_test.go:69`'s `nestedLayout.HostPatternLink(slug)` becomes `filepath.Join(fabricengine.WorktreePath(nestedLayout, slug), nestedLayout.AnchorRel, pattern.DirName)` — both files gain the `internal/pattern` import, and both are `package fabricengine_test`, which the exported `WorktreePath` from batch 1 card 4 already serves. `fabricengine/pull.go:299` keeps needing the bare name as a git pathspec, so `fabricengine` declares its own `patternDirName = "_pattern"` — and **this card also deletes the transitional `PatternDirName` const card 25 left in `lyxcwd.go`, cutting over every remaining reader in the same card**: `pull.go:299` takes the new in-package `patternDirName` (and its comments at `:269` and `:283` are corrected), while the test readers — `fabriccli/cli_test.go:416`, `pull_integration_test.go:258,292`, `unwire_test.go:65,71` and `junction_pattern_integration_test.go`'s nine sites — take `pattern.DirName`. Because this card both declares `fabricengine`'s `_pattern` literal and deletes `lyxcwd`'s, it updates the ownership row in the same breath: in `internal/lyxcwd/enforcement_test.go`, `_pattern`'s row becomes `internal/pattern` plus `internal/fabricengine`, dropping the transitional `internal/lyxcwd` entry — card 36 then leaves that row untouched. `hostjunction_test.go` needs nothing here: card 32's lift already rewrote its `_pattern` expectations onto the generic join, so this card's deletions cannot reach it — verify, do not re-edit. Delete the corresponding sub-tests from `internal/lyxcwd/weft_test.go` and `internal/lyxcwd/geometry_test.go`; the coverage they stood in for is the generic-junction assertion above, not a per-accessor one.
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
- **Requirements:** Move each token's ownership row in lockstep with the owner this batch created. `_portals`, `_launchers` and `_raddle` become owned by `internal/fabricengine` alone. `_pattern`'s row is already finished and is left untouched: card 25 registered `internal/pattern` when it declared the token, and card 35 swapped the transitional `internal/lyxcwd` entry for `internal/fabricengine` when it moved the pathspec const. `_board` and `-HUB` become dual-owned by `internal/lyxcwd` and `internal/fabricengine`. `-weft` stays owned by `internal/weftname` and `_lyx` keeps the `internal/configengine` plus transitional `internal/lyxcwd` pair from batch 4 — batch 8 removes the transitional entry. The guard is only a real assertion when it names an ownership that already exists: written up front it would assert a layout the tree does not have, written only at the end it would leave four batches red. Moving each entry with its owner is also what proves this batch **moved** ownership rather than copying code — a token still declared in `internal/lyxcwd` after its row points at `internal/fabricengine` fails the guard loudly. `internal/lyxtest` needs **no** entry: it gets `-weft` from `weftname` and `_lyx` from `configengine`, so it constructs no geometry literal of its own. `fabricengine`, `ideengine`, `buildercli`, `webstercli` and `perchcli` all reference `_lyx` and all stay off the map — the map records **declarers**, not users.
- **Commit:** `test(lyxcwd): move the guard's token ownership rows onto fabricengine`

## Batch Tests

`verify` runs the repo-wide tagged type-check plus the untagged suites of every package this batch touches. `internal/fabricengine`'s own suite is the load-bearing half: roughly 30 of its ~50 test files are in-package and reach unexported identifiers, while the other ~20 are `package fabricengine_test` — which is why the relocated constructors with external-package test callers stay exported (cards 31-33 name each one) — so a relocation that broke an invariant surfaces in this suite rather than at a call site.

The coverage that must not be lost is the `_pattern` path. Card 35 deletes four accessors whose only consumers were tests, so the assertions move to `junction_pattern_integration_test.go` and now exercise `_pattern` through the generic config-driven junction machinery — wired, health-checked and repaired like any other pathspec name. That is what those tests should have asserted against all along, and it is the check that proves deleting the accessors lost nothing. Card 30's requirement that user-visible output stay byte-identical across the seven leak sites is covered by the existing reporting assertions in `websterengine/audit_test.go`, `loomengine/preflight_integration_test.go` and the `buildercli`/`webstercli` weft integration tests.
