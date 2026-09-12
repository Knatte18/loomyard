# Batch: shedengine-step

```yaml
task: "lyx loom step + external supervisor skill"
batch: "shedengine-step"
number: 1
cards: 3
verify: go test ./internal/shedengine/...
depends-on: []
```

## Batch Scope

This batch delivers the single-iteration primitive the whole task rests on: `(*Shed).Step`, plus the `StepResult` type it returns, extracted from `(*Shed).Run`'s existing `for` body so the two callers physically share one routing implementation.
It is one batch because the extraction and its two test files are meaningless apart — the extraction is only safe if the equivalence test proves `Run` and repeated `Step` calls agree, and that test cannot exist before the extraction compiles.
The external interface batch 4 consumes is `StepResult`'s seven fields and `Step`'s `(StepResult, error)` signature, including its `ErrShedBusy` refusal.

Batch-local decision that differs from the overview: card 1 writes production code before its tests, inverting the discussion's TDD preference. Go test files referencing `Step` and `StepResult` do not compile until those symbols exist, so a tests-first card would commit a package that fails to build — which `## Shared Decisions`' behaviour-preserving rule cannot tolerate, since a non-building package makes the existing `Run` suite unrunnable too. The discussion's intent (tests drive the contract) is preserved by pinning the full `StepResult` field mapping in card 1's Requirements below, before any code is written.

## Cards

