# Discussion: llm-driven child can park forever on Claude Code's own workspace-trust dialog

```yaml
task: llm-driven child can park forever on Claude Code's own workspace-trust dialog
slug: llm-driver-trust-dialog-hang
status: discussing
parent: main
```

## Problem

A shed run seeded with `driver: llm` boots its driver as a Claude Code session (running the `ly-drive` skill) in a reed strand.
When the child worktree's absolute path has never been trusted on this host (no entry in `~/.claude.json`'s `projects` map), Claude Code opens its one-time "trust this folder" dialog, and nothing ever presses a key.
The session parks forever.
From outside, the parked session is indistinguishable from a healthy long run, so batten's `Run-Shed` waits out its whole bounce budget (1440 bounces at 30s = 12h) before anything notices.

The cause is a wiring gap, not a missing capability.
`internal/shuttleengine` already classifies the dialog (`Engine.Startup` → `StartupTrustPrompt`) and dismisses it (`Engine.TrustDismissSequence`), but only inside `Run.Wait`'s startup probe (`checkLivenessTick`, `internal/shuttleengine/wait.go`).
Loom's llm arm (`internal/loomcli/start.go`, `runDriverSpawnAndWait`) starts the driver run and never calls `Wait` — it only polls reed for pane liveness (`awaitDriverPane`, `internal/loomcli/driverlaunch.go`).
A pane sitting on the dialog is live, so the probe reports ready, and the bootstrap returns success.
Inner phase agents are unaffected, because producers do go through `Run.Wait`.
Crucible round sonnet-xhigh-r5 confirmed that the hang is in the outer ly-drive launch only.

Why now: the batten end-to-end campaign carried this finding across rounds r2–r5.
Round r3 lost it by reusing an already-trusted fixture path, and round r4 pinned the absolute-path trigger.
An llm-driven child is unusable on any fresh fixture path until this is fixed.

## Scope

**In:**

- A new exported startup-await method on `*shuttleengine.Run` that runs the existing startup probe loop (liveness + capture + `Startup` classification + trust-gate dismissal) until the provider reaches `StartupReady` or the startup window closes.
- Loom's llm arm calls it through its `driverHandle` seam in place of `awaitDriverPane`.
  `awaitDriverPane`, `findStrandByGUID` (if nothing else uses it), `driverPanePollInterval` and `driverPaneAttempts` are deleted along with their tests.
- The same dismissal also covers the bypass-permissions gate, since `claudeengine.Startup` classifies both one-time gates as `StartupTrustPrompt`.
  The driver runs with `--dangerously-skip-permissions` (`Interactive: false`), so a host that never accepted that gate would park the same way.
  This comes for free with the mechanism, not as separate work.
- Doc updates: the `start` command's `Long` help text ("the strand's own pane coming alive for an ly-drive driver") and `docs/overview.md`'s `lyx loom start` description, if it names the readiness signal.
  Also doc comments that describe the probe: `driverSpec`'s comment ("polled through the pane-liveness probe"), `startLLMDriverArm`'s comment, `runDriverSpawnAndWait`'s comment, and the step-6 llm-arm block comment in `start.go`.
- The reproduction recipe below, recorded in this discussion and followed as the task's live verification.

**Out:**

- The observability gap from round r5: "still running" stays ambiguous between a healthy long campaign and a session parked on something else.
  Batten also still waits out its full budget for any other kind of park.
  Dismissing the dialog does not narrow this, and folding it in would turn a wiring change into an observability redesign.
- Pre-seeding `~/.claude.json` (rejected — see Decisions).
- Trusting an ancestor directory (rejected — see Decisions).
- Dismissing a dialog on a driver strand that is already live when `lyx loom start` runs (`driverStrandLive` → no spawn → no probe).
  A strand left parked by a pre-fix bootstrap stays parked, and the operator sees the dialog on attach.
  Only newly spawned drivers are covered.
- The go-driver arm and its run-lock handshake: untouched, byte for byte.
- Any change to `claudeengine`'s classifier or dismiss choreography.
- batten, `battenshed`, `battencli`, `battenrecipe`: not touched.
- An automated live test that launches a real `claude` at an untrusted path (see Testing).

