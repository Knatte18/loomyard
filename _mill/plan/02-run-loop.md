# Batch: run-loop

```yaml
task: 'Shed: outer phase-FSM skeleton'
batch: run-loop
number: 2
cards: 7
verify: go test ./internal/shedengine/... && go test -run 'TestTierPurity_|TestHermeticGitEnv_' ./cmd/lyx/
depends-on: [1]
```

## Batch Scope

This batch delivers `Run` — the six-step loop that is this task's entire deliverable — plus the shared test fakes every later test batch consumes, plus the routing scenarios that exercise the outcome branches directly.
It is one batch because the loop is one function whose branches cannot be split without leaving a half-routed engine, and because the routing tests are the ones that read most naturally beside the routing code they pin.

The external interface batches 3, 4, and 5 consume: the exported `func (s *Shed) Run(ctx context.Context) (Result, error)`, and the unexported test helpers in `internal/shedengine/testsupport_test.go` — `funcProducer`, `newTestShed`, `seedStatus`, and `readStatus`.
Batches 3 and 4 add their own test files against those helpers and must not edit `internal/shedengine/testsupport_test.go` or `internal/shedengine/run_routing_test.go`.

Batch-local decisions, beyond `## Shared Decisions` in the overview:

- The loop lives in one file, `internal/shedengine/run.go`, split across three cards that build it front to back. Cards 8 and 9 edit the file card 7 creates.
- `history[].at` is written from a direct `time.Now().UTC()` call with no injectable clock field, because an injectable clock would add a field to the `Shed` struct shape the design pins, for a value the tests assert structurally instead.

## Cards

### Card 7: Run's prologue and loop steps 1 through 3

- **Context:**
  - `manifest/designs/shed.md`
  - `_mill/discussion.md`
  - `internal/state/state.go`
  - `internal/lock/lock.go`
  - `internal/treadleengine/run.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/validate.go`
  - `internal/shedengine/activity.go`
  - `internal/shedengine/errors.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/run.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedengine/run.go` with a file-level comment naming it as the six-step loop, and declare `func (s *Shed) Run(ctx context.Context) (Result, error)` plus an unexported helper `func findProducer(producers []ProducerDef, name string) (ProducerDef, int, bool)` returning the matching definition, its index in the list, and whether it was found.

  `Run`'s prologue, in this order, before the loop:

  1. Call `s.validate()` and return `Result{}, err` on failure. Nothing on disk has been touched at this point, which is what makes the equal-lock-paths rule an error rather than a hang.
  2. `os.MkdirAll` the parent directory of `LockPath` and the parent directory of `StatusLockPath`, each `0o755`, returning a wrapped error on failure. State in a comment that `internal/lock` opens its lock file with `O_CREATE` but never creates parents, which is why both `internal/loomengine/preflight.go` and `internal/treadleengine/run.go` do this first, and that this is not path derivation — the paths are still told, `Shed` only ensures the told path is usable.
  3. Acquire `LockPath` with `lock.TryAcquireWriteLock`, mirroring `internal/treadleengine/run.go`'s own acquisition. A returned error propagates wrapped. A false `locked` returns `Result{}, fmt.Errorf("%w: ...", ErrShedBusy, ...)` naming the lock path, so a caller can `errors.Is`-match the sentinel. On success `defer` the release. Comment that an OS advisory lock is reclaimed on process death, so a killed run never bricks a later resume, and that `internal/state`'s own per-write lock does not substitute for this one: it is held only for the duration of one write, never across a whole `Call`, so two concurrent runs could otherwise both read the same `current_producer` and both spawn it.
  4. Resolve the bounce budget into a local counter: `MaxBounces` when non-zero, `defaultMaxBounces` when zero. Comment that the counter is per-`Run`-call and in-memory by design — deliberately unpersisted, so a crash-restart or a human-resumed blocked run starts again with the full budget, because every event that resets it is a new human-initiated invocation, which is exactly the outcome the budget exists to force.

  Then an unbounded `for` loop whose first three steps are:

  - **Step 1, the read gate.** `state.ReadJSONStrict[Status](s.StatusPath, s.StatusLockPath)`. A returned error propagates wrapped. A `found` of false is a hard error naming the path and stating that `Shed` never seeds a status file. Then reject a persisted `State` that fails `valid`, naming the offending value verbatim — this covers the empty string too. Every one of these returns before any producer is called and writes nothing.
  - **The already-done short-circuit, positioned after step 1's read and before step 2's lookup.** When the read state is `StateDone`, return `Result{Outcome: RunDone, HaltedProducer: <the file's current_producer>, History: <the file's history>}, nil` immediately, calling no producer and writing nothing. Fill `HaltedProducer` and `History` from the file rather than returning a bare `RunDone`, so a re-run's `Result` is identical to the original completing run's. Comment on the deliberate consequence of the position: a done file whose `current_producer` is no longer in the list returns cleanly and does not hard-error, because a finished task must not become un-queryable because someone later edited the producer list. `StateBlocked` and `StateFailed` deliberately do **not** short-circuit — the loop proceeds and re-calls `current_producer`, which is how a human resumes after fixing whatever caused the halt.
  - **Step 2, the lookup.** `findProducer` against the read `current_producer`. Not found is a hard error that changes nothing on disk, naming both the missing value and the fact that the producer list has changed since the file was last written. Comment that `Shed` never guesses: it neither restarts from the first producer nor advances to the nearest match, because both fabricate a status nobody confirmed.
  - **Step 3, the pause and cancellation check.** When the read `pause_requested` is true **or** `ctx.Err()` is non-nil, take the pause exit: persist `StatePaused` with an empty error text, the history unchanged, `current_producer` unchanged, and `pause_requested` cleared to false, then return `Result{Outcome: RunPaused, HaltedProducer: <the unchanged current_producer>, History: <the unchanged history>}, nil`. Implement this write **inline** in card 7 as a direct `state.UpdateJSON` call — do not call a `persist` method, which does not exist until card 8 refactors this branch into one. Comment that the two conditions are treated identically on purpose — an operator's Ctrl-C or a parent deadline is an operational stop, not a failure, exactly as resumable as an explicit pause request — and that clearing the flag in the same persist is what stops the next `Run` re-pausing forever on the flag it is resuming from; the durable record of "this run is paused" is `state`, not the flag.

  Leave steps 4 through 6 to card 9; end card 7's loop body with a placeholder that card 9 replaces, or stage the file so it compiles — either way `go build ./internal/shedengine/` must succeed at the end of this card, which is why the pause exit's write is inline here rather than a call into card 8's not-yet-existing method.
