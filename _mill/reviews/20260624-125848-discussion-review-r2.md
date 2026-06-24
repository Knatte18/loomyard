Confirmed: the weft prime is initialized with branch `main`, mirroring the host's `main`. The naming-mirror invariant holds in the fixture, and host branch name = weft branch name is consistent.

I have verified the core claims. The discussion is well-grounded against source. The r1 gaps were resolved. All decisions have rationale and rejected alternatives. Testing uses discriminating merge-base assertions. The signature-ripple and enforcement-test concerns are addressed and verified. Scope in/out is crisp.

Let me consider whether any genuine GAP remains. One area: the discussion specifies the host-branch resolution command (`rev-parse --abbrev-ref HEAD`) and the detached-HEAD guard (returns literal `"HEAD"`), but the "unborn branch" detection mechanism is slightly less precise — `rev-parse --abbrev-ref HEAD` on an unborn branch actually returns the branch name (e.g. `main`) with a non-zero exit, not `"HEAD"`. The discussion does say "or fails (unborn branch), abort." This is a minor implementation nuance, not a scope gap, and the discussion covers it under "fails." I'll note it as a NOTE rather than a GAP since it's a plan-time detail already gestured at.

This is a discussion review, and the bar is "must resolve before plan writing can proceed." Nothing blocks plan writing. I'll record one NOTE and approve.

MILL_REVIEW_BEGIN
# Review: Ensure weft branches are orphan branches

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-24
```

## Findings

### [NOTE] Unborn-branch detection differs from detached
**Section:** Decisions → detached-head-guard / Technical context
**Issue:** `git rev-parse --abbrev-ref HEAD` returns the literal `"HEAD"` for detached HEAD, but for an unborn branch it returns the branch name with a non-zero exit code — two distinct signals the plan must handle separately, which the discussion folds into "or fails."
**Fix:** Note in the plan that the guard must treat both `"HEAD"` output and non-zero exit as abort triggers, so unborn-branch is not mistaken for a valid start-point.

## Verdict

APPROVE
Scope, decisions, constraints, and discriminating tests are complete and source-verified; r1 gaps resolved.
MILL_REVIEW_END