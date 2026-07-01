All source claims check out against the repository. The round-1 gap (abort scope when a hard-error guard fires) is now fully resolved with an explicit decision and an implementation note pinning step ordering. No unresolved contradictions found in the mirrored-logic claims, signature changes, or file/line references.

MILL_REVIEW_BEGIN
# Review: Add lyx init --undo / deinit command

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\lyx-deinit\_mill\discussion.md
date: 2026-07-01
```

## Findings

### [NOTE] RealDirectoryGuard test setup ordering is implicit
**Section:** Testing — `TestRunInit_Undo_RealDirectoryGuard`
**Issue:** `WireJunctions`/`seedLyxJunction` (`internal/warpengine/junction.go`) hard-errors on a real non-junction `_lyx` *before* gitignore/exclude/reconcile run, so "real directory + prior init leaves gitignore/exclude/weft-content in place" is only achievable by running init successfully first, then swapping the junction for a real directory afterward — not by pre-creating the real directory before init.
**Fix:** Spell out the two-phase setup (init succeeds → replace junction with real dir → run `--undo`) in the test description so the plan writer doesn't attempt the infeasible single-phase ordering.

### [NOTE] No explicit case for commit-succeeds/push-fails then rerun
**Section:** Testing / Decisions — weft-side content commit+push
**Issue:** The `PartialRecovery` test only covers "junction removed, weft content still present"; it doesn't cover "weft deletion committed locally but push failed," which is a plausible partial state given `weftengine.Push`'s own rebase-retry logic (`internal/weftengine/sync.go`).
**Fix:** Either explicitly fold this into `TestRunInit_Undo_PartialRecovery`'s scope or note it as accepted residual risk for mill-plan.

## Verdict

APPROVE
Round-1 gap resolved; remaining items are non-blocking test-setup clarifications, not design gaps.
MILL_REVIEW_END