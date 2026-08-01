# Batch: reconcile-declarative-convergence

```yaml
task: 'fabric: clone-does-everything + subpath-in-weft + init dissolution'
batch: reconcile-declarative-convergence
number: 2
cards: 5
verify: go test -tags integration ./internal/fabricengine/...
depends-on: [1]
```

## Batch Scope

This batch makes junction wiring a declarative convergence to the **repo-wide** `pathspec` and moves the pathspec read base to `BoardDir`. Today `checkJunctionHealth` and `Reconcile` load the junction name-set from each pair's own weft base (`filepath.Join(hostLayout.WeftWorktree(), hostLayout.RelPath)`); after this batch they load it from `hubgeometry.BoardDir(l.Hub)` — one repo-wide source so `reconcile` converges every worktree to the same set. It adds the missing half of convergence: **stale-removal** (scan on-disk link entries under the host worktree root, exclude `HubReservedNames()`, unwire any absent from `pathspec`), with a fail-closed guard so a missing/unparseable repo-wide `fabric.yaml` aborts stale-removal and touches nothing.

External interface later batches consume: the repo-wide-name helper and the on-disk link-scan helper (batch 6's `unwire` reuses the scan; batch 4's clone reuses the repo-wide names via `WiredNames(BoardDir)`). Depends on batch 1 because `Reconcile`/`Status` build each `hostLayout` via `hostLayoutFor`→`SiblingLayout`, which must return the correct subpath `RelPath` (batch 1) for a subpath-anchored hub — otherwise the scan and wiring target the wrong subpath.

Batch-local decision: introduce `repoWideFabricBase(l *hubgeometry.Layout) string { return hubgeometry.BoardDir(l.Hub) }` in `junctionnames.go` as the single place the repo-wide fabric base is named, and route the name-set reads through it. The stale-removal helper lives in `reconcile.go`.

## Cards

### Card 6: Repo-wide fabric base helper + retarget the name-set reads to BoardDir

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/fabricengine/junctionnames.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/junctionnames.go`, add `func repoWideFabricBase(l *hubgeometry.Layout) string` returning `hubgeometry.BoardDir(l.Hub)` — the single named source of the repo-wide fabric config base (the `weft:main` checkout holding `_lyx/config/fabric.yaml`). Add a Layout-taking convenience `func RepoWiredNames(l *hubgeometry.Layout) ([]string, error)` returning `WiredNames(repoWideFabricBase(l))`, so callers that hold a `*hubgeometry.Layout` read the repo-wide junction name-set without re-deriving the base. Keep the existing `junctionNames(baseDir)`/`WiredNames(baseDir)`/`filterHubReserved` functions unchanged (clone in batch 4 still calls `WiredNames(BoardDir)` directly with an explicit base). Update the package/file doc comment where it references reading the pair's own weft base to note the repo-wide base is now the source for reconcile/status.
- **Commit:** `feat(fabricengine): add repo-wide fabric base + RepoWiredNames`

### Card 7: Switch `checkJunctionHealth` and `Reconcile` to the repo-wide name-set

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/hostlayout.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `checkJunctionHealth(hostLayout *hubgeometry.Layout)` (reconcile.go:340), replace the name-set load `junctionNames(filepath.Join(hostLayout.WeftWorktree(), hostLayout.RelPath))` (reconcile.go:341) with `RepoWiredNames(hostLayout)` (repo-wide base). In `Reconcile` (reconcile.go:95), replace the per-pair name-set load used before the `WireJunctions(hostLayout, slug, names)` call (reconcile.go:161) with `RepoWiredNames(hostLayout)` so wiring converges to the repo-wide set. Preserve the load-bearing behavior documented at reconcile.go:156-160 (names must be a real config load, not `t.cfg`) — `RepoWiredNames` is still a fresh config load, just from `BoardDir` instead of the per-pair weft base. Verify `status.go`'s use of `checkJunctionHealth` (status.go:149) still compiles unchanged (the signature is untouched); if `status.go` independently loads a name-set for health, retarget it to `RepoWiredNames` too. Update the load-bearing comment text at reconcile.go:156-160 to say the name-set comes from the repo-wide `BoardDir` base. **Also, in `internal/fabricengine/hostlayout.go`, change `hostLayoutFor`'s non-sibling fallback (hostlayout.go:25) from `hubgeometry.Resolve(worktreeRoot)` to `hubgeometry.ResolveWorktree(worktreeRoot)` (batch 1's gate-free resolver):** the fallback resolves *another* worktree from its root (above a subpath anchor), so the gated `Resolve` would now hard-error with `ErrCwdOutsideAnchor` for a subpath-anchored hub — the gate-free `ResolveWorktree` reads the recorded anchor for `RelPath` without the cwd-legitimacy check, which is exactly the geometry-derivation contract this fallback needs. Update hostlayout.go's doc comment (hostlayout.go:14-20) accordingly (both paths stay byte-equivalent to `ResolveWorktree(worktreeRoot)`, not the gated `Resolve`).
- **Commit:** `refactor(fabricengine): read junction name-set from repo-wide BoardDir`

### Card 8: On-disk link-scan helper (children minus reserved names)

- **Context:**
  - `internal/fslink/fslink.go`
  - `internal/fslink/fslink_linux.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `reconcile.go`, add `func scanOnDiskJunctionNames(worktreeRoot, relPath string) ([]string, error)` that lists link entries directly under `filepath.Join(worktreeRoot, relPath)`: `os.ReadDir(dir)`, and for each entry call `fslink.IsLink(filepath.Join(dir, entry.Name()))`; collect the names where `IsLink` reports true. Exclude every name in `hubgeometry.HubReservedNames()` (`_board`/`_portals`/`_launchers`/`_raddle`) — those are hub-structural, never per-worktree junctions, and must never be swept. Return the collected fabric-junction names. Model the child-scan on `fslink.RemoveLinksIn` (fslink.go:50) read-only (do not remove anything here). Document the known caveat (weftwiring.go:153-157): this scans only the immediate children of the `worktreeRoot/relPath` directory — the correct granularity for fabric junctions, which are direct children of the anchored subpath.