- **Commit:** `feat(shedengine): add Run's prologue, read gate, and pause exit`

### Card 8: the merging persist

- **Context:**
  - `manifest/designs/shed.md`
  - `_mill/discussion.md`
  - `internal/state/state.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/activity.go`
  - `internal/shedengine/shed.go`
- **Edits:**
  - `internal/shedengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one unexported method to `internal/shedengine/run.go` that is the single write path for the whole loop.
  Name it `persist` and give it a signature carrying everything one commit point needs: the next `current_producer`, the next `State`, the next error text, the next full history slice, and a bool saying whether this persist is the one that consumes a pause request.
  Its body is a single `state.UpdateJSON` call against `StatusPath` and `StatusLockPath` whose mutate:

  - returns an error, without writing, when `UpdateJSON` reports `found` as false — naming the path and stating that `Shed` refuses to create a status file that vanished mid-run;
  - overwrites exactly the Shed-owned fields on the re-read copy: `current_producer`, `state`, `error`, `history`, and `activity` recomposed via `composeActivity` from the values being written;
  - sets `pause_requested` to false only when the pause-consuming bool is set, and otherwise leaves the field exactly as re-read;
  - never touches `product`.

  Document, on the method, why the merge exists rather than a whole-file rewrite from an in-memory copy: `Shed` is not the file's only writer, so a pause requested during a long producer call and an external `product` update must both survive.
  Document, in the same place, the honest limit of that safety — `internal/state`'s lock is advisory and keyed on the caller-supplied path, so the merge is safe against a concurrent external writer *that takes the same `StatusLockPath`*, and against no other; never state it unconditionally.
  Document why the `found` guard is not defensive noise: `state.UpdateJSON` treats a missing file as a non-error and writes whatever the mutate returns, so without the guard a status file deleted mid-run would be silently re-created from a zero value, contradicting the rule that `Shed` never seeds one.
  Also document the accepted leniency asymmetry: `UpdateJSON` re-reads through a plain unmarshal with no unknown-field rejection, so strictness is the contract of the read gate only.
  Malformed JSON still fails loud on this path, but an unknown top-level key written by an external actor after the read gate passed is silently destroyed by the full-struct marshal — accepted, because `product` is the sanctioned channel for everything an external writer legitimately owns, and a key outside it is a mistake nothing here promises to preserve.
  State plainly that such a key is *not* caught by a later strict read, because the merge strips it and the next read sees a clean file.

  Then refactor card 7's inline pause-exit write into a call to this method, deleting the inline `state.UpdateJSON` call it replaces, and confirm the pause persist passes the pause-consuming bool as true and leaves the history and `current_producer` unchanged.
- **Commit:** `feat(shedengine): add the merging single-commit-point persist`

### Card 9: step 4, routing, and the single commit point

- **Context:**
  - `manifest/designs/shed.md`
  - `_mill/discussion.md`
  - `internal/shedengine/status.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
