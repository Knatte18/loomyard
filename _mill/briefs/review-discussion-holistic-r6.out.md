MILL_REVIEW_BEGIN
# Review: reed: pane reap isn't applied consistently across up/add's mutating paths

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:decision] Scrubbed-reed.json remedy contract has no disposition
**Section:** Scope → In / zero-strand-sessions-end-up-header-only
**Issue:** `internal/reedengine/state.go:152-156` justifies the "delete reed.json by hand to keep the session" remedy precisely by *"planReconcile's untracked reap does not fire (it needs a bound present pane) ... so the panes and their processes keep running untracked and can be attached to"*, and the live operator-facing string it returns (`state.go:165`) promises the same — after this fix that promise holds only until the next `up`, which is exactly M22's decided behaviour; neither `state.go` nor the string is in scope or given a disposition.
**Fix:** Decide and state explicitly whether the `unreadableStateError` comment and its error string are rewritten (and to what trade), and add `state.go` to the In list if so.

### [BLOCKING:scope] Doc-surface sweep keyed on "adopt" only, Go files only
**Section:** Scope → In ("Adoption prose everywhere it appears")
**Issue:** The prescribed enumeration is `grep -rn "adopt" internal/reedengine/*.go internal/reedcli/*.go`, but this task falsifies a *second* premise — "the untracked reap needs a bound present pane" — whose prose contains no "adopt" (e.g. `state.go:154`), and the file set excludes markdown, missing `tools/sandbox/SANDBOX-REED-SUITE.md:249` (M13's *"adopted a dead leftover pane"* FAIL diagnosis), even though the sandbox doc is otherwise acknowledged as adoption-premised surface (only M16/M22 are in scope).
**Fix:** State the sweep as two greps — the adoption term and the reap-gate premise — over an explicit file set that includes `tools/sandbox/*.md`, with the same rewrite/leave-alone disposition rule.

### [NIT:design] Logger-capture unit test as specified captures nothing
**Section:** Testing → "Unit — the reap log line"
**Issue:** The plan is `logger.SetOutput(&buf)` plus asserting an `Info` line, but `internal/logger`'s stderr half defaults to the Warn threshold (`configureFromEnv`, `SetVerbosity`) and its durable half is disabled under `testing.Testing()` (`sink.go:79`), so an `Info` record reaches neither sink without `SetVerbosity(1)`.
**Fix:** Name `SetVerbosity(1)` (with restore in the same `t.Cleanup`) as part of the capture helper.

## Verdict

REQUEST_CHANGES
Two premises the fix falsifies are under-enumerated; one live operator-facing remedy string is undisposed.
MILL_REVIEW_END
