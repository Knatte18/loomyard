# Discussion: Shuttle guarantees a started run is past its startup gates

```yaml
task: Shuttle guarantees a started run is past its startup gates
slug: shuttle-start-guarantees-readiness
status: discussing
parent: main
```

## Problem

Shuttle's `Run` contract lets a caller start a run and never pass through the startup probe — provider readiness plus dismissal of any one-time startup gate, driven through `Engine.Startup` and `Engine.TrustDismissSequence`.
That probe runs only inside `Run.Wait` (via `checkLivenessTick`), so a caller that starts a run and never waits on it skips it.
That is how `#018 llm-driver-trust-dialog-hang` happened: loom's llm driver arm started the driver run, never called `Wait`, and nothing dismissed Claude Code's workspace-trust dialog, so the child parked forever on a live pane.

PR #267 (commit `292a5a74b`) fixed the symptom by adding `Run.AwaitStarted` and having loom call it.
That still leaves each caller responsible for remembering, and exploration found a second caller with the same latent bug:
`websterengine.RecoverBatch` (`internal/websterengine/recoverbatch.go`) starts the recovery strand with `Starter.Start` and then polls the events file and strand liveness through its own `awaitTerminal` loop — it never calls `Wait` or `AwaitStarted`, so a recovery strand whose provider shows a trust gate parks until `RecoveryTimeoutMin`.

Principle (from the task): everything about how a provider is launched and set up belongs to shuttle and its engines.
Loom, shed, and webster must not need to know a provider has startup gates at all.
A second provider plugs in as its own `Engine`, and no caller changes.

## Scope

**In:**

- `internal/shuttleengine`: `Runner.StartGated` (and therefore `Start`, `RunGated`, `Run`) runs the startup probe to resolution before returning.
  A freshly started `*Run` handle is only ever issued past its startup gates.
- A not-ready start (pane died, or startup window expired without readiness) is torn down and reported as an error from `Start`/`StartGated`;
  `RunGated`/`Run` keep reporting it as `Result{Outcome: OutcomeDied}` with a nil error, as today.
- `Run.AwaitStarted` removed from the public surface; its loop becomes the private startup step inside start.
- `Run.Wait`'s `started` seed changes from `run.attached && run.state.Started` to `run.state.Started`.
- `internal/loomcli`: remove `driverHandle.AwaitStarted`, the llm-arm "Step 6" await block in `start.go`, and the `bootstrapLock`-held await; a not-ready driver now surfaces as `StartDriver`'s error.
  Update the `start` command's `Long` help text where it describes a strand "left in place by an earlier readiness refusal".
- `internal/websterengine` / `internal/webstercli`: no control-flow change — `RecoverBatch`'s `Starter.Start` and `Run`'s `StartMaster` inherit the guarantee.
  Both spawns run under webster's state-mutation lease, which is now held across the startup window;
  the lease's own contract wording and the affected doc comments change to state that bounded hold, the `recover-batch` blocking bound is reworded in the Master stencil, the verb's `Long` help, and `websterengine/doc.go`, and both persist-before-block residuals (Master and recovery strand) are stated (see Decisions "Webster's state-mutation lease across the startup window" and "Webster's persist-before-block windows").
