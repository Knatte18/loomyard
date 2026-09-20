# Batch: shuttle-gate-loop

```yaml
task: 'Producer gates: mechanical gates before session release'
batch: 'shuttle-gate-loop'
number: 1
cards: 8
verify: go test ./internal/shuttleengine/...
depends-on: []
```

## Batch Scope

This batch builds the whole gate mechanism inside `internal/shuttleengine` and nothing else: the four contract types, the gated entry points on `Runner`, the single verdict site in `run.finalize`, and the re-prompt attempt loop in `Run.Wait`.
Every addition is an *added* form — `RunGated`/`AttachGated` beside `Run`/`Attach`, a `Gate` field on `Result`, a `gate` field on `*Run` — so every ungated caller in the repo keeps today's behaviour bit-for-bit and the tree still builds with no other package touched.

The external interface the next batches consume is exactly four exported names (`Gate`, `GateResult`, `GateSpec`, `GateOutcome`), two new `Runner` methods (`RunGated`, `AttachGated`), and one new `Result` field (`Gate *GateOutcome`).

Batch-local decision, differing from nothing in `## Shared Decisions`: the gate verdict is memoised **per attempt**, not per run.
`finalize` computes it only when no memo is stored, which is what makes "the gate runs exactly once per settling" true; `Wait` clears the memo immediately before each re-prompt so the next attempt re-validates.
A memo that were per-run would make every attempt after the first read the first attempt's stale verdict.

## Cards

### Card 1: Declare the gate contract in shuttleengine

- **Context:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/gate.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare, in the new file, four exported types plus two unexported constants and one unexported helper.
  `GateResult` is a struct with `Passed bool` and `Findings string`, the latter documented as the detailed what-is-wrong text, empty when `Passed`.
  `Gate` is `func() (GateResult, error)` — no argument, because the two production validators take materially different path shapes and a closure captures its own told paths.
  `GateSpec` is a struct with `Gate Gate` and `Attempts int`, whose zero value (a nil `Gate`) means "ungated" and is what every row but the four gated ones supplies; `Attempts` of 0 means the package default.
  `GateOutcome` is a struct with `Passed bool`, `Attempts int`, and `FindingsPath string`.
  `GateOutcome.Attempts` counts re-prompts actually sent on this run and nothing else: a gate that passed first try reports 0, a Done reached with no live session reports however many re-prompts had already been sent before the session was lost, and a deadline that expires after N sends reports N.
  `GateOutcome.FindingsPath`'s doc comment must state in full that it is diagnostic text and never a path to dereference after `Wait` returns, because the findings file lives in the run directory that `finalize` deletes on the Done cleanup every exhausted gate takes, and that a producer which opens it is a defect.
  Add `defaultGateAttempts = 3` and `gateFindingsFileName = "gate-findings.md"` as unexported constants, and a method `func (s GateSpec) attempts() int` returning `defaultGateAttempts` when `s.Attempts <= 0` and `s.Attempts` otherwise.
  Add `func gateRepromptText(findingsPath string) string` returning a single line with no newline, naming the absolute findings path and instructing the agent to read it, fix every finding, and end its turn — the one-line shape `validateSendText` in `internal/shuttleengine/run.go` requires.
  Add a file doc comment naming the Shuttle Provider-Seam Invariant obligation this file inherits: the gate loop asks "is a new event in" only through the existing `Engine.ParseEvents` seam and knows nothing about any provider's hook payloads.
- **Commit:** `feat(shuttleengine): declare the Gate/GateSpec/GateResult/GateOutcome contract`

