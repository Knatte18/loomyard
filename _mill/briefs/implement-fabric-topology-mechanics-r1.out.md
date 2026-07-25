5 of 5 cards committed and verified. All commit subjects match the batch file's `## Cards` `Commit:` messages exactly.

{"status":"success","commit_sha":"d70a34b7","session_id":"722842f1-5d6c-45f9-9563-ca39a099b44c"}

Summary of work completed for batch `fabric-topology-mechanics` (5 of 5 cards committed):

- Card 8 (`b9654e9b`): Created `internal/fabricengine/hostlayout.go`, `ancestors.go`, `ancestors_test.go`, `portals.go`, `launchers.go`, `launcher_content.go`, `launcher_content_test.go` — adapted copies of warpengine's filesystem/launcher mechanics; the `fabric-checkout<ext>` launcher invokes `lyx fabric checkout`.
- Card 9 (`22e18fdf`): Created `internal/fabricengine/junction.go`, `hook.go`, `post-checkout.sh`, `hook_test.go` — `FABRIC_SENTINEL` const, and the hook script computes `EXPECTED_WEFT_BRANCH="${HOST_BRANCH}-weft"` (shell-side data) instead of a literal branch-name compare. Tests assert both an in-sync suffixed state (no warning) and a diverged state (warning) for prime and child worktrees.
- Card 10 (`04d15d54`): Created `internal/fabricengine/weftwiring.go` — `pushWeftBranch` takes `fabricengine.SyncOptions` (already established in fabric-core with the same `SkipGit`/`SkipPush` shape as warp's `AddOptions`) rather than a new duplicate type; no branch-name composition in this file.
- Card 11 (`3fdff12a`): Created `internal/fabricengine/clone.go`, `clone_test.go` — `CloneHub` with board parity; new `suffixWeftPrimaryBranch` helper renames the weft primary onto `WeftBranchName(<its branch>)` after clone (e.g. `main` → `main-weft`). Own `RemoveAll` teardown seam.
- Card 12 (`d70a34b7`): Created `internal/fabricengine/clone_differential_test.go` (package `fabricengine_test`) — runs `warpengine.CloneHub` and `fabricengine.CloneHub` against shared local bare fixtures, asserting equivalent layout, `resolvedBoardURL` (derived + explicit), and the weft-primary-branch delta; plus strict-abort equivalence.

Verify command `go test -tags integration ./internal/fabricengine` passes. Working tree is clean (no uncommitted tracked changes).
