# Batch: shuttle-blocking-start

```yaml
task: Shuttle guarantees a started run is past its startup gates
batch: shuttle-blocking-start
number: 1
cards: 5
verify: go test ./internal/shuttleengine/... ./internal/loomcli/... ./internal/shuttlecli/... ./internal/shedadapters/... && go test -tags integration ./internal/loomcli/ && go test -tags smoke -run 'TestSmokeDriverStrand|TestSmokeGate|TestSmokeBurlerRound|TestSmokeSingleLLM' ./internal/loomcli/
depends-on: []
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits -- touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original -- that breaks git rename history and inflates review diffs.

For card 4's move of `internal/shuttleengine/awaitstarted_test.go` to `internal/shuttleengine/startup_test.go`, the "surgical edits" are the port itself: each existing test is retargeted from `run.AwaitStarted()` on a hand-built `*Run` to `runner.StartGated`/`runner.RunGated` on a real `*Runner` over the fakes, keeping its scenario, its fake doubles (`frozenClock`, `flakyStatusReed`, `undismissableEngine`) and its assertions' intent.

## Batch Scope

This batch moves shuttle's startup probe from `Run.Wait`/`Run.AwaitStarted` into `Runner.StartGated`, so every `*Run` handle `Start`/`StartGated` issues has already resolved its provider's startup gates, and a provider that never becomes ready is torn down and reported as `ErrNotStarted` (or as `OutcomeDied`/`OutcomeTimeout` from `RunGated`/`Run`).
It also removes loom's own readiness await, extends `Attach`'s leftover rule so a torn-down not-ready record is respawn-eligible, removes the `Run.attached` field, and lands the doc changes the behavior change requires (`CONSTRAINTS.md` bullet, repro doc, overview row).
It is one batch because `internal/shuttleengine` and `internal/loomcli` are compile-coupled through `Run.AwaitStarted`: loom must stop calling it before shuttle deletes it.
Batch 2 consumes the new blocking-start contract only in prose (webster's lease and residual doc comments, stencil wording) and adds one webster test that names `shuttleengine.ErrNotStarted`.
Batch-local decisions beyond `## Shared Decisions`: none.

## Cards

### Card 1: Attach treats a terminal untracked record as respawn-eligible

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/wait.go`
- **Edits:**
  - `internal/shuttleengine/attach.go`
  - `internal/shuttleengine/attach_test.go`
  - `internal/shuttleengine/completionsignal_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Implements the discussion's decision "Attach treats a torn-down not-ready record as respawn-eligible".
  In `internal/shuttleengine/attach.go`, add a private helper `isTerminalOutcome(outcome string) bool` that reports true exactly for `string(OutcomeDone)`, `string(OutcomeAsking)`, `string(OutcomeDied)` and `string(OutcomeTimeout)`, and false for the empty string, `runOutcomeRunning`, and any other value.
  In `leftoverThenAgeVerdict`, insert a new check AFTER its existing `allOutputFilesExist(spec.OutputFiles)` check and BEFORE its `now.Sub(c.dirMtime) >= minAge` age check: when `isTerminalOutcome(c.state.Outcome)` is true, return `verdictRespawnEligible`.
  Rewrite `leftoverThenAgeVerdict`'s doc comment to state the new rule and its rationale from the discussion: parity with `dispositionCandidate`'s tracked-and-live branch, which already treats a terminal Outcome as respawn-eligible whatever reed says of the pane;
  an untracked `timeout`/`asking` record written by `finalize` may still have a live pane behind it (finalize cleans up only on `OutcomeDone`), and respawning beside it adds no hazard class the tracked-live branch does not already accept;
  an empty (legacy) or unrecognised Outcome keeps the age rule.
  In `internal/shuttleengine/attach_test.go`, add a table-driven test `TestAttach_UntrackedTerminalRecord_RespawnEligibleRegardlessOfAge` using the existing helpers `newAttachTestRunner`, `seedPresentReedState`, `seedAttachRun` and `setDirAge`: for each Outcome in `done`, `asking`, `died`, `timeout`, a candidate whose strand reed does not track, output files absent, and a run dir younger than `2 × StartupTimeoutS`, `Attach` returns `found == false` with a nil error.
  Add a second case set for a tracked strand with a cleared pane binding (`deadStatus(guid, "")`, not anchor:hidden), which also routes through `leftoverThenAgeVerdict`, asserting the same.
  Keep the existing `TestAttach_UntrackedStrand_AgeRule` and `TestAttach_BindingClearedStrand_AgeRule` cases unchanged in intent: an untracked `running` candidate younger than the age guard still errors, and an empty-Outcome legacy record keeps the age rule — add an explicit empty-Outcome young-dir case asserting the error if neither test already pins it.
  In `internal/shuttleengine/completionsignal_enforcement_test.go`, update `auditedNegativeVerdictReturns`'s `"leftoverThenAgeVerdict [verdictRespawnEligible]"` count from 2 to 3 and extend that entry's justification line: the new return sits after the function's file-contract check, so it is reached only when the output files are not all present.
  `auditedFileContractCallSites` is unchanged.
