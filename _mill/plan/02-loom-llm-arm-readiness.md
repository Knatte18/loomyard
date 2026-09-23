# Batch: loom-llm-arm-readiness

```yaml
task: llm-driven child can park forever on Claude Code's own workspace-trust dialog
batch: loom-llm-arm-readiness
number: 2
cards: 2
verify: go test ./internal/loomcli/ && go test -tags smoke -run TestSmokeDriverStrand ./internal/loomcli/
depends-on: [1]
```

## Batch Scope

This batch rewires loom's llm arm onto batch 1's `Run.AwaitStarted`: `driverHandle` gains the method, `runDriverSpawnAndWait`'s step-6 llm block calls it in place of `awaitDriverPane`, and `awaitDriverPane`, `findStrandByGUID`, `driverPanePollInterval` and `driverPaneAttempts` are deleted with their tests.
The readiness signal `lyx loom start` (and `--no-attach`) waits on becomes "the driver's provider TUI is ready, with any one-time gate dismissed", so the `start` command's `Long` text, the `--no-attach` usage string, and every comment and log line that describes the llm arm's readiness as pane liveness change in the same card and commit as the behavior.
The smoke test that drives three real bootstraps is adapted to the new signal.
It is one batch because every edit is in `internal/loomcli` and all of it consumes the one interface batch 1 exports.
Batch-local decision: the go arm's run-lock handshake, `bootstrapLock`'s acquire/release points, `driverPaneProbe` (still used for the pre-branch strand read and corpse removal) and the refusal envelope's wording are all unchanged.

## Cards

