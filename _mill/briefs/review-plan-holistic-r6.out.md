MILL_REVIEW_BEGIN
# Review: llm-driven child can park forever on Claude Code's own workspace-trust dialog — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: 2026-09-23
```

## Findings

### [BLOCKING:design] AwaitStarted probes at 10x Wait's startup cadence
**Location:** batch 1 / card 1 (the `AwaitStarted` shape and its "runs the startup probe `Wait` runs" doc bullet).
**Issue:** `Wait` in `internal/shuttleengine/wait.go` calls `checkLivenessTick` only when `tick%livenessEvery == 0`, so the shipped template (`poll_interval_ms: 500`, `liveness_every_n_polls: 10`) probes and re-plays `TrustDismissSequence` every 5s. The prescribed loop calls `checkLivenessTick` on every 500ms tick instead. `checkLivenessTick`'s `StartupTrustPrompt` branch replays the dismissal on every tick whose capture still shows a gate. At 500ms, a capture taken before Claude redraws after the first Enter can show the trust gate with its caret already on the accepting line, and `claudeengine.TrustDismissSequence` then returns a bare Enter. That Enter can land on the next gate while its caret is on "No, exit", which reproduces the R5-2 quit that the capture-driven sequence was built to prevent. Neither the plan nor the discussion addresses this re-dismissal risk, and the 500ms cadence was never proven live.
**Fix:** state a decision on cadence. Either probe every `LivenessEveryNPolls` ticks as `Wait` does, and scale `awaitStartedTickCap` to match, or record why replaying at 500ms is safe and pin that reasoning with a test.

### [BLOCKING:scope] Card 3 Context omits internal/loomcli/bootstrap.go
**Location:** batch 2 / card 3 (the accepted-residual sentence for the step-6 comment, and the `Long` text's no-re-check clause).
**Issue:** `Requirements:` has the implementer state behavior of `resolveDriverStrandAction` and `driverStrandLive` (skip the spawn, succeed without re-checking readiness). Both are defined in `internal/loomcli/bootstrap.go`, as is the `mustSpawnDriver` predicate that behavior rests on. That file is in neither `Context:` nor `Edits:`.
**Fix:** add `internal/loomcli/bootstrap.go` to card 3's `Context:`.

## Verdict

REQUEST_CHANGES
Probe cadence diverges from Wait without a decision, risking a stray Enter; card 3 lacks bootstrap.go context.
MILL_REVIEW_END
