MILL_REVIEW_BEGIN
# Review: webster: rewrite for flat card list

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] Bisect checkout has no gitrepo primitive
**Section:** Decisions → integration-suite-fork-with-bisect / gitrepo-verification-substrate
**Issue:** Bisect "checks that SHA out in-place in the single worktree, then restores HEAD" — a live v0 path (identity batcher yields N per-card SHAs) — but gitrepo (verified: CurrentSHA/StageAndCommit/StageAllAndCommit/SHAExists/ChangedFilesSince/SnapshotSHA) has no checkout/restore primitive, contradicting the "every primitive exists today / v0 adds no new gitrepo method" claim.
**Fix:** State whether v0 adds a gitrepo checkout/restore primitive (and reconcile with the no-new-method assertions) or defers bisect execution itself, not just its "exact mechanism."

### [NOTE] Who executes verify during bisect is unspecified
**Section:** Decisions → integration-suite-fork-with-bisect
**Issue:** The integration suite is "one dedicated final fork," but the bisect re-runs of `## verify:` at candidate SHAs are not attributed — webster in-process (Go running the command) vs. spawning a fork per candidate (extra LLM spawns) is left open.
**Fix:** Name the executor of the logarithmic bisect re-runs, since it drives cost and the "no concurrent forks" guarantee.

### [NOTE] Deviation-list producer left implicit
**Section:** Decisions → fork-return-contract
**Issue:** The deviation list (changed files outside the batch's declared file-ops union) is "returned by the fork," yet Master already has `ChangedFilesSince` to derive it independently; whether the fork computes it or Master cross-checks/overrides is not pinned.
**Fix:** Note that the deviation list is fork-reported and whether Master recomputes it (both are informational), so the plan writer knows the source of truth.

## Verdict
GAPS_FOUND
Bisect's in-place checkout needs a gitrepo primitive that does not exist, contradicting the no-new-method claim.
MILL_REVIEW_END
