All 4 of 4 cards committed, matching the batch's declared 4 cards. All commits match the batch's `Commit:` messages for cards 23, 24, 25, and 26 respectively.

Summary of work completed (4 of 4 cards committed):

- **Card 23**: `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/cleanup.go` — `deleteWeftBranch` now routes through the gate's `deleteBranch` executor.
- **Card 24**: `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/checkout.go` — `rollbackSwitch`'s forked-branch deletion now routes through `deleteBranch`, with a `*destructiveRefusal` logged via `logger.Warn` since the function stays void.
- **Card 25**: `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/add.go` — `Topology.Add` creates the warp worktree via `createGitWorktree`; `rollbackAdd` gained a `warpTok createdToken` parameter and now routes both its worktree removal and branch deletion through the gate.
- **Card 26**: `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/weftwiring.go`, `remove.go`, `add.go` — `removeWeftWorktree`'s branch deletion now routes through `deleteBranch`; its boolean parameter renamed `alsoDeleteBranch`; both call sites updated to pass their receiver's `BranchPrefix`.

During verify, two pre-existing integration tests (`TestAddRollback_AdoptedWeftBranchSurvives`, `TestAddRollback_UnwiresJunctionsOnPostWiringFailure` in `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/add_rollback_adopt_test.go`) failed because their fixtures used an empty `Config{}` (no `BranchPrefix`, so the gate could never recognize the warp branch as fabric-managed) and lacked a `_board` worktree (so `primaryWeftBranch` could not be read). Per the Test Integrity Guardrail's explicit allowance to fix a test that conflicts with the new design, I fixed both fixtures to set `BranchPrefix: "task/"` and to add the `_board` worktree (matching the existing `newFabricFixture` pattern used elsewhere in the package) — no assertions were weakened; both tests still assert the identical warp-branch-gone / weft-branch-preserved outcome, just checking against the correctly-prefixed branch name.

Verify passed: `go test ./internal/fabricengine/...` and `go test -tags integration ./internal/fabricengine/...` both green, including the two explicitly pinned tests `TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout` and `TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut`. Module-wide `go build ./...` is clean.

{"status":"success","commit_sha":"59917cd56bff892ef889cc84dc0c3a51d43731c4","session_id":"fe86e9cf-b666-4bb4-97ca-769f57f02b2b","cards_done":[23,24,25,26]}
