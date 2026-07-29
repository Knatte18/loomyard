MILL_REVIEW_BEGIN
# Review: fabric: config-driven junction list

```yaml
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude Opus 4.x (model id claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [NOTE] Wiring-guard filter set: only accessor option is single-source
**Section:** Decisions → wiring-guard (Note for mill-plan)
**Issue:** The note offers "a single hubgeometry accessor OR a fabricengine const," but `IsReservedHubName` must itself hold `{_board,_portals,_launchers,_raddle}` internally, so the fabricengine-const alternative duplicates it — the exact two-drifting-lists hazard the note warns against.
**Fix:** State that only the hubgeometry-accessor option (an exported hub-reserved-names accessor fabric calls) actually achieves the single source; drop the const alternative.

### [NOTE] Enforcement claim overstates the guard's reach
**Section:** Technical context → Enforcement interaction
**Issue:** It implies a `[]string{"_lyx","_pattern"}` const in fabric Go would be machine-caught; `enforcement_test.go`'s `TestEnforcement_GeometryLiterals` catches only filepath.Join args, binary-`+` operands, and BasicLit const values — a slice/composite literal is not a caught context.
**Fix:** Note the "don't hardcode config-migrated names in fabric" rule rests on review discipline, not the machine check, so the plan writer does not assume a false backstop.

### [NOTE] Config-load failure surfaced as junction-health reason may misdirect
**Section:** Decisions → no-fallback
**Issue:** `checkJunctionHealth` returns `(bool, string)`; surfacing a `fabric.yaml` load failure as an "unhealthy reason" can read as junction drift to an operator whose actual fault is an unreadable config.
**Fix:** Require the reason string to name the config-load failure distinctly (e.g. "cannot load fabric.yaml"), not as a generic junction-unhealthy message.

## Verdict

APPROVE
Scope, decisions, and testing are sound and source-grounded; the r1 `_raddle` regression is resolved.
MILL_REVIEW_END