## Decisions

### Mechanism: an exported startup-await on `*shuttleengine.Run`, reusing `checkLivenessTick`

- Decision: add a method on `*shuttleengine.Run`, working name `AwaitStarted() (bool, error)`, in a new file in `internal/shuttleengine`, for example `startup.go`.
  It loops `run.checkLivenessTick(&started, startupDeadline)` on `run.clock`.
  The deadline is `cfg.StartupTimeoutS` from now, and each tick sleeps `pollInterval(cfg)`.
  Loom's `driverHandle` interface gains the method; `*shuttleengine.Run` satisfies it directly, as it already does for `StrandGUID`/`RunDir`.
  `start.go`'s step-6 llm block calls `driverRun.AwaitStarted()` instead of `awaitDriverPane`.
  The existing refusal envelope ("loom: driver strand did not come up; see run dir … (strand …)") is kept for the not-ready answer.
  The existing error pass-through is kept for the error answer.
- Rationale: `checkLivenessTick` is the one place that already owns classify→dismiss, including the hard-won adjacency and caret rules and the startup-window deadline.
  Calling it keeps one implementation.
  The proposal's own framing is "wiring an existing seam into one more code path, not building anything."
  `TrustDismissSequence` stays behind the provider-agnostic `Engine` interface, so loomcli learns nothing Claude-specific (Shuttle Provider-Seam Invariant).
- Rejected:
  - A loomcli-side probe that captures the pane and calls `engine.Startup`/`TrustDismissSequence` itself duplicates shuttle's loop and deadline logic in a second package, and it forces loomcli to hold the `Engine` value.
  - Running `Run.Wait` in a goroutine does not work because the bootstrap process exits after handover.
  - A one-shot "dismiss if showing" call after the liveness probe is racy: the dialog renders a beat after the pane goes live.
  - Pre-seeding `~/.claude.json` couples loomcli to Claude Code's private config schema, while `TrustDismissSequence` sits on the provider-agnostic `Engine` interface.
    It also read-modify-writes a blob (`oauthAccount`, `machineID`, caches) that every live session rewrites wholesale, which is a clobber race.
  - Trusting an ancestor directory does not work because the gate's ancestor walk stops at the repo boundary, and every fixture is its own repo.
    Observed: `/home/hanf/Code` is trusted, yet `/home/hanf/Code/r5sandbox/lyx-test-HUB/lyx-test` still holds its own entry.

### Readiness signal becomes "provider TUI ready", not "pane live"

- Decision: the llm arm's readiness signal is now `StartupReady` observed, which is strictly stronger than pane liveness.
  The time budget is shuttle's own `startup_timeout_s` (the same window every producer run gets), replacing loom's 50×100ms constants.
  `lyx loom start` (and `--no-attach`) therefore returns only once the driver's TUI is up and any one-time gate is dismissed.
  That can take a few seconds longer than today.
- Rationale: it closes the trust-dialog case, and it also closes the "binary booted into a shell that shows no TUI" case that `awaitDriverPane`'s doc comment names as a residual.
  Reusing `startup_timeout_s` avoids a second, loom-owned timeout for the same question.
- Rejected: keeping the liveness probe and adding the dismissal after it creates two probes with two budgets for one question.

### Result mapping and bookkeeping

- Decision, per `checkLivenessTick` answer:
  - `StartupReady` observed (`started` flips to true) → return `(true, nil)`.
    `checkLivenessTick` already persists `run.state.Started = true` to run.json at that moment.
    That write is desirable: a later `Attach` then skips the startup probe on a mid-turn pane.
  - `OutcomeDone` (the file contract is already satisfied: the driver wrote its report before readiness was observed) → return `(true, nil)`.
    The driver finished, so the bootstrap does not treat that as a boot failure.
  - `OutcomeDied` (the pane died, or the startup window expired with no `StartupReady`) → return `(false, nil)`, and the caller refuses with the existing envelope.
  - A `checkLivenessTick` error (a reed.Status error, `errStrandNotTracked`, or `errStrandPaneBindingCleared`) gets the same consecutive-failure tolerance `Wait` applies (`maxStatusRetries`, reset on a successful tick).
    After the cap it returns a wrapped error of the same wording family `Wait` uses.
