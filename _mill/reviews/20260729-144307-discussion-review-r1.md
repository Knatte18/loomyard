MILL_REVIEW_BEGIN
# Review: fabric: config-driven junction list

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: claude-opus-4-8
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] `_raddle` silently loses reserved-slug status
**Section:** Decisions → reserved-names
**Issue:** New `IsReservedHubName` = config junctions (`_lyx _pattern`) ∪ `{_board, _portals, _launchers}`; `_raddle` is in neither, so it drops from today's reserved set (verified: `hubgeometry.go:426` currently reserves `_raddle`) — a worktree could now be named `_raddle`, colliding when raddle later appends to `pathspec`. The design (`fabric-unified-view.md:104`) explicitly expects this slice to migrate `_raddle`'s reserved entry, not delete it.
**Fix:** Decide explicitly — either keep `_raddle` in the hardcoded hub-structural set (it has `WeftRaddleDir` and is a known future junction) or state and document the accepted removal of its reservation.

### [NOTE] Appending a hub-level name to `pathspec` would wire a bad junction
**Section:** Testing → Edge cases (name colliding with hub-structural token)
**Issue:** The edge case only covers the reserved-set union; it ignores that putting `_board`/`_portals`/`_launchers` in `pathspec` would also *wire* a per-worktree junction, conflicting with those dirs' hub-level geometry — the exact reason reserved-names kept them geometry-intrinsic.
**Fix:** Note this operator-error hazard and decide whether the wiring loop should skip/guard hub-structural names (or consciously accept it as misconfiguration).

### [NOTE] Junction order now follows `pathspec`, not guaranteed `_lyx`-first
**Section:** Technical context (order is observable)
**Issue:** `UnwireResult.JunctionsRemoved` order and first-unhealthy-wins now derive from `pathspec` token order; nothing pins `_lyx` first if a future append reorders the list.
**Fix:** State whether `_lyx`-first remains a contract (e.g. helper sorts/pins it) or that config order is authoritative.

## Verdict

GAPS_FOUND
One unaddressed behavior change: `_raddle` silently loses its reserved-slug protection.
MILL_REVIEW_END