### Card 2: Carry the gate on Result and on the run handle

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shuttleengine/rundir.go`
- **Edits:**
  - `internal/shuttleengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `Gate *GateOutcome` to the `Result` struct, documented as a 1:1 report of this run's gate with nil meaning "this run had no gate" — never "the gate passed".
  Add three unexported fields to the `Run` struct: `gate GateSpec` (the told spec, zero for an ungated run), `gateVerdict *GateOutcome` (the per-attempt memo, nil when the current attempt has not been evaluated yet), and `gateSent int` (the count of re-prompts successfully delivered on this run).
  Each field carries a doc comment; `gateVerdict`'s states that the memo is per attempt rather than per run, and that `Wait` clears it before each re-prompt so the next attempt re-validates.
  Nothing else in this file changes and no existing signature is widened: `Runner.Start` keeps building a `*Run` whose `gate` is the zero `GateSpec`, which is exactly today's behaviour.
- **Commit:** `feat(shuttleengine): carry the gate verdict on Result and the run handle`

### Card 3: Gated entry points on Runner, spawn and attach alike

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shuttleengine/spec.go`
- **Edits:**
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/attach.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (r *Runner) RunGated(spec Spec, gate GateSpec) (Result, error)` carrying today's `Runner.Run` body plus one line setting `run.gate = gate` between `Start` and `Wait`, and reduce `Runner.Run` to `return r.RunGated(spec, GateSpec{})`.
  Move the whole body of `Runner.Attach` into a new `func (r *Runner) AttachGated(spec Spec, gate GateSpec) (Result, bool, error)` and reduce `Runner.Attach` to `return r.AttachGated(spec, GateSpec{})`.
  Give `reconstructAndWait` a third parameter `gate GateSpec` and have it set the reconstructed handle's `gate` field alongside the `attached: true` it already sets; update all four of its call sites inside `AttachGated` to pass the gate through.
  Both new methods carry a doc comment stating that the added-method shape is deliberate rather than a widening of `Run`/`Attach`, because the shared seams these methods back are held by callers that have no gate and never will.
  This card changes no return statement's results in either file, so the Completion Signal tripwire's counts are unaffected by it — except for the enclosing-function key rename card 6 handles.
- **Commit:** `feat(shuttleengine): add RunGated and AttachGated beside Run and Attach`

### Card 4: The single verdict site in finalize

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir.go`
- **Edits:**
  - `internal/shuttleengine/wait.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an unexported method `func (run *Run) evaluateGate() (*GateOutcome, error)` that returns `run.gateVerdict` unchanged when it is non-nil, returns `nil, nil` when `run.gate.Gate` is nil, and otherwise calls the closure once, stores the resulting `*GateOutcome` in `run.gateVerdict`, and returns it.
  A non-nil error from the closure is returned verbatim and stores no memo: a gate that could not run has found no defect, it is an infrastructure fault.
  On a `GateResult` whose `Passed` is false the method writes `GateResult.Findings` to `filepath.Join(run.runDir, gateFindingsFileName)`, overwriting any previous attempt's file so the agent always reads the current complaint, and sets `FindingsPath` to that path; a write failure is a returned error, not a failed gate.
  Every evaluated verdict sets `Attempts` from `run.gateSent`.
  Change `finalize` so that, immediately on entry and only when `outcome == OutcomeDone`, it calls `run.evaluateGate()`, returns `(result, fmt.Errorf("shuttle: gate: %w", err))` on a non-nil error, and otherwise stamps the returned pointer onto `result.Gate` — all of it strictly before the `run.state.Outcome` write, before the fork-audit block, and before the cleanup, so no Done escapes ungated and an exhausted gate still takes the ordinary Done-path cleanup.
  `finalize`'s doc comment gains a paragraph stating that one verdict site reached by all four of `Wait`'s `finalize` calls is what makes "no Done escapes ungated" true by construction rather than by four correct edits, and that `classifyStartupWindow`/`classifyDeadlineExpiry` cannot host the gate because they return an `Outcome` rather than a `Result`.
  The gate error return in `finalize` is a genuine new negative-verdict return site; card 6 updates the audited set for it.
- **Commit:** `feat(shuttleengine): run and stamp the gate verdict in finalize`

