# Batch: pause-and-resume-scenarios

```yaml
task: 'Shed: outer phase-FSM skeleton'
batch: pause-and-resume-scenarios
number: 3
cards: 4
verify: go test ./internal/shedengine/... && go test -run 'TestTierPurity_|TestHermeticGitEnv_' ./cmd/lyx/
depends-on: [2]
```

## Batch Scope

This batch covers every scenario about *stopping and starting again*: the two clean-stop paths (an explicit pause request and context cancellation), the resume that must follow each, crash recovery across two separate `Shed` values, and the already-done short-circuit's idempotence rules.
It is one batch because all of these drive `Run` more than once against the same status file and share the same mental model — what survives on disk between invocations — and because they all land in one new test file, `internal/shedengine/run_pause_test.go`, which no other batch touches.

This batch adds no production code and exposes no interface to a later batch.
It runs in parallel with batch 4;
neither touches the other's file, and neither may edit `internal/shedengine/testsupport_test.go` or `internal/shedengine/run_routing_test.go`, both owned by batch 2.

Batch-local decisions, beyond `## Shared Decisions` in the overview:

- Several scenarios here need a producer that mutates external state mid-`Call` (cancelling a context, setting a flag). Every such mutation happens from inside the fake producer's own closure, because that is the only point in the loop where the test is guaranteed to be between step 1's read and step 5's persist.
- Any mid-`Call` mutation of the **status file** in this batch goes through `internal/state` — `state.UpdateJSON` against a lenient map type, or `state.WriteJSON` — using the same `StatusLockPath` the `Shed` was told, never a bare `os.WriteFile` or an unlocked struct write. That is the lock-cooperating shape the package doc states as a caller-side obligation, and it is the only external-writer shape this design supports; batch 4 pins the identical rule for its own scenarios. Cancelling a context is not a status-file mutation and is unaffected by this rule.

## Cards

