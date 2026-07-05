# Batch: run-loop

```yaml
task: 'Build internal/shuttle: one LLM agent via a swappable engine'
batch: run-loop
number: 4
cards: 4
verify: go test ./internal/shuttleengine/...
depends-on: [1, 3]
```

## Batch Scope

The provider-invariant core: `Runner`/`Start`/`Wait` (one agent run end-to-end over the
file contract), outcome classification, cleanup + orphan-sweep wiring, and the
`Interrupt`/`Send` handle methods. mux is consumed only through a package-local `MuxOps`
interface so the whole loop is testable against fakes. External interface consumed by
batch 5: `Runner`, `NewRunner`, `Run` (handle), `Result`, `Runner.Start/Run`,
`Run.Wait/Interrupt/Send`.

## Cards

### Card 14: MuxOps seam and test fakes

- **Context:**
  - `internal/muxengine/strand.go`
  - `internal/muxengine/io.go`
  - `internal/muxengine/lifecycle.go`
  - `internal/muxengine/state.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/mux.go`
  - `internal/shuttleengine/fakes_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `mux.go`: exported `type MuxOps interface { AddStrand(spec
  muxengine.AddSpec) (muxengine.Strand, error); RemoveStrand(guid string, recursive bool)
  (muxengine.Removed, error); Status() (muxengine.StatusResult, error); SendText(guid,
  text string, submit bool) error; SendKey(guid, key string) error; CapturePane(guid
  string) (string, error) }` — godoc: satisfied by `*muxengine.Engine`; the seam exists
  so the run loop is hermetically testable. Add a compile-time assertion
  `var _ MuxOps = (*muxengine.Engine)(nil)`. `fakes_test.go`: `fakeMux` (records calls,
  scriptable Status liveness per poll, scriptable CapturePane returns, error injection
  per method) and `fakeEngine` implementing `Engine` (canned `Launch`, pass-through
  `ParseEvents` splitting a simple fixture format, scriptable `Startup` sequence,
  canonical `InterruptSequence`/`ComposeSend`). Fakes carry no assertions themselves —
  tests inspect their recorded calls.
- **Commit:** `feat(shuttle): MuxOps seam and hermetic fakes`

### Card 15: Runner and Start

- **Context:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/mux.go`
  - `internal/muxengine/state.go`
  - `internal/muxengine/strand.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/run_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `run.go`: `type Runner struct { mux MuxOps; engine Engine; layout
  *hubgeometry.Layout; cfg Config }` with `func NewRunner(mux MuxOps, engine Engine,
  layout *hubgeometry.Layout, cfg Config) *Runner`. `type Result struct { Outcome
  Outcome; SessionID, StrandGUID string; LastAssistantMessage string; RunDir string }`.
  `type Run struct` (unexported fields: runner, spec, runDir, state RunState, events
  offset, deadline). `func (r *Runner) Start(spec Spec) (*Run, error)`:
  (1) `spec.validate(r.layout.WorktreeRoot, r.cfg)`;
  (2) opportunistic orphan sweep — read `muxengine.LoadState(r.layout.DotLyxDir())`,
  build the live-guid set (nil state ⇒ empty set), call `sweepOrphans(runDirRoot(...),
  guids, 2*startupTimeout, time.Now())`, LOG-AND-CONTINUE on sweep error (a sweep failure
  must never block a new run);
  (3) create the run dir (`newRunID`);
  (4) `r.engine.Prepare(runDir, spec, r.cfg)` → `Launch`;
  (5) `r.mux.AddStrand(muxengine.AddSpec{Role: spec.Role, Round: spec.Round, Parent:
  spec.Parent, Cmd: launch.Cmd, ResumeCmd: launch.ResumeCmd, SessionID:
  launch.SessionID, Display: spec.Display})`;
  (6) write `run.json` (`saveRunState` — StrandGUID from the returned strand, SessionID,
  Interactive, resolved OutputFiles, artifact paths, CreatedAt `time.Now().UTC().Format
  (time.RFC3339)`);
  (7) return the handle. On AddStrand failure remove the just-created run dir before
  returning the error (nothing to resume). `func (r *Runner) Run(spec Spec) (Result,
  error)` = `Start` + `Wait`. Tests (fakes): happy Start wires AddSpec fields verbatim
  (incl. SessionID and Display passthrough), validation failure short-circuits before any
  mux call, AddStrand failure cleans the run dir, sweep error does not block Start.
- **Commit:** `feat(shuttle): Runner.Start — prepare, AddStrand, run.json`

### Card 16: Wait loop — polling, startup probe, classification, cleanup

- **Context:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/mux.go`
  - `internal/muxengine/lifecycle.go`
