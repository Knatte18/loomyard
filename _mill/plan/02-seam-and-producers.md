# Batch: seam-and-producers

```yaml
task: 'Producer gates: mechanical gates before session release'
batch: 'seam-and-producers'
number: 2
cards: 8
verify: go test ./internal/burlerengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/loomrecipe/...
depends-on: [1]
```

## Batch Scope

This batch carries the `GateSpec` from the two producers down to the gated shuttle calls, and maps a failed gate onto `shedengine`'s outcome contract at both producers.
`burlerengine.Shuttle` gains `RunGated` and `shedadapters.Shuttle` gains `RunGated`/`AttachGated`, both as added methods beside the existing ones, so `Bouncer` — which has no gate and never will — keeps every one of its call sites unchanged.
Go admits no partial interface implementation, so every test fake implementing either seam gains the new methods in this batch too; that churn is the intended cost of the added-method shape and matches this seam's own precedent, where `Attach` was added to the shared seam rather than type-asserted as an optional interface.

Nothing is wired to a real validator yet: every production caller still passes the zero `GateSpec`, so the tree builds and every existing test stays green.
The external interface batch 4 consumes is `burlerengine.RunOpts.Gate` and `shedadapters.NewSingleLLMProducerGated`.

Batch-local decision: `shedadapters.NewSingleLLMProducer` is **not** widened.
A second constructor, `NewSingleLLMProducerGated`, takes the gate and the original delegates to it with the zero `GateSpec`.
This is the same added-form-not-widened reasoning the two seams themselves take, and it leaves the generic `SingleLLM` registry row, the smoke harness, and roughly thirty existing test call sites untouched.

## Cards

### Card 9: burlerengine carries a gate on RunOpts and reports one on Result

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shuttleengine/run.go`
  - `internal/burlerengine/doc.go`
- **Edits:**
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/engine.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `Gate shuttleengine.GateSpec` to the `RunOpts` struct, documented as the per-invocation, non-rendered knob it is — the zero value is ungated, which is what the Webster segment's round passes, always.
  Its doc comment states why it sits on `RunOpts` rather than on `Profile`: `Profile` is validated, path-resolved data the round renders into prompts, so a func field there would have to be excluded from `validate` and from every profile test's comparison, while `RunOpts` already carries `Model`, `Effort`, `Timeout`, `Round`, and `NoteID`, which is exactly what a gate is.
  Add `RunGated(shuttleengine.Spec, shuttleengine.GateSpec) (shuttleengine.Result, error)` to the `Shuttle` interface, beside the existing `Run`; the compile-time assertion that `*shuttleengine.Runner` satisfies it keeps holding because batch 1 gave the runner that method.
  Add `Gate *shuttleengine.GateOutcome` to the `Result` struct, documented as a 1:1 passthrough of the shuttle `Result`'s own field exactly as `RunDir` and `ForkAudit` already are, with nil meaning the round ran ungated.
- **Commit:** `feat(burlerengine): carry a gate on RunOpts and report its outcome on Result`

### Card 10: A burler round's gate repairs its own report before it is parsed

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/verdict.go`
- **Edits:**
  - `internal/burlerengine/engine.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `Engine.Run`'s `e.shuttle.Run(spec)` call with `e.shuttle.RunGated(spec, gateSpec)`, where `gateSpec` is `opts.Gate` when `opts.Gate.Gate` is nil and otherwise a copy whose closure is wrapped in a per-round closure.
  The wrapper calls the told closure; on a non-nil error or a passing result it returns that verbatim; on a failing result it appends to `GateResult.Findings` a blank line and an instruction naming this round's own `p.ReviewPath` and `p.FixerReportPath` absolute paths and requiring both to be rewritten to reflect the repair the agent is about to make.
  The instruction rides the findings *file* and never the `Send` line, which must stay a single line, and it is composed here rather than in the closure the caller built because only `Engine.Run` knows the round's two paths.
  It exists because a burler round writes both of those files *before* its gate runs, so a gate that fails, re-prompts, and then passes would otherwise leave `Run` parsing a verdict written against the pre-repair artifact — a report claiming a fix over a state that has since changed, which is exactly what the segment's judge then consumes.
  Copy `shuttleResult.Gate` onto `result.Gate` in the same assignment block that already copies `RunDir`, and add a branch immediately after the existing `result.Outcome != shuttleengine.OutcomeDone` early return: when `result.Gate` is non-nil and its `Passed` is false, return the populated `result` with `Verdict` and `Findings` left empty and a nil error, before the cluster-audit block and before the review file is read at all.
  A failed gate is not an error and not a synthesised `Outcome` — the round genuinely classified `OutcomeDone` and the gate is a separate fact about it — so `Engine.Run`'s existing error contract is unchanged.
  Extend `Engine.Run`'s doc comment with the new step and with the reason a round whose fix left the artifact invalid has no verdict worth parsing.
- **Commit:** `feat(burlerengine): gate a round before its review file is parsed`

### Card 11: burlerengine gate tests

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/profile.go`
- **Edits:**
  - `internal/burlerengine/engine_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the package's existing `fakeShuttle` with `RunGated`, which records the received `GateSpec`, delegates to its own `Run` body, and then — when the received spec's `Gate` is non-nil and the delegated outcome is `shuttleengine.OutcomeDone` — invokes the closure exactly once, returns its error if non-nil, and otherwise stamps a `*shuttleengine.GateOutcome` onto the returned `Result`.
  The fake runs no re-prompt loop; there is no pane to send into, and the loop's own coverage lives against the real `Wait` in batch 1's tests.
  Add tests: a round whose gate fails returns a `Result` with `Gate` populated and `Passed` false, `Verdict` and `Findings` empty, `Outcome` still `OutcomeDone`, and a nil error, with the review file never read — asserted by leaving no parseable review file on disk and still expecting no error; a round carrying the zero `GateSpec` behaves exactly as today with `Result.Gate` nil, which is the guard for the Webster segment's ungated round; a round whose gate fails receives findings text that names both this round's own review path and its own fixer-report path, which is the per-round wrapper's whole subject.
- **Commit:** `test(burlerengine): cover the round gate and its report-repair instruction`

### Card 12: The shedadapters Shuttle seam gains its gated methods

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shuttleengine/run.go`
  - `internal/shedadapters/doc.go`
  - `internal/shedadapters/bouncer.go`
