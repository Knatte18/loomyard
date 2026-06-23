All claims hold up. The discussion is internally consistent and well-grounded. The only inaccuracy I found is the `.WeftBare` grep claim in the discussion (it says "zero `.WeftBare` references in `*_test.go`" but TestCopyPaired in lyxtest_test.go has two). This is a NOTE — the consequential narrower claim ("no SkipPush test reads .WeftBare") is correct, and the lean variant is for SkipPush worktree tests, not the lyxtest TestCopyPaired which stays on the full fixture.

I have verified the load-bearing claims across all four workstreams. The discussion is thorough, all decisions have rationale + rejected alternatives, scope in/out is explicit, failure modes (ambient-env leakage, rollback) are addressed, testing strategy is named per workstream, and r1/r2 GAPs are resolved. I'll produce my review.

MILL_REVIEW_BEGIN
# Review: Speed up and stabilize the integration test tier

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-23
```

## Findings

### [NOTE] `.WeftBare` grep claim is overbroad
**Section:** Technical context (Fixtures, workstream C) / Q&A r2
**Issue:** Discussion states "grep: zero `.WeftBare` references in `*_test.go`", but `internal/lyxtest/lyxtest_test.go:114-115` (`TestCopyPaired`) reads `fixture.WeftBare` twice.
**Fix:** Narrow the claim to its true, load-bearing form — no `SkipPush:true` *worktree* test reads `.WeftBare`; `TestCopyPaired` stays on the full fixture and is unaffected.

## Verdict

APPROVE
All four workstreams are source-grounded and decision-complete; one minor grep-claim inaccuracy, non-blocking.
MILL_REVIEW_END