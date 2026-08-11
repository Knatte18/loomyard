{"status":"success","commit_sha":"47d231a7ba3ec00273abbac25de1d093035979ac","session_id":"5b4f4b2e-ecb1-4e25-be04-af46346bb1a6","cards_done":[32,33,34]}

Card-count self-check: 3 of 3 cards declared in this batch (32, 33, 34) are committed, each with its exact declared `Commit:` message, confirmed against `git log edce0bf2..HEAD --oneline`. All complete.

Summary of work:

- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/07-gap-integration-tests.md`: extended each card's Context/Edits to name `internal/fabricengine/export_test.go`, per the mid-batch discovery that several gate predicates needed direct test-only exports (STOP-and-extend-plan procedure), committed separately before the code change.
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/export_test.go`: added `RemoveWarpWorktreeDirForTest`, `TeardownHubForTest`, `LooksLikeHubForTest`, `IsWarpCheckoutForTest`, `IsRegisteredLinkedWorktreeInForTest`, `WorktreeDirtyTrackedForTest`/`WorktreeDirtyAllForTest`, and `DeleteBranchForTest` — thin re-exports following the file's existing `NewPairedForTest`/`WarpForTest` idiom, needed because the corresponding application-level verbs (`Topology.Remove`, `CloneHub --reset`, `Topology.Cleanup`, `ResetHard`) each short-circuit before ever reaching the specific gate state this batch needed to prove.
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/destructivegaps_integration_test.go` (new file, `//go:build integration`, `package fabricengine_test`):
  - Card 32 — one test per newly-closed gap (junction-removal containment via `UnwireJunctions` and via Add's rollback; hub-teardown containment vs. its token-based half-built-hub counter-case; the warp-worktree-removal fallback's dirtiness check against a registered worktree with an untracked file).
  - Card 33 — the registered-linked-worktree, warp-checkout, and fabric-hub ownership kinds against every shape named in the plan, plus both dirtiness scopes asserted separately across four worktree states.
  - Card 34 — the branch-ownership kind's four refusal shapes (each under force=false and force=true), its positive case, and the cross-site property driven end-to-end through Add's rollback.

Verify (`go test -tags integration ./internal/fabricengine/...`) passes, as does the untagged tier and `go build ./...`. Working tree is clean (only the mill-managed `_mill/briefs/...` file is untracked, which is out of this task's scope).

{"status":"success","commit_sha":"47d231a7ba3ec00273abbac25de1d093035979ac","session_id":"5b4f4b2e-ecb1-4e25-be04-af46346bb1a6","cards_done":[32,33,34]}
