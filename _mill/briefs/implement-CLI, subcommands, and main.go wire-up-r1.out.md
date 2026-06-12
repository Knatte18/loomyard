Excellent! All tests pass. Let me generate the final report:

## Summary

I have successfully implemented all 7 cards of Batch 2 (CLI, subcommands, and main.go wire-up):

**Card 5:** Created `cli.go` with the canonical `Config` struct and implemented `RunCLI` with full flag parsing for:
- `--psmux`, `--pwsh`, `--claude`, `--launch`, `--resume`, `--width`, `--height`, `--interval`
- Subcommand routing to `up`, `review`, `attach`, `status`, `down`, `daemon`
- Created `daemon.go` placeholder for Batch 3
- Removed stub `Config` from `cmd.go`

**Card 6:** Implemented `up.go` with:
- `coldStart`: spawns new psmux session, waits for it to come up, launches primary claude instance, saves state
- `coldRecover`: reconnects to existing session, restarts all panes from saved state, handles review pane splitting

**Card 7:** Implemented `review.go`:
- Adds new reviewer pane via split-window (30% height)
- Launches review claude instance with unique session-id
- Appends pane to state and persists

**Card 8:** Implemented `attach.go`:
- Launches session in maximized terminal via `spawnAttach`
- Validates session is running before attaching

**Card 9:** Implemented `status.go`:
- Returns comprehensive status with all 7 required fields
- Reports state existence, server status, session metadata, and live pane information

**Card 10:** Implemented `down.go`:
- Idempotent teardown: kills server and deletes state
- Distinguishes intentional shutdown from crash recovery

**Card 11:** Wired muxpoc into `main.go`:
- Added muxpoc import and switch case
- Updated module documentation

All tests pass: `go build ./...` and `go test ./internal/muxpoc/ ./cmd/mhgo/`

{"status":"success","commit_sha":"01a91461ac9e21953abbaf0a538dd581f34bf59f","session_id":"b5d549b1-8562-4b98-b0cb-efcac69a255c"}
