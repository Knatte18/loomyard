MILL_REVIEW_BEGIN
# Review: reed: per-hub daemon reaps orphaned sessions

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, as reported by the runtime
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Hub-probe short-circuit re-exposes the in-flight hazard
**Section:** Scope In (`planReapCycle` bullet) vs `ordering-within-a-cycle` + Testing (in-flight case)
**Issue:** `hubLive == false` is specified to short-circuit to `(nil, live)`, i.e. it returns the *full* live list including names whose reap goroutine is still running, while the in-flight rule requires such a name to be absent from both return values precisely so `planSessionDiff` cannot read it as appeared and enter a session mid-kill.
**Fix:** State what the hub-down branch returns for in-flight names — return `live` minus `inFlight` (counters still untouched), and add that to the hub-probe test case.

### [BLOCKING:consistency] Superseded "--shell validated non-empty" bullet left standing
**Section:** `the-daemon-is-told-its-shell`, first Decision bullet
**Issue:** It still says `--shell` is "validated non-empty in the same pre-flight", which the very next Decision bullet, and Scope's `validateWatchdogFlags` (deliberately takes no shell parameter), both reverse; a plan writer following the first bullet would add the validator the design forbids.
**Fix:** Rewrite the first bullet to state the flag is added and passed unconditionally, with no pre-flight on it.

### [BLOCKING:consistency] Testing names a `reapSessionTmux` the split decision does not have
**Section:** Testing, `internal/reedengine` untagged bullets
**Issue:** Tests are specified against `reapSessionTmux` "lists panes before it kills", but `ReapSession-is-a-second-engine-less-exported-function` mandates two separate functions (`reapSessionPanes`, `reapSessionKill`) precisely so the closure step runs between them — a single list-then-kill function is the ordering bug the decision exists to prevent.
**Fix:** Rename the test bullets to the two functions and assert the ordering at the `ReapSession` level (untagged: panes listed, then kill against `=<name>`; list-panes failure still kills).

### [NIT:scope] Who logs the reap's wait is unstated
**Section:** Constraints (Live-Substrate Spawn Observability)
**Issue:** The constraint note says both the kill and the wait are logged, but no bullet names the emitter — `reapPaneChildren` is a shared helper `Engine.Down` also uses, so adding logging there is a change outside this task's stated surface.
**Fix:** Name the log site explicitly (reap goroutine in `reedcli`, or `ReapSession` itself) and say `reapPaneChildren` is untouched.

## Verdict

REQUEST_CHANGES
Hub-down branch contradicts the in-flight rule; two superseded/mismatched specifications remain.
MILL_REVIEW_END
