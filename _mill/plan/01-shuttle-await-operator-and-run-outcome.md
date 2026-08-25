# Batch: shuttle-await-operator-and-run-outcome

```yaml
task: 'loom: interactive Discussion-Write'
batch: 'shuttle-await-operator-and-run-outcome'
number: 1
cards: 3
verify: go test ./internal/shuttleengine/
depends-on: []
```

## Batch Scope

This batch closes **Defect A** (an ask is terminal, so an interactive interview dies at its first question) and lays the one persisted field batch 2's `Attach` reads.
Both changes live entirely inside `internal/shuttleengine` and are independent of each other, which is why they share a batch: each is a handful of lines plus its tests, and splitting them would leave two batches editing the same four files back to back.

The external interface batch 2 and batch 4 consume:

- `shuttleengine.Spec.AwaitOperator bool` — a new exported field. `loomengine.DiscussionSpec` sets it in batch 4.
- `shuttleengine.RunState.Outcome string` — a new persisted `run.json` field carrying the sentinel `"running"` or a terminal classification. `Attach` reads it in batch 2.
- the unexported constant `runOutcomeRunning` — batch 2's `Attach` compares against it in the same package.

Batch-local decision that differs from nothing in the overview: the `Outcome` persistence write in `finalize` is **best-effort**. A failed write logs a `logger.Warn` and returns the classified `Result` unchanged rather than converting a successful run into an error.
The accepted residual is stated in `attach-only-a-run-that-never-terminated`: such a record keeps `"running"`, so if its pane is also still live and idle it stays attachable for that one run.

## Cards