- **Edits:**
  - `internal/shedadapters/singlellm.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `RunGated(shuttleengine.Spec, shuttleengine.GateSpec) (shuttleengine.Result, error)` and `AttachGated(shuttleengine.Spec, shuttleengine.GateSpec) (shuttleengine.Result, bool, error)` to the `Shuttle` interface, beside the existing `Run` and `Attach`.
  Extend the interface's doc comment with the reason the methods are added rather than the existing pair widened: this seam is shared by three consumers — `SingleLLMProducer`, `BurlerProducer`'s attach probe, and `Bouncer` — and `Bouncer` has no gate and never will, so widening `Run`/`Attach` would rewrite its six call sites to pass a zero value that means nothing there.
  Record in the same comment that every test fake implementing this seam gains both methods because Go admits no partial implementation, and that this is the intended cost, matching the precedent already stated here for `Attach`: a compile error in a test fake is a better failure than a producer that quietly stops probing.
  The compile-time assertion that `*shuttleengine.Runner` satisfies the seam keeps holding because batch 1 gave the runner both methods.
- **Commit:** `feat(shedadapters): add RunGated and AttachGated to the shared Shuttle seam`

### Card 13: SingleLLMProducer runs gated and maps a failed gate onto Stuck

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `internal/shedadapters/singlellm.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `gate shuttleengine.GateSpec` field to the `SingleLLMProducer` struct and a constructor `NewSingleLLMProducerGated` taking the five parameters `NewSingleLLMProducer` already takes plus a trailing `gate shuttleengine.GateSpec`; reduce `NewSingleLLMProducer` to a delegation passing the zero `GateSpec`, so every existing call site compiles and behaves unchanged.
  Change `Call`'s two shuttle calls to the gated forms — `p.shuttle.AttachGated(spec, p.gate)` for the probe and `p.shuttle.RunGated(spec, p.gate)` for the spawn — so a resumed run is gated exactly as a fresh one is; nothing else about the probe-before-archive ordering changes.
  In `mapOutcome`, split the `shuttleengine.OutcomeDone` case after its existing empty-`OutputFiles` guard: when `result.Gate` is nil or its `Passed` is true, return today's `shedengine.Done` with `OutputPointer{Path: spec.OutputFiles[0]}`, unchanged; when `result.Gate` is non-nil with `Passed` false, consult `cancelErr` as every other non-success exit does, log a `logger.Warn` carrying the producer name, the engine label, the attempts spent, the findings path, the session id, and the strand guid, and return `shedengine.Stuck` with `OutputPointer{Path: spec.OutputFiles[0]}` — the **artifact** pointer, not an empty one.
  The pointer is non-empty deliberately and the field's meaning is what carries it: a writer row has no judge downstream of its `Stuck` because the run halts there, so its pointer is free to carry the meaning the commit decorators key on, which is what keeps a gate-failed artifact committed and diagnosable rather than sitting in a dirty weft.
  `OutcomeAsking` keeps returning `Stuck` with an empty pointer and is therefore still not committed, correctly: an asking run never satisfied its file contract, so there is nothing to commit.
  Record in `mapOutcome`'s doc comment that the empty-pointer-on-gate-failure shape belongs to `BurlerProducer` alone, where emptiness tells the segment's `Bouncer` there is no round artifact to judge.