- **Commit:** `feat(fabricengine): add on-disk junction link scan for stale-removal`

### Card 9: Declarative stale-removal step in Reconcile (fail-closed)

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Reconcile` (reconcile.go:95), after the add-missing wiring for a healthy-or-repaired pair, add a stale-removal step that converges the pair to the repo-wide `pathspec`. For the pair's `hostLayout`:
  1. Load the desired set: `desired, err := RepoWiredNames(hostLayout)`. **Fail-closed:** if `err != nil`, abort stale-removal for this pair, touch nothing, and record the abort in the pair result Detail (e.g. `"stale-removal skipped: cannot load repo-wide fabric.yaml: <err>"`) — never interpret an errored/empty `pathspec` as "remove every junction". Add-missing may already have run only when a valid set loaded.
  2. Scan on-disk: `onDisk, err := scanOnDiskJunctionNames(hostLayout.WorktreeRoot, hostLayout.RelPath)`; on scan error, record and skip removal (do not blanket-remove).
  3. Compute `stale := onDisk \ desired` (names present on disk, absent from the desired repo-wide set). For each stale name, unwire it via the existing single-junction primitives: `removeHostJunction(hostLayout, slug, []string{name})` (weftwiring.go:137, best-effort host-link removal) followed by removing its git-exclude entry (mirror `UnwireJunctions`'s `unseedGitExclude` for that name). Record removed names in the pair Detail.
  Reserved names are already excluded by `scanOnDiskJunctionNames`, so a `_board`/`_raddle` link on disk is never in `stale`. Keep the whole convergence within the existing per-worktree loop; a load failure aborts convergence for that one pair only, not the whole sweep.
- **Commit:** `feat(fabricengine): declarative stale junction removal in Reconcile`

### Card 10: Reconcile convergence tests (stale-removal, fail-closed, reserved, converge-all)

- **Context:**
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/template.yaml`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/reconcile_stale_removal_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/reconcile_stale_removal_test.go` (first line `//go:build integration`) covering the new convergence:
  - a junction in the repo-wide `pathspec` but missing on disk is wired (add-missing, existing behavior still holds);
  - a junction on disk but absent from the repo-wide `pathspec` is **removed** (new stale-removal behavior);
  - a correct junction is a no-op;
  - the sweep converges **all** worktrees to the one repo-wide `pathspec` — seed a repo-wide `fabric.yaml` at `<BoardDir>/_lyx/config/fabric.yaml` adding a third name (e.g. `_raddle` is reserved, so use a non-reserved custom name like `_extra`), run `Reconcile`, assert every host worktree gained `_extra`;
  - **fail-closed:** with the repo-wide `fabric.yaml` absent or unparseable, `Reconcile` strips NO junction (assert every pre-existing junction still present) and reports a skip/abort outcome, never a blanket sweep;
  - a hub-structural reserved name present on disk (`_board`/`_portals`/`_launchers`/`_raddle`) is **never** removed even though absent from `pathspec`.
  Build synthetic multi-worktree hubs with `lyxtest`; write the repo-wide fabric config via `hubgeometry.ConfigFile(hubgeometry.BoardDir(hub), "fabric")`. Reuse fixture patterns from `reconcile_stale_registration_test.go`.
- **Commit:** `test(fabricengine): cover declarative junction convergence and fail-closed guard`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/...` runs the whole fabricengine package. Whole-package scope is justified: this batch edits `reconcile.go`/`status.go`/`junctionnames.go` — shared files exercised by many integration tests (`reconcile_stale_registration_test.go`, `junction_repoint_test.go`, `junction_pattern_integration_test.go`, `remove_junctions_integration_test.go`, `config_driven_junctions_integration_test.go`, `status`-related tests) plus the new `reconcile_stale_removal_test.go` — and the base-switch to `BoardDir` could regress any of them. The `-tags integration` superset also runs the untagged unit tests (`junctionnames_test.go`, `junction_test.go`, `weftwiring_test.go`, `add_test.go`). This is the natural granularity for a change to fabric's convergence core.
