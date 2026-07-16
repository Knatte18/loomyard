MILL_REVIEW_BEGIN
# Review: Fork-based cluster review in burler

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-16
```

## Findings

### [NOTE] `default` fan name collides with "never default"
**Section:** Decisions › Standard library content; Constraints › "Forking is never default"
**Issue:** A seeded fan literally named `default` that is inert unless a profile sets `cluster-fan: default` invites the exact operator misread the constraint warns against.
**Fix:** Have the plan/docs state explicitly that the `default` fan is dormant until named, or rename it (e.g. `core`) to remove the "default = active" implication.

### [NOTE] Low-core concurrency cap vs exact-N and the N=2 smoke
**Section:** Decisions › Cluster timeout; Technical context (CC cap min(16, cores−2))
**Issue:** On ≤2-core hosts the concurrency cap floors low, so forks serialize; exact-N still holds (all transcripts eventually exist) but wall-time balloons and the opt-in N=2 smoke could time out on a constrained CI runner.
**Fix:** Note that serialized forks still satisfy exact-N (only wall-time grows) and state the host/timeout floor the smoke assumes, so a slow runner isn't misread as a fork-shortfall failure.

## Verdict

APPROVE
Scope, decisions, failure modes, and tests are grounded and complete; two doc-clarity NOTEs, no gaps.
MILL_REVIEW_END
