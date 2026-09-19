MILL_REVIEW_BEGIN
# Review: Seeded driver choice: ly-drive strand as the child's driver

```yaml
duration_s: 137.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-x class (self-assessment; reported id is claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [NIT:consistency] Probe refusal names a go-path-only artifact
**Demoted-from:** BLOCKING
**Section:** § Testing, "readiness probe" bullet
**Issue:** The bullet requires the refusal message to name "the driver log", but `loomengine.LoomDriverLog` is the *detached Go driver's* captured stdout/stderr (`internal/loomengine/config.go:134`, written only inside `start.go`'s `exec.Command` branch at `:157-166`); the `llm` path writes no such file — its evidence is the pane and shuttle's run dir (`prompt.md`/`settings.json`/events).
**Fix:** State which artifact the `llm`-path refusal names (run dir / strand GUID / pane id), so a plan writer does not wire `LoomDriverLog` into a path that never writes it.

### [NIT:consistency] AskUserQuestion deny is not the backstop claimed
**Section:** § `ly-drive-gains-an-autonomous-mode`, change 1
**Issue:** "the tool is not even available" is true of the deny, but `SKILL.md:33` offers its operator choice as a *numbered text list*, not via the tool — so the deny structurally prevents nothing the skill actually does; only the SKILL.md edit does.
**Fix:** Drop the deny from the rationale or recast it as defence-in-depth, so the SKILL.md edit is understood as the sole enforcement.

### [NIT:scope] `shedrun` ephemeral accessor assumed, not inventoried
**Section:** § Constraints, Lyxdirs bullet
**Issue:** The report path must come from "`shedrun`'s ephemeral accessor", but the decided contract quoted elsewhere names only `SeedFile`/`SelfRunID`; if the predecessor ships no `.lyx/shed/<run-id>/` accessor, adding one is unlisted work (and `loomcli` may not spell `.lyx` itself).
**Fix:** Say explicitly that this task adds the accessor to `shedrun` if the rebase shows it absent — the same hedge already used for the bootstrap-lock relocation.

### [NIT:design] "Cap sentence" is an unpinned test target
**Section:** § `ly-drive-gains-an-autonomous-mode`, change 3
**Issue:** SKILL.md already contains `40` and "near a hundred steps"; the `cmd/lyx` test asserting `AutonomousDriveStepCap` "appears in its cap sentence" gives no rule for locating that sentence, so a later reword can leave the test green against the operator cap.
**Fix:** Name the anchor the test matches on (a stable heading or literal phrase in the Autonomous section).

## Verdict

APPROVE
One path-specific artifact named wrongly in the testing plan; everything else verified against source.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
