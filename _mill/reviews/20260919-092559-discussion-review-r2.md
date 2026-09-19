MILL_REVIEW_BEGIN
# Review: reed: born-as-strand for loom start's operator attach

```yaml
duration_s: 146.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: claude-opus-4-class (self-assessment; reported build label "Opus 5")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Watchdog seam has no named home or guard source
**Section:** Decisions §watchdog-parity; Technical context §"Watchdog extraction"
**Issue:** The discussion says `ensureWatchdogSpawned` is extracted to "a seam both callers use" but never says which package owns it; `loomcli` imports no `*cli` package today (verified: no `internal/reedcli` import outside `cmd/lyx` and smoke tests), and the CLI/Cobra Invariant's package-naming rule names only `<module>cli` → `<module>engine`. It also never says where `loomcli` gets the `suppressWatchdogSpawn` value — `reedcli` derives it from `testing.Testing()` in its constructor (`internal/reedcli/cli.go:55`), a field `loomcli` does not have.
**Fix:** Name the seam's owning package and the import direction, and state how `loomcli` supplies the test-suppression value.

### [BLOCKING:design] Row-budget effect of a permanent operator pane unaddressed
**Section:** Decisions §display-below-parent-focused-no-shrink; Technical context §"planPaneTarget and Selvage"
**Issue:** `render/height.go`'s `stackHeights` splits the remaining rows equally across every non-strip full pane, and `orderStack` sorts by chain depth — a parentless, childless operator strand is never a strip and, once any deeper agent strand exists, is never the active pane either, so it permanently consumes a full share of window rows for every agent pane for the rest of the run. The discussion asserts only that "the layout engine then places both under AnchorBelowParent rules".
**Fix:** State the accepted layout consequence (or the mitigation) for agent-strand heights once the operator pane is permanent.

### [BLOCKING:design] Whether `--no-attach` also skips the watchdog spawn is unstated
**Section:** Decisions §watchdog-parity vs §no-attach-skips-the-strand
**Issue:** §watchdog-parity says "the handover also spawns the watchdog", which implies the `mustAttach` gate, but never says so; `reedcli`'s own `up` verb spawns it independently of any attach, and `loom start`'s substrate step is the `up` analogue, so a plan writer could place the call on either side of the gate.
**Fix:** State explicitly whether the watchdog spawn sits inside or outside the `mustAttach` gate, with the reason.

### [NIT:design] `ShrinkWhenWaitingOnChild: false` rationale rests on a false premise
**Section:** Decisions §display-below-parent-focused-no-shrink
**Issue:** The stated reason ("shrinking it would collapse the one pane they are typing in") cannot occur: `stackHeights` marks a strand a strip only when `isAncestor(s, stack) && ShrinkWhenWaitingOnChild`, and the operator strand has no children, so the flag's value is inert.
**Fix:** Re-ground the choice (e.g. future-proofing against a child ever being parented onto it) rather than on a collapse that the height policy cannot produce.

### [NIT:scope] Watchdog batch's scope is deferred to the reviewer
**Section:** Decisions §watchdog-parity, "Note for the reviewer: this is separable"
**Issue:** The in/out boundary for the watchdog work is handed to review rather than settled in the discussion, so the plan writer's work inventory is conditional.
**Fix:** Record it as unconditionally in scope (as the Q&A log already does) and drop the deferral, or move it out.

## Verdict

REQUEST_CHANGES
Three unresolved design points: seam ownership, layout row budget, watchdog gating.
MILL_REVIEW_END
