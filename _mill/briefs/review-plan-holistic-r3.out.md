MILL_REVIEW_BEGIN
# Review: loom: Discussion-Write producer — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-24
```

## Findings

### [NIT:scope] Two cards name functions from a file outside their own Context
**Location:** batch 3, card 16; batch 3, card 21
**Issue:** Card 21 names `fabricengine.NewMutations`/`EnvSyncOptions` (declared in `mutation.go`/`fabric.go`, neither in Context); card 16 names `shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}` (declared in `run.go`/`engine.go`, neither in Context).
**Fix:** Low practical risk — both calls are fully spelled out verbatim in Requirements and their exact usage pattern is already visible in a different Context file each card does list (`internal/loomcli/run.go` for card 21; `internal/shedadapters/singlellm.go` for card 16) — but add the declaring files to Context for literal completeness.

## Verdict

APPROVE
Every batch is tightly grounded against current source; decisions, DAG, file-touch inventory, and card sequencing are all internally consistent.
MILL_REVIEW_END
