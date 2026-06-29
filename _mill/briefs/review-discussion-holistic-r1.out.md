MILL_REVIEW_BEGIN
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-29
```

## Findings

### [GAP] lyxtest leaf-enforcement banned list not updated
**Section:** Testing → Guard tests / Constraints → lyxtest Leaf Invariant
**Issue:** `internal/lyxtest/leaf_enforcement_test.go` hardcodes `bannedImports` = `internal/board`, `internal/warp`, `internal/weft`; after the rename those paths vanish, so the test keeps passing but silently stops guarding the new `*engine`/`*cli` feature packages. The discussion enumerates the cmd/lyx guards (registration/longlist/helptree/drift) but omits this one.
**Fix:** Add the leaf-enforcement `bannedImports` update (to the new `boardengine`/`warpengine`/`weftengine` and `*cli` paths) to the final guard batch.

### [NOTE] weftengine file list omits weft.go
**Section:** Technical context → weft per-module placement
**Issue:** `internal/weft/weft.go` (package doc + domain constants `commitMessage`/`lockDirName`/`writeLockFile`/`pushLockFile` + `scopedPathspec`, used by `sync.go`) is pure domain but is not assigned to any package in the file-by-file enumeration; for a file-complete sweep this is an unlisted production file.
**Fix:** Explicitly place `weft.go` in `weftengine` in the file list.

### [NOTE] Stale prose references to old package paths
**Section:** Scope (docs) / Documentation Lifecycle
**Issue:** Comment-only references to renamed paths exist outside the importer list — `internal/lyxtest/doc.go` (`internal/warp`, `internal/weft`), `tools/sandbox/main.go` (`internal/warp/clone.go`), `cmd/lyx/main_test.go` (`internal/board`) — none functional but they drift on rename.
**Fix:** Note that these prose/comment references should be updated for accuracy alongside the moves.

## Verdict
GAPS_FOUND
One enforcement-test guard (lyxtest leaf invariant) is unaccounted for in the rename plan.
MILL_REVIEW_END
