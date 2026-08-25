All 3 cards committed and match the batch's declared `Commit:` messages exactly. 3 of 3 cards committed — this batch is fully complete.

{"status":"success","commit_sha":"b9c9e52c63722b8ba0e927c06c0ee912a6b0d719","session_id":"b3786b8e-e532-4a3a-a140-84d0a135a3a4","cards_done":[4,5,6]}

Summary: implemented `Runner.Attach` in `/home/knatte/Code/loomyard/wts/loom-discussion-write-interactive/internal/shuttleengine/attach.go` (Card 4), added its supporting seams to `run.go`/`wait.go`/`rundir.go`, wrote the full test matrix in `attach_test.go` (Card 5, which also caught and fixed a real bug in `collectAttachCandidates`'s directory-mtime ordering — `entry.Info()` must run before `loadRunState`, since the latter's read-lock creates a lock file that bumps the directory's own mtime), and documented `Attach` in `doc.go` (Card 6). All 3 cards committed (3 of 3, exactly matching the batch's declared card count), `go vet ./...` and `go test ./internal/shuttleengine/` (the batch's `verify:`) both pass, and the working tree has no uncommitted tracked changes.

Files touched:
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-interactive/internal/shuttleengine/attach.go` (new)
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-interactive/internal/shuttleengine/attach_test.go` (new)
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-interactive/internal/shuttleengine/run.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-interactive/internal/shuttleengine/wait.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-interactive/internal/shuttleengine/rundir.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-interactive/internal/shuttleengine/doc.go`

{"status":"success","commit_sha":"b9c9e52c63722b8ba0e927c06c0ee912a6b0d719","session_id":"b3786b8e-e532-4a3a-a140-84d0a135a3a4","cards_done":[4,5,6]}
