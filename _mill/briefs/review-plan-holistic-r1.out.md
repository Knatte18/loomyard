MILL_REVIEW_BEGIN
# Review: loom: interactive Discussion-Write — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-25
```

## Findings

### [NIT:scope] Card 4's rationale cites claudeengine identifiers absent from its Context
**Location:** batch 2 / card 4 (`Runner.Attach`) **Issue:** Requirements names `claudeengine.Startup`, `StartupPending`, and the trust-dialog needles when justifying why `started` seeds `true`, but `internal/shuttleengine/claudeengine/startup.go` — where all three live — is not in card 4's `Context:` list (only `settings.go` appears, in card 1). **Fix:** add `internal/shuttleengine/claudeengine/startup.go` to card 4's `Context:`. Risk is low: `_mill/discussion.md` (already in Context) states the identical fact verbatim, so no cold-start exploration is actually needed to write the comment.

### [NIT:design] finalize's Outcome-write placement leaves a ForkSubagents-audit failure stuck at "running"
**Location:** batch 1 / card 2 (`RunState.Outcome`) **Issue:** placing the `Outcome` write "after the fork-audit block" means a run that classifies `OutcomeDone` but whose `AuditForks` call errors never advances `Outcome` past `"running"` — the exact live-but-idle state the sentinel exists to prevent, if the pane is still alive on a later resume. Unreached by this task's own rows (none set `ForkSubagents`), but undocumented as a forward-looking gap. **Fix:** either write `Outcome` before the audit call, or add one sentence naming this as an accepted residual, matching how the plan already discloses every other accepted residual.

### [NIT:consistency] Card 15's Done-entry rationale contradicts the link it prescribes
**Location:** batch 5 / card 15 (roadmap move) **Issue:** the card states a Done entry "points at the module's own package documentation rather than at a design doc," then instructs linking `manifest/designs/loom.md`'s crash-recovery section — itself a design doc. The concrete instruction is correct (it matches the two existing `loom:` Done entries, both of which link `designs/loom.md`), but the connecting sentence contradicts the action it introduces. **Fix:** drop or reword the "rather than at a design doc" clause so the stated rule doesn't conflict with the established, correctly-followed precedent.

## Verdict

REQUEST_CHANGES
Three low-risk NIT findings; the plan is otherwise exceptionally well-grounded against source and the discussion's decisions.
MILL_REVIEW_END