### Card 3: Await the llm driver's provider readiness instead of pane liveness

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/loomcli/bootstrap_test.go`
- **Edits:**
  - `internal/loomcli/driverlaunch.go`
  - `internal/loomcli/driverlaunch_test.go`
  - `internal/loomcli/start.go`
  - `internal/loomcli/start_driver_test.go`
  - `internal/loomcli/driverspec.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Batch 1 added `func (run *Run) AwaitStarted() (bool, error)` on `*shuttleengine.Run` (signature inlined, no file read needed): `(true, nil)` when the provider's TUI reached ready with any recognized one-time gate dismissed, or its file contract is satisfied; `(false, nil)` when the pane died or the `startup_timeout_s` window closed without readiness; an error when reed's liveness check failed repeatedly.

  In `internal/loomcli/driverlaunch.go`:
  - add `AwaitStarted() (bool, error)` to the `driverHandle` interface, with a method doc comment summarizing the three answers above;
  - reword `driverHandle`'s type doc ("the two identities the bootstrap needs after launch") so it names the two identities plus the startup await;
  - reword `runnerDriverStarter.StartDriver`'s doc so the returned `*shuttleengine.Run` satisfies `driverHandle` via its `StrandGUID`, `RunDir` and `AwaitStarted` methods;
  - delete `driverPanePollInterval`, `driverPaneAttempts`, `findStrandByGUID` and `awaitDriverPane`, with their doc comments, and drop the now-unused `time` import;
  - leave the file header comment, `driverStarter`, `driverPaneProbe`, `reedDriverPaneProbe`, `newReedDriverPaneProbe`, `Strands` and `RemoveDriverStrand` unchanged.

  In `internal/loomcli/start.go`, rewrite the step-6 llm block in `runDriverSpawnAndWait` (the `if mustSpawn && mustUseLLMDriverArm(driver)` block after the go arm's handshake block) so it calls `driverRun.AwaitStarted()` instead of `awaitDriverPane`:
  - on an error, release `bootstrapLock` and report `err.Error()` on the envelope, as today;
  - on `false`, release `bootstrapLock` and report the existing refusal text unchanged: `loom: driver strand did not come up; see run dir <RunDir> (strand <StrandGUID>)`;
  - on `true`, replace `logger.Info("loom: driver strand pane is live", …)` with `logger.Info("loom: driver strand is ready", "guid", driverRun.StrandGUID(), "runDir", driverRun.RunDir())`.

  Rewrite that block's leading comment ("probe the just-launched driver strand's pane for liveness") so it says step 6 awaits the driver run's provider readiness through the handle's `AwaitStarted`, which dismisses a recognized one-time startup gate (the workspace-trust and bypass-permissions dialogs) that would otherwise park the session forever on a live pane.
  Keep its existing explanation of why the run-lock handshake is not used on this arm and why the refusal names the run directory and strand guid rather than the driver log.
  Add a sentence stating the accepted residual, per the discussion's out-of-scope decision on already-live driver strands: a not-ready refusal leaves the strand in place for diagnosis, so if its pane is still live the next `start` resolves it as `driverStrandLive` through `resolveDriverStrandAction`, spawns nothing, skips this step entirely and succeeds without re-checking readiness; the operator sees what the pane is stuck on by attaching, and only a newly spawned driver is awaited.
  Add one sentence noting that `bootstrapLock` stays held across this await, up to `startup_timeout_s`, exactly as it was held across the old probe, and that the ly-drive session's own first `lyx shed step` waits on the same lock until this bootstrap releases it.

  Also in `internal/loomcli/start.go`:
  - `startLLMDriverArm`'s doc comment: replace "run the pane-liveness probe against it" (wrapped across two lines) with wording that the caller awaits the run's readiness through the handle's `AwaitStarted`;
  - `runDriverSpawnAndWait`'s doc comment: replace "the llm arm's strand launch and pane-liveness probe" with "the llm arm's strand launch and readiness await";
  - the `start` command's `Long` text: replace "the strand's own pane coming alive for an ly-drive driver" with wording that, for an ly-drive driver, the signal is the driver's provider TUI coming up ready, with any one-time startup gate (such as the workspace-trust dialog) dismissed along the way, within shuttle's `startup_timeout_s`; add that this signal is checked only for a driver this invocation spawns, so an ly-drive strand already live from an earlier invocation (including one left in place by an earlier readiness refusal) is attached to, or returned over with `--no-attach`, without re-checking readiness; keep the rest of the paragraph's meaning, re-flowing its hard-wrapped lines as needed;
  - the `--no-attach` flag's usage string: replace "return once the driver has taken the run lock" with wording covering both arms and scoped to a freshly spawned driver, for example "return once a driver this invocation spawns is confirmed up (the Go driver has taken the run lock; an ly-drive driver's provider TUI is ready, with any one-time startup gate dismissed), instead of handing the terminal to the session";
  - leave `Short` unchanged.

  In `internal/loomcli/driverspec.go`, reword the last clause of `driverSpec`'s doc comment ("polled through the pane-liveness probe rather than waited on") so it says the session is awaited only until its provider is ready (`Run.AwaitStarted`, which reads the runner config's `startup_timeout_s`, not the spec) and is never waited on to completion.
  The sentence's claim that nothing reads `Timeout`, `KeepPane` or `AwaitOperator` stays.

  In `internal/loomcli/driverlaunch_test.go`, delete `TestAwaitDriverPane_AlreadyDead`, `TestAwaitDriverPane_LiveOnFirstPoll`, `TestAwaitDriverPane_AbsentStrand_TreatedAsNotReady`, `TestAwaitDriverPane_SeamErrors` and `TestAwaitDriverPane_AttemptBudgetCountedNotTimed`, and drop the now-unused `errors` import.
  Keep the compile-time assertion `_ driverHandle = (*shuttleengine.Run)(nil)`: it now also proves `*shuttleengine.Run` satisfies the widened interface.
  `countingWait` in `internal/loomcli/bootstrap_test.go` stays, since the run-lock handshake tests still use it.

  In `internal/loomcli/start_driver_test.go`:
  - give `stubDriverHandle` three more fields, `ready bool`, `awaitErr error` and `awaitCalls *int`, and an `AwaitStarted` method that increments `*awaitCalls` when non-nil and returns `(ready, awaitErr)`; existing literals that set only `guid`/`runDir` keep compiling;
  - rename `TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnProbeRefusing` to `TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnReadinessRefusing` and drive it with a handle whose `ready` is false: assert `false` is returned, `AwaitStarted` was called exactly once, the envelope output contains `driver strand did not come up`, the run dir and the strand guid, and the bootstrap lock was released; rewrite its doc comment, which no longer spends a real ~5s budget;
  - add `TestRunDriverSpawnAndWait_LLMArm_ReleasesLockOnReadinessError`: a handle whose `awaitErr` is set returns `false`, the envelope output contains that error's text, and the bootstrap lock was released;
  - in `TestRunDriverSpawnAndWait_LLMArm_NeverConsultsTheRunLockHandshake`, use a handle with `ready: true`, replace the two-phase `strandsFn` with `noStrands`, and additionally assert `AwaitStarted` was called exactly once;
  - renumber the failure-site doc comments from "of 6" to "of 7" (the readiness error becomes site 7), and update the "six" wording in the `noStrands` and `assertBootstrapLockReleased` doc comments to "seven".

  Before committing, run `grep -rnE "pane-liveness|liveness probe|awaitDriverPane|driverPane(Attempts|PollInterval)|pane (is live|coming alive|for liveness)" internal/loomcli/ internal/shuttleengine/ docs/` and confirm no hit describes the llm arm's readiness as pane liveness; also read any comment line ending in "pane-liveness" or "pane" in the edited files for a wrapped match.