- **Edits:**
  - `internal/shedengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Complete the loop body in `internal/shedengine/run.go`.

  **Step 4.** Call the looked-up definition's `Call` with the loop's `ctx`, capturing the outcome, the output pointer, and the error.

  **Steps 5 and 6, computed then committed once.** Compute the whole route in memory first — the next `current_producer`, the next `State`, the next error text, the returned `Result`, and whether a history entry is appended — then make exactly one `persist` call per iteration.
  Never write twice per iteration: written as two writes, a crash between them leaves `current_producer` still naming the producer that just finished, so the next `Run` re-calls it and appends a duplicate history entry, defeating the exact crash-safety property step 5 exists to provide.

  The history entry, when appended, records the producer's `Name`, the `Outcome` value returned verbatim, the output pointer's `Path`, and `At` as `time.Now().UTC()` formatted with `time.RFC3339`.
  Append it to a copy of the history read at step 1 — do not mutate the read slice in place.

  The routing branches, in this order:

  1. **Non-nil error with a cancelled context.** When `Call` returned a non-nil error *and* `ctx.Err()` is non-nil, take the pause exit exactly as step 3 does: persist `StatePaused`, no history entry appended, `current_producer` unchanged, pause consumed, and return `RunPaused` with a **nil** error. Document why the predicate is `ctx.Err() != nil` and specifically not an `errors.Is` check against a cancellation sentinel: the context's own state is ground truth about whether an operator stopped the run and stays correct even if a producer wraps or discards the sentinel, whereas a producer whose own internal derived context times out returns a deadline error while the parent context is perfectly healthy — a genuine producer failure, not an operator stop. Document why no history entry is appended: the producer never reached a verdict, so there is nothing to record, and leaving `current_producer` put means the next `Run` simply re-calls it, the same semantics as a crash before the persist. Note the accepted trade: a producer returning a genuine, unrelated error in the same instant an operator cancels is reported as a pause, which is harmless because the producer is re-called on resume and the real error surfaces again then.
  2. **Non-nil error with a healthy context.** Persist `StateFailed` with the error's own text, `current_producer` unchanged, history entry appended. Return `Result{}` and the error. No further producer is called. This is an engine-level failure, never a producer verdict, so it is never routed anywhere — a human resolves it.
  3. **`Stuck`.** Append the history entry, then branch three ways. When the definition's `OnStuck` is empty, persist `StateBlocked` with the error text exactly `stuck with no OnStuck target` and `current_producer` unchanged, and return `RunBlocked` with `Reason` set to that identical string and a nil error. When `OnStuck` is set but the bounce counter has reached zero, persist `StateBlocked` with the error text exactly `bounce budget exhausted`, same shape, same identical-text rule for `Reason`. Otherwise decrement the counter, persist `StateRunning` with `current_producer` set to the `OnStuck` target, and continue the loop. Pin the boundary in a comment: `MaxBounces` bounces are permitted and the next `Stuck` that would otherwise route is the one refused, so a budget of three performs three bounce-backs and blocks on the fourth `Stuck`. State that `Result.Reason` and the persisted error text are deliberately one string written to two places, never two phrasings of the same fact that could drift apart.
  4. **`Done`.** Append the history entry. When the current producer is the last entry in the list, persist `StateDone` with `current_producer` left holding **this producer's own name** — never the empty string — and return `RunDone` with `HaltedProducer` set to that same name. Document why: `activity.now` is defined as `current_producer`'s name and `HaltedProducer` as the producer `current_producer` named when `Run` returned, so an empty value would leave both fields meaningless at the happy-path terminal a reader of a finished status file most wants to understand. Otherwise persist `StateRunning` with `current_producer` advanced to the next entry's `Name` and continue the loop.
  5. **Anything else.** An `Outcome` that is neither `Done` nor `Stuck`, returned with a nil error, is an engine-level failure. Append the history entry recording the literal value received, persist `StateFailed` with an error text naming both the offending value and the producer that returned it, and return `Result{}` with that same error. Document why hard-failing is the only disposition consistent with never guessing a status: `Outcome` is a string type and therefore open, so the routing would otherwise have an undefined fourth case, and coercing an unknown value to `Stuck` would consume bounce budget for a broken adapter while coercing it to `Done` would advance past a producer that may not have done its work.

  On every path where `persist` itself returns an error, halt and return that error immediately, **without** attempting a compensating `StateFailed` write — that write is the exact same operation that just failed, so retrying it to record the failure is the one action already known not to work.
  The file keeps its last-good contents, so `current_producer` still names the producer whose `Call` just ran and it is simply re-called next time.

  Every clean exit's `Result.History` is the full persisted history as it stands at that moment, not only the entries this invocation appended.
- **Commit:** `feat(shedengine): complete the loop's producer call, routing, and exits`

