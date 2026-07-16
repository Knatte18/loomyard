MILL_REVIEW_BEGIN
# Review: Fork-based cluster review in burler — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-16
```

## Findings

### [BLOCKING] Card 9 Context omits profile.go (clusterLenses)
**Location:** Batch 4 (burler-cluster-round) / Card 9
**Issue:** Requirements define `clusterRulesBlock(p *Profile)` composed from `p.clusterLenses`, but `Profile` and the new unexported `clusterLenses []Lens` field live in `internal/burlerengine/profile.go`, which is absent from Card 9's `Context` and `Edits` (only config.go/template.go/stencil.go/spike are listed); `Lens` is in config.go but the field name/type is only knowable from profile.go, forcing cold-start exploration.
**Fix:** Add `internal/burlerengine/profile.go` to Card 9's `Context`.

### [NIT] Card 8 does not name the obsolete engine_test to remove
**Location:** Batch 4 / Card 8
**Issue:** Card 8 deletes `ClusterN` and `ErrClusterUnsupported`, but `engine_test.go:145` `TestEngine_Run_ClusterUnsupported` uses `p.ClusterN = 1` and `errors.Is(err, ErrClusterUnsupported)` and will fail to compile; the card's engine_test.go edits (New-helper Config arg, ForkSubagents case) never mention removing/replacing this test.
**Fix:** Add an explicit instruction to delete/replace `TestEngine_Run_ClusterUnsupported`.

### [NIT] Card 5 worktree-root argument named only vaguely
**Location:** Batch 2 (claudeengine-fork-mode) / Card 5
**Issue:** The `AuditForks(state.SessionID, layout-worktree-root)` call is specified as "use whatever the run loop already holds for the worktree root" rather than the concrete identifier; wait.go reaches it as `run.runner.layout.WorktreeRoot`.
**Fix:** Name `run.runner.layout.WorktreeRoot` explicitly in the requirement.

## Verdict

REQUEST_CHANGES
One context-completeness gap (Card 9); otherwise the plan is sound and well-grounded.
MILL_REVIEW_END
