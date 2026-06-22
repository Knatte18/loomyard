Confirmed: `TestPushIntegration_RebaseRetryOnNFF` does NOT set up a non-FF remote — it's a plain commit+push happy path, exactly as the discussion claims. The doc file has the expected structure (history blocks, headline, equivalence-guardrail pattern). The discussion's claims are well-grounded throughout.

I've verified the discussion thoroughly. The scope is well-defined, all file inventories and func counts match source exactly, named tests exist, and the load-bearing redundancy claims (TestRenderTaskStatus subset, RebaseRetryOnNFF not setting up non-FF) are accurate. Decisions have rationale and rejected alternatives, constraints are covered, testing strategy is explicit. Now producing the review.

MILL_REVIEW_BEGIN
# Review: Prune and consolidate the test suite (board first)

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\prune-board-tests\_mill\discussion.md
date: 2026-06-22
```

## Findings

### [NOTE] "~40 for board" target is non-binding by design
**Section:** § board / Q&A "How aggressive?"
**Issue:** The "61 → ~40" header reads like a target but Q&A explicitly makes it an expected outcome, not a quota; a plan writer could misread it as a hard goal.
**Fix:** Already reconciled in Q&A; no change needed, but the plan should restate "no hard count quota" to avoid a reviewer treating a miss as a failure.

### [NOTE] muxpoc Fold B subtest names not preserved as original func names
**Section:** § muxpoc Fold B
**Issue:** Fold B prescribes subtest names (`prefix_and_charset`, `stable_same_input`, `differs_root_vs_subdir`) that do not match the original func names (`TestSocketName`, `TestSocketNameStability`), unlike the `preserve-names-as-subtests` decision applied elsewhere.
**Fix:** Intentional (these consolidate a func plus an inline re-check); flag the deviation in the doc's name-map so the auditable diff still resolves, or list the originals as the keeper subtests.

## Verdict
APPROVE
Scope, decisions, constraints, and testing are complete; all source claims verified; only two non-blocking notes.
MILL_REVIEW_END