MILL_REVIEW_BEGIN
# Review: dev/test lyx.exe separated from production deploy — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-25
```

## Findings

### [NIT] resolveLyx runs before the clone no-op path
**Location:** Batch 3 / Card 9
**Issue:** `run()`'s build case resolves `lyxPath, _, err := resolveLyx()` unconditionally before `decideClone`; today the "Hub already exists, no `-reset`" branch returns success without needing `lyx`, so this makes a previously-succeeding no-op fail when neither a dev binary nor a PATH `lyx` is resolvable.
**Fix:** Resolve the binary inside the clone branch (only when a clone will actually run) rather than ahead of `decideClone`, or accept and note the edge-case change explicitly.

### [NIT] All Files Touched omits the deleted design doc
**Location:** Batch 4 / Card 16 (and 00-overview.md "All Files Touched")
**Issue:** Card 16 deletes `manifest/designs/dev-test-binary.md`, but that path is absent from the overview's `## All Files Touched` list (which does include the edited `manifest/roadmap.md`).
**Fix:** Add `manifest/designs/dev-test-binary.md` to the touched-files list (or mark deletions) so the manifest is complete.

## Verdict

APPROVE
Plan is well-grounded, constraint-safe, and complete; only two minor nits.
MILL_REVIEW_END
