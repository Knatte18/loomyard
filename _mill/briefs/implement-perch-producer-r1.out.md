Good, 40-char SHA (41 with newline): `357337e6279e5db7ff3fec9f5e1e1593a7abee80`.

All 3 of 3 cards committed and verified. Card-count self-check: 3 of 3 cards committed (Cards 5, 6, 7), matching the batch's declared 3 cards — full completion, no shortfall.

{"status":"success","commit_sha":"357337e6279e5db7ff3fec9f5e1e1593a7abee80","session_id":"be870bd6-8ef0-45c6-9c16-ddaf3b333a66","cards_done":[5,6,7]}

Summary of work done in this batch (perch-producer, batch 2 of 4):

- `/home/knatte/Code/loomyard/wts/shed-adapters/internal/shedadapters/perch.go` (new) — `PerchRunner` seam interface with compile-time proof against `*perchengine.Engine`, `PerchFactory` func type, `PerchProducer` struct with unexported fields, `NewPerchProducer` constructor validating `runIDPrefix`/`factory`/the three directory args, `resolveRunID` (hash-namespaced, highest-N, terminal-only advancement, mandatory `MkdirAll` before `TerminalOutcome` probe), and `Call` (entry check, run-id resolution, per-call factory/bridge, outcome mapping for Approved/Stuck/Paused/default, `cancelErr` consulted on every non-success path).
- `/home/knatte/Code/loomyard/wts/shed-adapters/internal/shedadapters/perch_test.go` (new) — in-package, untagged tests driven by `fakePerchRunner`/`fakeFactory`, covering the outcome mapping table, context/bridge rows, run-id advancement (including highest-N-not-first-gap and profile-hash namespacing), and the remaining filesystem rows (absent scratch sibling, corrupt `state.json`, runDir/scratchDir pairing, invalid prefix, seam error propagation).

Verify: `go test ./internal/shedadapters/...` passes (all SingleLLMProducer tests from batch 1 plus the new PerchProducer tests). `go vet` and `gofmt -l` are clean. Working tree has no uncommitted tracked changes.

{"status":"success","commit_sha":"357337e6279e5db7ff3fec9f5e1e1593a7abee80","session_id":"be870bd6-8ef0-45c6-9c16-ddaf3b333a66","cards_done":[5,6,7]}