- **Edits:**
  - `internal/shuttleengine/run.go`
- **Creates:**
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/wait_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `wait.go`: `func (run *Run) Wait() (Result, error)` — a poll loop
  (interval `cfg.PollIntervalMS`; inject a clock/sleeper seam so tests run instantly):
  each tick (a) read `events.jsonl` from the stored byte offset, feed new bytes through
  `engine.ParseEvents`, advance the offset; (b) on any new StopEvent: if every
  `OutputFiles` entry exists (`os.Stat`) → outcome `done`; else → outcome `asking` with
  the LAST event's `LastAssistantMessage`; (c) every `cfg.LivenessEveryNPolls`-th tick:
  `mux.Status()`, find the strand by guid — absent or `Live == false` → outcome `died`;
  during the startup window additionally `mux.CapturePane(guid)` +
  `engine.Startup(capture)`: `StartupTrustPrompt` → `mux.SendKey(guid, "Enter")` (the
  trust dismissal); `StartupReady` → mark started; still `StartupPending` when
  `cfg.StartupTimeoutS` expires → outcome `died` (fast-fail; include the last capture in
  the returned error/Result context via the Result's LastAssistantMessage staying empty
  and the error nil — died is a classified outcome, not an error); (d) `spec.Timeout`
  deadline passed → outcome `timeout`. Terminal handling: on `done` and NOT
  `spec.KeepPane` → `mux.RemoveStrand(guid, false)` + `os.RemoveAll(runDir)` (cleanup
  errors are logged, not fatal — the outcome stands); on `asking`/`died`/`timeout` →
  keep strand and run dir (diagnosis + attach). Wait returns `(Result, error)` where
  error is reserved for mechanism failures (unreadable events file after retries, Status
  error twice consecutively) — classified outcomes always return `error == nil`.
  Tests (fakes, scripted sequences): done happy path with cleanup calls recorded;
  done+KeepPane skips both cleanups; asking (stop event, missing output) carries the
  message and keeps the strand; died via Status not-live; died via startup-timeout with
  trust-dismiss recorded (Enter sent when TrustPrompt scripted); timeout keeps strand;
  multi-Stop offset tracking (second event classifies, first already consumed);
  events-offset resilience across a partial line (parse next tick).
- **Commit:** `feat(shuttle): Wait loop — classification, startup probe, cleanup`

### Card 17: Interrupt and Send handle methods

- **Context:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/mux.go`
- **Edits:**
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/run_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `func (run *Run) Interrupt() error` — play
  `engine.InterruptSequence()` through the mux seam (each `PaneInput`: Key →
  `SendKey`, Text → `SendText(text, Submit)`). `func (run *Run) Send(text string) error`
  — reject text containing `\n` or `\r` with an error stating the single-line contract
  (multiline updates ride the file contract: write a file and send a one-line pointer —
  cite discussion "In-agent interrupt"); then play `engine.ComposeSend(text)` the same
  way. Both are safe to call concurrently with a blocked `Wait` (they only call mux,
  whose op lock serializes; no Run-local state is mutated — document that in godoc).
  Also export `func playInputs(mux MuxOps, guid string, inputs []PaneInput) error` as
  the shared unexported helper both methods use — batch 5's CLI interrupt/send verbs
  reuse the same choreography through the engine, so keep the helper's signature
  package-internal but the sequence semantics identical. Tests: Interrupt plays exactly
  `[{Key:Escape}]`; Send rejects newlines; Send plays Esc-then-text-with-submit; fake
  records the call order.
- **Commit:** `feat(shuttle): Interrupt/Send — ESC-and-hold plus one-line follow-up`

## Batch Tests

`verify: go test ./internal/shuttleengine/...` — the whole loop runs against `fakeMux` +
`fakeEngine` with an injected clock: all four outcomes, KeepPane, trust-dismiss, offset
tracking, sweep-at-start, AddStrand-failure cleanup, and the Interrupt/Send choreography.
No psmux, no claude, no sleeping (clock seam). The real-world integration is batch 6's
smoke layer.
