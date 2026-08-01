MILL_REVIEW_BEGIN
# Review: fabric: warp-rebase / remote-reconcile recovery

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (1M context)
reviewed_file: _mill/discussion.md
date: 2026-08-01
```

## Findings

### [NOTE] Empty correspondence index not addressed
**Section:** rebase-detection-scope / safe-vs-unsafe-reconcile
**Issue:** Detection reads "the single latest correspondence entry's Warp-SHA"; the no-surviving-anchor case covers zero *reachable* entries, but a weft with zero entries at all (no Warp-SHA trailers yet) has no latest entry to check, an undefined empty-state.
**Fix:** State the zero-entries outcome (treat as no-drift + plain ff-advance, or abort), one sentence, distinct from the zero-reachable abort.

### [NOTE] Reachability primitive vs. divergence classifier naming
**Section:** warp-refresh-primitives
**Issue:** Primitive (2) is described as comparing *local warp HEAD* to the remote ref, but detection and the anchor walk both need `is-ancestor(arbitrary-Warp-SHA, new-ref)`; the doc equates them ("same primitive") without naming the general is-ancestor method a plan reader must pin per the gitrepo Client Boundary Invariant.
**Fix:** Name the general `is-ancestor(sha, ref)` reachability method explicitly among the gitexec-bound primitives so the pinned-list obligation isn't missed.

## Verdict

APPROVE
Mature after five rounds; claims source-verified, two non-blocking edge-state notes only.
MILL_REVIEW_END