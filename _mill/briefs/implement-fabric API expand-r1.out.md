No uncommitted tracked changes. All 4 cards committed, matching the batch's declared 4 cards. Verify passes.

All 4 cards for batch 01 (fabric API expand) are committed and verified: `go test -tags integration ./internal/fabricengine/` passes.

Files created:
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/open.go` and `open_integration_test.go` (Card 1: `Open(l)`)
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/ready.go` and `ready_integration_test.go` (Card 2: `Ready(l)`)
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/committed_test.go` and `committed_lyxonly_integration_test.go` (Card 3: `CommitResult.Committed()`, plus an edit to `internal/fabricengine/commit.go`)
- `/home/knatte/Code/loomyard/wts/fabric-weft-visibility-cleanup/internal/fabricengine/refscanner.go` and `refscanner_test.go` (Card 4: `RefScanner`)

Card-count self-check: 4 of 4 cards committed (matches the batch's declared 4 cards).

{"status":"success","commit_sha":"14cc70cbc5d50dd65eb22ebae9bf6ab917be96f7","session_id":"871e7dd3-7358-43a8-92ac-058ce435f28f","cards_done":[1,2,3,4]}
