# Batch: shuttle-attach

```yaml
task: 'loom: interactive Discussion-Write'
batch: 'shuttle-attach'
number: 2
cards: 3
verify: go test ./internal/shuttleengine/
depends-on: [1]
```

## Batch Scope

This batch adds `Runner.Attach(spec Spec) (Result, bool, error)` — the whole of **Defect B**'s fix on the `shuttleengine` side.
It is one batch because the implementation, its reconstruction discipline, and its test matrix are one indivisible design: the scan, the reed-disposition enumeration, the candidate-combination order, and the five pieces of `Run` state `Attach` seeds rather than inherits are each meaningless without the others.

The external interface batch 3 consumes: `func (r *Runner) Attach(spec Spec) (Result, bool, error)`.
The bool reports whether a live run was found; `false` comes back with a zero `Result` and a nil error and means "nothing to attach to, start one".

Batch-local decisions that differ from nothing in the overview:

- `Runner` gains an unexported `clock clock` field, filled by `NewRunner` with `realClock{}`.
  `Start` reads it in place of its current inline `clk := clock(realClock{})`, and `Attach` reads it too.
  This is the only test seam that lets an attach test control the reconstructed run's deadline, because `Attach` returns a `Result` rather than a `*Run` and a test cannot reach inside to patch `run.clock` afterwards.
- `Run` gains an unexported `attached bool` field. `Wait` seeds its loop-local `started` from it instead of hard-coding `false`.

## Cards

