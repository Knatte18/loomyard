4 of 4 cards committed (card 6 as `02.1`, cards 7–9 combined into one commit named after the later card `02.4`, per the brief's same-file combined-commit allowance). Verify (`go test ./internal/builderengine/...`) passes. No uncommitted tracked changes remain.

Files touched:
- `C:\Code\loomyard\wts\plan-format-file-ops\internal\builderengine\plan.go` — added `PlanBatch.HasRenameMechanic` and the `hasHeading` helper.
- `C:\Code\loomyard\wts\plan-format-file-ops\internal\builderengine\plan_test.go` — added `TestParsePlan_HasRenameMechanic`.
- `C:\Code\loomyard\wts\plan-format-file-ops\internal\builderengine\validate.go` — added `checkMoveFormat`, `checkMoveRedundant`, `checkMoveSourceMissing`, `checkMoveTargetCollision`, `checkMoveMechanicMissing`, plus `createsUnion`/`movesTargetsUnion`/`pathExistsOnDisk` helpers, wired into `Validate`.
- `C:\Code\loomyard\wts\plan-format-file-ops\internal\builderengine\validate_test.go` — added coverage for all five new checks.
- `C:\Code\loomyard\wts\plan-format-file-ops\internal\builderengine\runlevel_test.go` — fixed `newRunFixture`'s `WorktreeRoot` (was an unrelated `t.TempDir()`) to point at the copied `planDir`, required by the new on-disk move checks; this file was added to card 8's `Edits:` via a separate plan-edit commit first, per protocol.
- `C:\Code\loomyard\wts\plan-format-file-ops\_mill\plan\02-move-checks.md` — plan-edit commit extending card 8's scope.

{"status":"success","commit_sha":"71b6242","session_id":"8751b115-3bcf-4998-9396-de181414072a"}
