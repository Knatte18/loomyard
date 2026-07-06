I have enough to assess the discussion against source. The discussion is thorough, decisive, and its claims about current code (Spec, buildLaunchCmd `--model` handling, buildSettings mutual-exclusion, pollEventsTick last-event classification, the deny hooks echoing to stdout) all match the source I read. My findings are NOTE-level.

MILL_REVIEW_BEGIN
# Review: Add Effort to shuttle's run Spec

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-06
```

## Findings

### [NOTE] Marker-hook mechanism not live-verified
**Section:** Decision ask-signal-mechanism / Technical context
**Issue:** The discussion live-verifies effort values, deny behavior, and payload shapes, but the one novel mechanism — a *non-denying* `PreToolUse(AskUserQuestion)` hook that `cat >>`-appends and still lets the tool proceed, producing a real-time line before the tool blocks — is asserted by analogy to the `Stop` hook, not confirmed live.
**Fix:** Note a live smoke-test of the interactive marker (line written and tool proceeds) as an implementation checkpoint, matching the discussion's own verification bar.

### [NOTE] Test fake must synthesize both Event kinds
**Section:** Technical context / Testing
**Issue:** The file-change list and Testing section omit that `internal/shuttleengine/fakes_test.go`'s `fakeEngine.ParseEvents` (which today only synthesizes turn-end events into `StopEvent`) must be extended to emit `EventAsk`, plus the rename touches `wait_test.go`/`events_test.go`.
**Fix:** Call out extending the neutral test fake's event-synthesis format to cover both `Kind`s so the wait_test real-time-asking case has a source.

### [NOTE] Interactive runs now terminate Wait at first question
**Section:** Decision ask-signal-outcome
**Issue:** With the marker, every interactive run returns `OutcomeAsking` at the first `AskUserQuestion` rather than continuing — previously such an ask was invisible until timeout; "ask, get answered live, continue within one Wait" no longer happens in a single Wait call.
**Fix:** State explicitly that this behavior change is intended for all interactive runs (re-Wait/resume is orchestration-layer, out of scope), so a plan writer does not treat it as a regression.

## Verdict

APPROVE
Scope, decisions, and constraints are complete and grounded; only NOTE-level polish remains.
MILL_REVIEW_END