### Card 4: `Runner.Attach` — scan, disposition, combine, reconstruct

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/reed.go`
  - `internal/shuttleengine/doc.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/render/types.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/rundir.go`
- **Creates:**
  - `internal/shuttleengine/attach.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shuttleengine/attach.go` with a file-header comment in the style of the package's other files, and implement `func (r *Runner) Attach(spec Spec) (Result, bool, error)` there.
  Its behaviour is specified by `attach-lives-in-shuttleengine`, `attach-normalizes-the-spec-it-matches-on`, `mechanism-failures-do-not-attach-and-do-not-blindly-respawn`, `attach-only-a-run-that-never-terminated`, `leftover-run-dir-from-a-completed-run`, `candidate-evaluation-order`, `one-live-match-or-none`, `attach-restarts-the-deadline`, and `attach-reconstructs-the-run-explicitly` in `_mill/discussion.md`.

  **Preconditions.** Return `r.toldErr` immediately when non-nil, exactly as every other public method on `Runner` does.

  **Normalization, on a local copy of the spec, never by calling `Spec.validate`.**
  `Attach` never calls `Start`, so it never reaches `validate`, and `validate`'s reject-if-already-exists check would refuse every attach whose agent has written one of its output files — a normal mid-interview state.
  Perform exactly three of `validate`'s normalizations plus one of its checks:
  reject a negative `Timeout` with the same error text `validate` raises, before scanning and before any reed read;
  resolve every relative `OutputFiles` entry against `r.worktreeRoot` with the same `filepath.IsAbs` / `filepath.Clean(filepath.Join(...))` rule `validate` uses;
  replace a zero `Timeout` with `time.Duration(r.cfg.RunTimeoutMin) * time.Minute`;
  and default an empty `Display.Anchor` to `render.AnchorBelowParent`.
  Perform none of `validate`'s other checks.
  Each of the four needs a comment saying what breaks without it: an unresolved relative entry never set-matches a `run.json`'s always-absolute record; a zero `Timeout` makes the attached run's deadline `now + 0` so it classifies `OutcomeTimeout` on its first tick; an empty `Anchor` makes every binding-cleared strand take `checkLivenessTick`'s hidden-strand carve-out and classify `OutcomeDied` instead of surfacing `errStrandPaneBindingCleared`; and a negative `Timeout` would report a **live interview** as timed out.

  **Phase 1 — collect.**
  Resolve the scan root with the existing `runDirRoot(r.cfg, r.anchorPath)` and never build a `.lyx` path of your own (Told-Geometry and Lyxdirs Single-Declarer Invariants).
  Scan `<root>/*/run.json` with the same shape and skip-discipline as `findRunByStrand`: iterate `os.ReadDir`, skip non-directories, call `loadRunState`, and skip an unreadable or absent `run.json` rather than aborting the scan.
  A root that does not exist is not an error — it is zero candidates.
  A candidate is a record whose `OutputFiles` **set**-matches the normalized spec's: order-insensitive and duplicate-insensitive, because `RunState.OutputFiles` records whatever order the caller happened to supply and two specs naming the same files in a different order describe the same run.
  Add an unexported helper for that comparison rather than inlining it.
  Record each candidate's run directory and its directory mtime (from the `os.DirEntry`'s `Info()`, as `sweepOrphans` does) alongside its `RunState`.
  **Zero candidates returns `(Result{}, false, nil)` immediately, without reading reed state at all.**
  This precedence is load-bearing and needs its own comment: `reedengine.LoadState` returns `(nil, nil)` for an absent `reed.json`, and with `Attach` probed on every `SingleLLMProducer.Call`, a worktree that has not yet had a reed session would otherwise hard-error on its very first `Discussion-Write` or `Plan-Write` call, with nothing to attach to and nothing wrong.

  **The two reed reads, with fixed roles and a fixed order.**
  First, `reedengine.LoadState(filepath.Join(r.anchorPath, lyxdirs.DotLyxDirName))` — the identical call `sweepOrphansOpportunistic` already makes — answers "does reed have a state table at all", as a gate.
  A non-nil error (unreadable/corrupt) and a `(nil, nil)` return (absent) are **both errors from `Attach`**, at any directory age.
  An absent strand table is not evidence that anything is dead, and respawning on it is the duplicate-agent hazard.
  This read must not be replaced by `ReedOps.Status()`: `reedengine`'s `loadOrInitStateLocked` substitutes an empty `&ReedState{}` for a not-found file, so `Status()` succeeds with zero strands for an absent state file, indistinguishable from a healthy table that simply does not list this guid.
  Second, and only once `LoadState` reported present, `r.reed.Status()` answers "is this guid tracked, and is its pane live".
  A `Status()` error is an error from `Attach`, at any directory age, never `found == false`.
  Both error dispositions must log loudly via `logger.Warn`, naming the run directory, the strand guid, and `lyx reed status`, because the operator escape is out of band.

  **Phase 2 — disposition each candidate independently**, yielding exactly one of three verdicts: *attachable*, *respawn-eligible*, or *error*.
  Resolve the strand with the existing `strandStatusByGUID` helper.
  - Tracked, `Live == true`, and `rs.Outcome == runOutcomeRunning` → **attachable**.
  - Tracked, `Live == true`, and `rs.Outcome` is anything else (a terminal value, the empty string, or an unrecognized one) → **respawn-eligible**, at any directory age.
  - Tracked, `Live == false`, and **not** the binding-cleared case → **respawn-eligible**, at any directory age.
    A dead pane is unambiguous evidence the agent is gone, so this needs neither the age rule nor the output-files tie-breaker.
  - Tracked, `Live == false`, with `PaneID == ""` and the normalized `spec.Display.Anchor != render.AnchorHidden` (the `errStrandPaneBindingCleared` case), or **not tracked at all** (the `errStrandNotTracked` case) → resolved by the leftover-then-age rule below.
  - **Leftover-then-age rule**, applying only to those last two answers, in this order: if every entry of the normalized `spec.OutputFiles` exists on disk (reuse the existing `allOutputFilesExist`), the candidate is **respawn-eligible at any directory age** — a leftover from a run that already finished, per `leftover-run-dir-from-a-completed-run`;
    otherwise, if the candidate's directory mtime is at least `2 * time.Duration(r.cfg.StartupTimeoutS) * time.Second` old (the identical `minAge` guard `sweepOrphans` applies), it is **respawn-eligible**;
    otherwise it is an **error** verdict.
    Comment why the age escape exists here and deliberately not for the absent-state-file answer: erroring unconditionally on an untracked candidate deadlocks resume permanently, because the only thing that ever removes such a directory is `sweepOrphansOpportunistic`, which runs inside `Start`, which the error path never reaches — while an absent or unreadable `reed.json` is repaired in-band by `lyx reed up`, or simply by `lyx loom run`, which calls `reed.Up()` itself.

  **Phase 3 — combine, in this precedence, and comment that the multiplicity rule applies only to the surviving attachable set and never to raw matches:**
  any candidate whose verdict is *error* → `Attach` errors, whatever the others say;
  else more than one *attachable* candidate → error naming every matching run directory, never a silent pick (`one-live-match-or-none`);
  else exactly one *attachable* candidate → attach it;
  else (every candidate respawn-eligible) → `(Result{}, false, nil)`.

  **Reconstructing the `Run`.**
  Build a `*Run` directly, without calling `Start`, filling every field explicitly:
  `runner: r`; `spec:` the caller's own normalized spec, never one rebuilt from `run.json` (`RunState` persists only `OutputFiles` out of the whole `Spec`, so rebuilding one is not possible and not wanted);
  `runDir:` and `state:` from the matched candidate, so `Wait` reads the persisted `EventsPath`, `StrandGUID`, and `SessionID`;
  `offset: 0`, deliberately replaying the whole `events.jsonl` — comment that seeding at EOF would mean a terminal `Stop` that landed while the driver was down is never observed, converting a completed step into an `OutcomeTimeout` failure, and that a replayed backlog ending in an ask is correct in both modes;
  `clock: r.clock`; and `deadline: r.clock.Now().Add(spec.Timeout)` — a fresh deadline computed at attach time, never `CreatedAt + Timeout`, because a run that hit `OutcomeTimeout` leaves both its strand and its run dir behind and inheriting `CreatedAt` would re-attach and re-time-it-out on every resume forever.
  Set the new `attached: true` field so `Wait` seeds `started` true.
  Log a `logger.Info` "shuttle: run attached" naming the run dir, the strand guid, and the session id, per the `Live-Substrate Spawn Observability` invariant's requirement that a re-attach to a live agent be as instrumented as a spawn.
  Return `run.Wait()` with the bool `true`.

  **Supporting edits outside `attach.go`, kept surgical.**
  In `run.go`: add an unexported field `clock clock` to `Runner`; fill it with `realClock{}` in `NewRunner`; and change `Start` to read `clk := r.clock` in place of its current `clk := clock(realClock{})`, leaving the rest of `Start` alone.
  Also add the unexported field `attached bool` to the `Run` struct with a doc comment saying it is set only by `Attach` and read only by `Wait`'s `started` seed.
  In `wait.go`: change `Wait`'s `started := false` to `started := run.attached`, with a comment stating why — without the seed an attached run re-runs `CapturePane` plus `engine.Startup` against a pane that is mid-turn, `claudeengine.Startup` returns `StartupPending` for any capture lacking its ready markers, and `classifyStartupWindow` then reports `OutcomeDied` one `startup_timeout_s` after attach, killing a live interview; worse, a capture that trips the trust-dialog needles would play the dismiss key sequence into a live agent's pane.
  Note in the same comment that the not-tracked and not-live branches of `checkLivenessTick` sit above the `started` short-circuit, so an attached run keeps full liveness coverage and only the startup probe is skipped.
  In `rundir.go`: no behavioural change — only extend `loadRunState`'s or `RunState`'s doc comment if needed to name `Attach` as a second reader alongside `findRunByStrand`.
- **Commit:** `feat(shuttle): add Runner.Attach to re-attach a live run instead of respawning over it`

### Card 5: `Attach`'s test matrix

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/attach.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/fakes_test.go`
  - `internal/shuttleengine/wait_test.go`
  - `internal/shuttleengine/rundir_test.go`
  - `internal/shuttleengine/run_test.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/render/types.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/attach_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shuttleengine/attach_test.go`, untagged (Tier 1), driven over a temp run-dir root with hand-written `run.json` files and the existing `fakeReed`/`fakeEngine`/`fakeClock` doubles.
  Reed's own state file is seeded with the exported `reedengine.SaveState(dotLyxDir, *reedengine.ReedState)` rather than by hand-writing JSON, so the absent/present/unreadable three-way answer is exercised through the real decoder.
  Reuse `rundir_test.go`'s `seedRun` helper where its fixed shape suffices and add a local seeding helper that takes `OutputFiles` and `Outcome` where it does not.
  Reuse `wait_test.go`'s `fakeClock` by assigning it to the `Runner`'s new `clock` field.

  Cover, one test or table row per case:
  - No run dirs at all, and separately a root that does not exist: `found == false`, nil error, **asserted with reed's state file absent**, proving the scan short-circuits before any reed read and that the ordinary first call is not blocked by the reed gate. Assert `fakeReed.CallLog` is empty for this case.
  - A matched record whose persisted `Outcome` is terminal (`asking`, `timeout`, `died`, `done`), with the strand tracked and live: `found == false` — the live-but-idle regression guard.
  - The same record with `Outcome: "running"` and a live strand: attached.
  - The same record with an **empty** `Outcome`, written from a `run.json` fixture that omits the field entirely rather than one that sets it to `""`, with a live strand: `found == false` — the upgrade-path regression guard, proving the decode path and not just the comparison.
  - An unrecognized `Outcome` value: respawn-eligible, same as empty.
  - **Two** matched records both carrying `Outcome: "asking"`, both tracked and live: `found == false`, **not** the multiplicity error.
  - One candidate dispositioned *error* (untracked, young) alongside one dispositioned *attachable*: `Attach` errors, proving the error verdict dominates.
  - Two candidates both *attachable*: the multiplicity error.
  - A matched record whose strand is tracked with `Live == false`: `found == false` at both directory ages.
  - A run dir whose `OutputFiles` do not match the spec's: not selected.
  - A matching run dir whose strand is absent from reed's `Status()` table, directory **younger** than `2 * StartupTimeoutS`: error, not a respawn.
  - The same, directory **older** than that: `found == false`, nil error.
  - Both of the above again for a strand that is tracked but holds no pane id.
  - reed's state file absent, and separately unreadable/corrupt: error in both cases, never `found == false`, at any directory age, with a companion assertion that `Attach` does **not** consult `Status()` for the absent question.
  - `Status()` itself returning an error (a fake reporting a torn-down session, and separately a tmux fault): error, never `found == false`, at any directory age.
  - A **negative** `Timeout` on the spec: rejected before scanning, with no reed read attempted.
  - A matched record in a non-attachable branch whose output files all exist: `found == false` at any directory age, including a directory younger than `2 * StartupTimeoutS` (the fast-bounce-after-failed-cleanup case) and a `KeepPane` leftover.
  - The same output files present but the strand tracked and live: attached, not treated as leftover, and the first tick classifies `OutcomeDone` with the output files still on disk.
  - `Display.Anchor` defaulting: a spec with an empty `Anchor` attaches to a binding-cleared strand and surfaces `errStrandPaneBindingCleared`, rather than taking the hidden-strand carve-out and classifying `OutcomeDied`.
  - `KeepPane` on an attached run suppresses the `Done` cleanup, and its absence performs it.
  - A matching run dir whose strand is tracked with a live pane: `found == true`, and `Wait` runs against the persisted `EventsPath`.
  - An unreadable or truncated `run.json` mid-scan does not abort the scan.
  - The attached run's deadline is `now + spec.Timeout`, proven by attaching to a record whose `CreatedAt` is already older than `spec.Timeout` and asserting it does not immediately time out.
  - A spec with a **zero** `Timeout` attaches with the config default applied and does not classify `OutcomeTimeout` on its first tick.
  - Output-file matching is on the resolved absolute set: a spec with relative entries matches a `run.json` written with absolute ones, and a spec naming the same files in a different order still matches.
  - `offset` starts at 0: a pre-existing `events.jsonl` whose last event is a completion (output files present) classifies `OutcomeDone` on the first tick after attach — the missed-terminal-`Stop` case.
  - The same backlog with output files absent classifies `OutcomeAsking`: terminal without `AwaitOperator`, dropped with polling continuing under it.
  - `started` is seeded true: attaching to a strand whose `CapturePane` returns a mid-turn capture (no ready markers) must not classify `OutcomeDied` after `startup_timeout_s`, and must not play the trust-dismiss sequence when the capture happens to contain a trust-dialog phrase. Assert both directly against `fakeReed.CallLog` and `fakeEngine.StartupCalls`.
  - An attached run whose strand later goes not-live with output files present still classifies `OutcomeDone`, confirming the inherited `checkLivenessTick` branches are reached.
- **Commit:** `test(shuttle): cover Runner.Attach's scan, disposition, and run reconstruction`

### Card 6: `shuttleengine` package documentation for `Attach`

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/attach.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir.go`
- **Edits:**
  - `internal/shuttleengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend the package documentation with a paragraph on `Runner.Attach`.
  It must state what `Attach` answers — "is there a still-live, never-terminated run for this exact output-file set, and if so, wait on it instead of starting a second agent" — and that the question is answered on live-agent evidence plus the persisted `RunState.Outcome`, never on output-file existence at the caller's level.
  It must state the two-line-of-defence framing: the record answers "did this run end", reed answers "is the process there", and they fail in different directions.
  It must state that `Attach` reconstructs its `Run` without calling `Start`, so `sweepOrphansOpportunistic` never runs on the attach path, and that the probe must therefore be made before anything that could sweep the directory it is looking for.
  It must name the two dispositions that are errors rather than `found == false` — an absent or unreadable reed state file, and a `Status()` failure — and say the operator escape is out of band via `lyx reed status`.
  Keep the file's existing statement that the run-loop half drives a live agent through the `ReedOps` seam accurate by mentioning that `Attach` joins `Start`/`Wait` in that half.
- **Commit:** `docs(shuttle): document Runner.Attach in package docs`

## Batch Tests

`verify: go test ./internal/shuttleengine/` runs the whole package again — the same scope and the same justification as batch 1.
The new `attach_test.go` is the bulk of the batch's coverage; `wait_test.go` and `run_test.go` are the regression guards for the two supporting edits (`Run.attached` seeding `started`, and `Runner.clock` replacing `Start`'s inline `realClock{}`), and `seam_enforcement_test.go` re-proves the `Shuttle Provider-Seam Invariant` across the new file's imports — `attach.go` imports `internal/reedengine` and `internal/lyxdirs`, both of which `run.go` already imports, and never `internal/shuttleengine/claudeengine`.

Every new test is hermetic: no tmux, no `claude`, no real sleeping.
`reedengine.SaveState` writes a plain JSON file through `internal/state`, and `fakeClock.Sleep` advances virtual time, so the whole matrix runs at zero wall-clock cost and stays inside the `Test Tier Purity Invariant`.

The overview's module-wide `verify: go vet ./...` type-checks every other package at this boundary; nothing outside `internal/shuttleengine` should be affected yet, since `Attach` has no callers until batch 3.
