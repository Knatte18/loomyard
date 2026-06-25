MILL_REVIEW_BEGIN
# Review: Move config templates home by removing the lyxtest->configreg edge

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\config-test-cleanup\_mill\discussion.md
date: 2026-06-25
```

## Findings

### [NOTE] Empirical-sweep set is open, not enumerated
**Section:** Decision: where-real-config-is-needed / Scope
**Issue:** The full set of `CopyPaired` consumers needing `SeedConfig` is deferred to a batch-1 runtime sweep; `worktree/weft_test.go` has full-`CopyPaired` cases (lines 252, 336) that push to weft-bare and are flagged "to be confirmed," so a plan writer cannot statically know the final touch-list.
**Fix:** Acceptable as designed (the rule + sweep procedure are unambiguous), but the plan should state that the consumer list is determined at implementation time, not estimable up front.

### [NOTE] buildWeftOnly singular config.yaml deferred, but it is a fixture used now
**Section:** Technical context / Decision: lyxtest-tidy-separate-batch
**Issue:** `buildWeftOnly` writes a literal `_lyx/config.yaml` ("test") at lyxtest.go:269 — a non-`paths`-helper, non-layout path — and its resolution is deferred to the tidy batch; nothing confirms no current consumer depends on that exact path before it is altered.
**Fix:** Confirm in the tidy batch that no test reads `_lyx/config.yaml` literally before aligning it; the deferral itself is fine.

## Verdict

APPROVE
Scope, decisions, constraints, and testing are complete and source-verified; only non-blocking notes remain.
MILL_REVIEW_END
