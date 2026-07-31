MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: /home/knatte/Code/loomyard/wts/trace-logging/_mill/discussion.md
date: 2026-07-31
```

## Findings

### [NOTE] Count-bound "newest 50" sort key not stated
**Section:** Decisions → retention
**Issue:** The age bound explicitly reads the filename timestamp (not mtime), but the count bound ("keep the newest 50") never names its ranking key, leaving room to sort by mtime and reintroduce the exact long-running-file hazard the age bound was written to avoid.
**Fix:** State that the count bound also ranks by the filename timestamp segment, same as the age pass.

### [NOTE] Liveness-skip vs. count budget interaction unspecified
**Section:** Decisions → retention (liveness rule) + Testing → Retention liveness rule
**Issue:** Whether pid-live files that are skipped by the liveness rule still consume the "newest 50" budget is not stated; both readings are safe (a live file is never unlinked either way), but the retention test cannot be written deterministically without picking one.
**Fix:** Note whether live-skipped files count toward the 50, so the count-bound test has a definite expectation.

### [NOTE] Header worktree-root field under the test seam
**Section:** Decisions → sink-open-triggers / lazy-sink-open; Testing → "Sink naming and lazy open"
**Issue:** The header's worktree-root field is filled "once geometry resolution has actually run," but the untagged test seam points the sink at a `t.TempDir()` and never resolves geometry, leaving that field's value on the seam path undefined (the header tests only assert command + trace-ID, so it is unpinned).
**Fix:** Say what the worktree-root field holds when the sink is opened via the seam (e.g. left empty, or derived from the seam directory).

## Verdict

APPROVE
Complete and internally consistent; three non-blocking clarifications on retention ordering and header composition.
MILL_REVIEW_END
