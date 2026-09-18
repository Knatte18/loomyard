MILL_REVIEW_BEGIN
# Review: Launch ly-supervise and orchestrator via lyx reed add

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (self-reported; exact build unknown)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] "live" is undefined for --if-absent
**Section:** `### add-if-absent-flag` / Testing (smoke)
**Issue:** reed carries two distinct predicates — `liveIDSet` ("present in the window, alive or dead") and `aliveIDSet` ("present AND not dead") (`internal/reedengine/apply.go:18-37`) — and `Resume` deliberately picks `aliveIDSet` with a comment that a strand bound to a dead-but-present pane must be relaunched (`lifecycle.go:748-750`); the discussion's "exists and is live" / "exists but is not live" names neither, and the smoke case "kill the pane, `add --if-absent` again, assert the strand is live again" fails outright under the `liveIDSet` reading because reed keeps the sole dead pane.
**Fix:** state that `--if-absent` classifies liveness with `aliveIDSet`, matching `planResumeLaunches`, and say so in the decision rather than leaving the predicate to the plan writer.

### [BLOCKING:design] Relaunch branch's command and field reconciliation unstated
**Section:** `### add-if-absent-flag`
**Issue:** `Resume` launches with `ResumeCmd` falling back to `Cmd` from persisted state (`lifecycle.go:761-765`), while an `add --if-absent` invocation supplies fresh `--cmd`/`--resume-cmd`/`--parent`/`--anchor`/`--focus`; the discussion says only "relaunched through the same `launchStrandLocked` path `resume` uses" and never says which command string is passed, nor whether a name-matched strand's stored `Cmd`/`ResumeCmd`/`Parent`/`Display` are overwritten from the new flags or left as recorded.
**Fix:** decide explicitly — e.g. relaunch uses the stored `ResumeCmd`-else-`Cmd` and the matched strand's persisted spec fields are never rewritten by an `--if-absent` add — and record the rejected alternative.

### [NIT:design] No-op branch's disposition of --focus / display flags
**Section:** `### add-if-absent-flag`
**Issue:** the launch chain passes `--focus`, but the "exists and is live" branch is described only as a no-op printing guid and name, leaving unsaid whether focus/anchor from the repeat invocation are applied to the existing strand.
**Fix:** state that the no-op branch mutates nothing, including `Display`/focus, so a reopen never re-arranges a running layout.

### [NIT:scope] Presentation block not distributed across the four tasks
**Section:** `### sequenced-tasks-not-shell-operators` / Testing (`internal/vscode`)
**Issue:** today's single task carries `presentation` (`echo`, `reveal: always`, `panel: new`) (`internal/vscode/config.go:69-73`); splitting into four tasks means deciding each one's presentation so the operator lands in the `attach` terminal rather than a stack of four panels, and the generated-shape assertions listed in Testing do not mention it.
**Fix:** name the per-task presentation intent (quiet for `up`/`add`, revealed for `attach`) in the decision and include it in the asserted generated shape.

## Verdict

REQUEST_CHANGES
Two unresolved semantics in `--if-absent`: liveness predicate and relaunch command/field reconciliation.
MILL_REVIEW_END