### Card 10: shared test fakes and helpers

- **Context:**
  - `_mill/discussion.md`
  - `internal/state/state.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/run.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/testsupport_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedengine/testsupport_test.go` in `package shedengine`, holding the fakes and helpers every test file in this package uses.
  It contains no `Test` function of its own.

  Declare `funcProducer`, a struct adapting a closure onto `ShedProducer`: a `fn` field of the same signature as `Call`, and a `calls` counter incremented on every invocation.
  Its `Call` method must have a pointer receiver so the counter survives, and must record the call before delegating.
  Add a small constructor returning a `funcProducer` that always reports one fixed outcome, so the common "this producer just succeeds" case is one line at each call site.

  Declare a helper that builds a `Shed` wired to a fresh `t.TempDir()`: the status file under a `durable` subdirectory of the temp dir, and both lock paths under a separate `ephemeral` subdirectory, with distinct file names.
  The status file's own parent directory must be created by the helper, since `Shed` never creates it;
  the lock parents must be left uncreated by default so the ordinary path exercises `Run`'s own `MkdirAll`.
  Return both the `Shed` and the three paths so a test can seed, inspect, and corrupt them.

  Declare a helper that seeds a status file from a `Status` value via `state.WriteJSON`, and a helper that reads one back via `state.ReadJSONStrict`, both taking `*testing.T`, calling `t.Helper()`, and failing the test on error.
  Add a convenience that builds the common seed — a given `current_producer`, `StateRunning`, no error, no pause, empty history — so each test states only what it varies.

  Add a helper that asserts a `HistoryEntry`'s `At` field parses as RFC3339 with a zero UTC offset, and one that asserts a slice of entries is non-decreasing in time.
  Never assert an exact timestamp literal.

  Every helper stays in this file;
  batches 3 and 4 consume them and must not redeclare them.
- **Commit:** `test(shedengine): add shared producer fakes and status-file helpers`