- Decision: `AwaitStarted` never calls `finalize` and never writes a terminal `Outcome` to run.json.
  It is a readiness probe, not a completion verdict: the ly-drive run is never `Wait`ed, and the run directory must keep its `runOutcomeRunning` sentinel.
- Decision: the loop is bounded by tick COUNT as well as by the deadline, per the Live-Substrate Spawn Observability retry clause.
  The cap is `ceil(startupTimeout / interval) + 1` ticks plus the `maxStatusRetries` slack, whichever formulation mill-plan finds cleanest.
  After the cap, the answer is `(false, nil)`, routed through `classifyStartupWindow`/`classifyDeadlineExpiry` so the file-contract rule still wins.
  The loop plays keys and spawns nothing, so the count cap is belt-and-braces.
- Decision: short-circuit on `run.attached && run.state.Started` exactly as `Wait` seeds `started`, returning `(true, nil)` immediately.
  Loom only calls this on a freshly started run, so the branch is for consistency.
- Rationale: every negative answer keeps honoring the file contract (Completion Signal Invariant), because both reach it through `checkLivenessTick`/`classifyStartupWindow`, which already consult `allOutputFilesExist`.
- Rejected: returning `(Outcome, error)` exposes a terminal-outcome vocabulary for a non-terminal probe and invites callers to persist it.

### Not-ready refusal leaves the strand in place

- Decision: unchanged from today — on a not-ready answer, release the bootstrap lock, refuse with the run dir and strand guid, and do not remove the strand.
- Rationale: the pane is the evidence, and an operator attaching sees what it is stuck on (for example an unrecognized gate).
  The next `start` already handles a dead strand as a corpse through `driverStrandDead`.
- Rejected: auto-removing the strand on refusal destroys the only diagnostic artifact.

### Completion Signal tripwire

- Decision: put `AwaitStarted` in its own file.
  If `internal/shuttleengine/completionsignal_enforcement_test.go`'s AST scans cover the new file (they pin return sites in `wait.go`/`attach.go`), then either:
  - add the new sites with a justification comment beside each entry, or
  - confirm the method adds no negative-verdict return site, since it returns a bool.

  Whichever applies, mill-plan must check the scan's file list rather than assume it.
- Rationale: the tripwire exists to force a human look at new negative-answer exits, and `(false, nil)` is one in spirit.
  It routes through `classifyStartupWindow`, so it is already guarded.

### Reproduction recipe (live verification)

- Decision: the task is not verified until this recipe has passed on a path this host has never trusted.
  It is not automated.
  1. Build `lyx` from this branch (`go build ./cmd/lyx`; cgo required).
  2. Choose a hub path containing a fresh UTC timestamp so it cannot have been trusted before, for example `$HOME/Code/r5sandbox/trust-repro-<YYYYMMDD-HHMMSS>-LYXHUB`.
     Create a fixture hub there through lyx's own clone flow (`lyx clone`; pin the exact invocation from `lyx clone --help`).
     The fixture must not be hand-assembled (hubforge Fabric-Fixture Invariant spirit).
  3. Before launching, prove that the child worktree's absolute path is untrusted: `jq --arg p "<abs worktree path>" '.projects | has($p)' ~/.claude.json` must print `false`.
     If it prints `true`, pick a new timestamp; the run would pass silently and prove nothing, which is how round r3 lost the finding.
  4. In that worktree, run `lyx shed seed <run-id> --recipe loom --driver llm`, then `lyx loom start --no-attach`.
  5. Pre-fix expectation (baseline, optional, from a `main` build): `start` returns success, and `tmux capture-pane -p -t <driver pane>` shows the trust dialog ("Yes, I trust this folder") indefinitely.
  6. Post-fix expectation: `start` returns only after dismissal, the driver pane shows the ly-drive session working, and the step-3 `jq` check now prints `true` (Claude recorded the acceptance).
     A run.json under the driver's run dir carries `started: true`.
  7. Tear the fixture down afterwards (`lyx` teardown / removing the hub dir).
     The `~/.claude.json` entry it leaves is harmless: Claude Code appends an entry for every launch directory anyway.
