All 4 of 4 cards committed, matching every `Commit:` message declared in `02-debug-logging.md` (Card 3, Card 4, Card 5, Card 6). Verify passed, plus the smoke test and contract canary were run once as required by Batch Tests, both passing.

{"status":"success","commit_sha":"8da3579e","session_id":"3180941e-496a-41e5-87f9-1a451f0aa3b0"}

Summary (4 of 4 cards committed):

- `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/hubgeometry/hubgeometry.go` — added `Layout.HubLogsDir()`.
- `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/hubgeometry/hubgeometry_unit_test.go` — added `TestHubLogsDir`.
- `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxengine/config.go` — added `Config.DebugLog`.
- `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxengine/config_test.go` — added the `DebugLog == "0"` template-default assertion.
- `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxengine/template.go`, `template_posix.yaml`, `template_windows.yaml` — added `debug_log` key.
- `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxengine/serverlog.go` (new) — `debugLogArgs` and `planLogPrune` pure helpers.
- `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxengine/serverlog_test.go` (new) — `TestDebugLogArgs`, `TestPlanLogPrune`.
- `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxengine/lifecycle.go` — boot-time log-dir creation/prune, debug-flag argv wiring, `-c` cwd pin, and reconciled stale cwd-ownership prose.
- `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxengine/doc.go` — documented the `-v`/`-vv` contract-surface addition.
- `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxcli/up.go` — `Long` now documents `debug_log`/`LYX_MUX_DEBUG`.
- `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxcli/smoke_debuglog_test.go` (new) — `TestSmokeDebugLog`, verified locally with `LYX_MUX_PSMUX=/usr/bin/tmux go test -tags smoke ./internal/muxcli/ -run TestSmokeDebugLog -v -count=1` (PASS) and `TestMultiplexerContract` (PASS, `-tags integration`).

Verify command `go test ./internal/hubgeometry/... ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...` and `go vet ./...` both pass; working tree has no uncommitted tracked changes.

{"status":"success","commit_sha":"8da3579e","session_id":"3180941e-496a-41e5-87f9-1a451f0aa3b0"}