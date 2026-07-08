MILL_REVIEW_BEGIN
# Review: Build perch - the review gate loop

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-08
```

## Findings

### [NOTE] Double-non-done error's run-dir path lacks a seam
**Section:** Non-done outcomes / Result contract
**Issue:** The error is specified to carry "the kept shuttle run-dir path", but the Burler seam's return (`burlerengine.Result`, engine.go:50) exposes only Outcome/Verdict/Findings/paths/SessionID/StrandGUID/LastAssistantMessage — no shuttle `RunDir` (burler drops the `RunDir` that `shuttleengine.Result` does carry).
**Fix:** State the mechanism — perch resolves it via `shuttleengine.FindRun(cfg, layout, StrandGUID)` (rundir.go:148), or extend the burler seam to surface RunDir.

### [NOTE] Profile-hash mismatch on explicit --run-id resume unhandled
**Section:** Run identity derived / Run dir layout
**Issue:** `state.json` records the profile hash, but resume behavior is only defined for unfinished/empty/terminal state; an edited profile pointed at an existing run dir via `--run-id` (stored hash ≠ incoming) is not addressed, risking continuation under a different profile than the recorded rounds.
**Fix:** Specify that resume validates the stored profile hash against the incoming profile and fails loud on mismatch.

### [NOTE] Gate command mode vs verdict-based judge triggers
**Section:** Pluggable gate / Verdict-judge model
**Issue:** Judge and milestone gates trigger on a "BLOCKING round" (burler verdict), but in `command`/`both` mode a round can be APPROVED-verdict yet non-converged (command failed); such rounds silently skip both the per-round circling check and a rung's "mandatory" continuation gate.
**Fix:** Confirm/state that judge triggering is burler-verdict-based (not convergence-based) even in command modes, and that this skip is intended (hard cap still bounds it).

### [NOTE] configreg module-list pinned test not in the update list
**Section:** Scope (Docs) / Technical context (Config module)
**Issue:** Registering perch in `configreg.Modules()` breaks the pinned `Names()` list in `internal/configreg/configreg_test.go` (`{"board","mux","shuttle","warp","weft"}`), but the enumerated pinned-set updates name only helptree/registration/longlist/sandbox/drift.
**Fix:** Add `configreg_test.go`'s Names list to the same-commit pinned-set updates.

## Verdict

APPROVE
Thorough and decision-complete; only minor wiring/edge-case clarifications remain, none blocking plan writing.
MILL_REVIEW_END