- Rationale: the trigger depends on host state that a hermetic test cannot hold.
  An automated live test would spend subscription quota, mutate the operator's real `~/.claude.json`, and fall outside the Test Tier Purity boundaries.

## Technical context

- `internal/shuttleengine/wait.go`:
  - `checkLivenessTick(started *bool, startupDeadline time.Time) (Outcome, error)` is the reuse target.
    It checks reed status, handles not-tracked/not-live via the file contract, captures the pane, and classifies it.
    On `StartupReady` it sets `*started` and persists `Started`.
    On `StartupTrustPrompt` it plays `TrustDismissSequence(capture)` non-fatally.
    It returns `classifyStartupWindow(deadline)` while not yet started.
  - `Wait`'s liveness block shows the `statusFailures`/`maxStatusRetries` pattern and the error wording to mirror.
  - `pollInterval(cfg)` and `realClock` live here too.
- `internal/shuttleengine/run.go`: the `Run` struct (`clock`, `attached`, `state`, `runner`, `spec`) and the `StrandGUID`/`RunDir` accessors.
  The new method's doc comment should say that, like those, it serves a caller that starts a run and never Waits on it.
- `internal/shuttleengine/engine.go`: the `StartupState` enum and the `Engine.TrustDismissSequence` contract.
  An implementation that cannot locate the accepting option returns no inputs, and the window then expires.
- `internal/shuttleengine/claudeengine/startup.go`: the classifier covers both the trust gate and the bypass-permissions gate.
  No change is needed there.
- `internal/loomcli/driverlaunch.go`: the `driverHandle`, `driverStarter` and `driverPaneProbe` seams, plus `awaitDriverPane` and its constants.
  The file's header comment explains why the seams exist (Test Tier Purity bars real spawns in untagged tests).
  `driverPaneProbe.Strands` stays: `runDriverSpawnAndWait` uses it for the pre-branch strand read and the corpse check.
- `internal/loomcli/start.go`: `runDriverSpawnAndWait`'s step-6 llm block (around the `awaitDriverPane` call) is the only call site.
  The `start` command's `Long` text describes the readiness signal per driver.
- `internal/loomcli/driverspec.go`: the doc comment on `Timeout`/`KeepPane`/`AwaitOperator` says nothing reads them because `Wait` is never entered.
  That stays true, since `AwaitStarted` reads the runner config's `StartupTimeoutS`, not the spec, but the "polled through the pane-liveness probe" wording must be updated.
- Tests: `internal/loomcli/start_driver_test.go` and `driverlaunch_test.go` fake `driverHandle`/`driverStarter`.
  `internal/shuttleengine/fakes_test.go` has `fakeReed` (scripted `CapturePane`, `Status`, recorded `SendKey`) and `fakeEngine` (scripted `Startup`, recorded `TrustDismissCaptures`), plus a fake clock, all reusable for the new method's tests.
- Provenance: `crucible/campaigns/batten-end-to-end/` on main, rounds r2–r5.

## Constraints

- **Shuttle Provider-Seam Invariant:** no Claude specifics in `shuttleengine`/loomcli; dismissal stays behind `Engine.TrustDismissSequence`.
- **Completion Signal Invariant:** every negative answer consults `allOutputFilesExist` first.
  `AwaitStarted` routes all negatives through `checkLivenessTick`/`classifyStartupWindow` and never finalizes; the tripwire test must be checked (see Decisions).
- **Live-Substrate Spawn Observability:** the retry loop caps attempt COUNT, not only time.
  Log the dismissal attempt; `checkLivenessTick` already `Warn`s on failure, so add an `Info`/`Debug` line when a gate is dismissed during the driver's startup only if mill-plan finds no existing log.
  Log readiness in loomcli ("driver strand is ready", replacing "pane is live").
