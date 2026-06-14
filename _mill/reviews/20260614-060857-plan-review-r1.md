This is `TestLoad_DotMhgoIgnored` — the guardrail test that proves `.mhgo/board.yaml` is ignored. It legitimately contains `.mhgo/board.yaml` strings (lines 105, 107). The plan's batch-1 tests note (line 142) explicitly references `TestLoad_DotMhgoIgnored` as a guardrail that "remains green untouched" and per the `docs-and-doc-comments-only` decision, test files are not changed. Correct to preserve.

However, the batch-3 handoff staleness grep (`03-tree-sweep.md` line 152) is:
```
grep -rn "\.mhgo/board\.yaml\|three-layer\|Target redesign\|not yet implemented\|layered YAML" docs/ internal/
```
This grep over `internal/` WILL match `config_test.go:105-107` (`.mhgo/board.yaml`). The plan's batch-3 Batch Tests describes the grep as "returning only legitimate runtime-state-dir references and the explicit 'now-removed config layer' mentions" — it does not enumerate the test-file `.mhgo/board.yaml` matches as expected/legitimate. This is a minor accuracy gap in the handoff verification description, not a blocking plan defect: the test references are legitimately preserved (guardrail proving `.mhgo/` is ignored), but the grep guard's "expected clean" framing omits them. Worth a NIT so the handoff reviewer doesn't treat the test-file hits as failures or attempt to "fix" them.

Everything else is accurate and complete. Let me compile the review.

MILL_REVIEW_BEGIN
# Review: Fix stale .mhgo/ config-layer docs + cross-cutting stale-docs sweep — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-14
```

## Findings

### [NIT] Handoff grep will hit guardrail test references
**Location:** Batch 3 (03-tree-sweep.md) Batch Tests; Card 7 / Decision `docs-and-doc-comments-only`
**Issue:** The handoff staleness grep `grep -rn "\.mhgo/board\.yaml\|...\|not yet implemented\|layered YAML" docs/ internal/` will match `internal/config/config_test.go:105-107` (the `.mhgo/board.yaml` strings inside `TestLoad_DotMhgoIgnored`, which legitimately prove `.mhgo/` is ignored and must NOT be edited), yet the batch-3 note describes the grep as returning "only legitimate runtime-state-dir references and the now-removed config layer mentions" — it omits these test-file hits.
**Fix:** Add `internal/config/config_test.go`'s `TestLoad_DotMhgoIgnored` `.mhgo/board.yaml` lines to the enumerated set of expected-legitimate grep hits so the handoff reviewer treats them as guardrail noise, not stale docs to fix.

## Verdict

APPROVE
Plan is accurate, complete, and DAG-valid; the lone NIT is a handoff-grep documentation gap.
MILL_REVIEW_END
