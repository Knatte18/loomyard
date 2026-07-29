MILL_REVIEW_BEGIN
# Review: fabric: Fabric.Commit classify+dispatch + unified diff/status

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Async push behavior on partial failure unspecified
**Section:** `async-push-both-sides-detached` + `partial-failure-report-not-rollback`
**Issue:** The commit-then-fire-push sequence is defined only for full success; when warp lands but the weft commit fails (or the benign committed-but-unrecorded third outcome), it is unstated whether the async push still fires to push the landed warp commit, or whether the early error return skips it entirely — two observably different behaviors with different test assertions.
**Fix:** State whether `Fabric.Commit` fires the both-repo async push before returning the `*PartialCommitError` (pushing whatever landed) or skips the push on any partial failure, and add the corresponding assertion to the partial-failure integration cases.

## Verdict

GAPS_FOUND
Push-vs-error-return ordering on a partial-failure commit is undefined; one decision needed.
MILL_REVIEW_END
