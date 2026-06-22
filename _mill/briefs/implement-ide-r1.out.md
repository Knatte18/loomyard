## Summary

Implemented all 4 cards of the IDE batch.

### Card 10: Fold pickColor palette tests
- Folded 4 tests into table-driven TestPickColor, preserved RelPath per row, kept serial.

### Card 11: Fold CLI error envelopes
- Folded 3 tests into table-driven TestRunCLIErrors. Kept TestRunCLISpawnDispatch separate.

### Card 12: Fold Spawn tests, drop redundant color-selection test
- Folded 3 tests into table-driven TestSpawn; dropped TestSpawnColorSelection (redundant).

### Card 13: Dedup menu zero-worktree path
- Dropped TestMenuZeroWorktreeMessage (redundant with TestMenuRequiresLyxDir).

### Test Results
- All 11 tests pass; Coverage 75.4% (meets floor).

{"status":"success","commit_sha":"7d3bbe6d59b3df8be313d8df7e6c214af00a5b4e","session_id":"db399861-5461-4294-8d4a-764e89c0aea6"}