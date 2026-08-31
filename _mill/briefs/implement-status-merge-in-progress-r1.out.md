2 of 2 cards committed — all cards complete. Both `go test ./internal/fabriccli/...` and `go test -tags integration ./internal/fabriccli/...` pass.

{"status":"success","commit_sha":"001dfbd0532e8d6dda26c810c239fdba738e1e30","session_id":"d5267f88-d1a6-4837-91bf-716834262ae9","cards_done":[1,2]}

Summary: 2 of 2 cards committed and verified.

- Card 1: `/home/knatte/Code/loomyard/wts/fabric-status-merge-in-progress/internal/fabriccli/status_mergeinprogress_integration_test.go` (new, commit `898d2a29c`) — pins `merge_in_progress` on the `status` envelope for the clean-pair and parked-merge cases.
- Card 2: `/home/knatte/Code/loomyard/wts/fabric-status-merge-in-progress/internal/fabriccli/weft_verbs.go` (statusCmd RunE + Long), `/home/knatte/Code/loomyard/wts/fabric-status-merge-in-progress/internal/fabricengine/doc.go`, `/home/knatte/Code/loomyard/wts/fabric-status-merge-in-progress/docs/overview.md`, `/home/knatte/Code/loomyard/wts/fabric-status-merge-in-progress/tools/sandbox/SANDBOX-FABRIC-SUITE.md`, `/home/knatte/Code/loomyard/wts/fabric-status-merge-in-progress/manifest/roadmap.md` (commit `001dfbd05`) — implements the field and lands all doc/roadmap updates in one commit.

Both `verify:` sub-invocations passed: `go test ./internal/fabriccli/...` and `go test -tags integration ./internal/fabriccli/...`. Working tree is clean (no uncommitted tracked changes) except the untracked brief file, which is expected.

{"status":"success","commit_sha":"001dfbd0532e8d6dda26c810c239fdba738e1e30","session_id":"d5267f88-d1a6-4837-91bf-716834262ae9","cards_done":[1,2]}