### Card 5: The re-prompt attempt loop in Wait

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shuttleengine/run.go`
- **Edits:**
  - `internal/shuttleengine/wait.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change only the events-tick Done branch of `Wait` — today the bare `return run.finalize(outcome, message)` reached when `outcome != ""` and the `AwaitOperator` ask branch did not fire.
  That branch becomes: when `outcome` is not `OutcomeDone`, or `run.gate.Gate` is nil, return `run.finalize(outcome, message)` exactly as today.
  Otherwise call `run.evaluateGate()`; on a non-nil error return `run.identity()` together with a `fmt.Errorf` wrapping it, without attempting any `Send`.
  On a verdict whose `Passed` is true, or one whose `Passed` is false with `run.gateSent >= run.gate.attempts()`, return `run.finalize(outcome, message)` — which reads the memo rather than re-validating.
  Otherwise the gate failed with budget remaining: call `run.Send(gateRepromptText(verdict.FindingsPath))`; on a non-nil error log a `logger.Warn` naming the producer-facing identities and the error and then return `run.finalize(outcome, message)`, ending the loop with the attempts spent so far and never retrying the send; on success increment `run.gateSent`, log a `logger.Warn` carrying the strand guid, the attempt number, the budget, and the formatted findings text, set `run.gateVerdict` to nil so the next attempt re-validates, and continue the poll loop without returning.
  `Run.Interrupt` must not be used anywhere in this loop: the gate fires at a turn boundary, when there is no in-progress turn to interrupt.
  No new deadline is introduced and `run.deadline` is never extended — the loop runs under the deadline `Start` already set from `spec.Timeout`, so a timeout mid-loop still reaches `classifyDeadlineExpiry`, which classifies `OutcomeDone` when the files are present and therefore still runs the gate one final time through `finalize`.
  The other three `finalize` call sites in `Wait` are left untouched: each is reached precisely because the session is gone, the clock ran out, or the bookkeeping broke, so no `Send` is attempted there and `finalize`'s own evaluation reports `Attempts` as it stands.
  `Wait`'s file doc comment gains a paragraph stating this split.
  The gate error return is a genuine new negative-verdict return site; card 6 updates the audited set for it.
- **Commit:** `feat(shuttleengine): hold a gated run's handoff with a bounded re-prompt loop`

### Card 6: Re-audit the Completion Signal tripwire

- **Context:**
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/attach.go`
  - `internal/shuttleengine/gate.go`
- **Edits:**
  - `internal/shuttleengine/completionsignal_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The two AST scans key on the enclosing function name, so this batch moves three counts and must be re-audited deliberately rather than silenced.
  Rename the `auditedNegativeVerdictReturns` key `"Attach [Errorf]"` to `"AttachGated [Errorf]"`, keeping its value at 5, because card 3 moved that body wholesale into the new method and added no return of its own.
  Raise `"Wait [Errorf]"` from 4 to 5 and `"finalize [Errorf]"` from 1 to 2, each for the gate-evaluation error return cards 5 and 4 add.
  Extend the var block's own justification comment with one line per changed entry, in the style every existing entry already carries: the `Wait` line states that the gate error is an infrastructure fault that never burns an attempt and never reaches the LLM, so it needs no file-contract consultation — the run reached a positive Done and the gate ran strictly after it; the `finalize` line states the same for the three non-events-tick paths; the `AttachGated` line states that it is a rename of an unchanged set rather than a new site.
  Leave `auditedFileContractCallSites` unchanged: the gate adds no `allOutputFilesExist` call and removes none.
- **Commit:** `test(shuttleengine): re-audit the completion-signal tripwire for the gate returns`