### Card 1: extract Run's loop body into stepLocked, add StepResult and the exported Step

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/errors.go`
  - `internal/shedengine/validate.go`
  - `internal/shedengine/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/shedengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add an exported `StepResult` struct to `internal/shedengine/run.go` with exactly seven fields, in this order: `Producer string`, `Outcome Outcome`, `Output string`, `Next string`, `State State`, `Reason string`, `History []HistoryEntry`. Document each field's meaning per the mapping table below; `Producer` is empty when no producer was called, `Outcome` is empty when no producer reached a verdict, `Output` is the called producer's `OutputPointer.Path` (empty for a gate or terminal row), `Next` is `current_producer` as persisted by this step, `Reason` is populated only alongside `StateBlocked`, and `History` is the full persisted history as it stands when the step returns.

  Add an unexported method `func (s *Shed) preflight() error` holding exactly the three statements `Run` performs today before acquiring the run lock, in today's order: `s.validate()`, `os.MkdirAll(filepath.Dir(s.LockPath), 0o755)` wrapped as `"shedengine: create run lock parent dir: %w"`, and `os.MkdirAll(filepath.Dir(s.StatusLockPath), 0o755)` wrapped as `"shedengine: create status lock parent dir: %w"`. Carry the existing comment explaining that `internal/lock` opens with `O_CREATE` but never creates a parent directory onto this method, and add to it that `Step` is exported for every shed in the repo, so it cannot assume some caller already made the ephemeral directory.

  Add an unexported method `func (s *Shed) stepLocked(ctx context.Context) (StepResult, error)` holding the current `for` body verbatim — the step-1 read gate, the already-done short-circuit, the step-2 lookup, the step-3 pause/cancel check, the step-3b conditional resume write, the step-4 `def.Producer.Call(ctx)`, the `appendHistory` closure, and the five-arm routing `switch` — with every load-bearing comment carried over unchanged. `stepLocked` assumes the run lock is already held and never acquires it; say so in its doc comment. Rewrite each arm's terminator as follows, and change nothing else about any arm's persist call, its arguments, or its ordering:

  - already-`StateDone` short-circuit: return `StepResult{Next: st.CurrentProducer, State: StateDone, History: st.History}`, nil.
  - step-3 pause/cancel exit: after the existing `persist(st.CurrentProducer, StatePaused, "", st.History, true)`, return `StepResult{Next: st.CurrentProducer, State: StatePaused, History: st.History}`, nil.
  - `callErr != nil && ctx.Err() != nil`: after the existing persist, return `StepResult{Producer: def.Name, Next: st.CurrentProducer, State: StatePaused, History: st.History}`, nil. `Outcome` stays empty — the producer reached no verdict — and no history entry is appended, exactly as today.
  - `callErr != nil`: return `StepResult{}, callErr`, and on a persist failure `StepResult{}, errors.Join(callErr, persistErr)` — the zero-value `StepResult` alongside a non-nil error, mirroring `Run`'s existing unpopulated-`Result` contract.
  - `outcome == Stuck` with `def.OnStuck == ""`: return `StepResult{Producer: def.Name, Outcome: outcome, Output: output.Path, Next: st.CurrentProducer, State: StateBlocked, Reason: reason, History: nextHistory}`, nil.
  - `outcome == Stuck` at the budget boundary: identical, with `reason` being `"bounce budget exhausted"`.
  - `outcome == Stuck` default (the bounce): after the existing `persist(def.OnStuck, StateRunning, "", nextHistory, false)`, return `StepResult{Producer: def.Name, Outcome: outcome, Output: output.Path, Next: def.OnStuck, State: StateRunning, History: nextHistory}`, nil — replacing today's bare `continue`.
  - `outcome == Done` with `def.OnDone == ""`: after the existing `persist(def.Name, StateDone, "", nextHistory, false)`, return `StepResult{Producer: def.Name, Outcome: outcome, Output: output.Path, Next: def.Name, State: StateDone, History: nextHistory}`, nil. `Next` is `def.Name`, never empty, for the same reason the persist writes `def.Name`.
  - `outcome == Done` with a non-empty `OnDone`: after the existing persist, return `StepResult{Producer: def.Name, Outcome: outcome, Output: output.Path, Next: def.OnDone, State: StateRunning, History: nextHistory}`, nil — replacing today's bare `continue`.
  - `default` (unrecognised outcome): return `StepResult{}, failErr`, and `StepResult{}, errors.Join(failErr, persistErr)` on a persist failure.

  Rewrite `Run` as: `s.preflight()`, then today's `lock.TryAcquireWriteLock(s.LockPath)` block with its `ErrShedBusy` refusal and `defer runLock.Release()` unchanged, then an unbounded `for` whose body is `res, err := s.stepLocked(ctx)`; return `Result{}, err` on a non-nil error; `continue` when `res.State == StateRunning`; otherwise return `Result{Outcome: RunOutcome(res.State), HaltedProducer: res.Next, Reason: res.Reason, History: res.History}, nil`. Add a comment on the `RunOutcome(res.State)` conversion recording why it is a conversion rather than a lookup table: `shed.go` pins `RunOutcome`'s three string values as deliberately identical to `State`'s three clean-exit values, and `StateRunning` never reaches this line while `StateFailed` only ever arrives alongside a non-nil error. Add a second comment recording that `HaltedProducer` equals `res.Next` universally, because in every arm above `Next` is the value `persist` wrote as `current_producer`.

  Add the exported `func (s *Shed) Step(ctx context.Context) (StepResult, error)`: call `s.preflight()`, returning `StepResult{}, err` on failure; acquire the run lock exactly as `Run` does, returning `StepResult{}, fmt.Errorf("%w: %q", ErrShedBusy, s.LockPath)` when it is held; `defer runLock.Release()`; return `s.stepLocked(ctx)`. Document that the per-call lock window is what makes stepping possible — between steps there is by definition no driver running — and that mutual exclusion with a live detached driver is the practical payoff.

  Leave `Result`, `RunOutcome`, `findProducer`, `nowRFC3339`, `episodeStuckCount`, `effectiveMaxBounces`, and `persist` untouched. Add no import beyond the package's existing stdlib/`state`/`lock` set, per `CONSTRAINTS.md`'s Shed Producer-Seam Invariant. Update `run.go`'s own top-of-file comment so it describes the extracted shape rather than only `Run`.
- **Commit:** `feat(shedengine): extract Run's loop body into an exported (*Shed).Step`

### Card 2: unit tests for Step's routing arms, preamble, and lock behaviour

