MILL_REVIEW_BEGIN
# Review: Spike: structured Go reference/call-graph lookup (go/packages / gopls) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-17
```

## Findings

### [BLOCKING] VTA soundness-anchor false-positive count/enumeration is wrong
**Location:** `docs/research/codeintel-spike.md:210-216`
**Issue:** The doc claims VTA's 30-entry transitive-caller set for `hubgeometry.WeftHostSlug` "pads [the true chain] with 10 extra entries" listing `configcli.* ×5` among sibling `RunCLI`s. Cross-checked against the harness's own raw output (`.scratch/codeintel/weftHostSlug-callgraph-vta.json`), `configcli` actually contributes **8** distinct functions to the set (`Command$1`, `RunCLI`, `dispatch`, `editOne`, `menu`, `runConfig`, `runConfig$1`, `setModule`), not 5, and the real count of sibling-module false-positive entries is **18** (not 10); `(*cobra.Command).ExecuteContext` is also present in the raw set but goes entirely unmentioned/unaccounted in the doc's chain-vs-extras breakdown. The soundness claim itself (no missed real callers) still holds, but the specific false-positive characterization — presented as the evidentiary basis for the Defer verdict's "bounded, explainable... down to the exact repo pattern" framing — does not match the tool's own data.
**Fix:** Recount the extras directly from `weftHostSlug-callgraph-vta.json`'s `transitive_callers` list (18 sibling entries + `ExecuteContext`) and correct the enumeration and the "10 extra entries" figure in the doc.

## Verdict

REQUEST_CHANGES
Doc's sole committed deliverable has a verifiable numeric/enumeration error in its flagship VTA false-positive analysis.
MILL_REVIEW_END
