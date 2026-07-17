MILL_REVIEW_BEGIN
# Review: Spike: structured Go reference/call-graph lookup (go/packages / gopls)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] Transitive ground-truth method unspecified
**Section:** Testing / recommendation-rubric (callgraph sub-verdict)
**Issue:** Direct-reference precision is graded against grep+manual ground truth, but the testing section demands CHA/RTA/VTA "precision against ground truth" without saying how a *transitive* caller set is hand-verified — manually enumerating a transitive graph is impractical, and the rubric elsewhere frames it as relative divergence (VTA as gold).
**Fix:** State whether transitive precision is measured against a hand-built ground truth (and for which one symbol) or against VTA-as-reference / inter-algorithm divergence.

### [NOTE] Revert/cleanup is stated but not a discrete plan step
**Section:** Scope (Out) / Technical context (Dependencies)
**Issue:** "Harness deleted, `x/tools` reverted, `.lsp.json` removed before merge" is asserted but not machine-checked; go.sum is not mentioned alongside go.mod, and nothing verifies the branch's final diff is doc-only.
**Fix:** Have the plan carry an explicit final "revert harness + go.mod/go.sum + .lsp.json, confirm merge diff is only docs/research/codeintel-spike.md" step.

### [NOTE] CC-native LSP driving feasibility in this env
**Section:** cc-native-lsp-mismatch / Technical context
**Issue:** Driving `ENABLE_LSP_TOOL=1` requires an interactive CC session with the tool enabled and gopls installed; the discussion timeboxes misbehavior but doesn't confirm the spike author can actually toggle that env and restart in this harness environment.
**Fix:** Note the docs-only fallback is the accepted outcome if the tool cannot be enabled at all here, not just if it "misbehaves."

## Verdict

GAPS_FOUND
One methodological gap (transitive ground truth); otherwise thorough and well-grounded.
MILL_REVIEW_END