- **Context:**
  - `internal/shedengine/run.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/errors.go`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/run_routing_test.go`
  - `internal/shedengine/run_pause_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/step_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write one test per routing arm, each asserting both the returned `StepResult` and the persisted `Status` read back via the existing `readStatus` helper. Reuse `newTestShed`, `seedStatus`, `readStatus`, `commonSeed`, `fixedOutcomeProducer`, `funcProducer`, and `assertRFC3339UTC` from `testsupport_test.go` — do not redeclare any of them. Cover exactly these cases:

  a `Done` verdict with a non-empty `OnDone` (`Producer` is the called row, `Outcome` is `Done`, `Next` is the `OnDone` target, `State` is `StateRunning`, one new history entry);
  a `Done` verdict with an empty `OnDone` (`State` is `StateDone`, `Next` is the called row's own name, never empty);
  a `Stuck` verdict within budget (`Next` is the `OnStuck` target, `State` is `StateRunning`);
  a `Stuck` verdict with no `OnStuck` (`State` is `StateBlocked`, `Reason` is `"stuck with no OnStuck target"`);
  a `Stuck` verdict at the budget boundary, driving a `ProducerDef.MaxBounces` of three through three bounces and asserting the fourth `Stuck` blocks with `Reason` `"bounce budget exhausted"`;
  `pause_requested` set before the call (`Producer` empty, `State` is `StatePaused`, and `pause_requested` is false on disk afterwards, and the producer's own call counter is zero);
  a cancelled context paired with a producer error (`Producer` is the called row, `Outcome` empty, `State` is `StatePaused`, and the history length is unchanged);
  a producer hard error with a healthy context (non-nil error returned, the returned `StepResult` equal to its zero value, and `StateFailed` plus the error text on disk);
  an unrecognised outcome (non-nil error, `StateFailed` on disk);
  entry on an already-`StateDone` file (`Producer` empty, `State` is `StateDone`, history length unchanged, and the producer's call counter zero);
  entry on each of a `StateBlocked`, `StateFailed`, and `StatePaused` file (the producer **is** called and the step-3b resume write fires — assert the call counter is one);
  a producer returning an empty `Outcome` together with an error (no history entry appended at all).

  Add three further tests. First, `Step` returns an error satisfying `errors.Is(err, ErrShedBusy)` when the run lock is already held by a separately-acquired `lock.TryAcquireWriteLock` on the same `LockPath`. Second — the property that makes stepping possible at all — two sequential `Step` calls against the same `Shed` both succeed, proving the lock is released between them. Third, the preamble: a `Shed` whose `LockPath` and `StatusLockPath` parent directories do not exist yet must have them created and succeed (`newTestShed` deliberately leaves those parents uncreated), and a `Shed` with an invalid producer list must fail `validate()` before touching the lock — assert the latter by confirming no file exists at `LockPath` after the failed call.

  Assert history timestamps structurally via `assertRFC3339UTC`, never against a literal.
- **Commit:** `test(shedengine): cover Step's routing arms, preamble, and lock window`

### Card 3: the Run/Step equivalence test

- **Context:**
  - `internal/shedengine/run.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/activity.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/testsupport_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/step_equivalence_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  This is the test that protects the refactor. For each fixture, build one `Shed` and drive it to a terminal state with repeated `Step` calls, build a second identical `Shed` over a fresh `t.TempDir()` and drive it once with `Run`, then assert the two resulting status files agree.

  Compare the clock-independent fields only: `current_producer`, `state`, `error`, all three of `activity.now`/`activity.last`/`activity.wait`, and the history entries' `producer`/`outcome`/`output` sequence and length. Exclude `history[].at` and state the exclusion in the test's own comment: it is `time.Now()` via `nowRFC3339()`, `shed.go` pins the absence of an injectable clock on `Shed` as a deliberate design decision, and a byte-identical whole-file assertion would therefore pass only when both drives happened to land inside the same second — flaky by construction. Record in the same comment that adding a clock seam to `Shed` purely to strengthen this assertion is rejected, because it would widen the struct shape the design pins for a field these assertions already cover structurally.

  Also assert that `Run`'s `Result` agrees with the final `StepResult` of the stepped drive: `Result.Outcome` against `StepResult.State` by string comparison, `Result.HaltedProducer` against `StepResult.Next`, `Result.Reason` against `StepResult.Reason`, and the same history length.

  Run this across at least three fixtures: one reaching a done terminal, one reaching a blocked terminal, and one that bounces at least once before reaching done. Build the linear fixtures with `linearChain` from `testsupport_test.go`. Cap the stepping loop with an explicit iteration bound and fail the test if it is reached, so a regression that never leaves `StateRunning` fails loudly instead of hanging the suite.
- **Commit:** `test(shedengine): pin Run and repeated Step to the same persisted outcome`

## Batch Tests

`verify: go test ./internal/shedengine/...` runs the whole package. That scope is deliberate and is the justification the `verify-full-suite` carve-out asks for: this batch rewrites `Run`, the routine every other test file in the package exercises, so the four existing suites (`run_routing_test.go`, `run_persist_test.go`, `run_pause_test.go`, `run_commitstatus_test.go`) are precisely the guardrail proving the extraction is behaviour-preserving, and narrowing the run to only the two new files would skip them. `go test` on a single package is seconds, not the multi-minute suite the scoping rule exists to avoid.

The new files are `step_test.go` (routing arms, preamble, lock window) and `step_equivalence_test.go` (the `Run`/`Step` agreement property). The existing four suites must pass unedited; an edit to any of them to accommodate the refactor is a batch failure, not a fix.
