Perfect! All cards are complete and tested. Here's the summary:

## Implementation Complete

All three cards of the batch have been successfully implemented and committed:

### Card 1: sandbox Go tool + unit tests
- Created `tools/sandbox/main.go` — a package main tool that builds the dogfood Hub by invoking `lyx warp clone` as a subprocess
- Implemented flag parsing for required `-parent` (absolute path) and optional `-reset` (bool) flags
- Implemented decision logic to check Hub existence, handle reset, and clone via the on-PATH `lyx` binary
- Created testability seams (`cloneRun` and `removeAll` variables) for unit testing without network
- Created comprehensive table-driven unit tests in `tools/sandbox/main_test.go` covering:
  - Hub path computation from both absolute and relative parent paths
  - Clone runner invocation when Hub is absent
  - No-op success when Hub exists without reset
  - Hub removal and clone when reset is true
  - Error propagation from the clone runner
- All tests pass and the path-invariant enforcement test confirms no banned tokens (`os.Getwd` or `--show-toplevel`) are present

### Card 2: sandbox.cmd launcher
- Created `sandbox.cmd` at the repo root as a thin Windows launcher
- Hardcoded the machine-specific parent directory (`C:\Code`) in the launcher
- Launcher cd's to repo root, runs `go run ./tools/sandbox -parent C:\Code %*`, captures exit code, and restores cwd
- Included explanatory comment block documenting the separation of concerns

### Card 3: dogfood-hub docs
- Created `docs/dogfood-hub.md` documenting:
  - The two dedicated repos (host and weft) and the board URL derivation
  - Hub location at `C:\Code\lyx-test-HUB` (outside Loomyard)
  - The Hub topology (host, weft, and board subdirectories)
  - Prerequisites (GitHub wiki initialization and lyx on PATH)
  - Build/rebuild instructions (first build vs `-reset` option)
  - Purpose (testing lyx's core workflows)
  - Dedicated use note
- Updated `docs/overview.md` to add:
  - A new "Dogfood Hub" section (one paragraph summarizing the bench)
  - A bullet point in the "Other docs" list pointing to `dogfood-hub.md`

**Verify result:** All tests pass (unit tests for sandbox tool and path-invariant enforcement).

**Final commit SHA:** `5a2dbda99b056b8c4245f8b6bf3029ef6e789914`

{"status":"success","commit_sha":"5a2dbda99b056b8c4245f8b6bf3029ef6e789914","session_id":"93ab0359-8181-48cc-97e3-8169a06c8faa"}
