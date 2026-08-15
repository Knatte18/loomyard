# Batch: persistence-and-hard-error-scenarios

```yaml
task: 'Shed: outer phase-FSM skeleton'
batch: persistence-and-hard-error-scenarios
number: 4
cards: 5
verify: go test ./internal/shedengine/... && go test -run 'TestTierPurity_|TestHermeticGitEnv_' ./cmd/lyx/
depends-on: [2]
```

## Batch Scope

This batch covers everything about the status file as a *shared* artifact and everything that makes `Run` refuse to proceed: the read-modify-write merge against a concurrent external writer, the opaque `product` passthrough, the deliberate destruction of a stray external key, the two ways a persist can fail, all five read-gate and lookup hard errors, and the locking rules.
It is one batch because every scenario in it is about the boundary between `Shed` and the disk, and because they all land in one new test file, `internal/shedengine/run_persist_test.go`, which no other batch touches.

This batch adds no production code and exposes no interface to a later batch.
It runs in parallel with batch 3;
neither touches the other's file, and neither may edit `internal/shedengine/testsupport_test.go` or `internal/shedengine/run_routing_test.go`, both owned by batch 2.

Batch-local decisions, beyond `## Shared Decisions` in the overview:

- Every simulated external writer in this batch goes through `state.UpdateJSON` against a lenient map type using the same `StatusLockPath` the `Shed` was told, rather than a bare `os.WriteFile`. That is the lock-cooperating shape the package doc states as a caller-side obligation, and a test that modelled a non-cooperating writer would be asserting a property the design explicitly does not claim.
- The one exception is the file-deleted scenario, where the point is the file's absence rather than its content.

## Cards

### Card 18: the merge against a concurrent external writer

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
  - `internal/shedengine/run_persist_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedengine/run_persist_test.go` in `package shedengine` with a file-level comment naming its scope as the persistence-merge, hard-error, and locking scenarios, and add these tests:

  **External mid-producer write.** A multi-producer list whose first producer, from inside its own `Call`, sets `pause_requested` to true **and** replaces the `product` payload with a new value — both through `state.UpdateJSON` against a lenient map type using the same status path and status lock path the `Shed` was told.
  Assert both survive the persist that follows: after the run, the file's `product` holds the producer's new payload and the pause was honoured at the very next iteration, so the run exits `RunPaused` without calling the second producer.
  Comment that this is the regression test for the whole-file-clobber hazard, and that it is the reason the loop re-reads at the top of every iteration and merges rather than rewriting from an in-memory copy: as originally specified, a pause requested during a long producer call was both never observed and silently destroyed.

  **`product` passthrough.** Seed a status file carrying an arbitrary product payload, run a list to completion, and assert the payload survives with **semantic** equality — compare through an `any` unmarshal or a re-marshalled normal form, never a raw byte compare.
  State the reason in a comment on the assertion: persistence goes through an indenting marshal, which re-indents an embedded raw message, so a payload written with different whitespace survives semantically but not byte-for-byte.
  Assert too that `Shed` never inspected it: the payload may be a shape `Shed` has no type for at all.
- **Commit:** `test(shedengine): cover the merging persist and the product passthrough`

### Card 19: a stray external key is destroyed

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `internal/state/state.go`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/shedengine/run_persist_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the stray-key scenario to `internal/shedengine/run_persist_test.go`.

  A fake producer writes an **unrecognised top-level key** into the status file from inside its own `Call` — after the read gate for that iteration has already passed — through the same lock-cooperating lenient-map update the previous card uses.
  Assert the key is **absent** from the file after the next persist, and that the run itself completes normally rather than erroring.

  Comment on what this pins and why it is worth a test at all: the key is silently dropped by the full-struct marshal, *not* caught by a later strict read, because the merge strips it and the next read then sees a clean file.
  That is a corrected misconception rather than an obvious fact, so nothing but a test will stop a future reader re-deriving the wrong answer.
  Note the acceptability argument in the same comment — the `product` field is the sanctioned channel for everything an external writer legitimately owns, so a top-level key outside it is a mistake nothing in this design promises to preserve — and note that a key present *before* the read gate is a different case entirely, covered by the strict-decode scenario in card 21.
- **Commit:** `test(shedengine): pin that a stray external top-level key is destroyed`

