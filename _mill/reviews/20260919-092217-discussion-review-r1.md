# Review: reed: extract Selvage-pane lifecycle

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [NIT][:consistency] Audit numbers and cited line ranges have drifted from current HEAD
**Section:** Why now / Technical context
**Issue:** Re-running the counts against the actual worktree: `reconcile.go` has 20 matching lines / 23 occurrences of "Selvage", not the 32 the discussion states; `apply.go` has 8 lines / 11 occurrences, not exactly 10. These numbers are recorded as "the before-state baseline the new Status section is measured against" (Decisions § docs-in-the-same-commit / Discovered during exploration), so the baseline itself is off by a meaningful margin for `reconcile.go`. Separately, several cited line ranges in "Current homes of the code that moves" have drifted from HEAD: `ensureSelvagePaneLocked` is at `lifecycle.go:471` (discussion says 464), `bottommostPaneID` at 547 (discussion says 544), `splitSelvagePaneAtBottomLocked` at 588 (discussion says 557), `splitPaneBelowLocked` at 628 (discussion says 615), and `adoptPaneGenerationLocked` at `generation.go:126` (discussion says 151) — a 25-line drift. All the function *names* and their behavior descriptions checked out exactly against the actual code (verified `ensureSelvagePaneLocked`, `bottommostPaneID`, `splitSelvagePaneAtBottomLocked`, `splitPaneBelowLocked`, `planPaneTarget`, `planReconcile`, `clearConflictingPaneBindings`, `toRenderInputs`, `adoptPaneGenerationLocked` all exist with the claimed signatures and roles; the `R4-F4` review-finding citation in `lifecycle.go:562` and the `R4-F5`/`M16` citations in `spawn.go` are real, not fabricated).
**Suggested fix:** Re-run the per-file grep count immediately before writing the post-extraction Status section comparison in `manifest/designs/reed-selvage-pane-extraction.md`, rather than trusting the numbers already in this discussion. Line numbers in a plan/implementation brief should be treated as approximate pointers regardless — mill-plan's implementer will locate functions by name via grep, not by these line numbers — so this is not a blocker, but the numeric baseline claim is worth correcting before it's published as a measured "before" state.

## Verdict

APPROVE
The design is unusually well-grounded — every structural, decision, and technical claim I spot-checked against the actual `internal/reedengine` source (the `pinGeometryOptionsLocked`/`installResizePinsLocked` split that motivates dropping the design doc's stale "one complication" claim, the exact function set and their current homes, the `doc.go` "three module-local Selvage rules" text, `render.FixedHeightPins`' pin-ordering, and the cited review-finding IDs) matched precisely, and the scope boundaries (leaving `windowsize.go`, the watchdog, the render leaf, and `validateSplitCreatedNewPane` untouched) are each backed by a specific, verifiable reason rather than asserted. Only the numeric audit counts and a handful of line-range citations have drifted from HEAD, which is a NIT, not a design problem.
