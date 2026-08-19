# Batch: fabric-add-and-launcher

```yaml
task: 'loom: session bootstrap'
batch: fabric-add-and-launcher
number: 2
cards: 6
verify: go test ./internal/fabricengine/ && go test -tags integration ./internal/fabricengine/
depends-on: [1]
```

## Batch Scope

This batch wires batch 1's two primitives into the `add` verb and adds the third launcher script.
`Topology.Add` gains one write-and-commit step for the origin record, placed between junction wiring and the two pushes so the existing weft push carries the commit with no new push call; `writeLaunchers` gains a `run` script beside `ide` and `fabric-checkout`, and `removeLaunchers`' hardcoded script-name list gains the matching entry.
It is one batch because all four production edits land in the same two files and are proved by one integration suite over the same `add`/`remove` round trip.

The external interface batch 5 consumes: nothing new — batch 5 calls batch 1's functions directly.
What this batch delivers to the operator is that every pair created from now on carries its own recorded parent branch and its own double-click run launcher.

Batch-local decisions beyond `## Shared Decisions` in the overview:

- `rollbackAdd` gains no new step.
  Its existing weft-worktree removal already takes the record with it on the created-branch path, and on the adopted-weft-branch path the record commit is deliberately left in place — reverting or resetting an adopted branch to undo one commit is exactly the pre-existing-history destruction that rollback's own `!weftBranchAdopted` guard exists to prevent.
- The run launcher invokes the explicit two-word verb, not the root alias, so it keeps working regardless of what happens to the alias.

## Cards

### Card 5: Add writes and commits the origin record

- **Context:**
  - `internal/fabricengine/origin.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/fabricengine/add.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Topology.Add`, immediately after the `WireJunctionsWith` call that ends step 10b and before step 11's warp push, add one new step that records the pair's provenance.
  It calls `WriteOrigin(rec, l, slug, Origin{ParentBranch: parentBranch})`, reusing the `parentBranch` local the function already computes from `rev-parse --abbrev-ref HEAD` and today uses only to derive `parentWeftBranch` — do not re-derive it.
  On error it takes the same shape every step in this window already takes: call `t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)` with its return discarded, then return a zero `AddResult` and a wrapped error naming the record.
  It then commits the record on the new pair's weft branch with `CommitWeftPaths(weftPath, l.AnchorRel, []string{OriginRecordRel()}, "fabric: record parent branch for "+slug, opts)`, using the `weftPath` local already bound earlier in the function, and rolls back the same way on error.
  The commit's `sha`/`committed` returns are deliberately discarded and no `KindCommitCreated` is appended — state that in a comment, with the reason from the overview's `origin-record-carries-one-KindFileWritten-and-no-commit-record` decision.
  Add a comment at the new step explaining the placement: after junction wiring so the pair is fully wired, and before step 12's `pushWeftBranch` so the existing push carries the commit to the remote with no new push call.
  Update the file-header comment and `Topology.Add`'s own doc comment to mention the recorded provenance step.
  In `rollbackAdd`'s doc comment, add one sentence stating that the origin record needs no removal step of its own — on the created-branch path the existing weft-worktree and weft-branch removal takes it, and on the adopted path the commit is deliberately retained.
  Do not add a new removal call anywhere in this file.
- **Commit:** `feat(fabricengine): record and commit the pair's parent branch inside add`

### Card 6: writeLaunchers writes the run launcher

- **Context:**
  - `internal/fabricengine/launcher_content.go`
  - `internal/fabricengine/mutation.go`
- **Edits:**
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/launcher_content_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `writeLaunchers`, after the `fabric-checkout` script is written and before the never-clobber menu-launcher block, add a third script written by the identical mechanism: build its bytes and mode with `launcherScript(runtime.GOOS, spawnRel, "loom run")` reusing the `spawnRel` local already computed for the `ide` launcher, join its path as `filepath.Join(launcherDir, "run"+ext)`, and write it through the existing `writeLauncherScriptIfChanged(rec, root, l.HubPath, ...)` call so it inherits the same hub-rooted containment, the same record-only-on-changed-bytes rule, and the same repair-path idempotence.
  Wrap a write failure with the same message shape the two existing scripts use.
  Update the file-header comment and `writeLaunchers`' doc comment so the enumerated script set names all three.
  In `launcher_content_test.go`, add a case to the existing content table pinning the bytes and mode the run launcher's own lyx-argument string produces, on both the Windows and the non-Windows branch, so the double-click surface's exact command line is asserted rather than assumed.
- **Commit:** `feat(fabricengine): drop a run launcher beside ide and fabric-checkout`

### Card 7: removeLaunchers tears the run launcher down

- **Context:**
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/portallauncher_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `removeLaunchers`, add the run launcher's basename to the hardcoded script-name slice the removal loop iterates, so the slice reads as the three names `ide`, `fabric-checkout` and `run`, each suffixed with `ext`.
  Add a comment stating why this list is a mandatory edit point rather than a leak: the launcher-directory removal that follows is non-recursive, so a script left off this list makes the directory non-empty and fails `Remove`/`rollbackAdd` outright rather than merely orphaning a file.
  In `portallauncher_test.go`, extend `TestRemoveLaunchers_PreservesForeignContent` so its seeding loop writes all three scripts and its post-removal assertions confirm the run launcher was removed alongside the `ide` one, keeping the existing operator-file preservation assertion unchanged — this is the regression guard for the list.
