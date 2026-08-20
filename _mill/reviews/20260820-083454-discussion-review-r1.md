MILL_REVIEW_BEGIN
# Review: shedengine: per-producer bounce budget + explicit OnDone routing

```yaml
duration_s: 191.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build unverifiable from inside the session
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Done cycles are now unbounded at runtime, not just unvalidated
**Section:** §Decisions — "The three new `validate()` rules" / §Scope Out
**Issue:** Today `Done` always advances forward, so `run.go`'s `for {}` terminates by list length; with `OnDone` allowing backward jumps and `Done` consuming no budget, a two-producer `Done` cycle is an infinite loop with no cap of any kind, and the discussion's rationale ("any member may exit via `OnStuck`") is only true if a member ever returns `Stuck`.
**Fix:** State a disposition for the runtime case — either an explicit total-iteration / per-producer `Done` cap in `Run`, or an explicit "accepted, human-owned, unbounded" decision with rationale — rather than deferring solely to static-detection rejection.

### [BLOCKING:design] Blocking `Stuck` entry counts against the all-time budget
**Section:** §Decisions — "The count is all-time" / "Per-producer bounce budget"
**Issue:** `run.go:188` appends the `Stuck` history entry before the block branch, so after an exhaustion block producer X's history holds `budget+1` entries; the documented escape hatch "raise `MaxBounces` for that producer" then re-blocks immediately unless raised by at least two, and each resume inflates the count further.
**Fix:** Decide and state explicitly whether the block-time entry counts, and restate the escape hatch accordingly (e.g. "raise above the current entry count", or exclude the entry written on the block path).

### [NIT:consistency] Scope's test list contradicts the Testing section
**Demoted-from:** BLOCKING
**Section:** §Scope In (Tests) vs §Testing
**Issue:** Scope names four test files, while §Testing requires re-wiring `run_pause_test.go` and `run_persist_test.go` with explicit `OnDone` chains; `internal/loomshed/fixture_test.go:118` and `internal/loomshed/sequence_test.go`'s full-run scenario are also touched or exercised by the change but appear in neither list.
**Fix:** Make the Scope test inventory match §Testing, naming every test file the `OnDone` rewrite touches (or state that Scope lists new/substantially-rewritten files only, with mechanical re-wiring elsewhere expected).

### [NIT:design] `validate()`'s existing name set cannot express the `Segment` rule
**Section:** §Technical context — `validate.go`
**Issue:** The discussion calls the second loop "the natural home" for the `Segment` equality rule, but `seen` is a `map[string]bool` carrying no `Segment`, so the rule needs a name→`Segment` map or a `findProducer`-style lookup.
**Fix:** Note the extra lookup structure so a plan writer does not assume the existing `seen` map suffices.

## Verdict

REQUEST_CHANGES
Unbounded Done cycles, budget-count inflation after a block, and a contradictory test inventory.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
