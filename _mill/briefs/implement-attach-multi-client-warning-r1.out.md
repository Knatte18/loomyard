All 3 of 3 cards committed and verified. Summary:

- Card 15: `internal/reedengine/attach.go` (added `attachedClient` type and `parseClientList`), `internal/reedengine/attach_test.go` (added `TestParseClientList`) - commit `ad78f5af1`
- Card 16: `internal/reedengine/attach.go` (added `warnMismatchedClientsLocked`, wired into `AttachArgv`'s pre-flight, extended the doc comment) - commit `9502596a3`
- Card 17: `internal/reedengine/attach_test.go` (extended `attachScript`/`newAttachHook` with a `list-clients` branch, added `TestAttachArgv_MultiClientWarning`) - commit `5fcb7ba53`

`go test -count=1 ./internal/reedengine/` passes. `git status --porcelain --untracked-files=no` is clean (no uncommitted tracked changes).

{"status":"success","commit_sha":"5fcb7ba535628ee0aec1dd19b2bd3c4a9cc725cd","session_id":"950f2296-4b0e-44ff-9b5e-c4410765635b","cards_done":[15,16,17]}
