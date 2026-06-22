## Summary of Fixes Applied

Addressed all four findings from the holistic code review:

### 1. [BLOCKING] TestRenderOrphanDetection slash-in-name defect (FIXED)
- Renamed two rows to single base name TestRenderOrphanDetection. Commit 6c77e49.

### 2. [NIT] Unplanned test case TestRenderDeferredTask (RESOLVED)
- Added plan note to Card 1 in _mill/plan/01-board.md acknowledging the new case (preserves 62.5% floor). Commit 0f8dffa.

### 3. [NIT] Doc name-map omits TestSyncIntegration_EventuallyPushed (FIXED)
- Added entry and updated weft count 6 -> 7. Commit 6e1d6f7.

### 4. [NIT] Doc name-map subtest paths strip "Test" prefix (FIXED)
- Corrected all board entries to full subtest names with Test prefix. Commit 6e1d6f7.

All verify commands pass (board 62.5% floor, worktree, weft, ide, muxpoc).

90b4b9d5a55d304c640132c89b50da43b9517db9