### Card 14: pause request and resume

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `internal/state/state.go`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/run_pause_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedengine/run_pause_test.go` in `package shedengine` with a file-level comment naming its scope as the clean-stop, resume, and idempotence scenarios, and add these tests:

  **Pause requested mid-list.** A three-producer list where the first producer sets `pause_requested` to true from inside its own `Call`, then returns `Done`.
  That write goes through `state.UpdateJSON` against a lenient map type using the same status path and status lock path the `Shed` was told, per this batch's second local decision — never a bare unlocked write.
  Assert the run exits before calling the next producer — the second and third producers' call counters are zero;
  `Run` returns `RunPaused` with a **nil** error;
  the persisted state is `StatePaused`;
  and the persisted `current_producer` still names the producer the loop was about to call, so the next `Run` resumes there rather than skipping it.

  **Resume after pause.** Continue from the state the previous scenario left on disk, or seed the equivalent directly.
  First assert that the persist which honoured the pause also wrote `pause_requested` back to false, in the same write that recorded `StatePaused`.
  Then run a **second** `Run` against that same file with producers that all return `Done`, and assert it proceeds to completion instead of pausing again.
  Comment that without this scenario the permanently-unresumable bug is invisible: every single-`Run` pause test passes while the task can never restart, because the next `Run`'s own step 3 would re-read a still-true flag and pause again, forever.
- **Commit:** `test(shedengine): cover the pause request and the resume that follows it`

### Card 15: context cancellation, between and during a call

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/shedengine/run_pause_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the cancellation scenarios to `internal/shedengine/run_pause_test.go`.

  **Cancelled between producers.** A list whose first producer cancels the test's own context from inside its `Call` and then returns `Done` normally.
  Assert the observable outcome is identical to an explicit pause — `RunPaused`, a nil error, `StatePaused` on disk — and specifically that **no** producer is called after the cancellation.
  Comment that checking the context at the top of every iteration, rather than leaving it to producers, is what stops a cancelled run from launching new producer calls for however long the current one takes to notice.

  **Cancelled during a producer call.** A fake producer that cancels the context and then returns the context's own error from inside the same `Call`, modelling a well-behaved producer handed a cancelled context.
  Assert the persisted state is `StatePaused`;
  `Run` returns `RunPaused` with a **nil** error, not a failure;
  **no** new history entry was appended;
  and `current_producer` is unchanged, so the next `Run` re-calls that producer.
  Comment that this is the common Ctrl-C shape, and that without this scenario the top-of-iteration check passes while every real cancellation still lands in `StateFailed`.

  Do not assert the returned error is a specific cancellation sentinel anywhere in this card;
  the branch keys on the context's own state, and pinning a sentinel here would encode the predicate the design explicitly rejected.
- **Commit:** `test(shedengine): cover cancellation between and during a producer call`

### Card 16: crash recovery and resume after a halt

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/shedengine/run_pause_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the resume-across-invocations scenarios to `internal/shedengine/run_pause_test.go`.

  **Crash recovery.** Run a three-producer list partway by having the middle producer return a non-nil error, so the run halts with the persisted `current_producer` naming that middle producer.
  Then construct a **fresh** `Shed` value against the same status file and the same lock paths, with a fresh set of fake producers where the middle one now succeeds, and run again.
  Assert the second run resumes at the persisted `current_producer`: the first producer's fresh counter is zero, the middle and last producers ran, and the run reaches `RunDone`.
  Assert the second run's history contains the first run's entries as well as its own — the file's history is cumulative across invocations and `Result.History` reports all of it.
  Comment that the fresh `Shed` is the point of the scenario: nothing carries over in memory, so everything the resume relies on must have survived on disk.

  **Resume after a halt.** Assert directly that a status file carrying `StateBlocked` does **not** short-circuit: seed one whose `current_producer` names a producer that now returns `Done`, run, and assert that producer was called and the state was overwritten as the loop advanced.
  Repeat for `StateFailed`.
  Comment that this is deliberately asymmetric with `StateDone`, so a human can resume after fixing whatever caused the halt without hand-editing the status file.
- **Commit:** `test(shedengine): cover crash recovery and resuming a halted run`

### Card 17: already-done idempotence

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/shedengine/run_pause_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the short-circuit scenarios to `internal/shedengine/run_pause_test.go`.

  **Re-run idempotence.** Run a list to completion, capture the status file's bytes, then run a second time against the same file.
  Assert the second `Run` returns `RunDone` with a nil error, calls **zero** producers — assert the call counters, not merely the outcome — and leaves the file's bytes unchanged.
  Comment that without the short-circuit a second invocation would re-call the final producer, which for a merge-shaped terminal producer means re-running a merge.

  **Re-run `Result` equality.** Assert the second, short-circuited run's `Result` equals the first, completing run's: the same `Outcome`, the same `HaltedProducer`, and the same `History`.
  Comment that this is what proves the short-circuit fills both fields from the file rather than returning a bare `RunDone`, and that idempotence is meant at the API surface, not merely on disk.

  **Re-run with a changed producer list.** Take a status file carrying `StateDone` whose `current_producer` is **not** present in the `Shed`'s `Producers` list, and run.
  Assert it returns `RunDone` cleanly with a nil error, and specifically **not** the step-2 lookup error.
  Comment that this confirms the short-circuit sits after the read and before the lookup, and that the position is a decision rather than an accident: a finished task must not become un-queryable because someone later edited the producer list, and the never-guess protection exists to stop a *live* run resuming into the wrong producer, a risk that does not exist once nothing more will be called.
- **Commit:** `test(shedengine): cover already-done idempotence and its Result equality`

## Batch Tests

`verify: go test ./internal/shedengine/... && go test -run 'TestTierPurity_|TestHermeticGitEnv_' ./cmd/lyx/`, the same command batch 2 uses and for the same two reasons.

The package run covers this batch's new file, `internal/shedengine/run_pause_test.go`, alongside everything batches 1 and 2 already established.
Between them the scenarios pin: both clean-stop paths and their identical observable outcome, the pause flag being consumed rather than latched, the resume that proves the run is not permanently unresumable, cancellation arriving during a call appending no history entry and leaving `current_producer` put, crash recovery across two distinct `Shed` values against one status file, `StateBlocked` and `StateFailed` deliberately not short-circuiting, and the three already-done idempotence rules including `Result` equality and the changed-list case.

The scoped `cmd/lyx` guard run repeats here because this batch adds another untagged test file to the package;
the Test Tier Purity Invariant is enforced per file, not per package, so a new file needs the guard re-run even though the previous one passed.
