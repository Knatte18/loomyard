MILL_REVIEW_BEGIN
# Review: Shed engine adapters: SingleLLMProducer, perch, Webster

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-16
```

## Findings

### [BLOCKING:design] `StuckReason` has no channel on the seam
**Section:** Perch adapter / Webster adapter mappings, Testing
**Issue:** Both decisions say `Stuck` surfaces `StuckReason` "in the returned detail", but `ShedProducer.Call` returns only `(Outcome, OutputPointer, error)` (`internal/shedengine/producer.go:30-32`); the perch decision pins `OutputPointer` empty and `Stuck` carries a nil error, so no detail channel exists — the testing section admits this with "whatever detail channel the adapter uses".
**Fix:** Decide explicitly where `StuckReason` goes (logger only, or `OutputPointer.Path`, or dropped) and reconcile it with the empty-pointer decision.

### [BLOCKING:design] Perch pause bridge is not installable through the chosen seam
**Section:** Context cancellation; Seam interfaces; Technical context ("Told, never derived")
**Issue:** `PauseRequested` is a construction-time field of `perchengine.Options`, consumed by `perchengine.New` (`engine.go:41-67`); it cannot be set through the pinned `Run(Profile, string, string, string) (Result, error)` seam over an already-constructed engine, yet the testing section asserts "the fake engine observes a bridge".
**Fix:** State whether `PerchProducer` constructs the perch engine itself (widening what it is told), takes the bridge-injection point as its seam, or drops the perch bridge.

### [BLOCKING:design] Webster bridge mechanism undecided and seam-incompatible
**Section:** Context cancellation; Gotchas
**Issue:** The decision claims a `func() bool` is wired into "Webster's scratch-dir pause flag", but webster exposes no callback — pause is a file `beginbatch.go:129` stats, so the bridge requires a ctx-watching goroutine writing the flag; the Gotchas then reopen the whole question ("An alternative worth weighing at plan time: pass no bridge for Webster"), leaving a Decision and a Gotcha in contradiction.
**Fix:** Decide bridge-or-not for Webster in the Decisions section, and if yes, name the writer mechanism and its interaction with `Run`'s own `ClearPause` calls (`runlevel.go:436,629`).

### [BLOCKING:design] `SingleLLMProducer` mid-run cancellation unaddressed
**Section:** Context cancellation
**Issue:** `shuttleengine` has no pause seam at all (`Runner.Run(Spec)`, `run.go:174`), so the primary adapter gets entry/exit checks only — exactly the alternative the same decision rejects as "invisible until the producer finished on its own, potentially hours"; the discussion never states this consequence.
**Fix:** State the accepted consequence for shuttle explicitly (bounded by `Spec.Timeout`, or via the `Start`/`Run.Interrupt` handle) rather than leaving the rejection rationale self-contradicting.

### [BLOCKING:decision] `OutputPointer` selection left to plan
**Section:** Testing (`SingleLLMProducer`)
**Issue:** Which of a multi-entry `OutputFiles` becomes the pointer is explicitly deferred to mill-plan, yet it determines a value Shed persists into `history[].output` and is the one mapping decision not made here.
**Fix:** Decide it in the Decisions section (first entry, or a `Spec`-source-named primary), with rationale.

### [NIT:consistency] Absolute-`OutputFiles` requirement stated only as a gotcha
**Section:** Stale output files / Gotchas
**Issue:** The archive decision does not state the precondition; the gotcha leans toward "requiring absolute entries is simpler" without deciding, and does not say what the adapter does on a relative entry.
**Fix:** Promote it to the decision: require absolute entries and reject a relative one with an error.

### [NIT:consistency] `ErrNilBatcher` missing from the Webster decision's mapping list
**Section:** Webster adapter decision vs Testing
**Issue:** The testing error table includes `ErrNilBatcher`, the decision's error list does not.
**Fix:** Add it to the decision's error→non-nil-error list.

## Verdict

REQUEST_CHANGES
Cancellation-bridge mechanics and the `StuckReason` channel are undecided or seam-incompatible.
MILL_REVIEW_END