- **Commit:** `fix(fabricengine): tear the run launcher down with the rest of the set`

### Card 8: Integration coverage for the record, its commit, and the launcher set

- **Context:**
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/origin.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/add_branch_exists_test.go`
  - `internal/fabricengine/commit_lock_integration_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/fabricengine/launchers_containment_integration_test.go`
  - `internal/fabricengine/destroy_containment_toctou_integration_test.go`
- **Creates:**
  - `internal/fabricengine/origin_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** The new file carries the `integration` build constraint as its first non-empty line and a `TestMain` is already provided by the package, so add none.
  Cover, each as its own test function against a real hub built the way this package's existing integration tests build one:
  (a) `add` on a pair forked from a non-default warp branch writes the record with that branch as `parent_branch`, readable back through `ReadOrigin` from the acting worktree;
  (b) the same, in a subpath-anchored hub, landing at the anchor-relative path rather than at the weft worktree root;
  (c) the record is committed on the weft branch — assert it is present in that branch's tree, not merely on disk;
  (d) `CommitWeftPaths` serialises rather than races against a concurrently-held weft write lock, driven the way the existing weft-commit lock integration test drives its own contention case;
  (e) a forced post-write failure on the created-branch path leaves no record and no stray commit;
  (f) a forced post-write failure on the adopted-weft-branch path leaves the record commit in place on the preserved pre-existing weft branch and leaves that branch otherwise untouched — assert the retained-commit outcome explicitly, never merely tolerate it;
  (g) the `AddResult` mutation snapshot contains exactly one `KindFileWritten` entry whose target is the record;
  (h) after `add` the run launcher exists in the per-slug launcher directory and after the matching `remove` neither it nor the launcher directory survives, and that removal succeeds rather than being refused.
  For (e) and (f), force the failure at a step that runs after the record step — the warp push is the nearest one — using whichever failure-injection seam the existing rollback tests already use rather than inventing a new one.
  In `launchers_containment_integration_test.go`, add the run launcher's basename to the loop asserting no launcher landed outside the hub.
  In `destroy_containment_toctou_integration_test.go`, add the same basename to the canary list.
- **Commit:** `test(fabricengine): cover the origin record, its commit, rollback, and the run launcher`

### Card 9: Live-state matrix verification for the add verb

- **Context:**
  - `internal/fabricengine/livestate_matrix_test.go`
  - `internal/fabricengine/livestate_verbs_test.go`
  - `internal/fabricengine/livestate_mutationoracle_test.go`
  - `internal/fabricengine/livestate_manifest_test.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/origin.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run the live-state matrix cells for the `add` verb at both anchors and confirm the mutation oracle still passes in both directions with no assertion loosened.
  The expectation is that it passes unchanged: the omission direction is covered because the pair's own worktree-creation entries already cover every path beneath both worktree roots, including the second path the warp junction exposes for the same file, and the commission direction is covered because the record's own entry names a path outside the git metadata directory that does produce a manifest change.
  If a cell fails, report the exact oracle message and the enumerated adjustment needed rather than relaxing the oracle, changing its permitted-path list, or widening any coverage rule — a loosened assertion here is a review blocker, not a fix.
  This card writes no code and produces no diff; its whole output is the confirmation, or the enumeration if the confirmation does not hold.
- **Commit:** none

### Card 10: Correct the design doc's stale run-launcher paragraph

- **Context:**
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/launcher_content.go`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the "Entry point — the session bootstrap" section, rewrite the paragraph headed "The run-launcher." so it describes what this task actually ships.
  It must state that the launcher is a third script in the existing per-slug hub launcher directory, written by the same builder and torn down by the same pair as the `ide` and `fabric-checkout` scripts, cross-platform by the same GOOS-selected extension, and that it embeds no absolute path because it climbs relatively to the worktree subpath — so nothing is machine-bound.
  Delete the sentence naming a machine-local, untracked, absolute-path-embedding script in the worktree; that mechanism does not exist in this repo and is not what lands.
  Keep the existing cwd-authoritative and launcher-geometry cross-references intact, including their link targets, since the Markdown Link Integrity guard resolves both the file part and the anchor of every link in this file.
  Write in this repo's markdown style: one sentence per line, no fixed-column hard wrapping.
- **Commit:** `docs(loom): correct the run-launcher paragraph to the shipped mechanism`

## Batch Tests

`verify: go test ./internal/fabricengine/ && go test -tags integration ./internal/fabricengine/` runs both tiers of the one package this batch's production edits touch.
The untagged half covers card 7's edited `portallauncher_test.go` guard and card 6's launcher-content path.
The integration half is mandatory rather than optional here: card 8 edits two `//go:build integration` files and creates a third, and card 9's live-state matrix cells are themselves integration-tagged, so an untagged-only run would exercise none of this batch's real subject.
The full integration suite for this package is run rather than a `-run` subset because the origin write lands inside the shared `add` path that most of that suite drives, so a narrower filter would miss the regressions this batch is most likely to cause.