- **Commit:** `feat(shedadapters): gate SingleLLMProducer and map an exhausted gate onto Stuck`

### Card 14: BurlerProducer maps a failed gate onto a Bouncer hand-back, spawn and resume alike

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/profile.go`
  - `internal/shedadapters/archive.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `internal/shedadapters/burler.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Call`'s attempt loop, split the `shuttleengine.OutcomeDone` case: when `result.Gate` is nil or its `Passed` is true, keep today's behaviour byte-for-byte; when `result.Gate` is non-nil with `Passed` false, call the local `archiveRound` helper every other non-success exit already uses, log a `logger.Warn` carrying the producer name, the engine label, the round, the attempts spent, and the findings path, consult `cancelErr` as the neighbouring exits do, and return `shedengine.Stuck` with an **empty** `OutputPointer`.
  The empty pointer is the signal, and it is the same one the deleted validate producers used for exactly this meaning: a `Stuck` with a pointer names an artifact for the `Bouncer` to judge, a `Stuck` with an empty pointer says there is none.
  Archiving is what keeps the hand-back honest: `Call`'s own resume logic treats the highest complete round with no recorded verdict as "hand back for judgment, spawn nothing", so leaving a gate-failed round's review file in place would offer the `Bouncer` a review written over an artifact a Go validator already proved invalid.
  A failed gate must not consume or trigger the attempt-1/attempt-2 retry: that retry exists for `OutcomeDied`/`OutcomeTimeout`, which are infrastructure, while gate exhaustion is a determinate verdict the gate already re-prompted its whole budget over inside the session, and a second full round on the same input would re-spend an LLM generation to reach the same answer.
  In `probeLiveRound`, change `p.attach.Attach(spec)` to `p.attach.AttachGated(spec, p.opts.Gate)` and split its own `shuttleengine.OutcomeDone` branch identically to the spawn path's, so an attached Discussion or Plan fix round is gated exactly as a freshly-spawned one is.
  This is what makes "one `GateSpec` at every hop" true rather than aspirational: the same `RunOpts` field is read at the spawn hop and at the resume hop, and no second carrier is introduced into `NewBurlerProducer`.
  Extend `Call`'s archive-rule doc paragraph with the gate-failed exit, and `probeLiveRound`'s doc comment with the gated attach.
- **Commit:** `feat(shedadapters): fail a gated burler round back to its Bouncer, spawn and resume alike`

