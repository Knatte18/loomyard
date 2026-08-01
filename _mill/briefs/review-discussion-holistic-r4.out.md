MILL_REVIEW_BEGIN
# Review: fabric: warp-rebase / remote-reconcile recovery

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-01
```

## Findings

### [GAP] Clean fast-forward warp advancement unspecified
**Section:** `warp-refresh-primitives` / `safe-vs-unsafe-reconcile` / Testing
**Issue:** The named warp primitives fetch-without-merge and classify only; the warp reset that advances local warp to the new ref is explicitly gated to the non-ff-AND-clean-local reconcile branch, and the clean-ff case is described only as a "no-op regression guard" — so on an ordinary (non-rebase) warp pull nothing advances the local warp branch/worktree, defeating the Problem section's whole "bring warp's remote state down through fabric" goal. A plan writer could implement fetch-only on clean-ff and pass every listed test.
**Fix:** Specify the clean-ff step — fast-forward the local warp branch to the fetched ref — and name the primitive it uses (reuse the reset, or a ff-advance), so `pull` actually updates warp in the common case, not just on rebase.

### [NOTE] Cross-machine anchor commits can diverge weft and stall future pulls
**Section:** `anchor-commit-propagation` / `pull-partial-failure-contract`
**Issue:** Two machines each writing an anchor commit is framed as "redundant work, not a conflict," but an unpushed local anchor commit plus another machine's pushed anchor commit leaves weft diverged; since Pull returns immediately on a weft ff-pull failure, that machine's next `Fabric.Pull` can no longer reach warp reconciliation until the weft divergence is resolved by hand.
**Fix:** Note this interaction explicitly (a reconcile-created unpushed weft commit is itself a future ff-pull hazard), or scope the manual-remedy expectation, so it isn't mistaken for pure redundancy.

## Verdict

GAPS_FOUND
Clean fast-forward warp advancement is unspecified; the pull may not update warp in the common case.
MILL_REVIEW_END
