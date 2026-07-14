Correcting the commit sha (typo above) — the real HEAD is `81693be69e2da478613ebcee046978280de2cfa2`.

{"status":"success","commit_sha":"81693be69e2da478613ebcee046978280de2cfa2","session_id":"9ad52363-799b-4249-b058-92296a4890e2"}

Summary: 2 of 2 cards committed for batch `03-resume-hint`.

- Card 7 (`d7d59918`): Added `noSessionMessage(strandCount int)` and updated `requireSessionLocked` in `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxengine/lifecycle.go` to load persisted state and point at `lyx mux resume` when ≥1 strand is persisted, falling back to the old bare message on load error/absence. Added a table-driven test in `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxengine/lifecycle_test.go` and a new integration test `TestRunCLI_StatusNotUp_EnrichedResumeHint` in `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxcli/cli_integration_test.go`.
- Card 8 (`81693be6`): Reworded verbatim quotes of the old error string in `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxengine/strand.go` (AddStrand/UpdateStrand/RemoveStrand doc comments) and `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/muxcli/attach.go` to reference `requireSessionLocked`/`noSessionMessage` instead of pinning the exact string; added explanatory comments to the zero-strand before-up tests in `cli_integration_test.go`; verified and left unchanged `/home/knatte/Code/loomyard/wts/mux-server-crash/internal/builderengine/spawn_test.go`, `docs/research/linux-portability-survey.md`, `tools/sandbox/SANDBOX-MUX-SUITE.md`, and `tools/sandbox/SANDBOX-BUILDER-SUITE.md` (all describe fresh/zero-strand scenarios).

Verify command `go test ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...` passes, and the batch's designated integration tests (`FriendlyError` matches plus the new enriched-message test) all pass under `-tags integration`. Working tree is clean; both commits are pushed to `mux-server-crash`.

{"status":"success","commit_sha":"81693be69e2da478613ebcee046978280de2cfa2","session_id":"9ad52363-799b-4249-b058-92296a4890e2"}