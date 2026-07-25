{"status":"success","commit_sha":"6e965b8b010e13cef096150656b9c9f30dbc96ec","session_id":"67bf4c1a-206a-4822-824e-ce13b3169b18"}

Summary: all 5 cards of batch `fabric-weft-git` are committed (verified by counting commits between the `mill-go: start batch` marker and HEAD — 5 card commits plus 1 plan-edit commit, matching all 5 declared `## Cards` entries). The batch verify command `go test -tags integration ./internal/fabricengine` passes cleanly on a fresh run.

Files created/modified (all paths relative to `/home/knatte/Code/loomyard/wts/fabric`):
- `internal/fabricengine/index.go`, `internal/fabricengine/index_integration_test.go` (card 22)
- `CONSTRAINTS.md` (Weft Git Invariant amendment, card 22)
- `internal/fabricengine/weftgit.go` (card 23; further edited in card 25 to fix a discovered parity gap)
- `internal/fabricengine/syncweft.go`, `internal/fabricengine/revert.go`, `internal/fabricengine/revert_test.go` (card 24)
- `internal/fabricengine/weftgit_differential_test.go` (card 25)
- `internal/fabricengine/syncweft_integration_test.go` (card 26)
- `_mill/plan/05-fabric-weft-git.md` (scope-extension commit for the weftgit.go fix discovered while writing card 25's differential test)

Two implementation-discovery notes worth flagging for review:
1. `gitrepo.StageAndCommit` doesn't tolerate git's "did not match any files" add failure the way `weftengine.Commit` does; I fixed the tolerance at the `CommitWeft` layer (in scope, `weftgit.go`) rather than touching the shared `gitrepo` primitive (out of batch scope), following the plan-edit protocol.
2. The differential test's "broken-remote push" case documents (rather than forces false parity on) a genuine, pre-existing difference: `gitrepo.Push`'s error intentionally includes raw git stderr (per `gitrepo/doc.go`), unlike `weftengine.Push`'s hand-rolled no-leak error — each side is asserted against its own real, already-shipped contract.
