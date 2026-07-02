MILL_REVIEW_BEGIN
# Review: Build internal/mux: the window to the world (overlay + strands + render)

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\internal-mux\_mill\discussion.md
date: 2026-07-02
```

## Findings

### [NOTE] Remainder-row assignment among co-equal full panes undefined
**Section:** Render decision — Derived height policy / Testing (render invariants)
**Issue:** muxpoc guarantees "exactly fill" by having the single bottom pane absorb the leftover (`bottomH = usable - ancestorH*(n-1)`, `cmd.go:254`); v1's new "active + every shrink:false strand split the remainder equally" case leaves an integer-division remainder with no stated owner, so the "heights + dividers exactly fill window height" golden invariant is non-deterministic when ≥2 full panes exist.
**Fix:** State the rule (e.g. leftover rows go to the active/bottom pane) so the render test is deterministic and total.

### [NOTE] Single mux.lock acquisition point unspecified (self-deadlock risk)
**Section:** Cross-process concurrency decision
**Issue:** Both "separate CLI processes AND in-process engine calls" acquire `.lyx/mux.lock`, but the decision does not pin the acquisition to one layer; `internal/lock` (gofrs/flock) is non-reentrant across separate handles even in-process on Windows, so a CLI verb that locks and then calls an engine fn that also locks would self-deadlock.
**Fix:** State that the cycle lock is taken at exactly one layer (e.g. inside each engine mutator, with CLI verbs never re-wrapping it).

## Verdict
APPROVE
No blocking gaps; two minor render/lock clarifications would tighten determinism and safety.
MILL_REVIEW_END