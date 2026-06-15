The `_codeguide` claim holds (only in discussion.md, zero in Go tree). `List` exists.

Now I have a complete picture. Let me evaluate the discussion against the criteria. Key observations:

1. The discussion's `relative-climb-via-paths` decision describes current code as "hard-coded `..\..\`" but the current launchers.go ALREADY embeds RelPath on the target side. The decision's claimed output `(..\)^(2+N)<slug>\<sub>` is correct for the NEW mirrored location, but the discussion doesn't note that the existing flat-launcher already appends `<sub>` — which means the existing `NonEmptyRelPath` test asserts `..\..\task-b\subdir\nested` from a FLAT launcher dir. That test will need updating because the launcher dir moves. The discussion's testing section does mention extending the launcher table, so this is covered.

2. A subtle concern: the menu-mirror decision and the `removeLaunchers` prune. The discussion says the launcher subpath dir holds `ide-menu.cmd` so it won't be pruned while a menu exists. But `removeLaunchers` is called per-slug; the `ide-menu.cmd` is never removed by `removeLaunchers`. So the subpath dir is never actually prunable while menu exists — meaning launcher-side pruning of the `<subpath>` dir is effectively dead/never-fires for the leaf subpath dir. This is acknowledged in gotchas. But there's an ambiguity about intermediate dirs above the menu's subpath dir for multi-segment subpaths.

For launchers with subpath `services/api`: structure is `_launchers/services/api/<slug>` and `_launchers/services/api/ide-menu.cmd`. After removing the last slug, `_launchers/services/api/` still has `ide-menu.cmd`, so prune stops. Good — never prunes. But the menu is never removed, so launcher-side prune of the leaf is essentially never effective. The portals side has no menu, so it prunes fully. This asymmetry is internally consistent with the discussion.

MILL_REVIEW_BEGIN
# Review: internal/paths: subpath init + mirrored system dirs

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-15
```

## Findings

### [GAP] Launcher-side prune is unreachable for the leaf subpath dir
**Section:** Decisions / teardown-prune-empty (and Scope "Teardown")
**Issue:** `removeLaunchers` never deletes `ide-menu.cmd`, and the menu lives in the leaf `_launchers/<RelPath>/` dir, so for any subpath that dir stays non-empty forever — the launcher-side prune up to `LaunchersDir()` can therefore never fire above the per-slug dir, making the "prune empty mirrored ancestors" goal a no-op on the launcher side (only portals actually prune). The discussion frames this as a benign edge ("stops while a menu exists") but does not state that launcher intermediate dirs are thus effectively never reclaimed.
**Fix:** Clarify that launcher pruning only ever removes `LauncherDir(slug)` itself (the `<RelPath>` chain is retained by the menu), or decide whether the menu should be removed when the last slug under a subpath goes, enabling true ancestor prune.

### [GAP] Decision misstates the current ide.cmd format it replaces
**Section:** Decisions / relative-climb-via-paths
**Issue:** The decision says the current climb is "hard-coded `..\..\` for `ide.cmd`," but `internal/worktree/launchers.go` (lines 42-49) already appends the RelPath tail today (`..\..\<slug>\<relpath>`), asserted by the `NonEmptyRelPath` case in `launchers_test.go` (line 68) against a *flat* `_launchers/<slug>` dir. The new mirrored dir adds N segments, so that existing assertion becomes wrong and must change, not merely be extended. The discussion's testing section says "extend the table" but does not flag that the existing `NonEmptyRelPath` expectation is now incorrect and must be rewritten.
**Fix:** Note explicitly that the existing `NonEmptyRelPath` assertion (`..\..\task-b\subdir\nested`) must be updated to the deeper climb, and that `LauncherSpawnRel` must reproduce both the climb and the existing `<slug>\<sub>` tail.

### [NOTE] PortalLink vs createJunction mkdir-parent overlap unspecified
**Section:** Scope / "PortalLink(slug)"
**Issue:** `createJunction` already `MkdirAll`s `filepath.Dir(link)` (junction_windows.go line 31 / junction_other.go line 25), so the new mirrored `_portals/<RelPath>/` dirs get created there; the discussion does not say whether portals need any separate `MkdirAll`, leaving a plan writer to guess.
**Fix:** State that portal intermediate-dir creation is already handled by `createJunction`, so only the link path changes.

### [NOTE] enforcement_test skips _test.go — guard test placement matters
**Section:** Scope / "guard test … never references `_codeguide`"
**Issue:** A `_codeguide` guard modeled on `enforcement_test.go` must scan non-test source; if it lives in `paths_test.go` it should scan sibling `.go` files, since the existing walker explicitly skips `_test.go` (enforcement_test.go line 48). The discussion leaves scan-scope unspecified.
**Fix:** Specify the guard scans `internal/paths/*.go` non-test sources for the literal `_codeguide`.

## Verdict
GAPS_FOUND
Two gaps: launcher-side prune is effectively inert, and an existing launcher test assertion is silently invalidated.
MILL_REVIEW_END