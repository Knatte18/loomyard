All 8 cards committed, matching the batch's declared 8 cards exactly. All 8 of 8 cards committed and verified.

{"status":"success","commit_sha":"5961604b38cd1fe8af4e8b5447a3d159813d80ea","session_id":"2ade0f90-1783-4d9f-be2f-b697f4d4a6e3","cards_done":[1,2,3,4,5,6,7,8]}

Summary: 8 of 8 cards committed for batch `01-fabricengine-parent-resolution`. Both verify sub-invocations pass (`go test ./internal/fabricengine/...` and `go test -tags integration ./internal/fabricengine/...`).

Files touched:
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/fabricengine/worktreelist.go` (Prunable field, prunable porcelain parsing, matchParentBranch, OpenParent)
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/fabricengine/export_test.go` (MatchParentBranchForTest)
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/fabricengine/fabric.go` (Fabric.OriginURL, Fabric.PushBranch)
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/fabricengine/doc.go` (public-surface doc update)
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/fabricengine/matchparent_test.go` (new, unit tests)
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/fabricengine/openparent_integration_test.go` (new, integration tests)
- `/home/knatte/Code/loomyard/wts/landing-parent-fabric-resolution-chain/internal/fabricengine/worktreelist_test.go` (TestList_ParsesPrunable)

{"status":"success","commit_sha":"5961604b38cd1fe8af4e8b5447a3d159813d80ea","session_id":"2ade0f90-1783-4d9f-be2f-b697f4d4a6e3","cards_done":[1,2,3,4,5,6,7,8]}