- **Commit:** `fix(loom): await the llm driver's provider readiness, dismissing startup gates`

### Card 4: Adapt the driver-strand smoke test to the readiness signal

- **Context:**
  - `internal/loomcli/smoke_test.go`
  - `internal/shuttleengine/template.yaml`
  - `internal/shuttleengine/claudeengine/startup.go`
- **Edits:**
  - `internal/loomcli/smoke_driverstrand_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Under the new signal, the stub provider in `writeStubDriverScript` (a bare `sleep 3600`) never renders anything `claudeengine`'s `Startup` reads as ready, so every `loom start --no-attach` in `TestSmokeDriverStrand_ReentrantAcrossThreeBootstraps` would refuse at `startup_timeout_s` (90 in the shipped template) or be killed by the test's own 30s `runLoomCLINoFatal` timeout.

  In `internal/loomcli/smoke_driverstrand_test.go`:
  - change `writeStubDriverScript`'s script so it prints a ready-marker line before sleeping, `"#!/bin/sh\necho '? for shortcuts'\nsleep 3600\n"`; `Startup` classifies a capture containing `shortcuts` as `StartupReady`, and an ASCII marker avoids depending on the pane's encoding of the `❯` glyph; rewrite the function's doc comment to say why the line is printed;
  - change `driverShuttleConfig` to take `t *testing.T` first, and additionally replace `startup_timeout_s: 90` with `startup_timeout_s: 10` in the template text, calling `t.Fatalf` if either replaced substring is absent from the template, so a template drift fails loudly rather than silently leaving the 90s window; update its one call site and its doc comment, which states that the lowered window keeps a readiness regression surfacing as a refusal envelope inside the 30s per-invocation timeout;
  - extend the file header comment with one sentence saying this test now also proves the llm arm's readiness await (`Run.AwaitStarted`) succeeds against a real reed pane.

  - capture the exit code `runLoomCLINoFatal` returns for each of the three `loom start --no-attach` invocations (today discarded as `_`) and fail with the captured output unless it is 0: `runLoomCLINoFatal` returns a nil error for a non-zero exit, and a readiness refusal leaves the live strand in place, so without this assertion the strand-count checks would pass on a refusal and the lowered window would turn a readiness regression into a silent pass.

  The test's three-bootstrap strand-count and liveness assertions are otherwise unchanged.
- **Commit:** `test(loom): drive the driver-strand smoke stub to a ready marker`

## Batch Tests

`go test ./internal/loomcli/` runs the package's untagged suite, including the edited `internal/loomcli/start_driver_test.go` and `internal/loomcli/driverlaunch_test.go`; package scope is needed because `internal/loomcli/start.go` is exercised by several test files beyond the two edited ones.
`go test -tags smoke -run TestSmokeDriverStrand ./internal/loomcli/` runs the one adapted smoke test against a real tmux server and a built `lyx` binary, which is the live-substrate check of `AwaitStarted`'s success path; it skips itself when tmux is absent.
The `integration`-tagged `internal/loomcli/integration_driverbootstrap_test.go` stops at `startLLMDriverArm` and never reaches the changed step-6 block, so it is left to the configured `done_gate`.
