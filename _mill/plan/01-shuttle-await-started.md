# Batch: shuttle-await-started

```yaml
task: llm-driven child can park forever on Claude Code's own workspace-trust dialog
batch: shuttle-await-started
number: 1
cards: 2
verify: go test ./internal/shuttleengine/
depends-on: []
```

## Batch Scope

This batch adds `Run.AwaitStarted` to `internal/shuttleengine`: an exported startup-only probe that loops the existing `checkLivenessTick` (liveness, capture, `Engine.Startup` classification, trust-gate dismissal) until the provider reaches `StartupReady` or the startup window closes, for a caller that starts a run and never `Wait`s on it.
It also adds the `Info` log line for a successful gate dismissal inside `checkLivenessTick`, pins the new method's negative-verdict return sites and file-contract call sites in the Completion Signal tripwire, and covers the method with hermetic tests.
It is one batch because every change sits in one package and the tripwire test must move in the same commit as the method it scans.
The external interface batch 2 consumes is exactly `func (run *Run) AwaitStarted() (bool, error)`, per the overview's "the AwaitStarted contract batch 2 consumes" decision.
No batch-local decision differs from the overview's Shared Decisions.

## Cards

### Card 1: Add Run.AwaitStarted and log successful gate dismissals

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/rundir.go`
- **Edits:**
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/completionsignal_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/shuttleengine/wait.go`, add an unexported helper `awaitStartedTickCap(startupTimeout, interval time.Duration) int` and an exported method `func (run *Run) AwaitStarted() (bool, error)`, both placed directly after `Wait` and before `pollEventsTick`.

  `awaitStartedTickCap` returns the ceiling of `startupTimeout / interval`, plus 1, plus `maxStatusRetries`.
  A negative `startupTimeout` is treated as 0 before the division, so the cap is never below `1 + maxStatusRetries`.
  Its doc comment states that it is the Live-Substrate Spawn Observability retry clause's attempt-COUNT bound: the probe plays keys and spawns nothing, so the cap is belt-and-braces against a clock that never advances, and the `maxStatusRetries` slack keeps a run of tolerated status errors from eating the ticks the window itself needs.

  `AwaitStarted` has this shape (the implementer may reword log/error text only as noted):

  ```go
  func (run *Run) AwaitStarted() (bool, error) {
  	if run.attached && run.state.Started {
  		return true, nil
  	}
  	cfg := run.runner.cfg
  	interval := pollInterval(cfg)
  	startupTimeout := time.Duration(cfg.StartupTimeoutS) * time.Second
  	startupDeadline := run.clock.Now().Add(startupTimeout)
  	maxTicks := awaitStartedTickCap(startupTimeout, interval)

  	started := false
  	statusFailures := 0
  	for tick := 0; tick < maxTicks; tick++ {
  		outcome, err := run.checkLivenessTick(&started, startupDeadline)
  		if err != nil {
  			statusFailures++
  			if statusFailures >= maxStatusRetries {
  				if allOutputFilesExist(run.spec.OutputFiles) {
  					return true, nil
  				}
  				switch {
  				case errors.Is(err, errStrandNotTracked):
  					return false, fmt.Errorf("shuttle: startup await: reed did not track strand %q on %d consecutive liveness checks: %w", run.state.StrandGUID, maxStatusRetries, err)
  				case errors.Is(err, errStrandPaneBindingCleared):
  					return false, fmt.Errorf("shuttle: startup await: reed held no pane binding for strand %q on %d consecutive liveness checks: %w", run.state.StrandGUID, maxStatusRetries, err)
  				default:
  					return false, fmt.Errorf("shuttle: startup await: reed status failed %d times consecutively: %w", maxStatusRetries, err)
  				}
  			}
  		} else {
  			statusFailures = 0
  			if started {
  				return true, nil
  			}
  			switch outcome {
  			case OutcomeDone:
  				return true, nil
  			case OutcomeDied:
  				return false, nil
  			}
  		}
  		run.clock.Sleep(interval)
  	}
  	return allOutputFilesExist(run.spec.OutputFiles), nil
  }
  ```

  Keep every `return` statement free of the identifiers `OutcomeDied`, `OutcomeTimeout`, `errStrandNotTracked` and `errStrandPaneBindingCleared` exactly as shown: the Completion Signal tripwire keys on the identifiers inside a return's results, and the audited counts below assume only the three `fmt.Errorf` returns carry a marker.
  The `case OutcomeDied:` label is not a return result and adds no key.

  `AwaitStarted`'s doc comment must state, in this file's own prose style:
  - it runs the startup probe `Wait` runs (`checkLivenessTick`: liveness, pane capture, the engine's startup classification, and the engine's trust-dismiss sequence for a recognized one-time gate), on its own, until the provider reaches `StartupReady`;
  - like `StrandGUID` and `RunDir`, it exists for a caller that starts a run and never `Wait`s on it;
  - the answer is "the provider's input TUI is on screen, with any recognized one-time gate dismissed", never "the provider read its prompt": a ready-then-idle session still reads as ready, and that residual degrades to the caller's own outer watch budget;
  - the result mapping: `(true, nil)` on `StartupReady` (with `Started` persisted to run.json by `checkLivenessTick`, so a later `Attach` skips the startup probe) or on a satisfied file contract; `(false, nil)` when the pane died or the startup window closed without readiness; an error only when `checkLivenessTick` failed `maxStatusRetries` consecutive times with the file contract unsatisfied, worded in the same family `Wait` uses;
  - the window is `startup_timeout_s` verbatim, 0 included: a 0 makes the probe answer on its first tick unless the TUI is already ready, the same fast-fail every producer run's `Wait` applies under that config, and there is deliberately no floor;
  - it never calls `finalize` and never writes a terminal `Outcome`, because it is a readiness probe rather than a completion verdict and the run directory must keep its `runOutcomeRunning` sentinel;
  - every negative answer honors the Completion Signal Invariant: the not-ready answer is reached only through `checkLivenessTick`/`classifyStartupWindow`, which already consult `allOutputFilesExist`, and the retry-cap and tick-cap exits consult it directly;
  - the tick cap (`awaitStartedTickCap`) bounds the loop by count as well as by the deadline;
  - the `run.attached && run.state.Started` short-circuit mirrors `Wait`'s own `started` seed, for consistency, since today's only caller always passes a freshly started run.

  In `checkLivenessTick`'s `case StartupTrustPrompt:` branch in `internal/shuttleengine/wait.go`, bind the dismissal to a local, `inputs := run.runner.engine.TrustDismissSequence(capture)`, pass `inputs` to `playInputs`, keep the existing `logger.Warn("shuttle: dismiss trust prompt (non-fatal)", …)` on a play error, and add an `else if len(inputs) > 0` branch logging `logger.Info("shuttle: dismissed startup gate", "strandGUID", run.state.StrandGUID, "inputs", len(inputs))`.
  Add a one-line comment beside it saying it is `Info` because a dismissal is a lifecycle event that happens at most once or twice per run, and that `Wait`'s producers gain the line too.
  An empty `inputs` (an implementation that cannot locate the accepting option) logs nothing.

  Extend the file header comment of `internal/shuttleengine/wait.go` with one sentence saying the file also hosts `Run.AwaitStarted`, the startup probe on its own, for a caller that starts a run and never waits on it.

  In `internal/shuttleengine/run.go`, reword the `RunDir` doc comment's last clause ("batch 4's pane-liveness probe refuses the bootstrap with exactly this path") so it says loom's llm-driver bootstrap refuses with exactly this path when `AwaitStarted` reports the provider never came up.
  Make no other change to `internal/shuttleengine/run.go`.

  In `internal/shuttleengine/completionsignal_enforcement_test.go`:
  - add `"AwaitStarted [Errorf]": 3` to `auditedNegativeVerdictReturns`, with a justification bullet in that map's doc comment: the three retry-cap arms mirror `Wait`'s status-cap arms, each sits behind a direct `allOutputFilesExist` check, and `AwaitStarted` never finalizes an `Outcome`; the `(false, nil)` not-ready exit carries no marker and is guarded upstream, reached only when `checkLivenessTick` or `classifyStartupWindow` answers `OutcomeDied`, both already pinned;
  - add `"AwaitStarted": 2` to `auditedFileContractCallSites`, with a justification sentence in that map's doc comment naming the retry-cap guard and the tick-cap answer;
  - update each map's "as of" phrasing to name this task (`llm-driver-trust-dialog-hang`) as the latest audit.
