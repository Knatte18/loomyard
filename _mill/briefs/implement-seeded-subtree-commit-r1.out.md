Working tree is clean, and all 3 cards (9, 10, 11) have matching commits since the batch start commit. 3 of 3 cards committed — all complete.

{"status":"success","commit_sha":"61be1d3b14514fdbcfe489a0e5c88836fc763d55","session_id":"30ba313e-6217-4a6c-a1de-0983e7454a68","cards_done":[9,10,11]}

Summary: 3 of 3 cards committed (cards 9, 10, 11). Verify passed: `go test ./internal/fabricengine/... ./internal/stencilcli/... ./cmd/lyx/...` and `go test -tags integration ./internal/fabricengine/...` both green.

Files touched:
- `/home/knatte/Code/loomyard/wts/deploy-specs-like-stencils/internal/fabricengine/stencilcommit.go` (generalised `CommitSeededStencils`, added `StencilsSubtreeRel`/`SpecsSubtreeRel`)
- `/home/knatte/Code/loomyard/wts/deploy-specs-like-stencils/cmd/lyx/stencilseed.go` (caller updated)
- `/home/knatte/Code/loomyard/wts/deploy-specs-like-stencils/internal/stencilcli/cli.go` (caller updated)
- `/home/knatte/Code/loomyard/wts/deploy-specs-like-stencils/internal/fabricengine/stencilcommit_integration_test.go` (existing tests updated, new `TestCommitSeededStencils_SecondSubtreeCommitsAndRecordsItsOwnDirectory` added)
- `/home/knatte/Code/loomyard/wts/deploy-specs-like-stencils/internal/fabricengine/stencilhistory_integration_test.go` (`seedStencil` helper updated)

{"status":"success","commit_sha":"61be1d3b14514fdbcfe489a0e5c88836fc763d55","session_id":"30ba313e-6217-4a6c-a1de-0983e7454a68","cards_done":[9,10,11]}