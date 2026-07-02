All 7 cards of batch 06-muxcli implemented and committed: cli.go (muxCLI receiver, Command()/RunCLI seam, PersistentPreRunE, 7 verbs), up.go, add.go, remove.go, status.go, resume.go, attach.go (attachArgv pure fn), cli_test.go, smoke_test.go. Note: muxCLI carries cfg muxengine.Config alongside eng because attach needs the psmux path. go test ./internal/muxcli/... passes; go build ./... and go vet clean (default + -tags smoke); gofmt clean.

{"status":"success","commit_sha":"defe2a4884259f2c9585b91f1b8c58e292c90cf7","session_id":"fded9050-8f4e-46ef-93d9-64a8e0dae6a4"}
