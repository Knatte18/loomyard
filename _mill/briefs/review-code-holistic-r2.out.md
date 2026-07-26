MILL_REVIEW_BEGIN
# Review: fabric: cutover -- rewire consumers onto fabric, delete warp/weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-26
```

## Findings

### [BLOCKING] roadmap.md self-contradicts on fabric cutover's status
**Location:** `manifest/roadmap.md:15-17` (Planned, board item) vs `manifest/roadmap.md:145-146` (Done, fabric item)
**Issue:** Card 19 correctly moved fabric into Done ("fabric — unified host↔weft git-coordination module replacing warp/weft; cut over and old modules deleted"), but the pre-existing Planned `board: move storage to weft:main` item still reads "Depends on the Planned `fabric: cutover` item's branch-naming enforcement ... actually taking effect, not just `fabric`'s code existing alongside the old modules" — a named Planned item that no longer exists, describing a dependency that has in fact already landed. The same file now asserts both "fabric is Done, cutover complete" and "fabric: cutover is Planned, not yet in effect."
**Fix:** Update the board item's dependency clause to reference fabric as shipped (e.g. "now that `fabric`'s branch-naming enforcement is live") rather than pointing at a nonexistent Planned entry.

### [NIT] SANDBOX-CORE-SUITE.md's scenario-id range and session log are stale after S7/S8 deletion
**Location:** `tools/sandbox/SANDBOX-CORE-SUITE.md:113,344-345`
**Issue:** Card 13 deleted scenarios S7 (`Covers: weft`) and S8 (`Covers: warp`), but `ref` is the scenario id (`S0`-`S8`)" (line 113) and the session-log template's `S7:`/`S8:` lines (344-345) still describe S7/S8 as valid scenario ids, even though the scenario list now runs S0-S6, S9 with a gap.
**Fix:** Update the ref-range comment (e.g. "S0-S6, S9") and drop the S7/S8 lines from the session log format.

### [NIT] SANDBOX-FABRIC-SUITE.md cross-references a now-deleted CORE-SUITE scenario
**Location:** `tools/sandbox/SANDBOX-FABRIC-SUITE.md:204`
**Issue:** F3's guidance says "mirroring `SANDBOX-CORE-SUITE.md`'s S7 guidance for `weft sync`", but S7 was deleted by card 13, so the cross-reference now points at a scenario that no longer exists in that file.
**Fix:** Either restate the guidance inline or repoint the cross-reference at whichever surviving scenario (e.g. S5) now covers the equivalent point.

## Verdict

REQUEST_CHANGES
Implementation is otherwise clean and plan-faithful; fix the roadmap's self-contradictory fabric-cutover status.
MILL_REVIEW_END