- **Commit:** `feat(shuttle): add Run.AwaitStarted, the startup probe for a run that is never waited on`

### Card 2: Cover Run.AwaitStarted with hermetic tests

- **Context:**
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/wait_test.go`
  - `internal/shuttleengine/fakes_test.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/engine.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/awaitstarted_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shuttleengine/awaitstarted_test.go` in `package shuttleengine`, untagged, with a file header comment naming what it covers.
  Reuse, without redefining, `fakeReed` and `fakeEngine` from `internal/shuttleengine/fakes_test.go`, and `fakeClock`, `newFakeClock`, `newWaitTestRunner` and `captureLoggerOutput` from `internal/shuttleengine/wait_test.go`.
  Build each `*Run` directly, as `TestRun_Wait_Died_ViaStartupTimeout_TrustDismissRecorded` in `internal/shuttleengine/wait_test.go` does: `runner`, `spec` (with `OutputFiles` naming a file under a `t.TempDir()` run directory), `runDir`, `state` (with `StrandGUID` and `Outcome: runOutcomeRunning`), and `clock`.
  Seed each run directory with `saveRunState(runDir, state)` before calling `AwaitStarted`, so the run.json assertions below have a baseline.

  Test-local doubles, declared in this file:
  - a `frozenClock` whose `Now` always returns one fixed time and whose `Sleep` does nothing, with a `var _ clock = (*frozenClock)(nil)` assertion;
  - a `flakyStatusReed` that embeds `*fakeReed` and overrides `Status` to return a scripted error for its first `failFirst` calls before delegating to the embedded `fakeReed.Status`;
  - an `undismissableEngine` that embeds `*fakeEngine` and overrides `TrustDismissSequence` to return no inputs.

  Write one test per behavior below, each asserting the returned bool and error:
  - `pending → trust prompt → ready`: `StartupScript` of `StartupPending`, `StartupTrustPrompt`, `StartupReady`, a live strand, and a capture queue whose second entry is the gate capture. Returns `(true, nil)`; `SendKey(strand, "Enter")` is recorded; `engine.TrustDismissCaptures()[0]` equals the gate capture; `loadRunState(runDir)` reports `Started == true` and `Outcome == runOutcomeRunning`; the `captureLoggerOutput` buffer contains `shuttle: dismissed startup gate`.
  - ready on the first tick: `(true, nil)`, no `SendKey` calls, no `TrustDismissSequence` captures.
  - the pane going not-live mid-startup with no output files: a status queue of a live strand, then the same GUID with `Live: false` and a non-empty `PaneID` (an empty `PaneID` would take the cleared-binding branch instead). Returns `(false, nil)`.
  - an undismissable gate: `undismissableEngine` scripted `StartupTrustPrompt` forever, `StartupTimeoutS: 1`, `PollIntervalMS: 600`, a `fakeClock`. Returns `(false, nil)`; no `SendKey` calls; virtual elapsed time is at most the startup timeout plus one interval; the logger buffer does not contain `shuttle: dismissed startup gate`.
  - output files already present, pane pending forever: `(true, nil)` (the file contract wins through `classifyStartupWindow`).
  - one transient `Status` error, then ready (`flakyStatusReed` with `failFirst: 1`): `(true, nil)`.
  - `Status` erroring `maxStatusRetries` times with no output files (`flakyStatusReed` with `failFirst` of `maxStatusRetries`): a non-nil error wrapping the scripted error (`errors.Is`) and a false bool.
  - the same, with the output file present: `(true, nil)`, not the error.
  - reed never tracking the strand (a status whose strands omit the run's GUID) with no output files: an error for which `errors.Is(err, errStrandNotTracked)` holds.
  - the tick-count cap with a `frozenClock`, a live strand, and `StartupPending` forever (`StartupTimeoutS: 1`, `PollIntervalMS: 600`): `(false, nil)`, and the number of `"Status"` entries in `fakeReed.CallLog` equals `awaitStartedTickCap` for that config; a subtest with the output file present returns `(true, nil)` at the cap.
  - an attached run whose `state.Started` is true (the `*Run` literal additionally sets `attached: true`, the one case that departs from the build recipe above): `(true, nil)` with zero `Status` calls.
  - a table-driven test for `awaitStartedTickCap`, including a zero and a negative timeout, both yielding `1 + maxStatusRetries`.

  In every case other than the ready ones, assert `loadRunState(runDir)` still reports `Outcome == runOutcomeRunning`: `AwaitStarted` never writes a terminal `Outcome`.
  No test sleeps on the real clock.
- **Commit:** `test(shuttle): cover Run.AwaitStarted's readiness, refusal, retry and tick-cap paths`

## Batch Tests

`verify: go test ./internal/shuttleengine/` runs the whole package, covering the new `internal/shuttleengine/awaitstarted_test.go`, the Completion Signal tripwire in `internal/shuttleengine/completionsignal_enforcement_test.go` (which must pass with the two new audited entries), and the existing `internal/shuttleengine/wait_test.go` suite, which exercises the edited `checkLivenessTick` branch through `Wait`.
Package scope is the right grain: every edit is in this one package, and the tripwire scans source files rather than a single test's behavior.