### Card 11: happy-path and completion-terminal scenarios

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/run_routing_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedengine/run_routing_test.go` in `package shedengine` with a file-level comment naming its scope as the outcome-routing scenarios, and add these tests:

  **Happy path.** A three-producer list where every producer returns `Done`.
  Assert `Run` returns a nil error and `RunDone`;
  the persisted state is `StateDone`;
  the persisted history has exactly one entry per call, in list order, each with the right producer name and the `done` outcome;
  every entry's `At` parses as RFC3339 UTC and the entries are non-decreasing;
  and each producer's call counter is exactly one.

  **Completion terminal values.** On the same shape, assert the persisted `current_producer` holds the **last** producer's name rather than the empty string, that the persisted `activity.now` matches it, and that `Result.HaltedProducer` matches it too.
  Assert `Result.History` equals the persisted history — the full file's history, not a this-run-only slice.

  **Unconditional re-call.** Seed a status file whose `current_producer` names a producer, and create the file its `OutputPointer.Path` will name **before** running, so the artifact already exists on disk.
  Assert the producer is still called exactly once — never skipped.
  Comment that this is an explicit design guarantee rather than an incidental behaviour: `Shed` cannot tell a stale output file from a fresh one by existence alone, notably after a bounce-back, so the three-case respawn discipline is delegated whole to each engine adapter instead.
- **Commit:** `test(shedengine): cover the happy path and completion terminal values`

### Card 12: stuck, bounce, and budget scenarios

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/shedengine/run_routing_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the `Stuck`-routing scenarios to `internal/shedengine/run_routing_test.go`.

  **Stuck with an `OnStuck` target.** A list where a later producer returns `Stuck` once and then `Done`, with its `OnStuck` naming an earlier producer.
  Assert `current_producer` moved to the named target, that the target producer ran again (its call counter is two), that the bounce is recorded in the history as its own entry with the `stuck` outcome, and that the run still reaches `RunDone`.

  **Stuck with no target.** A producer returning `Stuck` whose `OnStuck` is empty.
  Assert `Run` returns a nil error and `RunBlocked`;
  `Result.HaltedProducer` names that producer;
  the persisted state is `StateBlocked`;
  and both `Result.Reason` and the persisted error field are exactly the string `stuck with no OnStuck target`.
  Assert the two are equal to each other, not merely each equal to the literal.

  **Bounce-budget exhaustion.** Two producers that always return `Stuck`, each `OnStuck`-pointing at the other, with `MaxBounces` set to three.
  Assert the run performs exactly three bounce-backs and then blocks on the fourth `Stuck` — assert the exact total call count that boundary implies, not merely that the run terminated — and that the persisted error field and `Result.Reason` are both exactly `bounce budget exhausted`.
  Comment that the exact count is the whole point of the assertion, because this is the classic off-by-one seam.

  **`MaxBounces` of zero resolves to the internal default.** The same always-`Stuck` cycle with `MaxBounces` left at zero.
  Assert the run performs the internal default number of bounces rather than blocking on the first `Stuck` — zero means "use the default", never "no bounces allowed".
  Reference `defaultMaxBounces` rather than hard-coding ten a second time.
- **Commit:** `test(shedengine): cover stuck routing, the bounce budget, and its boundary`

### Card 13: error and unrecognised-outcome scenarios

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/shedengine/run_routing_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the engine-level-failure scenarios to `internal/shedengine/run_routing_test.go`.

  **Producer returns an error.** A three-producer list whose middle producer returns a non-nil error, run with a healthy, never-cancelled context.
  Assert `Run` returns a non-nil error carrying the producer's own message;
  the persisted state is `StateFailed`;
  the persisted error field is populated with that message;
  and the third producer's call counter is zero — no further producer is called.
  Assert the failing call is recorded in the persisted history.

  **Producer error with a healthy context, explicitly.** Assert the same failure classification holds when the context is alive, so the cancellation-aware branch demonstrably keys on the context's own state and does not swallow genuine failures.
  This may share a table with the case above only if the assertion that the context was healthy is explicit;
  otherwise write it as its own test.

  **Unrecognised outcome.** A producer returning a nil error alongside an `Outcome` that is neither `Done` nor `Stuck` — cover both a plausible-looking wrong value and the empty string.
  Assert the persisted state is `StateFailed`;
  `Run` returns a non-nil error whose message names both the offending value and the producer;
  no further producer is called;
  and the persisted history entry records the **literal** value received, so a human diagnosing it can see exactly what the adapter returned.
- **Commit:** `test(shedengine): cover producer errors and unrecognised outcomes`

## Batch Tests

`verify: go test ./internal/shedengine/... && go test -run 'TestTierPurity_|TestHermeticGitEnv_' ./cmd/lyx/`.

The first half runs everything in the new package: batch 1's three unit-test files plus this batch's `internal/shedengine/run_routing_test.go`, which drives `Run` end to end against a real status file for every outcome-routing branch — happy path, completion terminal values, unconditional re-call, `Stuck` with and without a target, the bounce budget and its exact off-by-one boundary, the zero-means-default rule, producer errors with a healthy context, and unrecognised outcomes.

The second half is a scoped two-test run of the repo-wide tier guards, not the whole `cmd/lyx` suite.
It is here because this batch introduces the package's first untagged test files that drive real filesystem work, and the Test Tier Purity Invariant and the Hermetic Git Test Environment Invariant are both enforced from `cmd/lyx` rather than from the package under test — a violation introduced here would otherwise stay invisible until the task-wide done gate.
`-run` keeps it to the two guard tests, so it costs a compile and a directory walk rather than the full `cmd/lyx` suite (which includes cross-compilation).
