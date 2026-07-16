MILL_REVIEW_BEGIN
# Review: Fork-based cluster review in burler — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-16
```

## Findings

### [NIT] Audit error message overstates the enforced git rule
**Location:** `internal/burlerengine/cluster.go:81-87`
**Issue:** The hard-error message reads "a fork reviewer must never run any git command," but `auditClusterRound` (and `mutatingGitPattern`) only flags state-*mutating* git subcommands — `git log`/`git diff`/`git status` are deliberately allowed through per the plan and `cluster_test.go`'s own matrix. The prose instructions to the fork (prompt.go's `clusterRulesBlock`, "never run any git command") are broader than what the backstop actually enforces.
**Fix:** Reword the error to "ran a git-mutating command" (drop the "must never run any git command" generalization), or note in the doc comment that read-only git is intentionally out of scope for the audit backstop.

## Verdict

APPROVE
Implementation matches the plan precisely across all 5 batches: types, wiring, tests, docs, and CONSTRAINTS all align.
MILL_REVIEW_END
