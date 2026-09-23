# Review: Shuttle guarantees a started run is past its startup gates

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-23
```

## Findings

### [BLOCKING:design] A blocking Start now runs under webster's state-mutation lease in recover-batch
**Section:** Scope, "`internal/websterengine`: no code change required for the fix itself — `RecoverBatch`'s `Starter.Start` and `Run`'s `StartMaster` inherit the guarantee."; Q&A "Webster `recover-batch` has the same latent bug — in scope? … fixed by inheritance with no webster code change beyond doc comments."
**Issue:** `internal/webstercli/recoverbatch.go` calls `websterengine.RecoverSpawnOrAttach` (which calls `deps.Starter.Start`, `internal/websterengine/recoverbatch.go:183`) while holding the state-mutation lease, and releases it only after `SaveState` (webstercli `recoverbatch.go:160–183`).
`AcquireStateMutation`'s own contract (`internal/websterengine/state.go:86–87`) says callers hold it "across their WHOLE load-mutate-save sequence and Release it as soon as the save lands, never across a long block", and the recover-batch file header names the cost: holding it across a wait "would stall every concurrent verb and run entry for minutes".
Once `Start` blocks through the startup probe, that lease is held for the whole startup window (5–10 s typically, up to `startup_timeout_s` = 90 s on a stuck gate), stalling `begin-batch`, `record-batch`, `validate` and run entry for every concurrent fork during that time.
The discussion analyses `bootstrapLock` for loom and the persist-before-block window for the Master, but never this lease.
Relatedly, the recovery strand has the same widened persist-before-block window the discussion accepts for the Master: its guid reaches `state.json` only after `Start` returns, so a `recover-batch` process killed during startup leaves a live recovery strand that `recoverSpawn`'s `prior.StrandGUID` reclaim cannot see — the Master residual is documented, this one is not mentioned.
**Suggested fix:** Add a decision for recover-batch: either restructure the verb so the lease is released across the spawn (e.g. reserve/record intent under the lease, spawn with it released, re-acquire to persist the guid — mirroring how `RecoverAwait` already runs lease-free), or explicitly accept the bounded hold with a rationale that squares with `AcquireStateMutation`'s "never across a long block" contract (and update that contract's wording if accepted).
Either way, state the recovery strand's persist-before-block residual beside the Master's, and move webster out of "no code change required" if a restructure is chosen.

### [NIT:consistency] RunGated's rationale misstates how SingleLLMProducer handles OutcomeDied
**Section:** Decisions → "RunGated preserves its OutcomeDied contract", Rationale: "shed producers (`shedadapters.SingleLLMProducer`, `burlerengine`) map `OutcomeDied` onto their stuck/respawn ladder, while an error is a mechanism failure".
**Issue:** `SingleLLMProducer.mapOutcome` (`internal/shedadapters/singlellm.go:217–222`) maps `OutcomeDied`/`OutcomeTimeout` to a returned error, the same path a `RunGated` error takes (`singlellm.go:169–175`), so for that producer the two are not different ladders.
`burlerengine` does branch on `Result.Outcome` and reserves errors for hard failures, so the decision itself still holds.
**Suggested fix:** Narrow the rationale to `burlerengine` (and any other caller that genuinely branches on the outcome), and note that `SingleLLMProducer` is indifferent, so the plan does not rely on a distinction that does not exist there.

## Verdict

REQUEST_CHANGES
The design does make readiness a shuttle guarantee that no caller can skip and removes loom's responsibility cleanly, but it misses that a blocking `Start` inside `recover-batch` holds webster's state-mutation lease across the startup window, against that lease's own contract.
