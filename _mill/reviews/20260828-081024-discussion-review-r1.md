# Review: reed: header pane's boot sometimes leaves shell/log noise in its scrollback

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [NIT:consistency] "Fourth side effect" overstates coupling with the dev-WARN scenario
**Section:** Mechanism, paragraph starting "A fourth, related side effect falls out of the same reading"
**Issue:** The paragraph frames the `CommitSeededStencils` git-commit risk as a side effect of the same dev-build/stale-stencil scenario that produces noise class 3. Per `internal/stencilstore/reconcile.go`'s `reconcileOne` (`StateUntouched` branch), when `mode == ModeDev` and the hash differs, the function logs the Warn and explicitly returns `wrote = false` — that exact stencil is never added to `written`, so it cannot be the trigger for `CommitSeededStencils` in that run. A commit only fires from a different path entirely (a `StateAbsent` stencil, a `StateReconciled` restamp, or a prod-mode `StateUntouched` write) — none of which co-occur with the WARN.
**Suggested fix:** Reword to make clear the commit risk is a separate, independently-discovered exposure from reading the same pre-run code path, not something that happens together with the WARN-firing scenario. Worth getting right now since `internal/reedengine/doc.go`'s header-pane section is in this task's own doc-update scope and must not carry the inaccurate coupling forward.

## Verdict

APPROVE
Scope, decisions, constraint coverage, and testing are thorough and well-grounded against the actual source; one narrative inaccuracy to fix before it lands in doc.go.
