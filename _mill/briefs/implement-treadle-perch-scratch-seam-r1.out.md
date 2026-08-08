Confirmed full SHA: `463514772ae8a689f8a877a4e8b738f07d558f6f`.

All 6 cards (9-14) of the treadle-perch-scratch-seam batch are committed. Verify passed (`go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/...` and `go test -tags integration ./internal/perchcli/...` both green), and `go build ./... && go vet -tags integration ./...` (the overview's module-wide check) also passed. Working tree is clean.

Key files touched:
- `/home/knatte/Code/loomyard/wts/dotlyx-scratch-hygiene/internal/treadleengine/engine.go`, `run.go`, `state.go`, `state_test.go`, `engine_test.go`
- `/home/knatte/Code/loomyard/wts/dotlyx-scratch-hygiene/internal/perchengine/identity.go`, `engine.go`, `identity_test.go`, `run_test.go`
- `/home/knatte/Code/loomyard/wts/dotlyx-scratch-hygiene/internal/perchcli/cli.go`, `run.go`, `pause.go`, `run_test.go`, `cli_integration_test.go`, `run_integration_test.go`
- `/home/knatte/Code/loomyard/wts/dotlyx-scratch-hygiene/_mill/plan/02-treadle-perch-scratch-seam.md` (plan extended per STOP protocol, for `run_integration_test.go`)

One discovered/fixed bug worth flagging to the reviewer: `perchcli/pause.go` originally called `perchengine.TerminalOutcome(runDir, scratchDir)` before `os.MkdirAll(scratchDir, ...)`; `TerminalOutcome`'s read-lock acquisition needs the scratch directory to already exist, so I reordered the MkdirAll ahead of the TerminalOutcome call (caught by the widened integration tests in card 14).

{"status":"success","commit_sha":"463514772ae8a689f8a877a4e8b738f07d558f6f","session_id":"842032db-b6ba-4992-8a61-8abae95b5b59","cards_done":[9,10,11,12,13,14]}
