# Batch: delete-dead-methods

```yaml
task: 'fabric: collapse external API surface onto Commit — stop leaking warp/weft'
batch: delete-dead-methods
number: 5
cards: 4
verify: go test -tags integration ./internal/fabricengine/ ./internal/gitrepo/ ./internal/boardengine/
depends-on: [4]
```

## Batch Scope

Remove the warp/weft-named dead exported methods that have no production callers: delete `SyncWeft` and `RevertWithWeft` (plus their orphaned private support), and unexport `SnapshotWarpSHA` (kept for in-package Commit tests). Clean up the doc-comment cascade the deletions leave behind across `fabricengine` and `gitrepo`. CRITICAL correction verified against source: `resolveRevertTarget`/`classifyCorrespondence`/`revertResolution` are NOT orphaned — they back the live `Fabric.Diff` via `diff.go:33` and MUST be kept; only `RevertResult` and `ErrRevertRollbackFailed` are genuine `RevertWithWeft`-only orphans. `SyncWeft` and `RevertWithWeft` tests are entangled in the same `syncweft_integration_test.go`, so their removal is one card.

## Cards

### Card 18: Delete SyncWeft and RevertWithWeft with their orphans and tests

- **Context:**
  - `internal/fabricengine/diff.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/revert_test.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/fabric.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/revert.go`
  - `internal/fabricengine/diff_integration_test.go`
  - `internal/fabricengine/syncweft_integration_test.go`
  - `internal/fabricengine/weftgit_unborn_warp_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/fabricengine/syncweft.go`
- **Moves:** none
- **Requirements:** Delete the file `internal/fabricengine/syncweft.go` entirely — it defines `SyncWeft` (`:47`), the orphaned private helper `warpSHAFromTrailer` (`:109`), and the `SyncResult` type (`:23`), all with no surviving user once `SyncWeft` is gone (confirm `SyncResult`/`warpSHAFromTrailer` have no other reference via grep before deleting). In `revert.go`, delete the `RevertWithWeft` method (`:122`), the `RevertResult` struct (`:29`), and the `ErrRevertRollbackFailed` error var (`:20`); KEEP `resolveRevertTarget` (`:70`), `classifyCorrespondence` (`:53`), `revertResolution` (`:40`), `ErrStaleSHA`, and `ErrNoCorrespondence` — they are still used by `diff.go`'s `weftAnchorForWarpSHA`. Grep to confirm no residual reference to the three deleted symbols. Remove the now-dead tests: in `syncweft_integration_test.go` delete every `TestSyncWeft_*` and `TestRevertWithWeft_*` function (call sites at `:84,:125,:197,:244,:290,:470,:477,:517,:569` for SyncWeft and `:440,:482,:525,:597` for RevertWithWeft, per recon) — if the file is left with only unused helpers, delete the file; if it retains helpers used by other test files, keep it with just those helpers. In `weftgit_unborn_warp_test.go` delete the `TestSyncWeft_UnbornWarpHEAD_*` test(s) (`:118,:137`). `revert_test.go`'s `classifyCorrespondence` test bodies stay unchanged, but its file-header comment names `RevertWithWeft` (`revert_test.go:2`) and is reworded in card 20's cascade — do not delete the file. Migrate `diff_integration_test.go`'s correspondence-recording setup off `SyncWeft` onto `Fabric.Commit` (call sites `:59,:66,:103,:152`): replace each `f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{})` with `f.Commit([]string{"_lyx"}, DefaultCommitMessage, nil, SyncOptions{})`, keeping the `.Committed`/`res` assertions equivalent (use `CommitResult.WeftCommitted`). `Fabric.Commit` needs a resolvable repo-wide `fabric.yaml` — if the diff test fixture is a bare warp/weft pair without config, seed it the same way `commit_integration_test.go`'s fixtures do; run the test to confirm `Diff`/`Status` still get their correspondence coverage.
- **Commit:** `refactor(fabric): delete SyncWeft and RevertWithWeft dead methods`

### Card 19: Unexport SnapshotWarpSHA

- **Context:**
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/index.go`
- **Edits:**
  - `internal/fabricengine/snapshot.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the exported `Fabric.SnapshotWarpSHA(tag string) (string, error)` (`snapshot.go:62`) to package-private `snapshotWarpSHA`, same signature and body. It has no production caller; its only users are in-package tests. Update the three call sites in `commit_integration_test.go` (`:430`, `:457` in `TestCommit_UnchangedWeftContent_TagsStillAdvanceSnapshotBaseline`, and `:539` in `TestCommit_TagsOnly_LandsEmptyWeftCommit`) to `f.snapshotWarpSHA("raddle")`. Update `snapshot.go`'s file-header comment (which calls it "the sole exported entry point") and its method doc comment to the new casing and the trimmed `golang-comments` shape. Also update the six `Fabric.SnapshotWarpSHA`/`SnapshotWarpSHA` mentions in `internal/fabricengine/doc.go` (lines `84`, `86`, `90`, `92`, `102`, `104` — the snapshot-read-mechanism paragraphs, which describe it as "the sole exported entry point") to the unexported `snapshotWarpSHA` name and drop the "exported entry point" framing.