- **Test Tier Purity Invariant:** all new tests are untagged and use fakes and fake clocks, with no real spawns or `time.Sleep` ≥ 1s.
- **Told-Geometry Invariant:** `shuttleengine` stays a bound package; the new method derives no paths.
- **Driver Choice Single-Site Invariant:** the branch on the recorded driver stays in `start.go`'s existing `mustUseLLMDriverArm` sites, and no new reader is added.
- **CLI/Cobra Invariant:** the `Long` text is updated; `Short` is unchanged.
- **Documentation Lifecycle / CLAUDE.md:** docs land in the same commit as the observable CLI behavior change (`start`'s readiness semantics).
  There is no new cross-cutting invariant, so `CONSTRAINTS.md` is unchanged, and so is `manifest/roadmap.md` (this is a bugfix).
- Markdown: semantic line breaks.

## Testing

- **TDD candidate — `shuttleengine` `AwaitStarted`**, with fakes and a fake clock:
  - a capture sequence of pending → trust prompt → ready: the dismissal is played against the trust-prompt capture, the method returns true, and `Started` is persisted to run.json;
  - ready on the first tick returns true without any dismissal;
  - a pane going not-live mid-startup, with no output files, returns false;
  - the startup window expiring while stuck on a gate that `TrustDismissSequence` cannot dismiss (it returns no inputs) returns false within `startup_timeout_s`;
  - output files already present returns true (the file contract wins);
  - transient reed.Status errors below `maxStatusRetries` are tolerated and then recover to ready;
  - errors at the cap return a wrapped error;
  - the tick-count cap terminates even if the clock never advances;
  - an attached run with `Started` returns true immediately;
  - no terminal `Outcome` is ever written to run.json.
- **loomcli:**
  - `start_driver_test.go` fakes `driverHandle.AwaitStarted`: false gives the existing refusal envelope naming the run dir and strand, and releases the bootstrap lock;
  - an error gives an error envelope;
  - true gives success;
  - the go-arm handshake is still never consulted on an llm-seeded bootstrap.
  - Remove the `awaitDriverPane` tests.
- **Completion Signal tripwire:** keep it green; update pinned counts only with written justification.
- **Live:** the reproduction recipe above, run once post-fix on a fresh path.
  Record the `jq` before/after output and the pane capture in the task's completion notes.

## Q&A log

- **Q:** Where does the dismissal live — shuttle-exported startup-await, a loomcli-side capture/dismiss probe, or a one-shot dismiss after the liveness probe? **A:** [auto-pick] Shuttle-exported `Run.AwaitStarted` reusing `checkLivenessTick`. **Why:** one classify→dismiss implementation, provider seam intact, matches the proposal's "wire an existing seam" decision.
- **Q:** Keep pane-liveness as the readiness signal and add dismissal, or replace it with `StartupReady`? **A:** [auto-pick] Replace with `StartupReady` under `startup_timeout_s`. **Why:** a live pane on a dialog was the bug; one probe, one budget.
- **Q:** What does the bootstrap do on a not-ready answer? **A:** [auto-pick] Keep today's refusal and leave the strand for diagnosis. **Why:** the pane is the evidence; corpse cleanup already exists on the next start.
- **Q:** Also dismiss on an already-live driver strand found at bootstrap? **A:** [auto-pick] No, out of scope. **Why:** that path spawns nothing, and probing a possibly mid-turn pane risks playing keys into a live agent (the exact hazard `Wait`'s `started` seeding guards against).
- **Q:** Automate the untrusted-path reproduction as a live test? **A:** [auto-pick] No — a documented manual recipe used as live verification. **Why:** depends on mutable host state (`~/.claude.json`), spends subscription quota, and would violate Test Tier Purity in untagged form.
- **Q:** Weigh round r5's framing that the hang is a structural consequence of the interactive-tmux policy. **A:** [auto-pick] Accept the framing only as context; the fix stays in the driving code. **Why:** interactive sessions are mandated (CLAUDE.md), so the driving code must handle modal dialogs, which is exactly what shuttle already does for producers.