- **Commit:** `fix(shuttleengine): respawn over an untracked record that already reached a terminal outcome`

### Card 2: Start-path tests get a ready startup

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/wait_test.go`
- **Edits:**
  - `internal/shuttleengine/fakes_test.go`
  - `internal/shuttleengine/run_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Prepares the existing `Start` tests for card 4, under which `Runner.StartGated` probes the pane before returning: with `fakeReed`'s empty `StatusQueue` (no strands, so `errStrandNotTracked`) and `fakeEngine`'s empty `StartupScript` (always `StartupPending`), every successful `Start` in `run_test.go` would otherwise fail or sit out the startup window.
  In `internal/shuttleengine/fakes_test.go`, add a helper `readyStart(reed *fakeReed, engine *fakeEngine)` with a doc comment: it scripts reed's `StatusQueue` to report the strand `reed.AddStrandResult.GUID` as live with a non-empty `PaneID` (only when `StatusQueue` is empty), and scripts `engine.StartupScript` to `[]StartupState{StartupReady}` (only when `StartupScript` is empty), so a `Start` through the pair resolves its startup step ready on the first probe.
  In `internal/shuttleengine/run_test.go`, call `readyStart(reed, engine)` in every test whose `runner.Start(...)` call is expected to SUCCEED: `TestRun_RunDir_ReturnsStartCreatedDirectory`, `TestRunner_Start_HappyPath_WiresAddSpecVerbatim`, `TestRunner_Start_PersistsRunningOutcome`, `TestRunner_Start_SweepErrorDoesNotBlockStart`, `TestRunner_Start_SweepSkipsEntirelyOnReedStateReadError`, `TestRunner_Start_SweepSkipsEntirelyOnAbsentReedState`, and the success loop of `TestNewDetachedRunner_AcceptsStandaloneShapeAndBothPaneCwdPositions`.
  Tests whose `Start` is expected to fail before the strand exists (told-path refusals, spec validation, a failed strand registration, a failed run-state save) need no change.
  Where an edited test asserts on `reed.CallLog` or `RemoveStrandCalls`, keep the assertion true both before and after card 4 (for example assert with a prefix or membership check rather than exact equality), since card 4 adds `Status`/`CapturePane` calls to the start path.
  Every test here keeps passing against today's non-blocking `Start`: the helper only scripts values the current code never reads on that path.
- **Commit:** `test(shuttleengine): script a ready startup for Start-path tests`

