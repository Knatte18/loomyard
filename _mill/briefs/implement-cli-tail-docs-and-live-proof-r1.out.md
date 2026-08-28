All 6 cards (19-24) have matching commits. 6 of 6 cards committed, verify passes fully.

{"status":"success","commit_sha":"8661939d97681e32481d1fdb34f621968b3f4835","session_id":"343838bd-a6df-413c-982d-3a13d103d1d1","cards_done":[19,20,21,22,23,24]}

Summary of work (all file paths absolute):

- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedcli/header.go` — Card 19: `--blocking` tail now calls `Engine.Watch` via substitutable `headerWatch`/`headerPark` vars, discards the logger's stderr sink before the loop, logs (never surfaces) a watch error, and always parks. `Long` text documents the new watch loop and `watchdog: off`.
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedcli/header_test.go` — Card 20: new tests for nil-watch, error-watch (keepalive survival), non-blocking mode, and the `Long` watchdog mention; existing two tests untouched.
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/doc.go` — Card 21: appended the resize watchdog's load-bearing assumption bullets.
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/manifest/roadmap.md` — Card 22: amended the Someday `reed: watchdog daemon` entry in place.
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/tools/sandbox/SANDBOX-REED-SUITE.md` — Card 23: added scenario M26 and the M25/M26 session-log lines.
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/watchdog_integration_test.go` — Card 24: new `integration && linux` live pty proof (grow/shrink/burst/degraded/failure-survival/focus/no-self-trigger/hook-probe).
- `/home/knatte/Code/loomyard/wts/reed-watchdog-daemon/internal/reedengine/watchloop_test.go` — in-scope fix: replaced an unresolvable `time.Sleep(2 * timing.Quiet)` with an equivalent literal so `go vet -tags integration` compiling this file for the first time doesn't trip the tier-purity sleep guard.

Verify command (`go test ./internal/reedcli/... ./internal/reedengine/... ./cmd/lyx/... && go vet -tags integration ./internal/reedengine/...`) passes in full. 6 of 6 cards committed this turn.
