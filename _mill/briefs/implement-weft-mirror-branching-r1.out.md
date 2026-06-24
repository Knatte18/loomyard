Perfect! All tests pass, including the new tests:
- TestWeftForkPointMirrorsHost ✅
- TestWeftForkPointSubtaskIsolation ✅
- TestWeftMissingParentBranch ✅
- TestAdd/DetachedHEAD ✅

All four cards have been successfully implemented and committed. Here's a summary of what was done:

## Implementation Summary

**Card 1:** Modified `createWeftWorktree` signature to accept a `startPoint` parameter and pass it to the git worktree add command, allowing the new weft branch to fork from the parent weft branch instead of the weft repo's HEAD.

**Card 2:** Added parent branch resolution in `Add` that:
- Calls `git rev-parse --abbrev-ref HEAD` to get the parent branch name
- Guards against detached/unborn HEAD by checking for exit code != 0 or stdout == "HEAD"
- Passes the parent branch as `startPoint` to `createWeftWorktree`

**Card 3:** Added comprehensive tests covering:
- Fork point verification (new weft branch forks from captured parent weft branch tip)
- Subtask isolation (mid-work branch has its own fork point, not tied to main)
- Missing parent branch (full rollback when parent weft branch doesn't exist)
- Detached HEAD guard (error caught before any creation with zero residue)

**Card 4:** Documented the design:
- Updated `weft.go` header with durable design notes
- Added "Branch model" subsection to `docs/overview.md` explaining the non-orphan, mirror-host topology

All files are within scope, all commits follow the required format, and all tests pass.

{"status":"success","commit_sha":"a076d80a4b8323ac5d05ac144c657c62dfab7580","session_id":"295c2eb3-8901-4679-9459-51f479ed9f30"}