### Card 15: shedadapters fakes and producer gate tests

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/burler.go`
  - `internal/shedadapters/attachprobe_test.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `internal/shedadapters/singlellm_test.go`
  - `internal/shedadapters/burler_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the package's `fakeShuttle` with `RunGated` and `AttachGated` following the shared fake contract: record the received `GateSpec`, delegate to the existing `Run`/`Attach` body, and when the received `Gate` is non-nil and the delegated outcome is `shuttleengine.OutcomeDone`, invoke the closure exactly once, return its error if non-nil, and otherwise stamp a `*shuttleengine.GateOutcome` onto the returned `Result`.
  `fakeShuttleWithAttachHook` embeds `fakeShuttle`, so it needs an `AttachGated` override only where its `duringAttach` hook must still fire — keep its existing override shape.
  Add `SingleLLMProducer` tests asserting: a passing gate reaches `shedengine.Done` with the artifact pointer, unchanged from today; a failed gate reaches `shedengine.Stuck` with the pointer explicitly equal to `spec.OutputFiles[0]` and **not** the zero `OutputPointer`, since a test asserting an empty pointer here would silently disable commit-on-gate-failure; `OutcomeAsking` keeps its empty pointer; the ungated constructor leaves every existing assertion untouched; and the attach path is gated too, asserted by the recorded `GateSpec` on the fake's `AttachGated`.
  Add `BurlerProducer` tests asserting: a gate-failed round maps onto `Stuck` with an empty pointer **after** both round paths are archived; the gate-failed exit does not consume the attempt-1/attempt-2 retry, asserted by the runner fake's invocation count; the gate-passed path is byte-for-byte as today; and `probeLiveRound` passes `p.opts.Gate` into the gated attach and maps an attached round's failed gate identically — the regression guard for the resume hole, which must fail if the probe ever reverts to the ungated `Attach`.
  The existing attach-before-archive ordering tests must still pass unchanged.
- **Commit:** `test(shedadapters): cover both producers' gate mapping and the gated resume hop`

### Card 16: Teach the four downstream fakes the gated seam

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/singlellm_test.go`
- **Edits:**
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedrecipe/entries_bouncer_test.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/loomrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give each of the four `shedadapters.Shuttle` fakes — `fakeShuttle` in the `shedrecipe` fixture, `judgeSeamFakeShuttle` in the bouncer entry tests, `fakeShuttle` in the `shedbuild` fixture, and `fakeLoomShuttle` in the `loomrecipe` fixture — a `RunGated` and an `AttachGated` following the same shared fake contract card 15 applies: delegate to the existing `Run`/`Attach` body, then invoke a non-nil gate closure exactly once on an `OutcomeDone` result, returning its error if non-nil and otherwise stamping the `*shuttleengine.GateOutcome`.
  `fakeLoomShuttle`'s pair matters most and its doc comment must say so: it is the only fake a full recipe sequence drives, so it is what makes a gate-failed writer row genuinely halt a `loomrecipe` run rather than the wiring going silently untested there from batch 4 onward.
  No production file changes in this card and every existing assertion in the four packages must still hold, since every production caller still supplies the zero `GateSpec` until batch 4.
- **Commit:** `test: teach the downstream shuttle fakes the gated seam`

## Batch Tests

`verify: go test ./internal/burlerengine/... ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/loomrecipe/...` covers exactly the five packages this batch edits.
The scope is wider than the two packages carrying the real work because the added interface methods propagate at compile time into three fake-holding packages that this batch must leave green, and a narrower command would not compile-check them.
The load-bearing new coverage is in `internal/shedadapters/singlellm_test.go` and `internal/shedadapters/burler_test.go` (both producers' gate mapping, including the deliberately asymmetric pointers) and `internal/burlerengine/engine_test.go` (the round gate and its report-repair instruction).
The load-bearing regression surface is every existing test in those five packages: each production caller still passes the zero `GateSpec`, so any behaviour change at all in them is a defect this command catches.
`internal/shuttleengine` is not re-run here — batch 1's verify owns it and this batch does not edit it.
