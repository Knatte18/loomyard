MILL_REVIEW_BEGIN
# Review: Restore the Tier 1 floor: guards + perchengine

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-13
```

## Findings

### [NOTE] tierpurity rationale overstates spawn removal
**Section:** Decisions › tierpurity-evasion-left-alone
**Issue:** The rationale claims "After re-tiering, no untagged file spawns through that wrapper," but `gate_test.go`'s four remaining untagged tests (`_Pass`/`_Fail`/`_NotFound`/`_Timeout`) all still call `execGateCommand`, which spawns real `go` processes — the rejected-alternative paragraph even acknowledges these same tests spawn through the wrapper.
**Fix:** Reword to "no expensive/real-time spawn remains" — the residual four are cheap, guard-invisible-by-design Tier 1 spawns kept intentionally.

### [NOTE] boardtest seeded-count knob has hidden coupled literals
**Section:** Decisions › boardtest-bounded-shrink / Technical context (concurrency_test.go)
**Issue:** Reducing "seeded task count (100)" is named as a knob, but the reader hardcodes `GetTask("task-50")` and asserts `len(tasks) != 100`; lowering the seeded count below 51 (or off 100) breaks the test unless both literals move in lockstep. The `writes = 50` lever (the dominant re-render cost) has no such coupling.
**Fix:** Note the coupling, or steer the one bounded attempt to the `writes` knob, which the rationale already identifies as the cost driver.

## Verdict

APPROVE
Scope, decisions, and testing are sound and source-verified; two non-blocking accuracy NOTEs.
MILL_REVIEW_END
