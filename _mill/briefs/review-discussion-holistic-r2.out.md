MILL_REVIEW_BEGIN
# Review: webster: rewrite for flat card list

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] Per-card SHA capture names insufficient gitrepo primitives
**Section:** Decisions → per-card-commit-and-sha-capture; Testing
**Issue:** The decision says Master captures each card's SHA "by reading git log from the batch's start SHA to the fork-reported head SHA (via internal/gitrepo: CurrentSHA/ChangedFilesSince)", but neither primitive enumerates the commit SHAs in a range — `CurrentSHA` returns HEAD and `ChangedFilesSince` returns changed files, not intermediate commits (verified in `internal/gitrepo/gitrepo.go:73,240`). In v0 (identity batcher, batch≡card) capture is trivially the head SHA, so the multi-card enumeration path is either dormant or needs a missing primitive.
**Fix:** State whether multi-card per-card-SHA enumeration is live in v0 (then name/add the git-log-range primitive it needs) or dormant like the DAG seam (then the "per-card SHA capture across a batch" test line has no shipped batcher to exercise it).

### [NOTE] SHA-bisect re-run tree-staging mechanism unspecified
**Section:** Decisions → integration-suite-fork-with-bisect
**Issue:** Bisect re-runs the `## verify:` suite at intermediate per-card SHAs, which requires staging the tree at each candidate SHA (checkout or worktree), yet the same decision rejects a separate worktree as "out of scope in v0" and does not say how a re-run reaches a historical SHA in the single worktree.
**Fix:** Note that bisect re-runs check out per-card SHAs in-place after all batches land (or explicitly defer the staging mechanism to plan phase), resolving the tension with the no-separate-worktree ruling.

## Verdict

GAPS_FOUND
One SHA-capture feasibility gap; bisect staging worth clarifying before plan writing.
MILL_REVIEW_END
