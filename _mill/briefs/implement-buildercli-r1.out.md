All 4 of 4 cards committed, matching the batch file's `## Cards` exactly:
- Card 26 -> `30f7531` feat(builder): buildercli command tree and weft helper
- Card 27 -> `16cb55c` feat(builder): validate and status verbs
- Card 28 -> `729f41b` feat(builder): spawn-batch and poll verbs with weft commits
- Card 29 -> `b9316b5` feat(builder): run and pause verbs

Verify command (`go test ./internal/buildercli/... ./internal/builderengine/...`) passes cleanly, working tree is clean, and both commits are pushed to `origin/internal-builder`.

Relevant files (all under `internal/buildercli/`):
- `cli.go` — `builderCLI` struct, `Command()`, `RunCLI()`
- `weft.go` — `weftCommit` helper
- `validate.go` / `validate_test.go` — `validate` verb, shared `findingsEnvelope`
- `status.go` / `status_test.go` — `status` verb, shared `batchReportFileName`
- `spawnbatch.go` / `spawnbatch_test.go` — `spawn-batch` verb, `pausedEnvelope`
- `poll.go` / `poll_test.go` — `poll` verb, `pollTurnEnded`/`pollStrandLive`/`digestFields`
- `run.go` / `run_test.go` — `run` verb
- `pause.go` / `pause_test.go` — `pause` verb
- `cli_test.go` — group-level RunCLI tests

Notable implementation decisions worth flagging to the reviewer:
- `output.Err` has no room for structured extra fields, so `findingsEnvelope` (validate.go) and `pausedEnvelope` (spawnbatch.go) locally build the `{"ok":false,"error":...}` envelope plus one extra field (`findings` / `paused`) via `encoding/json` directly, rather than extending the shared `internal/output` package.
- `poll.go`'s `pollTurnEnded`/`pollStrandLive` are CLI-layer reimplementations of builderengine's package-private `turnEnded`/`strandLive` (poll.go), since those are unexported and buildercli cannot call them cross-package; they use the same seams (`shuttleengine.Engine.ParseEvents`, `shuttleengine.MuxOps.Status`).
- `poll.go`'s `pollRealClock` satisfies `builderengine.PollUntilTerminal`'s unexported `clock` parameter type via Go's structural interface satisfaction.

{"status":"success","commit_sha":"b9316b5","session_id":"7892aab8-d88d-4950-8a55-a84e236efec3"}
