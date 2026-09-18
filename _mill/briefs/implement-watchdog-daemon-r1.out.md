All 14 cards in this batch are committed: cards 28 through 41, each with its own commit matching the batch file's `Commit:` message (plus one auxiliary `plan: extend watchdog-daemon refs for logger.Error` commit to extend card 33's declared file scope, as required by the stop-and-extend-plan protocol). The batch's `verify:` command (`go test ./internal/reedcli/ ./internal/reedengine/ ./internal/clihelp/ ./cmd/lyx/`) passes. I also compiled and ran the full `//go:build integration` suite added in card 41 against a real tmux 3.6 binary locally — all five integration cases pass — even though that suite is not part of this batch's own verify scope.

Key files touched (all absolute paths under `/home/knatte/Code/loomyard/wts/reed-header-selvage`):
- `internal/reedengine/overlay.go`, `internal/reedengine/overlay_test.go` — `ListSessions`
- `internal/reedcli/statusline.go`, `internal/reedcli/statusline_test.go` (renamed from `header.go`/`header_test.go`)
- `internal/reedcli/watchdog.go`, `internal/reedcli/watchdog_test.go`, `internal/reedcli/watchdog_integration_test.go` — the daemon
- `internal/reedcli/spawnwatchdog.go`, `internal/reedcli/spawnwatchdog_test.go` — the spawn helper
- `internal/reedcli/cli.go`, `internal/reedcli/cli_test.go`, `internal/reedcli/up.go`, `internal/reedcli/resume.go`, `internal/reedcli/attach.go`
- `internal/logger/logger.go` (added `Error`, an out-of-scope addition flagged via the plan-edit protocol)
- `CONSTRAINTS.md`, `internal/reedengine/doc.go`, `internal/reedengine/watchloop.go`
- `cmd/lyx/helptree_test.go`, `cmd/lyx/stencilseedgate_test.go`, `cmd/lyx/stencilseed.go`, `internal/clihelp/annotations_test.go`
- `_mill/plan/05-watchdog-daemon.md` (extended for the `logger.Error` addition)

14 of 14 cards committed this turn (cards_done covers all of 28–41).

{"status":"success","commit_sha":"b8126d29749e74b7a5a271c8dcefa4af18188937","session_id":"398d825e-1d2c-489f-a26a-e2c536b749ed","cards_done":[28,29,30,31,32,33,34,35,36,37,38,39,40,41]}