### Card 20: persist failure and a status file that vanishes

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `internal/state/state.go`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/shedengine/run_persist_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the two write-side failure scenarios to `internal/shedengine/run_persist_test.go`.

  **Status file deleted mid-run.** A fake producer deletes the status file from inside its own `Call`.
  Assert `Run` returns a non-nil error and that **no** status file exists at the target path afterwards.
  Comment that this proves the persist's own missing-file guard holds the never-seed rule at the write as well as at the read: the underlying update primitive treats a missing file as a non-error and would otherwise write whatever the mutate returned, silently re-creating the file from a zero value.

  **Persist failure.** Inject the failure from inside a fake producer's `Call`, after the read gate has already succeeded, by removing the directory that contains the status file and creating a **regular file** at that same path — so the persist's own directory creation fails with a not-a-directory error on every platform regardless of privilege.
  Assert exactly two things and no more: `Run` returns a non-nil error, and no status file exists at the target path.

  Write a comment stating why each simpler injection was rejected, so a future reader does not "simplify" the test back into one that cannot fail: a merely-absent parent directory does not fail, because the update primitive creates it first;
  an unwritable parent directory is a no-op under root and on Windows;
  and routing the status path through an existing regular file fails too early, at the read gate rather than at the persist, testing the wrong thing.

  Write a second comment stating why the tempting third assertion is absent: "no compensating failed-state write was attempted" is **unobservable** under this injection, because the injection destroys the write target, so a hypothetical compensating write would fail identically to the persist itself and the test cannot distinguish "never tried" from "tried and also failed".
  That the compensating write is forbidden is a design decision enforced by review, not by this test.
  Do not assert the file keeps its last-good contents either — this injection necessarily destroys the file that assertion would read;
  the property is covered by batch 3's crash-recovery scenario instead.
- **Commit:** `test(shedengine): cover persist failure and a status file deleted mid-run`

### Card 21: read-gate and lookup hard errors

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `internal/state/state.go`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/shedengine/run_persist_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add every hard-error scenario to `internal/shedengine/run_persist_test.go`.
  In each, assert a non-nil error, a zero-valued `Result`, and that **no** producer was called.

  **Missing status file.** No status file at the target path.
  Assert the error, and assert specifically that the status file was **not** created.
  Assert too that the lock files and their parent directories *were* created, and comment that an assertion of "nothing created on disk" would be false: acquiring a lock creates the lock file, and only the status file is covered by the never-seed rule.

  **`current_producer` naming an absent producer.** Seed a status file whose `current_producer` is not in `Producers`.
  Assert the error and that the status file is byte-identical afterwards — capture its bytes before the run and compare.

  **Malformed status file.** Write bytes that are not valid JSON at the status path.
  Assert the error.

  **Unknown top-level key present before the read.** Seed an otherwise-valid status file carrying an extra top-level key.
  Assert the error, and comment that this is the strict read gate rejecting a key that was on disk when the gate ran, which is deliberately a different case from card 19's key written after the gate passed.

  **Unrecognised persisted state.** Two cases: a typo value, and the empty string.
  Assert each is a hard error at the read gate with no producer called, and comment that the empty string is rejected rather than tolerated because a mandatory enum string left empty by a partial seed would otherwise be silently treated as running, or as done.
- **Commit:** `test(shedengine): cover the read-gate and lookup hard errors`

### Card 22: locking scenarios

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `internal/lock/lock.go`
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/errors.go`
- **Edits:**
  - `internal/shedengine/run_persist_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the locking scenarios to `internal/shedengine/run_persist_test.go`.

  **Run lock already held.** Acquire the `Shed`'s own `LockPath` directly in the test via `internal/lock`'s non-blocking acquire — no second process is needed — then call `Run`.
  Assert it returns immediately with an error matching `ErrShedBusy` via `errors.Is`, that no producer was called, and that the status file is untouched.
  Release the lock at the end of the test.

  **Lock-parent creation.** Build a `Shed` whose `LockPath` and `StatusLockPath` sit under directories that do not yet exist, seed a valid status file, and run.
  Assert the run succeeds rather than failing with a raw lock-acquire error, and that both lock parents now exist.

  **Two-lock validation.** A `Shed` whose `LockPath` and `StatusLockPath` name the same file.
  Assert `Run` returns a validation error, and specifically that it does **not** hang.
  Comment that the assertion is the error itself: a test that deadlocked here would hang the whole suite rather than fail it, which is precisely the failure shape the same-path rule exists to convert into an error.
  Do not attempt to bound the call with a timer or a goroutine — validation runs before any lock is acquired, so a correct implementation cannot reach the blocking acquire at all, and a wrapper that tolerated a hang would hide the bug rather than catch it.
- **Commit:** `test(shedengine): cover the run lock, lock-parent creation, and the two-lock rule`

## Batch Tests

`verify: go test ./internal/shedengine/... && go test -run 'TestTierPurity_|TestHermeticGitEnv_' ./cmd/lyx/`, the same command batches 2 and 3 use and for the same two reasons.

The package run covers this batch's new file, `internal/shedengine/run_persist_test.go`, alongside everything batches 1, 2, and 3 established.
Its scenarios pin the write side and the refusals: a lock-cooperating external writer's `pause_requested` and `product` both surviving a persist and the pause being honoured on the next iteration, the `product` passthrough's semantic-equality rule, the deliberate destruction of a stray key written after the read gate, a status file deleted mid-run producing an error rather than a silent re-seed, a genuine persist failure via the one injection that works on every platform, all five read-gate and lookup hard errors, and the three locking rules including that the same-path mistake fails rather than hangs.

The scoped `cmd/lyx` guard run repeats here for the same reason it does in batch 3: the Test Tier Purity Invariant is enforced per test file, so this batch's new file needs the guard re-run even though the previous ones passed.
