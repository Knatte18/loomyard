MILL_REVIEW_BEGIN
# Review: board: use gitrepo as its git operator

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] Cross-lock interleaving of commit vs. push
**Section:** Decisions → push-path-collapses-onto-PushCoalesced (Consequence)
**Issue:** Today the single top-level `pushLock` held across the whole loop makes commit and push mutually exclusive across processes; the new design serializes commits under `writeLockFile` and pushes under gitrepo's `.gitrepo-push.lock` only, so process A's `PushCoalesced` `pull --rebase` (which rewrites the worktree/index) can run concurrently with process B's `commitDirty` `add -A`+commit on the same repo — a new `index.lock`/dirty-tree race that the "never less safe" claim does not cover.
**Fix:** State whether the two locks can overlap and, if so, resolve it (e.g. keep one shared lock spanning commit+push, or document/accept the transient `index.lock` failure as benign) rather than asserting equal safety.

### [NOTE] Changed concurrency is untested
**Section:** Testing
**Issue:** `sync_test.go` is migrated "as-is" and its assertions are single-process; nothing exercises the newly-separated commit/push locking, so the concurrency change above lands with no regression guard.
**Fix:** Note that no test covers concurrent `Sync` processes, and decide whether the migrated suite is a sufficient safety net for the altered locking.

## Verdict

GAPS_FOUND
Concurrency analysis for dropping the shared top-level lock is incomplete.
MILL_REVIEW_END