### Card 3: Loom stops awaiting driver readiness itself

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/wait.go`
- **Edits:**
  - `internal/loomcli/driverlaunch.go`
  - `internal/loomcli/start.go`
  - `internal/loomcli/driverspec.go`
  - `internal/loomcli/start_driver_test.go`
  - `internal/loomcli/smoke_driverstrand_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Implements the discussion's decision "Loom's llm arm".
  In `internal/loomcli/driverlaunch.go`: remove the `AwaitStarted() (bool, error)` method from the `driverHandle` interface, so it holds only `StrandGUID()` and `RunDir()`;
  rewrite `driverHandle`'s doc comment (no startup await);
  rewrite `driverStarter.StartDriver`'s doc comment from "returns a handle without blocking" to: it returns only once the run's provider is past its startup gates, and a provider that never became ready surfaces as the returned error;
  rewrite `runnerDriverStarter.StartDriver`'s doc comment to drop the `AwaitStarted` method from the list of methods `*shuttleengine.Run` satisfies `driverHandle` through.
  In `internal/loomcli/start.go`:
  delete the whole llm-arm "Step 6" block in `runDriverSpawnAndWait` (the `driverRun.AwaitStarted()` call, its error and `!ready` refusals, and its "loom: driver strand is ready" log line) together with its leading comment block;
  in `startLLMDriverArm`, after a successful `c.driverStarter.StartDriver(spec)`, log `logger.Info("loom: driver strand is ready", "guid", ..., "runDir", ...)` — merging it with the existing "loom: spawned driver strand" line into one line is acceptable;
  rewrite `startLLMDriverArm`'s doc comment: it returns the started run's handle for the caller to log, `StartDriver` already guarantees readiness, and a not-ready driver surfaces as `StartDriver`'s error, which `runDriverSpawnAndWait`'s existing failed-start refusal path reports with shuttle's own message (naming the run dir and strand);
  move the "Accepted residual" paragraph out of the deleted block into `startLLMDriverArm`'s doc comment and narrow it: a readiness refusal now removes the driver strand, so the next `start` spawns a fresh one;
  the strand is left in place only when shuttle's readiness check could not get an answer from reed at all (a startup mechanism failure), or when shuttle's not-ready teardown could not remove the strand, and a later `start` that finds that strand live resolves it as `driverStrandLive` and attaches without re-checking readiness;
  accepted because reed being unable to answer says nothing about the agent, so tearing the strand down could kill a working driver.
  Keep the note that `bootstrapLock` stays held across `StartDriver`, now including shuttle's startup step, for up to `startup_timeout_s` — the same duration it was held across the old await.
  If deleting the block leaves the `driverRun` variable in `runDriverSpawnAndWait` unused, drop the assignment accordingly (keep `startLLMDriverArm`'s error handling unchanged).
  Rewrite the `startCmd` `Long` help's sentence "This signal is checked only for a driver this invocation spawns, so an ly-drive strand already live from an earlier invocation -- including one left in place by an earlier readiness refusal -- is attached to, or returned over with --no-attach, without re-checking readiness." to say: a readiness refusal removes the driver strand, so the next `start` spawns a fresh one;
  the signal is checked only for a driver this invocation spawns, and the two cases that can still leave an unready ly-drive strand live — shuttle could not get a liveness answer from reed at all, or its teardown could not remove the strand — are attached to, or returned over with `--no-attach`, by a later `start` without re-checking readiness.
  Keep `Long` as help text only (no Go-identifier jargon beyond what it already uses) and keep `Short` unchanged.
  In `internal/loomcli/driverspec.go`, rewrite `driverSpec`'s doc comment paragraph that says Timeout is never read and cites `Run.AwaitStarted`: a zero `Timeout` defaults to `run_timeout_min` in `Spec.validate`, and on this path it bounds only shuttle's startup step inside `Start` (the startup window itself is `startup_timeout_s`), because `Wait` is never entered;
  `KeepPane` and `AwaitOperator` are read only by `Wait`.
  In `internal/loomcli/start_driver_test.go`: remove `stubDriverHandle`'s `ready`, `awaitErr` and `awaitCalls` fields and its `AwaitStarted` method, and update its doc comment;
  delete `TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnReadinessError`;
  replace `TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnReadinessRefusing` with `TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnNotReadyStart`, in which `fakeDriverStarter.startErr` is an `errors.New` message naming a run dir and a strand guid (the shape shuttle's not-ready error takes), and assert `runDriverSpawnAndWait` returns false, the envelope output contains that message, and the bootstrap lock is released;
  in `TestRunDriverSpawnAndWait_LLMArm_NeverConsultsTheRunLockHandshake`, drop the `awaitCalls` counter and its assertion;
  renumber the "failure site N of 7" comments on the remaining `ReleasesLockOn*` tests to match the sites that remain.
  In `internal/loomcli/smoke_driverstrand_test.go` (build tag `smoke`), rewrite the three comments that cite `Run.AwaitStarted`/`AwaitStarted` so they describe the readiness guarantee as shuttle's `Start` returning only once the provider is ready, and a readiness regression surfacing as `start`'s refusal; change no test logic.
- **Commit:** `refactor(loomcli): drop loom's own driver readiness await in favour of shuttle's start`

### Card 4: Start runs the startup probe before issuing a handle

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/gate.go`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/template.yaml`
  - `internal/shuttleengine/reed.go`
  - `internal/shuttleengine/attach.go`
  - `internal/shuttleengine/fakes_test.go`
  - `internal/shuttleengine/wait_test.go`
  - `internal/shuttleengine/attach_test.go`
  - `internal/shuttleengine/gate_test.go`
  - `internal/shedadapters/burler_test.go`
  - `internal/shuttlecli/cli_test.go`
  - `internal/loomcli/start.go`
  - `internal/loomcli/driverlaunch.go`
- **Edits:**
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/doc.go`
  - `internal/shuttleengine/run_test.go`
  - `internal/shuttleengine/completionsignal_enforcement_test.go`
  - `CONSTRAINTS.md`
  - `docs/reference/claude-trust-dialog-repro.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/shuttleengine/awaitstarted_test.go` -> `internal/shuttleengine/startup_test.go`
- **Requirements:**
  Implements the discussion's decisions "Readiness is guaranteed by a blocking start", "Not-ready start: error, strand torn down, run dir and last capture kept", "Startup mechanism failure: error with identity, no teardown", "RunGated preserves its OutcomeDied contract", "Startup step lives in a tripwire-scanned file", "AwaitStarted removed", "Run deadline anchoring" and "New CONSTRAINTS.md bullet", using the names fixed in the overview's Shared Decision "startup step shape and names".

  **`internal/shuttleengine/wait.go`.**
  Declare the exported sentinel `ErrNotStarted` (`errors.New`, message along the lines of "the provider never became ready") with a doc comment: `StartGated` wraps it when its startup step resolves not-ready, after tearing the strand down.
  Delete `Run.AwaitStarted`; rename `awaitStartedTickCap` to `startupTickCap` (body unchanged) and update its doc comment.
  Add `(run *Run) awaitStartup() (Result, error)`, whose loop is `AwaitStarted`'s loop moved unchanged in cadence and retry handling: probe interval `pollInterval(cfg) * LivenessEveryNPolls` (floored to 1 as `Wait` floors it), first probe immediate, `run.clock.Sleep(interval)` between probes, `statusFailures` reset on a successful probe, and no `run.attached && run.state.Started` short-circuit (a start is never attached).
  Its tick cap is `startupTickCap(window, interval)` where `window` is the shorter of `startup_timeout_s` and the time left until `run.deadline` (floored at 0).
  Resolution per probe, in this order:
  (a) `checkLivenessTick` errors `maxStatusRetries` consecutive times: if `allOutputFilesExist(run.spec.OutputFiles)` return `(Result{}, nil)`, otherwise return `run.identity()` with an `fmt.Errorf` in the same three-arm wording family `Wait` uses (`errStrandNotTracked`, `errStrandPaneBindingCleared`, reed status failed), prefixed `shuttle: startup:` and naming the strand guid and the run dir, wrapping the underlying error with `%w` and never `ErrNotStarted`; no teardown and no Outcome write;
  (b) after a successful probe, `started` true or outcome `OutcomeDone` → return `(Result{}, nil)`;
  (c) outcome `OutcomeDied` → return `run.abandonStartup(OutcomeDied)`;
  (d) after the probe, when `run.clock.Now().After(run.deadline)`: `run.classifyDeadlineExpiry(OutcomeTimeout)` of `OutcomeDone` returns `(Result{}, nil)`, otherwise return `run.abandonStartup(OutcomeTimeout)`.
  After the loop exhausts the tick cap: if `allOutputFilesExist(run.spec.OutputFiles)` return `(Result{}, nil)`, otherwise return `run.abandonStartup(OutcomeDied)` — this replaces `AwaitStarted`'s bare `(allOutputFilesExist, nil)` exit.
  Add `(run *Run) abandonStartup(outcome Outcome) (Result, error)`, the not-ready teardown, in this order:
  1. when `run.lastStartupCapture` is non-empty, write it to `filepath.Join(run.runDir, startupCaptureFileName)` (a write failure is a `logger.Warn`, and the message then says no capture was saved);
  2. call `run.finalize(outcome, "")`, which persists `RunState.Outcome`, skips the gate and cleanup for a non-`OutcomeDone` outcome, and logs "run finished";
  3. call `run.runner.reed.RemoveStrand(run.state.StrandGUID, false)` whatever `Spec.KeepPane` says, logging a failure at Warn;
  4. keep the run directory;
  5. `logger.Warn("shuttle: provider never became ready; strand torn down", ...)` naming run dir, strand guid and outcome (per the Live-Substrate Spawn Observability invariant);
  6. return the finalized `Result` with `fmt.Errorf("shuttle: start: %w ...", ErrNotStarted, ...)` whose message names the run dir, the strand guid, the outcome, and either that the last pane capture was saved to `startup-capture.txt` or that no pane capture was saved;
  when `RemoveStrand` succeeded it states the strand was removed, and when it failed it states the strand could NOT be removed, carries reed's removal error text, and tells the operator to remove it by hand (`lyx reed status` / `lyx reed remove`).
  In `checkLivenessTick`, set `run.lastStartupCapture = capture` right after a successful `CapturePane`; nothing else in it changes.
  Rewrite the doc comments this change falsifies: the file header (lines about `AwaitStarted`; `Wait` and the startup step are now the two places that sleep), `Wait`'s own doc comment where it describes the startup probe, and add the startup step to the "Completion Signal Invariant" section's list of exits consulting the file contract (the retry-cap and tick-cap exits call `allOutputFilesExist` directly, the not-ready answer comes through `checkLivenessTick`/`classifyStartupWindow`, and the run-deadline exit through `classifyDeadlineExpiry`).
  Leave `Wait`'s `started := run.attached && run.state.Started` seed and its comment untouched in this card (card 5 changes it).

  **`internal/shuttleengine/run.go`.**
  Add `startupCaptureFileName = "startup-capture.txt"` to the artifact-name `const` block beside `promptFileName`/`settingsFileName`/`eventsFileName`.
  Add the `lastStartupCapture string` field to `Run` with a doc comment (the last successful pane capture the startup step took, saved on a not-ready teardown).
  Move `StartGated`'s body into `(r *Runner) start(spec Spec, gate GateSpec) (*Run, Result, error)`: pre-strand failures return `(nil, Result{}, err)` exactly as today;
  after `saveRunState` and the "shuttle: run started" log it builds the `*Run` as today (`deadline: clk.Now().Add(spec.Timeout)` stays computed before the startup step), calls `run.awaitStartup()`, and returns `(nil, result, err)` on an error or `(run, Result{}, nil)` otherwise.
  `StartGated` becomes `run, _, err := r.start(spec, gate); if err != nil { return nil, err }; return run, nil`.
  `RunGated` becomes: `run, result, err := r.start(spec, gate)`; when `errors.Is(err, ErrNotStarted)` return `(result, nil)`; when `err != nil` return `(result, err)`; otherwise `return run.Wait()`.
  `run.go` constructs no negative-verdict marker itself; it only branches with `errors.Is`.
  Rewrite the doc comments: `Start` (it now blocks through the startup probe and returns a handle only past the provider's startup gates;
  a not-ready provider is torn down and reported as an error wrapping `ErrNotStarted`), `StartGated` (webster's Run persists Master's strand guid after the start returns, which now includes the startup window), `RunGated`/`Run` (a not-ready start is `OutcomeDied`/`OutcomeTimeout` with a nil error, preserving the burler round producer's one-retry ladder), `RunDir` (drop the `AwaitStarted` reference), `StrandGUID` if it cites the old non-blocking timing, the `Run` struct's own doc comment (drop the `AwaitStarted` line), and the file header.
  Beside `start`'s existing AddStrand-to-saveRunState residual comment, add the general form from the discussion's decision "Webster's persist-before-block windows": a caller that persists the strand guid only after `Start` returns now has a crash window as wide as the startup probe.
  Leave the `Run.attached` field and its doc comment in place in this card except for dropping its `AwaitStarted` mention (card 5 removes the field).

  **`internal/shuttleengine/rundir.go`.**
  Rewrite `RunState.Started`'s doc comment so it names the startup step inside `Start` and `Wait`'s probe for an attached not-started run as the two writers, instead of `AwaitStarted`.

  **`internal/shuttleengine/doc.go`.**
  Add a paragraph to the package doc: `Start`/`StartGated`/`Run`/`RunGated` run the startup probe (readiness plus dismissal of any one-time startup gate, through the Engine seam's startup classification and trust-dismiss sequence) before issuing a handle, so no caller probes readiness or plays gate keys;
  a not-ready provider is torn down (strand removed, run dir and `startup-capture.txt` kept, Outcome persisted), reported as `ErrNotStarted` from `StartGated` and as `OutcomeDied`/`OutcomeTimeout` from `RunGated`.

  **`internal/shuttleengine/startup_test.go`** (moved from `awaitstarted_test.go`, see the Rename mechanic).
  Update the file header to describe start-level startup coverage.
  Replace `newAwaitStartedTestRun` with a helper that builds a real `*Runner` via `NewRunner` over a temp worktree and anchor (the `newTestRunner` shape) with the given `Config`, sets the runner's `clock` field to the given fake clock so no test sleeps on the real one, and seeds `fakeReed.AddStrandResult` to `reedengine.Strand{GUID: "strand-1"}` and a `fakeEngine.PrepareLaunch`.
  The helper's default config must set `RunTimeoutMin` (for example 5) or each spec must set `Timeout`: `Spec.validate` turns a zero `Timeout` into `RunTimeoutMin` minutes, and a zero result puts `run.deadline` at start time, which the startup step's run-deadline check would then hit after the first pending probe — only the run-deadline tests below may let that deadline bind.
  Port each existing test onto `runner.StartGated(spec, GateSpec{})` (or `RunGated`), keeping its scenario:
  trust prompt then ready → dismiss sequence played once per gate-showing probe, the capture handed to `TrustDismissSequence` is the classified one, handle returned, `run.json` `Started: true` with Outcome `running`, "dismissed startup gate" logged;
  ready on first probe → handle returned with no `Sleep` before return;
  pane not live mid-startup → `errors.Is(err, ErrNotStarted)`, `RemoveStrand("strand-1", false)` recorded, `run.json` Outcome `died`, run dir kept, `startup-capture.txt` holds the last successful capture;
  undismissable gate until the window expires → same as pane-not-live;
  file contract satisfied while the pane stays pending → handle returned, `fakeReed.RemoveStrandCalls` empty;
  one transient status error then ready → handle returned;
  status errors exhausting `maxStatusRetries` with no output files → error that names the strand guid and run dir, is NOT `ErrNotStarted`, no `RemoveStrand`, `run.json` Outcome still `running`;
  status errors exhausting the cap with the output file present → handle returned;
  reed never tracking the strand → the `errStrandNotTracked` mechanism error, same no-teardown assertions;
  tick cap under `frozenClock` → loop bounded by `startupTickCap`, resolving `ErrNotStarted` with teardown and Outcome `died`, or returning the handle when the file contract is satisfied;
  `TestStartupTickCap` (the renamed boundary-arithmetic test);
  probe cadence equals `pollInterval × LivenessEveryNPolls`.
  Drop `TestAwaitStarted_AttachedAndStarted_ShortCircuits` (the short-circuit no longer exists; card 5 covers the attach side through `Wait`).
  Add new tests:
  capture always erroring until the window expires → `ErrNotStarted`, `startup-capture.txt` absent, and the message says no pane capture was saved;
  teardown `RemoveStrand` failing (`fakeReed.RemoveStrandErr`) → still `ErrNotStarted`, message says the strand could not be removed and contains reed's error text, `run.json` Outcome still persisted;
  `Spec.KeepPane: true` on a not-ready start → `fakeReed.RemoveStrandCalls` still records the strand;
  `RunGated` not-ready → `(Result{Outcome: OutcomeDied, StrandGUID, SessionID, RunDir}, nil)` and a `GateSpec` closure counter still at zero;
  `RunGated` startup mechanism failure → non-nil error and a Result carrying `SessionID`, `StrandGUID` and `RunDir` with an empty Outcome;
  run deadline shorter than the startup window with a never-ready provider (`Spec.Timeout` of a few seconds, `StartupTimeoutS` much larger, fake clock) → `RunGated` returns `OutcomeTimeout` with elapsed virtual time near `Spec.Timeout` rather than the startup window, `StartGated` returns `ErrNotStarted`, strand torn down, `run.json` Outcome `timeout`;
  the same with the output file present at the deadline → handle returned (and `RunGated` then classifies `OutcomeDone`);
  a failed start (strand removed, Outcome `died`, run dir younger than `2 × StartupTimeoutS`) followed by `runner.Attach` on the same output files, with `fakeReed.StatusQueue` rescripted to omit the strand and a present reed state file seeded via `seedPresentReedState` → `found == false` and a nil error.

  **`internal/shuttleengine/run_test.go`.**
  Add a test that a `Start` with `readyStart` scripting issues exactly one `CapturePane`/`Startup` probe before returning and persists `Started: true`, and adjust any `CallLog` assertion card 2 left in a form this card's added `Status`/`CapturePane` calls falsify.

  **`internal/shuttleengine/completionsignal_enforcement_test.go`.**
  Add `"ErrNotStarted": true` to `negativeVerdictMarkers` (overview Shared Decision "tripwire marker for the new sentinel").
  Remove the `AwaitStarted` entries from `auditedNegativeVerdictReturns` and `auditedFileContractCallSites`, and add entries for `awaitStartup` and `abandonStartup` with the counts the scan actually finds, each with a justification line in the ledger comment:
  `awaitStartup`'s three mechanism-failure `Errorf` arms sit behind a direct `allOutputFilesExist` check;
  its `abandonStartup(OutcomeDied)` returns are reached only through `checkLivenessTick`'s not-live branch (file contract consulted first) or the tick cap (direct check);
  its `abandonStartup(OutcomeTimeout)` return is reached only after `classifyDeadlineExpiry` answered not-done;
  `abandonStartup`'s own `ErrNotStarted` return is a finalization every caller reaches only past one of those checks.
  `auditedFileContractCallSites` gains `"awaitStartup": 2` (retry cap and tick cap).
  Update the two ledgers' "as of" sentences to name this task's batch.
  Confirm the tripwire still fails when one of the new guards is deleted (delete it locally, run the test, restore) and do not commit that experiment.

  **`CONSTRAINTS.md`.**
  Under `## Shuttle Provider-Seam Invariant`, add the bullet: "A `*Run` issued by `Start`/`StartGated` has already resolved its provider's startup probe; no caller outside `internal/shuttleengine` probes provider readiness or plays startup-gate keys."

  **`docs/reference/claude-trust-dialog-repro.md`.**
  Replace the `internal/shuttleengine.Run.AwaitStarted` reference with the startup step inside `Runner.StartGated`, and loom's `runDriverSpawnAndWait` with loom's llm arm starting the driver through `StartDriver`.
  In step 6, add that a not-ready driver would instead make `start` refuse with shuttle's `ErrNotStarted` message and leave `startup-capture.txt` in the driver's run dir as the diagnosis artifact.

  **`docs/overview.md`.**
  In the shuttle row, add one sentence: starting a run returns only once its provider is past its startup gates, and a provider that never comes up is torn down (run dir and last pane capture kept) and reported as an error from `Start`, or as `died`/`timeout` from `Run`.
- **Commit:** `fix(shuttleengine): issue a run handle only past the provider's startup gates`

### Card 5: Wait seeds its startup probe from Started alone; Run.attached removed

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/fakes_test.go`
  - `internal/shuttleengine/run_test.go`
- **Edits:**
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/attach.go`
  - `internal/shuttleengine/wait_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Implements the discussion's decision "Wait's startup handling after the move".
  In `internal/shuttleengine/wait.go`, change `Wait`'s seed to `started := run.state.Started` and rewrite the seed comment above it: `Wait` skips the startup probe exactly when `run.json` records `Started`, for a started and an attached run alike;
  a handle from `Start` carries `Started: true` unless it was issued on the satisfied-file-contract branch, where re-probing is harmless and still classifies `OutcomeDone` (on an events-tick Done, or at the latest at `Wait`'s own startup-window expiry through `classifyDeadlineExpiry`);
  the probe in `checkLivenessTick`/`classifyStartupWindow` is otherwise reached only for an attached run whose `run.json` was never marked `Started`, and `Wait` still computes its own `startupDeadline` from its entry time for that case.
  In `internal/shuttleengine/run.go`, delete the `attached` field and its doc comment from `Run`.
  In `internal/shuttleengine/attach.go`, drop `attached: true` from `reconstructAndWait`'s `*Run` literal and rewrite its comments (the doc comment's "alongside attached: true" and the inline comment that argues "Wait only ever skips the startup probe when BOTH are true") to the new rule stated above.
  In `internal/shuttleengine/wait_test.go`, drop the `attached: true` field from the literals in `TestRun_Wait_AttachedButNeverStarted_StartupProbeStillRuns` and `TestRun_Wait_RunDeadline_SatisfiedFileContractWinsOverTimeout`, and rewrite the former's doc comment to the new seed (a run whose `run.json` has `Started: false` still runs the startup probe and classifies `OutcomeDied` at the window's end).
  Grep every `*_test.go` in `internal/shuttleengine` for `attached:` and remove any other occurrence.
  Add `TestRun_Wait_StartedRun_SkipsStartupProbe`: a run obtained from `runner.StartGated` over `readyStart`-scripted fakes (runner clock set to a fake clock), with an events line `STOP:` and the output file written after start, whose `Wait` classifies `OutcomeDone` while `engine.StartupCalls` and the `CapturePane` count in `reed.CallLog` do not grow past what start itself recorded.
- **Commit:** `refactor(shuttleengine): seed Wait's startup probe from run.json's Started alone`

## Batch Tests

`verify:` runs the untagged tests of `internal/shuttleengine/...` (including `startup_test.go`, `attach_test.go`, `wait_test.go`, `run_test.go` and the completion-signal tripwire), `internal/loomcli/...` (`start_driver_test.go` and the help-tree tests covering `start`'s `Long` text), `internal/shuttlecli/...` (`TestRunCmd_MechanismFailure_EnvelopeCarriesRunIdentity`, which now reaches its mechanism failure inside `RunGated`'s startup step and must still carry the run identity) and `internal/shedadapters/...` (`TestBurlerProducer_Call_DiedThenDoneSucceedsWithRetry` is the existing guard that a first `OutcomeDied` attempt from the runner seam takes burler's one-retry path — the reason `RunGated` keeps `OutcomeDied`; no new test is needed there).
It also runs loomcli's `integration` tier (it touches `internal/loomcli`) and four of loomcli's `smoke`-tagged tests, which drive shuttle's real start path against real tmux and a stub launch rather than a real provider:
`TestSmokeDriverStrand_ReentrantAcrossThreeBootstraps` (card 3 edits it; it proves a real ly-drive strand comes up through `Start` alone),
`TestSmokeGate_RepromptsThroughARealPaneAndFixesTheArtifact` (`RunGated` through a real pane),
and `TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning` / `TestSmokeSingleLLM_HarvestsAFinishedRunWithReedStateGone` (`Start` then `Attach` against a real reed).
Their stub engines report `StartupReady`, so they pass through the new startup step on its first probe; measured together at about 20 s.
Loomcli's remaining smoke tests exercise the go-driver bootstrap, the operator strand and bootstrap wiring, none of which calls shuttle's start path; `pipeline.done_gate` does not run the smoke tier.
`internal/shuttlecli`'s smoke tier is not run: it drives a real `claude`, and this batch does not edit `internal/shuttlecli`.
