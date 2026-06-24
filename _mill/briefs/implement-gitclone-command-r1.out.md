All cards implemented and tested successfully.

Cards 1-6 completed: package + URL-derivation helpers, unit tests, clone orchestration with strict-abort teardown, RunCLI entry with JSON output, integration tests, and wiring git-clone into the lyx dispatcher.

Test results:
go test -tags=integration ./internal/gitclone/ ./cmd/lyx/ ./internal/paths/
ok  github.com/Knatte18/loomyard/internal/gitclone
ok  github.com/Knatte18/loomyard/cmd/lyx
ok  github.com/Knatte18/loomyard/internal/paths

Last commit: bf4420d3d70860dd9a1ca5377f5ff723249d7cce

{"status":"success","commit_sha":"bf4420d3d70860dd9a1ca5377f5ff723249d7cce","session_id":"efa75c0d-dc25-4a19-ac20-a38874fa2bd4"}
