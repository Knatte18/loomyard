Excellent! All 6 cards have been successfully implemented and committed. Let me summarize the work:

## Implementation Summary

I have successfully completed all 6 cards of the **docs-and-roadmap** batch:

**Card 13** - Added comprehensive weft overlay model documentation to `docs/overview.md`:
- Updated Path Invariants section with Hub/Prime terminology
- Added new "Weft overlay model" section with topology diagram, git ownership explanation, artifacts table, junction model, and weft suffix convention

**Card 14** - Updated `docs/roadmap.md`:
- Marked portals as deprecated
- Added milestones 5-7 for tasks 006-008 (Weft engine, Hub-creator, _codeguide junction)
- Renumbered subsequent milestones accordingly

**Card 15** - Updated `docs/shared-libs/paths.md`:
- Renamed Layout struct fields: `Container` → `Hub`, `Prime` → `Prime`
- Renamed method: `HubName()` → `PrimeName()`
- Updated all field descriptions and method documentation
- Added deprecation notices to portal methods

**Card 16** - Updated module documentation:
- `docs/modules/worktree.md`: Renamed container/hub terminology, added weft overlay model pointer, marked portals as deprecated
- `docs/modules/board.md`: Updated all `_lyx/board.yaml` references to `_lyx/config/board.yaml`, updated init and data flow sections

**Card 17** - Updated config path references:
- `docs/shared-libs/config.md`: Updated layout diagram and resolution model for new config path structure
- `docs/shared-libs/README.md`: Updated config reference
- `docs/benchmarks/board-performance.md`: Updated benchmark note references

**Card 18** - Fixed `CONSTRAINTS.md`:
- Renamed `HubName()` → `PrimeName()` in method list
- Updated prose: "container" → "hub" terminology

All commits follow the per-card pattern with detailed commit messages explaining the changes.

{"status":"success","commit_sha":"3b9996b99697d6f2a75ed6a3952cfbb99e3e7f3a","session_id":"69e077ee-244c-4643-afc1-b8fcfcc8dbf2"}