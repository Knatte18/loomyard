MILL_REVIEW_BEGIN
# Review: fabric: unify warp + weft into one git-coordination module

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [NOTE] Stale Q&A entry contradicts superseding decision
**Section:** Q&A log (adhoc orchestrator review, "coordinate how?")
**Issue:** That entry still asserts "the write lock serializes every commit path incl. `StageAllAndCommit`", which the "Most git mechanics grow into gitrepo" decision and the post-handoff Q&A explicitly reverse (no gitrepo-level write lock; fabric never calls `StageAllAndCommit`).
**Fix:** Mark the older Q&A entry as superseded (or drop the write-lock clause) so a plan writer does not follow the retracted "gitrepo-level lock" line.

## Verdict

APPROVE
Scope, decisions, failure modes, and testing are complete; only one stale log line, non-blocking.
MILL_REVIEW_END
