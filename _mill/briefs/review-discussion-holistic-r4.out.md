MILL_REVIEW_BEGIN
# Review: reed: pane reap isn't applied consistently across up/add's mutating paths

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:scope] Sandbox suite spec asserts the reaped behaviour
**Section:** Scope / no-new-CONSTRAINTS-entry / affected-test enumeration
**Issue:** `tools/sandbox/SANDBOX-REED-SUITE.md` (~lines 281-286) states the zero-strand foreign-pane step's follow-up `up` "must **not** destroy the session's pane set" and that "after that add the foreign pane is **deterministically reaped**" — after this change the reap fires on that `up`, one verb earlier, and the step's headless twin (`TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable`, named on line 286) is being rewritten for exactly that reason; the discussion's doc inventory names only `doc.go` and gives this file no disposition.
**Fix:** State a disposition for `tools/sandbox/SANDBOX-REED-SUITE.md` (rewrite the step's watch/verdict wording, or explicitly out with a reason), since the M16/M22 findings this task fixes were graded against it.

### [NIT:consistency] Adoption vocabulary survives in a live error string
**Section:** Scope (adoption-describing comments elsewhere)
**Issue:** `planPaneTarget` returns `fmt.Errorf("session has no panes to adopt or split")` (`spawn.go:41`); the in-scope list covers comments only, and that exact string is quoted in `smoke_lifecycle_test.go:217` and `SANDBOX-REED-SUITE.md:285`.
**Fix:** Say whether the error text changes, and if so that both quoting sites move with it.

### [NIT:scope] One adoption-framed smoke test has no disposition
**Section:** Affected-test enumeration
**Issue:** `internal/reedcli/smoke_lifecycle_test.go`'s `TestSmokeRemoveLastStrandThenAddRunsTheNewCommand` (lines 138-167, Windows/psmux-only) is framed entirely on "the old adopt path bound the next strand to that corpse"; the stated `grep adopt` sweep hits it but the disposition list omits it.
**Fix:** Add it as comment-only (its mechanics still hold: the corpse is dead-pane-killed and the header is split).

### [NIT:design] Chokepoint ordering pinned only by real tmux
**Section:** Testing
**Issue:** reap-before-allocate inside `launchStrandLocked` — the task's central structural fix — is guarded only by the M16 smoke regression, though `newTestEngine`'s `e.tmux.execHook` fake (`lifecycle_test.go:342`) already drives `list-panes`/`split-window` untagged and deterministically.
**Fix:** Name an untagged test asserting `launchStrandLocked` issues `kill-pane` and re-enumerates before `split-window`, or state why the smoke tier alone suffices.

## Verdict

REQUEST_CHANGES
Sandbox suite spec asserts the exact behaviour this change inverts, with no disposition.
MILL_REVIEW_END
