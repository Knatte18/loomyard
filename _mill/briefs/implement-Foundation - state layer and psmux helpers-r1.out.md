Perfect! I have successfully completed the Foundation batch. Let me summarize what was implemented:

## Summary

I've successfully implemented all 4 cards of the Foundation batch for the mhgo mux module:

**Card 1: State types, load, save, delete, UUID**
- Created `internal/muxpoc/state.go` with complete state persistence layer
- Implemented `MuxpocState` and `Pane` types with JSON serialization
- Implemented `LoadState`, `SaveState`, `DeleteState` with atomic writes and advisory locking
- Implemented UUID v4 generation via `newSessionID()`
- Implemented environment sanitization (`sanitizeEnv`, `strippedEnvKeys`) to remove Claude Code variables
- Implemented `socketName()` for deriving stable socket names from directory paths

**Card 2: State unit tests**
- Created `internal/muxpoc/state_test.go` with 8 comprehensive test functions
- `TestSanitizeEnv`, `TestStrippedEnvKeys`, `TestSocketName` verify core utility functions
- `TestLoadStateMissing`, `TestLoadStateCorrupt` verify error handling and edge cases
- `TestSaveLoadRoundtrip` verifies atomic save/load consistency
- `TestNewSessionID` verifies UUID v4 format and uniqueness
- `TestDeleteStateMissing` verifies idempotent deletion

**Card 3: Build-tagged spawn helpers**
- Created `internal/muxpoc/spawn_windows.go` with build tag `//go:build windows`
  - `spawnServer()` launches psmux windowless with `CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP`
  - `spawnAttach()` launches Windows Terminal or falls back to plain psmux
- Created `internal/muxpoc/spawn_other.go` with build tag `//go:build !windows`
  - `spawnServer()` launches with `Setsid` for process group isolation
  - `spawnAttach()` runs psmux interactively with inherited stdio

**Card 4: Psmux command helpers**
- Created `internal/muxpoc/cmd.go` with low-level psmux operations
- Implemented `Config` struct with paths and dimensions
- Implemented `PsmuxCmd` wrapper with methods:
  - `run()` executes psmux commands, discarding output
  - `output()` executes and captures output
  - `hasSession()` checks session existence (handles exit code 1 specially)
  - `listPanes()` parses pane list from psmux format
- Implemented `LivePane` struct for pane metadata
- Implemented helper functions `socketArg()` and `expandTpl()`

All 8 unit tests pass successfully. The code follows Go idioms, uses proper error handling, and integrates cleanly with existing patterns in the codebase (lock, board, output packages).

{"status":"success","commit_sha":"c29b338c8ed3fd2cfd6df564f2ec84f14be84785","session_id":"a27535f7-3812-45d8-9688-c314d4ef56e8"}
