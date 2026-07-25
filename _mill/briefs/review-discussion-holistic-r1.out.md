MILL_REVIEW_BEGIN
# Review: loom: Planner producer

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [NOTE] Input existence not pre-flighted
**Section:** Decision `input-decision-record-only` / shuttle contract
**Issue:** `PlanSpec` never stats `decision-record.md`; a missing/empty input surfaces only as an agent-runtime failure, with no loud pre-flight error.
**Fix:** Record that input-existence sequencing is the (out-of-scope) loom phase machine's job, exactly as `DiscussionSpec` leaves it, so the boundary is explicit.

### [NOTE] Re-run collision on shared PlanDir
**Section:** Technical context (hubgeometry) / done-sentinel-overview-last
**Issue:** `_lyx/plan/` is shared with builder's plan artifacts and shuttle `validate` rejects a pre-existing `00-overview.md`; a re-run or leftover file makes the run fail at validate, unmentioned.
**Fix:** Note this mirrors the Discussion producer and is deferred to the phase machine, so a plan writer does not add stat/cleanup logic here.

### [NOTE] LoadConfig godoc/struct scope
**Section:** Scope (loom.yaml config)
**Issue:** `config.go`'s type and `LoadConfig` godoc name only the "discussion role model-spec"; adding `plan`/`plan_timeout_min` requires refreshing that prose too.
**Fix:** Add the `plan` key to the Config godoc and grammar-validation comment in the same edit, matching the same-commit docs rule.

## Verdict

APPROVE
Scope, decisions, and constraints are grounded in source; only non-blocking notes remain.
MILL_REVIEW_END