### Card 1: `Spec.AwaitOperator` makes an `Asking` classification non-terminal in `Wait`

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/doc.go`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/fakes_test.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/claudeengine/settings.go`
- **Edits:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/wait_test.go`
  - `internal/shuttleengine/spec_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add an exported field `AwaitOperator bool` to `shuttleengine.Spec` in `spec.go`, placed immediately after the existing `Interactive` field.
  Its doc comment must state that it governs the wait loop only — when true, `Run.Wait` treats an `OutcomeAsking` classification as non-terminal and keeps polling, so the run still terminates on `OutcomeDone`, `OutcomeDied`, a liveness mechanism failure, or `OutcomeTimeout` — and must state why it is a second field rather than a widening of `Interactive`: `internal/shuttleengine/claudeengine/settings.go` installs a `PreToolUse(AskUserQuestion)` hook in every interactive run precisely so an ask classifies as a real-time asking signal, and `lyx shuttle run --interactive` depends on that signal staying terminal.
  The comment must also record the accepted failure mode from `asking-non-terminal-via-a-new-spec-field`: an interactive run whose agent is genuinely wedged now hangs until its `Timeout` rather than reporting `Stuck` promptly, which is the correct trade for a mode whose premise is that a human is watching the pane.
  `Spec.validate` must **not** inspect `AwaitOperator` — it needs neither defaulting nor rejection.

  In `wait.go`, change only the branch of `Run.Wait` that consumes `run.pollEventsTick()`'s result.
  Today it reads `if outcome != "" { return run.finalize(outcome, message) }`.
  It must instead drop an `OutcomeAsking` when `run.spec.AwaitOperator` is true — logging the observation via `logger.Info` naming the strand guid and the last assistant message, so the driver log records each ask — and continue the loop.
  `OutcomeDone` must still finalize under `AwaitOperator`, and every other `Wait` exit (the liveness branch's `run.finalize(livenessOutcome, "")`, its three mechanism-failure returns, and the deadline's `run.finalize(OutcomeTimeout, "")`) is untouched.
  Update `Wait`'s own doc comment and `wait.go`'s file-header comment to name the new behaviour.

  Extend `wait_test.go` with the defect-A coverage, driven against the existing `fakeClock`/`scriptedClock`, `fakeReed`, and `fakeEngine` seams:
  a run with `AwaitOperator: false` and one turn-end event with no output files present returns `OutcomeAsking` (pins today's behaviour);
  the same run with `AwaitOperator: true` does not return on that event, keeps polling, and returns `OutcomeDone` once the output files appear on a later tick;
  several asks in a row followed by a done, so a multi-batch interview is covered rather than a single ask;
  `AwaitOperator: true` still returns `OutcomeTimeout` once the deadline passes with the files absent;
  `AwaitOperator: true` still returns `OutcomeDied` for a tracked strand with a dead pane, and still surfaces `errStrandNotTracked` and `errStrandPaneBindingCleared` as mechanism failures.

  Extend `spec_test.go` with one assertion that `Spec.validate` leaves `AwaitOperator` untouched in both its true and false states, alongside the existing normalization coverage.
- **Commit:** `feat(shuttle): add Spec.AwaitOperator so an ask is non-terminal in Wait`

### Card 2: `RunState.Outcome` — `"running"` at `Start`, the classification at `finalize`

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/fakes_test.go`
  - `internal/state/state.go`
- **Edits:**
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/run_test.go`
  - `internal/shuttleengine/wait_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `rundir.go`, add an unexported constant `runOutcomeRunning = "running"` and a new field `Outcome string` with json tag `outcome` to the `RunState` struct, placed after `CreatedAt`.
  The field's doc comment must state the three writable states from `attach-only-a-run-that-never-terminated`: `Start` writes `runOutcomeRunning`; `Run.finalize` overwrites it with the classification for **every** terminal outcome, not only `OutcomeDone`; and any other value, **including the empty string**, means the record was written by a binary that did not know about this field and is therefore never attachable.
  It must state why the sentinel is explicit rather than "empty means attachable": every `run.json` written by the pre-change binary decodes with `Outcome == ""`, and treating that as attachable would make an in-flight worktree blocked on an `Asking` run at upgrade time attach to an idle pane and wait out a fresh `run_timeout_min`.
  Update `RunState`'s own struct doc comment to mention the new field alongside the ones it already enumerates.

  In `run.go`, `Runner.Start` must set `Outcome: runOutcomeRunning` in the `RunState` literal it builds before `saveRunState`.

  In `wait.go`, `Run.finalize` must persist the classification: set `run.state.Outcome = string(outcome)` and call `saveRunState(run.runDir, run.state)` for **every** outcome, and it must do so **before** the `cleaned` block that performs `RemoveStrand` and `os.RemoveAll`.
  A `saveRunState` failure here is best-effort: log a `logger.Warn` naming the run dir, the strand guid, the outcome, and the error, then continue — the classified `Result` is returned unchanged and is never converted into an error.
  Place the write **before** the fork-audit block, not after it.
  `finalize` returns early with `result, err` when `AuditForks` fails, so a write placed after the audit would leave a run that genuinely classified `OutcomeDone` persisted at `"running"` — the exact live-but-idle state the sentinel exists to prevent, if its pane is still alive on a later resume.
  No row in this task sets `ForkSubagents`, so the gap is unreached today, which is precisely why it must be closed by placement rather than left as an undocumented forward-looking hole.
  Writing first costs nothing: the audit's early return still returns the same `result` with the same error, and the write still precedes the cleanup block either way.
  Update `finalize`'s doc comment to describe the write and its best-effort disposition.

  Extend `run_test.go` with an assertion that a run started through `Runner.Start` persists `run.json` with `Outcome: "running"`.
  Extend `wait_test.go` with: one case per terminal outcome (`done`, `asking`, `died`, `timeout`) asserting `finalize` overwrote the persisted `Outcome` with the matching value;
  an assertion that on the `Done` path the `Outcome` write precedes the cleanup, observable under `KeepPane: true` where the directory survives and must hold `"done"` rather than a stale `"running"`;
  and an assertion that a failing `Outcome` write still returns the classified `Result` with its `Outcome` field intact rather than an error.
  For the failing-write case, make `saveRunState` fail by replacing the run directory's `run.json` path with a directory before `Wait` finalizes, following `fakeEngine.PrepareHook`'s existing precedent for planting on-disk state that makes a `saveRunState` call fail.
- **Commit:** `feat(shuttle): persist RunState.Outcome so a resume can tell a live run from an ended one`

### Card 3: `shuttleengine` package documentation for both new fields

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/rundir.go`
- **Edits:**
  - `internal/shuttleengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend the package documentation with a short paragraph covering the two new pieces of vocabulary this batch adds.
  It must say that `Spec.AwaitOperator` is the wait loop's "wait for the operator rather than reporting back" knob, orthogonal to `Spec.Interactive`'s "an operator is present" (which governs launch flags and the `AskUserQuestion` recording hook), and that one caller — `lyx shuttle run --interactive` — wants the first without the second.
  It must say that `RunState.Outcome` records whether a run ever ended, seeded `"running"` and overwritten on every terminal classification, so "has this run already ended?" is a fact on disk rather than an inference from pane liveness.
  Keep the existing statement that the package is provider-invariant intact — neither field may grow a Claude specific, per the `Shuttle Provider-Seam Invariant`.
  Do not describe `Attach` here; batch 2 adds its own paragraph.
- **Commit:** `docs(shuttle): document AwaitOperator and RunState.Outcome in package docs`

## Batch Tests

`verify: go test ./internal/shuttleengine/` runs the whole package's untagged Tier-1 suite, which is the right scope: every file this batch edits is in that one package, and the package's tests are fast and hermetic (`fakeReed`, `fakeEngine`, and `fakeClock` mean no tmux, no `claude`, and no real sleeping).

Covered files: `wait_test.go` (the `AwaitOperator` matrix and the `finalize` `Outcome`-write matrix), `run_test.go` (the `Start` seeds `"running"` assertion), `spec_test.go` (validate leaves `AwaitOperator` alone), and `rundir_test.go`/`run_inject_test.go`/`config_test.go`/`posix_test.go`/`seam_enforcement_test.go` as untouched regression guards — `seam_enforcement_test.go` in particular re-proves the `Shuttle Provider-Seam Invariant` held across these edits.

The overview's module-wide `verify: go vet ./...` additionally type-checks every other package's test files at this batch boundary, catching any caller broken by the `RunState`/`Spec` struct changes.
