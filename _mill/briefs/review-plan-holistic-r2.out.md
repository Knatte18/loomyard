MILL_REVIEW_BEGIN
# Review: planparser owns the plan-directory path — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-4.5 (best-effort self-assessment; harness-declared model ID is claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-17
```

## Findings

### [NIT:design] Validate's worktreeRoot rationale misstates current call sites
**Location:** 00-overview.md, Shared Decision "`planparser.Validate`'s `worktreeRoot` parameter is left untouched"
**Issue:** The rationale claims the two production callers "pass different roots" (`webstercli/validate.go` vs `websterengine/runlevel.go`'s `deps.WorktreeRoot`), so "at most one of them is right" — but `internal/webstercli/run.go:71` sets `RunDeps{..., WorktreeRoot: c.layout.AnchorPath()}`, the exact value `validate.go` passes directly, so today both callers already agree; there is no live divergence to adjudicate.
**Fix:** Reword the rationale to state the risk is a *future* change to either call site diverging, not that they currently disagree — the decision to leave the parameter untouched is still correct, only the stated reason is inaccurate.

### [NIT:consistency] notransients_test.go's PlanDir/PlanOverview rows get no weakening annotation
**Location:** Batch 2, Card 10 (`cmd/lyx/notransients_test.go`)
**Issue:** The "annotated as weakened" Shared Decision covers `constructoranchoring_test.go`'s two rows and `cli_test.go`'s `newTestCLI` flip, explaining that each is self-consistent under a wrong root and so a future reader must not over-trust it — but `notransients_test.go`'s `durableSet` rows for `planparser.PlanDir(l.AnchorPath())`/`PlanOverview(l.AnchorPath())` share that identical property (the expectation is itself derived from `l.AnchorPath()`), and card 10 adds no such note.
**Fix:** Either add the same one-line weakening note to `durableSet`'s two rows in card 10, or state explicitly in the Shared Decision why `notransients_test.go` is exempt from it.

## Verdict

REQUEST_CHANGES
Two NIT findings only; both are rationale/annotation-accuracy gaps, not functional defects.
MILL_REVIEW_END
