Perfect! All conflicts have been resolved successfully. Here's a summary of what was done:

## Conflict Resolution Summary

### **internal/reedcli/header.go**
- Combined both sides: kept `headerBlockingPayload()` helper function from origin/main along with `headerWatch` and `headerPark` package vars from HEAD
- Merged RunE implementation to use `headerBlockingPayload()` while preserving all HEAD's watch loop logic, logger output handling, and keepalive contract

### **internal/reedcli/header_test.go**
- Updated file-level comment to describe both test coverage areas
- Kept all test infrastructure from HEAD (stubbing, keepalive-survival tests) and TestHeaderBlockingPayload from origin/main
- Preserved test contract validation for both the watch loop and payload generation

### **internal/reedengine/apply.go**
- Kept HEAD's function signature introducing `applyLayoutLockedOpts` with `opts` parameter and `applyResult` return type
- Preserved HEAD's implementation which splits layout apply from focus handling

### **internal/reedengine/windowsize.go**
- Merged imports: combined HEAD's `errors`, `io/fs`, `os`, `runtime` with origin/main's `fmt`

### **internal/reedengine/windowsize_test.go**
- Merged imports: combined HEAD's `io/fs`, `os`, `path/filepath` with origin/main's `fmt`

### **manifest/roadmap.md**
- Kept HEAD's correct description of the watchdog daemon as a "watch loop hosted inside the existing per-worktree header pane" (not origin/main's "standalone per-worktree daemon")
- Preserved the accurate reflection of what's actually been shipped (resize-geometry half) and what remains (pane-reap half)

All files are now staged and ready for merge continuation.

{"status":"success"}
