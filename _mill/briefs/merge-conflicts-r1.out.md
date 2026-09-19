Perfect! All conflicts have been resolved successfully. Let me verify the key aspects one final time:

**Summary of resolutions:**

1. **docs/overview.md**: Combined both branch versions:
   - Kept parent's updated command list (statusline, watchdog) and interactive-handoff description
   - Added current branch's self-heal feature description for add/attach

2. **internal/reedengine/lifecycle.go**:
   - Kept current branch's upLocked extraction and ensureSessionLocked seam
   - Updated terminology from "header" to "Selvage" and "status-line" throughout
   - Fixed stale comments to reflect new terminology

3. **manifest/roadmap.md**:
   - Cleared Planned section conflicting items
   - Combined Done section with all three items: reed self-heal, fabric remote deletion, and worktree spawn/teardown

4. **manifest/designs/reed-fabric-standalone-api.md**:
   - Combined method counts: 18 methods (17 base + EnsureSession from current branch)
   - Combined identifier counts: 14 external identifiers (13 base + ListSessions from parent branch)

5. **manifest/designs/worktree-lifecycle-shed-producers.md**:
   - Accepted deletion (parent branch completed this item)

All conflicts are staged and ready for merge continuation.

{"status":"success"}
