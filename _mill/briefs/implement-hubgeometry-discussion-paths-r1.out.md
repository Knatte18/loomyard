Both cards are committed (2 of 2), tree is clean of tracked modifications, and verify passes.

{"status":"success","commit_sha":"6e67c83b","session_id":"6740a6ca-67ae-49c6-8b37-c0448bb3aa0e"}

Summary: 2 of 2 cards committed — Card 1 (`ff3a122a`, `feat(hubgeometry): add _lyx/discussion path accessors`) and Card 2 (`6e67c83b`, `test(hubgeometry): cover _lyx/discussion path accessors`). Both pushed to the `loom-discussion-producer` branch. `go test ./internal/hubgeometry/` passes. Working tree has no tracked modifications outstanding (only the pre-existing untracked brief file at `_mill/briefs/implement-hubgeometry-discussion-paths-r1.md`, which is out of scope).

Files touched:
- `/home/knatte/Code/loomyard/wts/loom-discussion-producer/internal/hubgeometry/hubgeometry.go` — added `DiscussionDir()`, `DiscussionDecisionRecord()`, `DiscussionSupportLog()` methods on `*Layout`, placed after `LoomStatusLock()`, WorktreeRoot-anchored per the Hub Geometry Invariant.
- `/home/knatte/Code/loomyard/wts/loom-discussion-producer/internal/hubgeometry/discussionpath_test.go` — new test file mirroring `loomstatus_test.go`, covering all three accessors plus the Cwd-vs-WorktreeRoot anchoring assertion.

{"status":"success","commit_sha":"6e67c83b","session_id":"6740a6ca-67ae-49c6-8b37-c0448bb3aa0e"}
