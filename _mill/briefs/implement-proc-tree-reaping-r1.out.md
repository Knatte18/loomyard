Clean working tree, all three cards committed distinct from batch start commit, verify passes. Emitting the final structured status.

{"status":"success","commit_sha":"15d868b","session_id":"bc7f195c-0b3c-4a70-9cd5-7964c0a420ed"}

Summary of work done for batch `01-proc-tree-reaping`:

- `internal/muxengine/proctree.go` (new) — pure, build-tag-free helpers: `parseStatPPID`, `descendantClosure`, `ProcCmdline`/`matchSocketCmdlines`.
- `internal/muxengine/proctree_test.go` (new) — table-driven fixtures for all three pure helpers, including the space-and-paren `comm` edge case, missing-parent/reparent/cycle cases, and the two near-miss socket-match shapes.
- `internal/muxengine/proctree_windows.go` (new) — verbatim-extracted `descendantClosurePIDs`/`serverProcessesOnSocket` WMI bodies.
- `internal/muxengine/lifecycle.go` (edited) — the two method bodies removed; all callers left unchanged; `os/exec` import still used by `ensureServerAndSessionLocked`, so no import cleanup was needed.
- `internal/muxengine/proctree_linux.go` (new) — `/proc`-backed `descendantClosurePIDs`/`serverProcessesOnSocket`, delegating to the pure helpers, with the deferred-follow-up caveat recorded verbatim from `_mill/discussion.md`.

Verify (`GOOS=linux go build ./internal/muxengine/... && go test ./internal/muxengine/...`) passed after each card and again at the end. Three commits: `f253a4a`, `c4634a6`, `15d868b`, all pushed to `facilitate-linux`. Working tree is clean.
