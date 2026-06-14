## Implementation Summary

**Card 1: Relocate the porcelain worktree-list parser into paths**
- Created `internal/paths/worktreelist.go` with `WorktreeEntry` struct and `List()` function
- Created `internal/paths/worktreelist_test.go` with comprehensive test coverage
- Created `internal/paths/helpers_test.go` with test utilities
- Commit: `ba8d07e`

**Card 2: Layout resolver, Getwd, geometry methods, typed errors**
- Created `internal/paths/paths.go` with Layout struct, Getwd(), Resolve(), ErrNotAGitRepo, geometry methods
- Commit: `e02b4a6`

**Card 3: Remove git.FindRoot**
- Removed FindRoot() and related tests
- Commit: `fdd36e2`

**Fix: Normalize MainWorktree path**
- Commit: `3eaf060`

All tests pass with `go test ./internal/paths/... ./internal/git/...`

{"status":"success","commit_sha":"3eaf060d6ae8a3acb19a9e7cd7ed072cfc482cbd","session_id":"3196606c-1ff1-4dd2-bb63-0c79a343d428"}
