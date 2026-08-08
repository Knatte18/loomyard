All 8 commit subjects match all 8 declared `Commit:` messages in the batch (cards 23-30). 8 of 8 cards committed — all complete.

{"status":"success","commit_sha":"2c83906bab43d5fbea2ae8d128a6733d1836c527","session_id":"4b400b4a-972d-4968-bccf-6a57abaea3e0","cards_done":[23,24,25,26,27,28,29,30]}

Summary: implemented all 8 cards of batch `dotlyx-group-reanchor-and-logger-sink` in `/home/knatte/Code/loomyard/wts/dotlyx-scratch-hygiene`. 8 of 8 cards committed (verified against `git log feacbe2a..HEAD --oneline`, matching all 8 `Commit:` messages in the batch file).

Key files touched:
- `internal/logger/sink.go`, `internal/logger/logger.go`, `internal/logger/logsdir_test.go` (new), `internal/logger/sink_test.go`, `internal/logger/worktreelogs_test.go` (deleted)
- `internal/shuttleengine/rundir.go`, `internal/shuttleengine/run.go`, `internal/shuttleengine/rundir_test.go`, `internal/shuttleengine/run_test.go`
- `internal/burlerengine/engine.go`, `internal/burlerengine/doc.go`, `internal/burlerengine/engine_test.go`
- `internal/reedengine/lifecycle.go`, `internal/reedengine/lock.go`, `internal/reedengine/spawn.go`, `internal/reedengine/strand.go`, plus their test files and `contract_integration_test.go`
- `internal/scoutengine/daemonstate.go`, `internal/scoutcli/cli.go`
- `cmd/lyx/constructoranchoring_test.go`, `docs/shared-libs/README.md`, `manifest/designs/fabric-unified-view.md`

Notable design decision made during card 25: the "sink survives a rename mid-process" test could not simply rename the directory away and reassert success at the old path, since a POSIX file descriptor survives a directory rename -- that wouldn't distinguish the old held-handle design from the new open/append/close design. Instead the test renames the directory away, recreates an empty directory at the original path, and asserts the second write lands there -- only the handle-free design (re-opens by path) writes into the freshly recreated directory. This was verified empirically by temporarily reverting to the pre-card-24 sink implementation and confirming the test fails, then restoring.

Full batch verify command passed: `go test ./internal/logger/... ./internal/shuttleengine/... ./internal/burlerengine/... ./internal/reedengine/... ./internal/scoutengine/... ./internal/scoutcli/... ./cmd/lyx/... && go test -tags integration ./internal/reedengine/...`. Working tree is clean (no uncommitted tracked changes).
