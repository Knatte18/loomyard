All the findings have been successfully addressed:

## Summary of Fixes Applied

**Finding 1: integration_test.go calls deleted board.AtomicWrite** FIXED
- Extended batch 2 (card 3) to include `internal/board/boardtest/integration_test.go` in the Edits scope
- Added `fsx` import to the integration test; replaced `board.AtomicWrite` with `fsx.AtomicWrite`

**Finding 2: state_test.go imports encoding/json beyond plan's stated stdlib subset** FIXED
- Updated batch 4 (card 9) requirements to permit `encoding/json`

**All verify commands pass:**
- `go test ./internal/fsx/...`
- `go test ./internal/board/...`
- `go test ./internal/muxpoc/...`
- `go test ./internal/state/...`

{"status":"success","commit_sha":"781c2fab4e93f3d92636bb0e441e77490ccd0b56","session_id":"454c3f35-eb85-4880-94e4-13d3781329b6"}
