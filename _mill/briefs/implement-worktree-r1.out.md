Perfect! Let me create a summary of what was accomplished:

## Summary

I have successfully completed both cards in batch 2 (worktree) of the prune-board-tests task:

### Card 6: Fold weft prechecks and migrate the no-weft-repo redundancy

**Changes made:**
- **Folded 4 functions into 1 table-driven test**: Combined `TestWeftPrechecksHardRequireWeftRepo`, `TestWeftPrechecksRejectExistingWeftWorktree`, `TestWeftPrechecksRejectExistingWeftBranch`, and `TestWeftHostPristineEnforced` into one `TestWeftPrechecks` function with a structured table containing setup functions, error assertions, and residue checks
- **Preserved original test names as subtests**: Each case is named after its original function (e.g., `TestWeftPrechecks/TestWeftPrechecksHardRequireWeftRepo`)
- **Migrated assertion**: The `result.Slug == ""` assertion from the dropped `TestWeftPrechecksHardRequireWeftRepo` was moved to the `NoWeftRepo` case in the `TestAdd` table
- **Trimmed TestWeftSpawnPairedWorktrees**: Removed branch-prefix and host-worktree assertions that are already covered by `TestAdd`; kept only weft-side assertions

### Card 7: Extract shared portal-setup helper

**Changes made:**
- **Created `setupPortalTarget` helper**: A file-local helper function that extracts the duplicated setup boilerplate from three test functions
- **Applied to all three portal tests**: Updated `TestCreatePortal`, `TestCreatePortalMultipleSubpaths`, and `TestCreatePortalRootRelPath` to use the new helper
- **Preserved all assertions**: All three test functions remain distinct with their original invariant assertions intact

### Verification Results

✓ All tests pass: `go test -tags integration ./internal/worktree/` **PASS**  
✓ Coverage maintained: **68.6%** (meets the 68.6% floor requirement)  
✓ Test count: 19 top-level tests (down from 22, as expected)  
✓ All original test names preserved as subtests for `-list` traceability  

### Files Modified

- `/internal/worktree/weft_test.go` - Folded 4 prechecks functions into 1 table-driven test
- `/internal/worktree/add_test.go` - Added `wantResultZero` assertion field and migrated assertion from dropped test
- `/internal/worktree/portals_test.go` - Extracted `setupPortalTarget` helper and applied to 3 test functions

### Commits Created

1. `574347d` - test(worktree): fold weft prechecks, migrate no-weft-repo assertion
2. `d071d87` - test(worktree): extract setupPortalTarget helper

{"status":"success","commit_sha":"d071d87e9a244e7e1d06dc3ec239d92dd84e27f2","session_id":"38def2ca-1590-453f-8709-8bdae77814bd"}