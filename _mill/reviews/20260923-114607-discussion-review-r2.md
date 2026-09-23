MILL_REVIEW_BEGIN
# Review: Shuttle guarantees a started run is past its startup gates

```yaml
duration_s: 136.2
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewed_file: _mill/discussion.md
date: 2026-09-23
```

## Findings

### [BLOCKING:design] Tripwire scans only wait.go/attach.go
**Section:** Constraints (Completion Signal Invariant); Technical context (`run.go` bullet) **Issue:** `completionSignalScannedFiles` is `{"wait.go", "attach.go"}`, but the discussion places the startup step on the `*Run` built inside `StartGated` and splits a private `start(spec, gate)` that builds a `Result{Outcome: OutcomeDied}` and an `ErrNotStarted` `Errorf`. If those land in `run.go`, the AST scan never sees the new negative-verdict sites or their `allOutputFilesExist` calls, and "updated to the new function" passes vacuously. **Fix:** Require the startup step and the not-ready finalize/teardown to live in a scanned file, or extend `completionSignalScannedFiles` to cover `run.go` and pin the new `start`/`StartGated`/`RunGated` sites.

### [NIT:scope] Recover-batch "blocks at most poll_wait_s" becomes false
**Demoted-from:** BLOCKING
**Section:** Decisions "Webster's state-mutation lease across the startup window"; Scope (webster bullet) **Issue:** A spawning `recover-batch` call now blocks for the startup window plus `waitBudget`, because `RecoverAwait`'s budget starts after `RecoverSpawnOrAttach` returns. The Master stencil `contracts/stencils/webster/webster-template-master.md` (both `poll_wait_s` lines), `recover-batch`'s `Long` help ("blocks for up to --wait") and `internal/websterengine/doc.go`'s poll_wait_s sentence still state the old bound. The Master can size its own tool-call timeout from that stencil claim. **Fix:** Add these to scope with a disposition: reword them to the new bound, or state that the stencil wording stays as it is and give the reason.

### [NIT:consistency] Loom Long-help residual survives mechanism failure
**Section:** Scope (loomcli bullet); Decisions "Startup mechanism failure" **Issue:** A mechanism-failure start tears nothing down, so a live strand can still be left in place, and the next `start` resolves it as `driverStrandLive`. The discussion only says to "update" the "left in place by an earlier readiness refusal" text and never states whether that residual remains for this case. **Fix:** State the new `Long` wording, which names the mechanism-failure residual, or state that this case is accepted.

### [NIT:design] Satisfied-contract handle: "first tick classifies Done" is inexact
**Section:** Decisions "Wait's startup handling after the move" **Issue:** With `Started: false` and a live strand, `checkLivenessTick` checks the file contract only through `classifyStartupWindow`'s expiry, so `Wait` can poll until its own `startupDeadline` before it classifies Done. It does not classify Done on the first tick. **Fix:** Correct the rationale. The outcome is still harmless.

### [NIT:decision] Capture-file disposition deferred to plan
**Section:** Testing (the "capture always erroring" case) **Issue:** "no capture file (or empty — plan decides)" leaves open whether the capture file is absent or empty. **Fix:** Pick absent or empty here.

### [NIT:scope] Start-family caller list is incomplete
**Section:** Technical context ("Production callers of the start family") **Issue:** The list comes from a partial search. It omits interface-routed `Run` callers (`shedadapters/bouncer.go`, `mergeresolve`, `frictionengine/reflect.go`, `treadleengine` targeting/judge). The `RunGated` `OutcomeDied` rationale applies to all of these callers, including bouncer's own died/timeout handling. **Fix:** Enumerate by the `Shuttle`/runner interface seams, or drop the claim that the list is complete.

## Verdict

REQUEST_CHANGES
The completion-signal tripwire's file scope and the recover-batch blocking-bound docs need decisions before planning.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
