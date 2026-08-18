MILL_REVIEW_BEGIN
# Review: websterengine + webstercli told-geometry, and Webster standalone entry — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-18
```

## Findings

### [NIT:consistency] Stale field reference in a test doc comment
**Location:** `internal/webstercli/verbs_test.go:833-834`
**Issue:** `TestPersistentPreRunE_PlanDirAnchoredAtSubpath`'s doc comment describes the production call site it covers as `c.planDir = planparser.PlanDir(layout.AnchorPath()) in PersistentPreRunE`, but card 36 deleted `websterCLI.planDir` outright and card 35 moved plan-dir resolution into `wiring.go`'s `wireHub`/`wireStandalone` (via `hubgeom.WebsterGeometry`/`standalonegeom.WebsterGeometry` plus the `--plan-dir` override), so no such field or assignment exists anymore.
**Fix:** Reword the comment to describe the current call site (`c.geom.PlanDir`, built inside `wire`/`wireHub`) instead of the pre-card-35/36 field name.

## Verdict

APPROVE
All eight batches faithfully implement their cards, shared decisions, and CONSTRAINTS.md rewords; only one stale test comment found.
MILL_REVIEW_END
