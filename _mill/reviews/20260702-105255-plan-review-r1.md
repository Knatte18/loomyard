I have verified all plan claims against source. Both load-bearing "single production caller" claims hold (`configengine.Set` only at `configcli.go:177`; `yamlengine.SetValues` only at `configengine/set.go:55`), warp's template contains only `branch_prefix` (so Card 10/5 orphan fixtures are valid), and the docs-gap claims are accurate. The reconcile drift-detection path already reports `removed` leaf keys, so the plan's theory (that `--set` was destroying the evidence reconcile needs) is coherent and needs no reconcile.go change.

MILL_REVIEW_BEGIN
# Review: Fix lyx config --set dropping unrecognized keys + reconcile not detecting drift — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-02
```

## Findings

### [NIT] Card 9 references test fns from an unlisted file
**Location:** Batch 3 / Card 9
**Issue:** Requirements names `TestConfigLong_MentionsEditorFallbackAndSet` and `TestConfigLong_ContainsModuleNames` (in `configcli_test.go`), which is neither in Card 9's `Context:` (`none`) nor `Edits:`; the strict Context-completeness rule flags this, though the substrings to preserve are given inline so no real cold-start is forced.
**Fix:** Add `internal/configcli/configcli_test.go` to Card 9's `Context:`, or drop the parenthetical test-name reference (keep only the "preserve EDITOR/VISUAL + module list" instruction).

### [NIT] No end-to-end test for the "reconcile detects drift" half
**Location:** Whole plan (task title's second clause)
**Issue:** The plan fixes drift-detection only indirectly (by making `--set` preserve the orphan so `reconcile`'s existing `removed` path can see it), but adds no test asserting a preserved orphan is subsequently reported by reconcile — the second symptom in the task title is never directly exercised.
**Fix:** Add one test (e.g. in Card 5 or Card 10) that seeds an orphan, runs `--set`, then asserts `configsync.ReconcileAll`/`config reconcile` reports the preserved key in `removed`, closing the loop.

## Verdict

APPROVE
Sound, well-sequenced plan; only two non-blocking nits — a Context listing gap and a missing drift-detection loop test.
MILL_REVIEW_END
