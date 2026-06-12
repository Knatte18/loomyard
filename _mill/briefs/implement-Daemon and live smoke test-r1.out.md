Perfect! All work is complete. Here's a summary of what was implemented:

## Implementation Summary

### Card 12: Daemon Subcommand
- **File**: `internal/muxpoc/daemon.go`
- Replaced the placeholder implementation with a fully functional daemon subcommand
- Implements a foreground polling loop using `time.Ticker` with configurable interval
- Includes crash-loop guard with constants: `maxRecoveries = 3`, `windowDur = 60 * time.Second`
- Maintains daemon-process-local recovery timestamp ring that resets on daemon restart
- Sets up OS signal handling for graceful shutdown (Ctrl+C, SIGTERM)
- On signal: prints "daemon stopping" to stderr and returns success
- On tick: checks session health via `LoadState` and `mux.hasSession()`, attempts recovery on death
- Calls `coldRecover` with `io.Discard` to suppress JSON output (appropriate for long-running daemon)
- Returns error on unrecoverable state issues

### Card 13: Live Smoke Test
- **File**: `internal/muxpoc/muxpoc_smoke_test.go`
- Created build-tagged test file with `//go:build smoke` constraint (skipped in normal CI)
- Implements `TestSmokeFullLifecycle` that tests complete workflow:
  1. Skips on non-Windows platforms (requires psmux)
  2. Skips if psmux binary not found
  3. Uses cheap placeholder commands: `Write-Host ready; Read-Host` instead of real claude
  4. Tests: up (fresh), status, review, kill-server (crash), up (cold recover), down
  5. Validates all JSON responses and field presence
  6. Verifies session recovery after simulated crash
  7. Ensures state cleanup after down
- Test runs via: `go test -tags smoke -v ./internal/muxpoc/`

Both cards have been committed with proper commit messages and the `go build ./...` verify command passes successfully.

{"status":"success","commit_sha":"674c965f73cd46d4d773c2bbf95f605d2a8e53df","session_id":"9492d633-9cd3-4ce1-93e8-6feaa61163a0"}