- Tests: shuttleengine start tests replacing `awaitstarted_test.go`, fake adjustments so existing Start/Run tests reach readiness, loomcli test updates, completion-signal tripwire counts.
- Docs: `docs/reference/claude-trust-dialog-repro.md`, shuttleengine doc comments (`run.go`, `wait.go`, `doc.go`, `rundir.go`'s `Started` comment), `docs/overview.md`'s shuttle row if its wording is affected, `CONSTRAINTS.md` (new bullet, see Decisions).

**Out:**

- `Runner.Attach`/`AttachGated`: unchanged. An attached run whose `run.json` says `Started: true` still skips the probe;
  one that is not started is still probed inside `Wait`, as today.
- `Interrupt`/`Send`/`Inject` and `requireReadyAgentPane`: unchanged — they serve CLI verbs acting on a guid from another process.
- `Engine` interface and `claudeengine`: unchanged. No new provider gate is recognised.
- Webster's persist-before-block ordering: no new callback/hook seam is added to shuttle (see Decisions).
- The go-driver arm of loom's bootstrap (`awaitRunLock` handshake): untouched.
- `manifest/roadmap.md`: this is a hardening fix, not a planned item.

## Decisions

### Readiness is guaranteed by a blocking start

- Decision: `Runner.StartGated` runs the startup probe after persisting `run.json` and before returning the handle.
  It returns a `*Run` only when the probe resolved ready (provider reached `StartupReady`, with `Started` persisted) or the run's file contract is already satisfied (`allOutputFilesExist`).
  `Start` delegates to `StartGated` as today, so both carry the guarantee; `RunGated`/`Run` inherit it through their `StartGated` call.
- Rationale: an error return in Go is the one result a caller cannot silently skip, and a handle that exists only after readiness makes "forgot to await" unrepresentable.
  Loom's own driver process exits after `start` hands the terminal over, so any asynchronous alternative dies with it.
- Rejected:
  - A background goroutine launched by `Start` running the probe — dies when a short-lived caller (loom `start`, webster `recover-batch`'s spawn phase) exits, which is exactly the case that broke.
  - A separate "ready handle" type issued by a second call — still a call the caller must remember to make.
  - Keeping `AwaitStarted` public and adding a lint/test that every `Start` caller calls it — enforcement by review, not by contract.

### Not-ready start: error, strand torn down, run dir and last capture kept

- Decision: when the startup step resolves not-ready (the pane is not live, or the startup window expires, both via `checkLivenessTick`/`classifyStartupWindow` — which already consult the file contract first), start:
  1. writes the last successful pane capture the probe took to `startup-capture.txt` in the run directory — the file is absent when no capture ever succeeded, never written empty;
  2. persists the resolved outcome to `RunState.Outcome` — `died`, or `timeout` when `run.deadline` expired first (decision "Run deadline anchoring") — so `Attach` never treats the record as attachable;
  3. removes the strand via `reed.RemoveStrand(guid, false)` (non-fatal on failure, logged at Warn);
  4. keeps the run directory;
  5. logs at Warn with run dir and strand guid;
  6. returns an error wrapping an exported sentinel `ErrNotStarted`, whose message names the run directory, the strand guid, that the strand was removed, and either that its last capture was saved to `startup-capture.txt` or that no capture was taken.
- The startup-capture file name is a new constant beside `promptFileName`/`settingsFileName`/`eventsFileName` in `run.go`.
- Rationale: a provider that never became ready has done no work worth preserving, and leaving its strand live caused two defects: loom's next `start` resolves the stuck strand as `driverStrandLive`, spawns nothing and "succeeds" without readiness (the accepted residual documented in `start.go`), and webster's `Run` never learns the strand guid of a Master that failed in `StartMaster`, so its entry-time reclaim can never remove it.
  Reed's add has no upsert semantics, so a lingering strand also collides by name with the respawn.
  The saved capture replaces "attach to the pane to see what it is stuck on" as the diagnosis channel — it is what diagnosed #018.
- Rejected: leaving the strand in place (today's `Wait`-path behavior for a startup `OutcomeDied`) — keeps both defects above.
  Removing the run directory too — loses the diagnosis artifacts; `sweepOrphansOpportunistic` already reclaims it once its strand is gone and it is older than `2 × startup_timeout_s`.

### Startup mechanism failure: error with identity, no teardown

- Decision: when `checkLivenessTick` errors `maxStatusRetries` consecutive times during the startup step and the file contract is unsatisfied, start returns an error in the same wording family `Wait` uses (`errStrandNotTracked`, `errStrandPaneBindingCleared`, or reed-status-failed), naming strand guid and run dir, and performs no teardown and no Outcome write.
  The error does not wrap `ErrNotStarted`.
- Rationale: identical to `Wait`'s own reasoning — reed's bookkeeping failing says nothing about the agent, so neither "died" nor a strand removal is justified.
- Rejected: tearing down on mechanism failure — a `RemoveStrand` against an unanswerable reed would likely fail anyway and could kill a live agent if it succeeded.

### RunGated preserves its OutcomeDied contract

- Decision: split the body of `StartGated` into a private `start(spec, gate)` that returns the handle plus, on a not-ready resolution, the finalized `Result` (`Outcome: OutcomeDied`, identity fields, `RunDir`).
  `StartGated` turns a not-ready resolution into the `ErrNotStarted` error; `RunGated` returns `(result, nil)` for it, then calls `Wait` otherwise.
  The finalize path used for the not-ready case must NOT evaluate the gate (the outcome is never `OutcomeDone`) and must record the outcome and log "run finished" as `finalize` does today.
  Whether that reuses `finalize` plus the teardown steps, or a dedicated helper, is a plan choice — but it must still run through the Completion Signal Invariant (see Constraints) and live where the tripwire scans (see decision "Startup step lives in a tripwire-scanned file").
  On a startup mechanism failure (decision "Startup mechanism failure"), the private `start` also hands back the run's `identity()` Result (`SessionID`, `StrandGUID`, `RunDir`, empty `Outcome`) beside the error, and `RunGated` returns `(identity, err)` — the same identity-with-error shape `Wait`'s own mechanism-failure exits return today, so a `RunGated` caller loses nothing compared with the failure surfacing from `Wait`.
  Failures before a strand exists (spec validation, `Prepare`, `AddStrand`, `saveRunState`) keep returning `Result{}` as today.
  `StartGated` returns `(nil, err)` for both failure kinds; its error message already names strand guid and run dir.
- Rationale: the burler round producer (`internal/shedadapters/burler.go`, over `burlerengine`'s `RunGated` call at `internal/burlerengine/engine.go:184`) branches on the outcome: a first `OutcomeDied`/`OutcomeTimeout` attempt is retried once as infrastructure, while a returned `RunGated` error fails the round immediately.
  Converting a startup failure into an error would silently drop that retry.
  `shedadapters.SingleLLMProducer` is indifferent — its `mapOutcome` (`internal/shedadapters/singlellm.go:217–222`) turns `OutcomeDied` into a returned error, the same path a `RunGated` error takes — so the plan must not rely on a distinction there.
- Rejected: making `RunGated` return the error — removes burler's one-retry ladder for startup failures.

### Startup step lives in a tripwire-scanned file

- Decision: the startup step (the moved `AwaitStarted` loop), the not-ready teardown, the construction of the not-ready `Result{Outcome: OutcomeDied}`, and the `ErrNotStarted`/mechanism-failure `Errorf` returns all live in `wait.go`, which `completionsignal_enforcement_test.go`'s `completionSignalScannedFiles` (`{"wait.go", "attach.go"}`) already scans.
  `run.go`'s `StartGated`/`start`/`RunGated` only call that step and pass its `*Run`, `Result` or error through; they construct no negative-verdict marker themselves.
  The tripwire's pinned counts are re-audited for the new function(s) in `wait.go`.
  If the plan finds `run.go` must construct any negative-verdict marker after all, it adds `run.go` to `completionSignalScannedFiles` and pins those sites in the same change — never leaves a new negative verdict in an unscanned file.
- Rationale: the tripwire only sees the files it lists; a verdict built in `run.go` would make "the tripwire was updated" pass vacuously.
- Rejected: adding `run.go` to the scan unconditionally — `run.go` carries many unrelated `Errorf` returns (spec validation, told-path checks, `Send`/`Interrupt`), which would bloat the pinned ledger without guarding a completion verdict.

### AwaitStarted removed

- Decision: delete `Run.AwaitStarted`.
  Its loop body (probe cadence `pollInterval × LivenessEveryNPolls`, tick cap `awaitStartedTickCap`, first probe immediate, status-retry handling) moves unchanged into the private startup step; rename `awaitStartedTickCap` to match (e.g. `startupTickCap`).
  `Run.RunDir()` stays exported (loom logs it; webster uses `FindRun`), with its doc comment updated to drop the AwaitStarted reference.
- Rationale: no caller remains once start guarantees readiness; the task brief asks for its removal when unused.
- Rejected: keeping it as a no-op for compatibility — this is an internal package with no external consumers.

### Wait's startup handling after the move

- Decision: `Wait` seeds `started := run.state.Started` (dropping the `run.attached &&` conjunct).
  A `*Run` from start carries `Started: true` unless it was returned on the satisfied-file-contract branch.
  There, `Wait` re-probing is harmless but not instant: with `Started: false` and a live strand, `checkLivenessTick` consults the file contract only through `classifyStartupWindow`'s expiry, so `Wait` classifies `OutcomeDone` on an events-tick Done (a turn-end event with every output file present) or, at the latest, when its own startup window expires (`classifyDeadlineExpiry` → `OutcomeDone`).
  Accepted as is: this branch needs an agent that wrote every output file before its TUI ever classified ready, which is an edge case, and the answer is still correct.
  The startup probe code in `checkLivenessTick`/`classifyStartupWindow` stays, now reached only for an attached run whose `run.json` was never marked `Started`.
  `Wait` still computes its own `startupDeadline` from its entry time for that attached case.
- Rationale: keeps one probe implementation for both the start and the attach paths; `Started` is already the persisted fact meaning "passed the probe".
- Rejected: moving the attach-path probe into `AttachGated` too — `Attach` always calls `Wait` itself, so it cannot be skipped, and the change would widen scope for no guarantee gained.

### Run deadline anchoring

- Decision: `run.deadline` stays `clock.Now().Add(spec.Timeout)` computed right after `run.json` is saved, before the startup step, so `spec.Timeout` keeps covering startup plus work.
  The startup step honours it: after each probe, when `run.deadline` has passed and the provider is still not ready, the step resolves not-ready with outcome `classifyDeadlineExpiry(OutcomeTimeout)` — `OutcomeDone` if the file contract is satisfied (the handle is then returned normally), `OutcomeTimeout` otherwise.
  That order matches `Wait` today, which checks the startup window inside `checkLivenessTick` and then `run.deadline` on the same tick, so a `spec.Timeout` shorter than `startup_timeout_s` (reachable via `lyx shuttle run --timeout`) still ends as `OutcomeTimeout` at `spec.Timeout` rather than `OutcomeDied` at the startup window.
  A run-deadline not-ready resolution takes the same teardown as any other not-ready one (capture file, Outcome persisted — `timeout` here — strand removed, run dir kept, `ErrNotStarted` from `StartGated`), and `RunGated` returns `(Result{Outcome: OutcomeTimeout, …}, nil)`.
  The tick cap is computed from the shorter of the two remaining windows, so the count bound still terminates the loop under a clock that never advances.
- Rationale: unchanged wall-clock budget semantics and unchanged outcome classification for every caller.
- Rejected: anchoring the deadline after readiness — silently extends every run's budget by its startup time.
  Ignoring `run.deadline` in the startup step — turns a short-timeout run's `OutcomeTimeout` into `OutcomeDied` after up to `startup_timeout_s`, an observable change.

### Webster's state-mutation lease across the startup window

- Decision: accept that webster's state-mutation lease (`websterengine.AcquireStateMutation`, `internal/websterengine/state.go:84–88`) is now held across a spawn's startup window, at both sites that spawn under it:
  `lyx webster recover-batch` (`internal/webstercli/recoverbatch.go`: lease acquired before `RecoverSpawnOrAttach`, released after `SaveState`) and `websterengine.Run`'s Master spawn (`internal/websterengine/runlevel.go`: lease acquired ~line 408, `StartMaster` ~line 612, released ~line 648).
  The hold is bounded: typically one or two probe intervals (about 5–10 s under the shipped config), at most `startup_timeout_s` (90 s), after which a not-ready start errors and tears its strand down.
  Concurrent verbs (`begin-batch`, `record-batch`, `validate`, run entry) block on the lease for that time rather than failing — `lock.AcquireWriteLock` blocks without a timeout.
  Update the contract wording on `AcquireStateMutation` to say what "never across a long block" means now: a spawn's startup window, bounded by `startup_timeout_s`, is part of the load-mutate-save sequence; an unbounded or poll-length wait (recover-batch's `RecoverAwait`, Master's `Wait`) still never runs under it.
  Update `internal/webstercli/recoverbatch.go`'s file header and `RecoverSpawnOrAttach`'s doc to match, and add a sentence at the Master spawn site in `runlevel.go`.
- A `recover-batch` call that spawns now blocks for the startup window and then up to `--wait`/`poll_wait_s`, because `RecoverAwait`'s budget starts only after `RecoverSpawnOrAttach` returns.
  A call that attaches to an existing recovery strand is still bounded by `poll_wait_s` alone.
  Reword every place that states the old bound:
  - `contracts/stencils/webster/webster-template-master.md` lines ~105 and ~198 ("each call blocks at most `{{.poll_wait_s}}` seconds") — say the call that spawns the recovery strand additionally waits for its provider to come up (normally seconds), and every re-poll after it is bounded by `{{.poll_wait_s}}`.
    No new template variable is introduced; the startup bound is described, not interpolated.
  - `recover-batch`'s `Long` help (`internal/webstercli/recoverbatch.go`, "for up to --wait") — same statement.
  - `internal/websterengine/doc.go` (~line 201, "most poll_wait_s") — same statement.
  The stencil is an embedded default read from the hub's stencils directory, so hubs with a copied stencil keep the old wording until refreshed; that is harmless, since the old wording only understates one call's duration.
- Rationale: holding the lease across the spawn is what serialises two concurrent `recover-batch` calls for the same batch, so the second one sees the first one's recorded guid and attaches instead of spawning a duplicate recovery strand.
  `RecoverSpawnOrAttach`'s attach test requires `prior.StrandGUID != ""`, so releasing the lease across the spawn would need a new "spawn in progress" state record with its own attach/timeout semantics, plus a re-acquire-reload-merge after the spawn.
  That costs far more than a bounded stall of concurrent verbs, which webster already tolerates for every other holder.
  At run entry no batch forks exist yet, so the Master-site hold stalls nothing in practice.
- Rejected: reserve an intent record under the lease, spawn with the lease released, re-acquire to persist the guid — needs the in-progress state and attach semantics above, and still leaves the killed-mid-startup residual below.

### Webster's persist-before-block windows

- Decision: accept that webster persists a spawned strand's guid to `state.json` only after the start call returns, which now includes the startup window, at both spawn sites:
  - `websterengine.Run` persists `MasterStrand` after `StartMaster` returns — a webster process killed inside the startup window leaves a live Master pane its entry-time reclaim cannot see;
  - `lyx webster recover-batch` persists the recovery `BatchState` (with its `StrandGUID`) after `RecoverSpawnOrAttach` returns — a process killed inside the startup window leaves a live recovery strand that the next `recoverSpawn`'s `prior.StrandGUID` reclaim (`removeStrandIfLive`) cannot see, so the next call spawns a second recovery strand beside it.
  State each as an Accepted residual: in `MasterHandle`'s doc comment, in `RecoverSpawnOrAttach`'s doc comment, and beside `Start`'s existing AddStrand-to-saveRunState residual comment in `internal/shuttleengine/run.go` (as the general form: a caller that persists the guid after `Start` returns now has a window as wide as the startup probe).
  A not-ready start no longer leaks at either site, because the strand is torn down (decision "Not-ready start").
- Rationale: both windows already exist (between `AddStrand` and the caller's own save); widening them is bounded by `startup_timeout_s`, while closing them would need a new pre-readiness callback seam on shuttle's `Spec` that exists only for webster.
- Rejected: a `Spec.OnRegistered func(guid string) error` hook called after `run.json` persists and before the probe — adds a public seam for one caller, and a callback that can fail mid-start complicates teardown.

### Loom's llm arm

- Decision: `driverHandle` shrinks to `StrandGUID()`/`RunDir()`.
  `startLLMDriverArm` returns `StartDriver`'s error unchanged; `runDriverSpawnAndWait`'s llm-arm "Step 6" block (the `AwaitStarted` call, its two refusals, and the "driver strand is ready" log) is deleted.
  The refusal loom prints for a not-ready driver becomes loom's existing error path for a failed `StartDriver`, whose message now comes from shuttle and already names the run dir and strand.
  Keep the "loom: driver strand is ready" `logger.Info` breadcrumb, moved to right after a successful `StartDriver` (merging with the existing "spawned driver strand" line is acceptable).
  `bootstrapLock` is still held across `StartDriver`, now including the startup step — the same duration it was held across `AwaitStarted`.
- `start`'s `Long` help: replace the sentence about "an ly-drive strand already live from an earlier invocation -- including one left in place by an earlier readiness refusal" with wording that says a readiness refusal removes the driver strand, so the next `start` spawns a fresh one.
  Keep, and name explicitly, the one remaining residual: when the readiness check could not get an answer from reed at all (decision "Startup mechanism failure"), the strand is left in place, and a later `start` that finds it live attaches to it without re-checking readiness.
  Update `start.go`'s "Accepted residual" code comment to the same narrower case.
  This residual is accepted: reed being unable to answer says nothing about the agent, so tearing the strand down could kill a working driver.
- Rationale: loom no longer knows about readiness at all, which is the stated principle.
- Rejected: loom keeping its own post-start check "just in case" — re-creates the per-caller obligation.

### New CONSTRAINTS.md bullet

- Decision: add under `## Shuttle Provider-Seam Invariant`:
  "A `*Run` issued by `Start`/`StartGated` has already resolved its provider's startup probe; no caller outside `internal/shuttleengine` probes provider readiness or plays startup-gate keys."
- Rationale: it is a cross-cutting rule for every current and future caller, and CLAUDE.md requires new cross-cutting invariants to land in `CONSTRAINTS.md` in the same commit.

## Technical context

- `internal/shuttleengine/run.go`: `StartGated` (sweep → `createRunDir` → `Engine.Prepare` → `reed.AddStrand` → `saveRunState` → return handle).
  The startup step goes after `saveRunState` and the `logger.Info("shuttle: run started" …)` line, on the constructed `*Run` (it needs `run.clock`, `run.spec`, `run.state`, `run.runDir`).
- `internal/shuttleengine/wait.go`: `Wait`, `AwaitStarted` (loop to move), `awaitStartedTickCap`, `checkLivenessTick` (does capture, `Engine.Startup`, `TrustDismissSequence` replay, persists `Started`), `classifyStartupWindow`, `classifyDeadlineExpiry`, `finalize`, `identity`.
  `checkLivenessTick` currently discards its capture; the teardown's `startup-capture.txt` needs the last capture — the plan must thread it out (e.g. record the last successful capture on the `*Run` or return it).
- `internal/shuttleengine/attach.go:193–221`: reconstructs `*Run` with `attached: true`; untouched apart from the doc comment on the `attached` field in `run.go` that references `AwaitStarted`.
- `internal/shuttleengine/rundir.go:94–106`: `RunState.Started` doc comment references `AwaitStarted`.
- `internal/shuttleengine/completionsignal_enforcement_test.go`: pins negative-verdict return sites and `allOutputFilesExist` call sites per function name (`"AwaitStarted [Errorf]": 3`, `"AwaitStarted": 2`); those entries move to the new function's name and counts are re-audited.
- `internal/shuttleengine/fakes_test.go`: `fakeEngine.Startup` returns `StartupPending` when `StartupScript` is empty, and `fakeReed.CapturePane` returns `""` when `CaptureQueue` is empty.
  Every existing `Start`/`Run`/`RunGated` test (about 55 call sites across `run_test.go`, `wait_test.go`, `attach_test.go`, `gate_test.go`, `posix_test.go`) would therefore sit in the startup window after this change.
  The plan must give those tests a ready startup (e.g. a helper or default that yields `StartupReady` for start, while tests that exercise `Wait`'s own startup window move to the attached-not-started path or to start tests).
  Also check how those tests configure `StartupTimeoutS` and the clock so a not-ready start cannot hang a test.
- `internal/shuttleengine/awaitstarted_test.go`: the existing coverage of the probe loop (cadence, tick cap, gate dismissal, status retries, file-contract short-circuit) — port to start-level tests.
- `internal/loomcli/driverlaunch.go` (`driverHandle`, `runnerDriverStarter`), `internal/loomcli/start.go` (`startLLMDriverArm` ~line 64–92, llm-arm Step 6 block ~line 236–271, `Long` help ~line 290–320), `internal/loomcli/driverspec.go:45` (comment referencing `Run.AwaitStarted`), `internal/loomcli/start_driver_test.go` (`stubDriverHandle.AwaitStarted`, await-call assertions), `internal/loomcli/smoke_driverstrand_test.go` (tagged smoke test proving readiness against a real reed).
- `internal/websterengine/runlevel.go:83–106, 612–651` (`MasterHandle`, `StartMaster`, persist then `Wait`), `internal/websterengine/recoverbatch.go:183` (`Starter.Start`, the second latent #018 caller), `internal/websterengine/strand.go:66–70` (`Starter` interface returning `*shuttleengine.Run`).
  Webster maps a non-done Master outcome to an error already, so a Master start error is equivalent at its caller.
- Webster's state-mutation lease: `internal/websterengine/state.go:84–88` (`AcquireStateMutation` and its "never across a long block" contract), `internal/webstercli/recoverbatch.go` (file header describing the three lease-scoped phases; lease held across `RecoverSpawnOrAttach` + `SaveState`, released before `RecoverAwait`), `internal/websterengine/recoverbatch.go:225–248` (`RecoverSpawnOrAttach`, attach condition `prior.Kind == "recovery" && !prior.Terminal && prior.StrandGUID != ""`), `internal/websterengine/runlevel.go:408–648` (lease held across `StartMaster` and both `SaveState` calls, released before `handle.Wait()`).
- Production callers of the start family found by a direct-call search (not guaranteed complete — several route through narrow interface seams that `*shuttleengine.Runner` satisfies structurally):
  - `StartGated`: `loomcli/cli.go:137` and `webstercli/cli.go:150` (both `runnerMasterStarter`);
  - `Start`: `loomcli/driverlaunch.go:51`, `websterengine/recoverbatch.go:183` (via `websterengine.Starter`);
  - `Run`/`RunGated` (directly or via a runner/`Shuttle` interface field): `burlerengine/engine.go:184`, `shedadapters/singlellm.go:169`, `shedadapters/bouncer.go`, `mergeresolve/mergeresolve.go`, `frictionengine/reflect.go`, `treadleengine/targeting.go`, `treadleengine/judge.go`, `shuttlecli/run.go:139`.
  The plan should enumerate by the interface seams (`grep` for methods named `Run`/`RunGated`/`Start`/`StartGated` on interfaces whose implementation is `*shuttleengine.Runner`) rather than trust this list.
  Every `Run`/`RunGated` caller keeps its current `OutcomeDied` semantics by construction, since `RunGated`'s contract does not change (decision "RunGated preserves its OutcomeDied contract"); only `Start`/`StartGated` callers see the new error.
- Shipped config (`internal/shuttleengine/template.yaml`): `poll_interval_ms: 500`, `liveness_every_n_polls: 10`, `startup_timeout_s: 90` — so the probe interval is 5 s and the first probe is immediate.

## Constraints

- **Completion Signal Invariant** (`CONSTRAINTS.md`, `wait.go` doc): the not-ready branch is a negative answer and must consult `allOutputFilesExist` (it does, via `checkLivenessTick`/`classifyStartupWindow`), and the retry-cap and tick-cap exits must consult it directly as `AwaitStarted` does.
  The tripwire test must be updated to the new function, not deleted.
- **Shuttle Provider-Seam Invariant**: nothing in the startup step, the teardown, or the capture file names a Claude specific; `shuttleengine` never imports `claudeengine`.
- **Told-Geometry Invariant**: `shuttleengine` derives no paths; the capture file lives in the told run directory.
- **Live-Substrate Spawn Observability**: the startup step's teardown is a lifecycle teardown and must be logged (Warn); the existing "dismissed startup gate" Info line stays.
- **Test Tier Purity**: real reed/tmux use stays in tagged smoke tests; untagged tests use `fakeReed`/`fakeEngine` and loom's stub seams.
- **CLI/Cobra Invariant**: loom `start`'s help text changes; the help-tree tests must still pass.
- **Documentation Lifecycle / task-completion rule**: docs and the new `CONSTRAINTS.md` bullet land in the same commit as the behavior change.
- Markdown in this repo uses semantic line breaks (CLAUDE.md).

## Testing

- **shuttleengine start (TDD candidates)** — port `awaitstarted_test.go` to start-level tests with `fakeReed`/`fakeEngine` and the fake clock:
  - trust prompt then ready → dismiss sequence played once per gate-showing probe, handle returned, `run.json` `Started: true`;
  - ready on first probe → no sleep before return;
  - pane not live → `ErrNotStarted`, strand removed, `run.json` Outcome `died`, run dir kept, `startup-capture.txt` holds the last capture;
  - window expires while pending → same as above;
  - capture always erroring until the window expires → `ErrNotStarted`, and `startup-capture.txt` is absent (it is written only when at least one capture succeeded), and the error message says no capture was taken rather than naming the file;
  - file contract satisfied during startup → handle returned, no teardown;
  - `maxStatusRetries` consecutive status errors → error naming guid and run dir, not `ErrNotStarted`, no `RemoveStrand`, no Outcome write;
  - tick cap terminates under a clock that never advances;
  - probe cadence equals `pollInterval × LivenessEveryNPolls`.
- **RunGated**: startup failure → `(Result{Outcome: OutcomeDied, …}, nil)`, gate closure never invoked; startup mechanism failure → `(identity Result, err)` with `SessionID`/`StrandGUID`/`RunDir` populated.
- **Run deadline during startup**: `spec.Timeout` shorter than the startup window with a never-ready provider → `RunGated` returns `OutcomeTimeout` at `spec.Timeout` (not `OutcomeDied`), `StartGated` returns `ErrNotStarted`, strand torn down, `run.json` Outcome `timeout`; with the file contract satisfied at that point → handle returned / Wait classifies `OutcomeDone`.
- **shedadapters burler round**: an existing or new test proves a startup-failed first attempt (`OutcomeDied` from the runner seam) still takes the one-retry path — guards the reason `RunGated` keeps `OutcomeDied`.
- **Wait**: a started run's `Wait` issues no `CapturePane`/`Startup` calls; an attached run with `Started: false` still runs the startup probe and still classifies `OutcomeDied` at the window's end; an attached run with `Started: true` still skips it.
- **Completion-signal tripwire**: updated counts pass and still fail if a guard is deleted.
- **Existing shuttleengine suites**: must pass unchanged in intent after the fake/helper adjustment for a ready start.
- **loomcli**: `StartDriver` error → bootstrap refuses with shuttle's message and releases `bootstrapLock`; successful start → no further readiness call exists (compile-level, since the method is gone); help-tree test passes with the new `Long` text.
  The tagged smoke test `smoke_driverstrand_test.go` still proves a real ly-drive strand comes up through `Start` alone.
- **websterengine**: a `Starter` fake returning an error from `Start` → `RecoverBatch` surfaces it (existing coverage may already hold this; verify rather than duplicate).

## Q&A log

- **Q:** How should shuttle guarantee readiness — blocking start, a readiness-gated handle type, or a background probe? **A:** [auto-pick] Blocking `Start`/`StartGated`. **Why:** the error return is the one result no caller can skip, and short-lived callers (loom `start`) would kill a background probe.
- **Q:** What happens to a started run whose provider never becomes ready? **A:** [auto-pick] Return `ErrNotStarted`, remove the strand, keep the run dir, save the last capture, persist Outcome `died`. **Why:** a live stuck strand makes loom's next `start` skip readiness and leaks an unreclaimable webster Master; the capture keeps the diagnosis.
- **Q:** Should `RunGated`/`Run` surface startup failure as an error or keep `OutcomeDied`? **A:** [auto-pick] Keep `(Result{OutcomeDied}, nil)`. **Why:** shed producers route `OutcomeDied` and errors through different ladders.
- **Q:** Keep `Run.AwaitStarted` public? **A:** [auto-pick] Remove it. **Why:** no caller remains, and the brief asks for removal when unused.
- **Q:** Close webster's widened persist-before-block windows (Master and recovery strand) with a new callback seam? **A:** [auto-pick] No — accept and document both residuals. **Why:** the windows already exist, are bounded by `startup_timeout_s`, and a one-caller hook on `Spec` costs more than it closes.
- **Q:** Webster's state-mutation lease is now held across the startup window in `recover-batch` and the Master spawn — restructure to release it across the spawn, or accept the bounded hold? **A:** [auto-pick] Accept the bounded hold and reword `AcquireStateMutation`'s contract. **Why:** the lease across the spawn is what stops two concurrent `recover-batch` calls from spawning duplicate recovery strands, and releasing it would need a new in-progress state record with its own attach semantics.
- **Q:** What should `Wait` do about startup after the move? **A:** [auto-pick] Seed `started` from `run.state.Started` alone; keep the probe for attached-not-started runs. **Why:** one probe implementation serves both paths, and `Started` is already the persisted "passed the probe" fact.
- **Q:** Does `spec.Timeout` still include startup time? **A:** [auto-pick] Yes — the deadline is anchored before the startup step. **Why:** unchanged budget semantics for every caller.
- **Q:** Does the startup step honour `run.deadline`, and with which outcome? **A:** [auto-pick] Yes, through `classifyDeadlineExpiry(OutcomeTimeout)`, with the same teardown as any not-ready start. **Why:** matches `Wait`'s per-tick deadline check today, so a short `--timeout` run still reports `timeout`.
- **Q:** Does `RunGated` keep returning identity fields on a startup mechanism failure? **A:** [auto-pick] Yes — the private `start` hands back `identity()` beside the error. **Why:** that is what `Wait`'s mechanism-failure exits return today.
- **Q:** Where do the startup step's negative verdicts live, given the completion-signal tripwire scans only `wait.go`/`attach.go`? **A:** [auto-pick] In `wait.go`; `run.go` only passes them through. **Why:** keeps the tripwire's coverage real without pinning `run.go`'s many unrelated `Errorf` returns.
- **Q:** Stencil and help text promise `recover-batch` blocks at most `poll_wait_s` — reword or leave? **A:** [auto-pick] Reword: a spawning call also waits for the provider to come up. **Why:** Master sizes its own expectations from that stencil claim.
- **Q:** Loom help after a mechanism-failure start still leaves a live strand — accept? **A:** [auto-pick] Accept and name it in the `Long` help. **Why:** reed being unable to answer is no evidence the driver is stuck, so tearing it down could kill a working one.
- **Q:** Webster `recover-batch` has the same latent bug — in scope? **A:** [auto-pick] Yes, fixed by inheritance with no webster control-flow change, only lease-contract and doc-comment updates. **Why:** that is the point of a shuttle-level guarantee.
