MILL_REVIEW_BEGIN
# Review: weft engine: paths geometry, paired worktrees, lyx weft

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-19
```

## Findings

### [GAP] Host _lyx junction link has no geometry method / location
**Section:** §2 In-scope; Technical context (geometry)
**Issue:** The discussion defines `WeftLyxDir()` (the junction *target*) but never names the host junction *link* path; §2 just says "seed the `_lyx` junction (host → weft)". The five new `Layout` methods listed are all weft-side targets — none yields the host link location.
**Fix:** Add an explicit host-link geometry method (e.g. the host `_lyx` at `<WorktreeRoot>/<RelPath>/_lyx`) so spawn/teardown derive the link path through `paths`, not ad hoc.

### [GAP] removeLinks does not strip host _lyx junction at subpaths
**Section:** teardown-dirty-gate-both; Technical context (worktree module)
**Issue:** The discussion asserts `removeLinks(target)` "already removes the host `_lyx` junction," but `links.go` only scans the *immediate children* of the worktree root. At `RelPath != "."` the host `_lyx` sits at `<root>/<RelPath>/_lyx`, which is not an immediate child, so it would be left behind (Windows lock hazard the design tries to avoid).
**Fix:** Specify explicit removal of the host `_lyx` junction at its mirrored RelPath location in teardown, rather than relying on `removeLinks` of the root.

### [GAP] Junction seeding collides with pre-existing host _lyx
**Section:** §2 In-scope; exclude-ownership-split
**Issue:** `createJunction` refuses to clobber an existing link/dir (junction_windows.go). If the new host worktree already contains a real `_lyx` directory (committed in some host repos), seeding host→weft will error. The hard-require + spawn ordering does not address this case.
**Fix:** Decide and state the behavior when host `_lyx` already exists (error, skip-if-junction-correct, or move-aside) and where in the Add sequence the junction is seeded relative to portal creation.

### [GAP] baseDir for lyx weft config/pathspec resolution unspecified
**Section:** §3; weft-config-pathspec-only; Technical context (config)
**Issue:** `config.Load` reads `<baseDir>/_lyx/config/weft.yaml` and `FindBaseDir` requires `<cwd>/_lyx` to exist. The discussion says weft.yaml lives in `_lyx/config/` but never states whether `lyx weft` resolves `baseDir` from cwd (host, through the junction) or from the weft worktree — and the junction-broken status case implies `<cwd>/_lyx` may not resolve.
**Fix:** State the `baseDir` source for `lyx weft` config load and the behavior when the host `_lyx` junction is broken (status should still report, not fail to load config).

### [NOTE] Initial weft push has no rollback / partial-state contract
**Section:** weft-initial-push-at-spawn; spawn-hard-requires-weft-repo
**Issue:** `Add`'s `rollbackAdd` covers host worktree/branch/portal/launchers, but the discussion does not say how a failure of the synchronous weft push (or weft `worktree add`) rolls back the already-created weft worktree+branch and the host junction, despite the "no partial state" goal.
**Fix:** Extend the documented rollback to enumerate weft worktree, weft branch, and host junction teardown on any post-create failure.

### [NOTE] Detached push lock/lockfile path not pinned to geometry
**Section:** detached-coalesced-push; Technical context (board prototype)
**Issue:** Lock files are stated to live "under the weft worktree (`<weft>/_lyx/*.lock`)" but the gitignored pathspec is `_lyx`; board ignores `*.lock` via committed `.gitignore`. It is unspecified whether weft commits a `.gitignore` inside the junction-shared `_lyx` (which is host-visible) vs. weft-root.
**Fix:** Pin the lock-file directory and the `ensureLockfilesIgnored` target relative to the weft pathspec, confirming locks never enter the committed `_lyx` tree.

## Verdict
GAPS_FOUND
Geometry of the host junction link and its teardown are under-specified; resolve before planning.
MILL_REVIEW_END