### Card 7: Gate-loop unit tests

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/attach.go`
  - `internal/shuttleengine/fakes_test.go`
  - `internal/shuttleengine/wait_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttleengine/gate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Drive the loop through the package's existing `fakeReed`/`fakeEngine` fakes, `newWaitTestRunner`, and the `newFakeClock`/`multiStepClock` seams, with a scripted events file — untagged, offline, no `exec.Command`, no `gitexec`, no `hubforge.NewHub`, no `time.Sleep` of a second or more, per the Test Tier Purity Invariant.
  Cover, one test each: a gate that passes on the first Done, asserting `Result.Gate.Passed` true, `Attempts` 0, zero `SendText` calls recorded on the fake reed, and a cleanup identical to an ungated run's; a gate that fails once and passes on the agent's next turn, asserting exactly one send whose text contains no newline and names the findings file, that the findings file holds the first `GateResult.Findings`, that `Attempts` is 1, and that the final outcome is Done with `Passed` true; a gate that fails every attempt, asserting exactly `attempts()` sends, `Passed` false, `Attempts` equal to the budget, `Outcome` still `OutcomeDone`, and `FindingsPath` naming the last attempt's file; a gate returning a non-nil error, asserting `Wait` returns an error wrapping the gate's own, no send is issued, and no attempt is charged.
  Cover the no-live-session Done paths with one case each, since each reaches a different one of `Wait`'s other three `finalize` calls: `checkLivenessTick`'s not-tracked branch, its not-live branch, `classifyStartupWindow`'s startup-window expiry, the run deadline, and `finishedDespiteMechanismFailure` — each asserting `Passed` false, no send, and `Attempts` equal to the re-prompts already sent, which is 0 in the common case.
  Cover that the gate runs exactly once per settling: a `finalize` reached after the memo is already stored reads it rather than re-invoking the closure, asserted by counting closure invocations.
  Cover a `Send` that fails mid-loop, asserting the loop ends with the attempts spent so far, issues no retry, and does not panic.
  Cover a deadline expiring between attempts, asserting the loop stops, the gate runs once more on the deadline-expiry Done, and the reported `Attempts` is honest about the sends that preceded it.
  Cover the zero `GateSpec` as the regression guard for every ungated row: `Result.Gate` is nil and the observable behaviour is byte-for-byte today's.
  Cover `AttachGated` threading its spec through `reconstructAndWait` so a resumed run is gated exactly as a fresh one is, and `Attach` leaving `Result.Gate` nil.
- **Commit:** `test(shuttleengine): cover the gate attempt loop end to end`

### Card 8: Pin the Agent-tool-denied precondition the narrowing rests on

- **Context:**
  - `internal/shuttleengine/template.yaml`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/template.go`
- **Edits:**
  - `internal/shuttleengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The per-attempt done-signal is the next turn boundary and nothing more, and that narrowing holds only because the in-process `Agent` tool is denied at every gated site.
  Add a test asserting that the shipped embedded template parses with `Config.ClaudeDenyAgentTool` true, so a future edit that flips the default fails here with a message naming the narrowing rather than silently making a gate able to fire while an async in-process subagent is still working.
  The test's failure message must say that flipping this default re-opens the compound-quiescence question this task deliberately declined and that the decision must be re-opened rather than the test updated.
  The paired half of the precondition — that none of the four gated sites sets `Spec.ForkSubagents` — cannot be seen from this package and is pinned in batch 5.
- **Commit:** `test(shuttleengine): pin claude_deny_agent_tool true in the shipped template`

## Batch Tests

`verify: go test ./internal/shuttleengine/...` runs the whole package, which is the right scope here: every card in the batch edits a file in it, and the batch's two most important guards — `completionsignal_enforcement_test.go`'s AST scans and `wait_test.go`'s thirty-odd existing outcome-classification tests — are the regression surface that must stay green while `Wait` and `finalize` are restructured around them.
The new `gate_test.go` carries the batch's own coverage; `run_test.go`, `attach_test.go`, and `spec_test.go` are the untouched-behaviour guards that prove the added-method shape left every ungated path alone.
No tagged test is edited by this batch, so the untagged run is complete for it.
