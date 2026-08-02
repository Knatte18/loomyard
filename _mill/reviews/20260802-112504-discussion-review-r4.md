MILL_REVIEW_BEGIN
# Review: fabric: collapse external API surface onto Commit — stop leaking warp/weft

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-02
```

## Findings

### [GAP] SnapshotWarpSHA drop guts kept Commit regression coverage
**Section:** Decision `dead-methods-diff-status-kept` / Testing
**Issue:** The decision drops `SnapshotWarpSHA` on a "no callers" premise, but the KEPT in-package `commit_integration_test.go` calls `f.SnapshotWarpSHA` to assert correspondence recording — including `TestCommit_UnchangedWeftContent_TagsStillAdvanceSnapshotBaseline` (:405-462), described in-file as "the correctness-hole regression this whole batch exists to close"; deleting the method removes that assertion tool, and the testing section treats its tests only as orphaned `snapshot*_test.go`.
**Fix:** Correct the premise and specify `SnapshotWarpSHA` is *unexported* (retained as `snapshotWarpSHA`, still callable by the in-package Commit tests), not deleted — parallel to the already-flagged `SyncWeft`/`diff_integration_test.go` cross-dependency — or migrate those Commit-test assertions onto another correspondence-read path.

### [NOTE] Drops cascade into stale doc comments and orphaned helpers
**Section:** Scope / Documentation Lifecycle
**Issue:** Dropping `SyncWeft`/`RevertWithWeft` leaves stale doc-comment references (`doc.go:6-7` headline cross-repo list, `boardengine/board.go:41` "routed through fabric.SyncWeft/fabric.RevertWithWeft", plus `index.go`/`topology.go`/`fabric.go`) and orphaned private helpers/types (`resolveRevertTarget`, `RevertResult`, `ErrRevertRollbackFailed`); scope enumerates only `doc.go:80` for revision.
**Fix:** Note that the three drops cascade into their doc comments and dead private helper/type cleanup, enumerating the stale references beyond `doc.go:80`.

## Verdict

GAPS_FOUND
One kept-path test dependency on a dropped method is unaddressed; otherwise thorough.
MILL_REVIEW_END
