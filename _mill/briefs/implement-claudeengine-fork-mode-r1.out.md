All 3 of 3 cards committed, verify passes.

Summary: Implemented batch `claudeengine-fork-mode` (batch 2 of the "Fork-based cluster review in burler" plan) in full — 3 of 3 cards committed.

- Card 3 (`04c75968`): `internal/shuttleengine/claudeengine/command.go`, `claudeengine.go`, `command_test.go`, `prepare_test.go` — added `forkSubagents` param to `buildLaunchCmd`/`buildResumeCmd`, new `forkSubagentEnvKey` constant, threaded `spec.ForkSubagents` through `Prepare`.
- Card 4 (`f1ef533d`): `internal/shuttleengine/claudeengine/settings.go`, `settings_test.go`, `claudeengine.go` — added `forkSubagents` param to `buildSettings`, new conditional Agent hook + `steerAgentNonForkDeny` constant.
- Card 5 (`cfe208b5`): new `internal/shuttleengine/claudeengine/audit.go` + `audit_test.go` + 4 `testdata/*.jsonl` fixtures implementing `AuditForks`; `internal/shuttleengine/engine.go` (interface addition), `wait.go` (`finalize` audit attach), `fakes_test.go`/`wait_test.go`, plus minimal `AuditForks` stubs added to every other compile-time `Engine` double in `internal/shuttlecli/cli_test.go`, `internal/builderengine/poll_test.go` + `spawn_test.go`, `internal/buildercli/poll_test.go` + `spawnbatch_test.go`.

Verify command (`go test ./internal/shuttleengine/... ./internal/shuttlecli/ ./internal/builderengine/ ./internal/buildercli/`) passes, and a full `go build ./...` / `go test ./...` repo-wide sweep also passes clean. Working tree has no uncommitted tracked changes (only the pre-existing brief file remains untracked, out of scope).

{"status":"success","commit_sha":"cfe208b5","session_id":"185f5f0e-9e04-4dd0-9d20-ca7a1f23fb4a"}
