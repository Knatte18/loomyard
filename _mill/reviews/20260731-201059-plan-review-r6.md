MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic)
reviewed_file: plan/
date: 2026-07-31
```

## Findings

### [NIT] Card 19/20 wording inconsistency on what SetOutput reassigns
**Location:** batch 5, Card 19 vs Card 20
**Issue:** Card 19 specifies `SetOutput` "rebuilds only the stderr handler inside the composite" (i.e. the package-level `log` *slog.Logger itself is never reassigned, only an internal field of the persistent composite handler is mutated), but Card 20 then describes the mutex as guarding "SetOutput's write to `out`/`log`," which reads as if `log` itself is reassigned — the two cards use different mental models for the same code.
**Fix:** Align Card 20's wording to say the mutex guards `out` and the composite's internal current-stderr-handler field, not a reassignment of `log`, so an implementer doesn't accidentally revert to the old "rebuild `log` from scratch" shape Card 19 explicitly moved away from.

## Verdict

APPROVE
Extensive cross-check of every line-numbered citation, the batch DAG, and All Files Touched found no defects.
MILL_REVIEW_END