- **Commit:** `refactor(fabric): unexport snapshotWarpSHA`

### Card 20: Doc-comment cascade cleanup in fabricengine

- **Context:**
  - `internal/fabricengine/diff.go`
  - `internal/fabricengine/revert.go`
  - `internal/fabricengine/snapshot.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/checkout_index_refresh_test.go`
  - `internal/fabricengine/commit_partial_integration_test.go`
  - `internal/fabricengine/revert_test.go`
  - `internal/boardengine/board.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update every stale doc-comment reference to the now-deleted `SyncWeft`/`RevertWithWeft` so no comment names a symbol that no longer exists. Where the underlying behaviour survives via the kept `resolveRevertTarget`/`Fabric.Diff` path, REWORD to name the surviving path rather than deleting the sentence: `doc.go:5-7` (drop `SyncWeft`/`RevertWithWeft` from the cross-repo-operations list, e.g. to `Commit`/`Pull`); `doc.go:78` (the `Fabric.Diff` "same nearest-older bridge RevertWithWeft uses" clause → reference `resolveRevertTarget`); `doc.go:98` (the two `RevertWithWeft` mentions → `resolveRevertTarget`/`WeftSHAForWarpSHA`); `index.go:7` (drop both names); `index.go:41` (`RevertWithWeft` → surviving `classifyCorrespondence`/`Fabric.Diff` caller); `index.go:306` (`a RevertWithWeft against such an answer` → `Fabric.Diff`/`weftAnchorForWarpSHA`); `index.go:331` (`RevertWithWeft` → `WeftSHAForWarpSHA`); `index.go:357` (`RevertWithWeft(warpSHA)` → the surviving resolver); `index.go:361` (strip `RevertWithWeft/`, keep `resolveRevertTarget`); `topology.go:9` (`weft-git verbs like SyncWeft` → `Commit`); `fabric.go:7` (`Commit, SyncWeft, RevertWithWeft, Pull` → `Commit, Pull` plus `Diff`/`Status`); `boardengine/board.go:41` (`fabric.SyncWeft/fabric.RevertWithWeft` → `fabric.Commit`). Also reword three surviving-test comment mentions of the deleted method: `checkout_index_refresh_test.go:9` (`and RevertWithWeft against such an answer` → the surviving `Fabric.Diff`/`weftAnchorForWarpSHA` consumer), `commit_partial_integration_test.go:26` (drop the reference to the deleted test `TestRevertWithWeft_WeftResetFailure_RollsWarpBack` — reword to describe the warp-commit setup without naming that test), and `revert_test.go:2` (the header's `RevertWithWeft's resolution step builds on` → `resolveRevertTarget`, since `classifyCorrespondence`/`resolveRevertTarget` still exist). Grep the `fabricengine` and `boardengine` packages for any remaining `SyncWeft`/`RevertWithWeft` mention and fix it. Trim edited comments to the `golang-comments` shape.
- **Commit:** `docs(fabric): drop stale SyncWeft/RevertWithWeft comment references`

### Card 21: Doc-comment cascade cleanup in gitrepo

- **Context:**
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/gitrepo/doc.go`
  - `internal/gitrepo/reset.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the three `gitrepo` doc-comment mentions of the deleted `RevertWithWeft`: `doc.go:190` (`ResetHard is the primitive fabric's RevertWithWeft history-recovery flow`), `reset.go:2` (`RevertWithWeft's history-recovery flow builds on:`), and `reset.go:20` (`ResetHard is the primitive RevertWithWeft history recovery builds on.`). Reword each to describe `ResetHard`'s role without naming the deleted method — e.g. reference fabric's coordinated history-recovery / revert path generically. Grep the `gitrepo` package for any remaining `RevertWithWeft` and fix it.
- **Commit:** `docs(gitrepo): drop stale RevertWithWeft comment references`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/ ./internal/gitrepo/ ./internal/boardengine/` runs the surviving fabric tests — crucially `diff_integration_test.go` (whose correspondence setup migrated onto `Fabric.Commit`, proving `Diff`/`Status` keep coverage), `commit_integration_test.go` (which now calls `snapshotWarpSHA`, including the correctness-hole regression `TestCommit_UnchangedWeftContent_TagsStillAdvanceSnapshotBaseline`), and `revert_test.go` (kept `classifyCorrespondence` unit tests). The module-wide `go build ./...` boundary check confirms nothing outside these packages referenced the deleted methods. Scope covers the packages whose comments/tests changed.
