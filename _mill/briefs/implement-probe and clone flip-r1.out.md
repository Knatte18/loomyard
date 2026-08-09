All 5 cards (3, 4, 5, 6, 7) are committed — 5 of 5 cards complete, matching the batch's declared 5 cards. Verify passes cleanly.

{"status":"success","commit_sha":"d62efbd204a2d8f8270a4ea4f824307552446750","session_id":"78df627b-9b79-45fa-84dc-c80ac42b76ac","cards_done":[3,4,5,6,7]}

Summary: All 5 cards in batch "probe and clone flip" committed (5 of 5) — card 3 (warpprobe.go, commit 4e33c331), card 4 (CloneOptions refactor in clone.go, commit 0738d56c), card 5 (fabriccli/clone.go positional flip, commit 28add1f6), card 6 (sandbox call-site flip, commit 18617e31), card 7 (13 test call sites updated, commit d62efbd2). Verify command `go build ./... && go test -tags integration ./internal/fabricengine/` passes. Working tree is clean.

Key files touched:
- `/home/knatte/Code/loomyard/wts/fabric-warp-binding-in-weft/internal/fabricengine/warpprobe.go` (new)
- `/home/knatte/Code/loomyard/wts/fabric-warp-binding-in-weft/internal/fabricengine/clone.go`
- `/home/knatte/Code/loomyard/wts/fabric-warp-binding-in-weft/internal/fabriccli/clone.go`
- `/home/knatte/Code/loomyard/wts/fabric-warp-binding-in-weft/tools/sandbox/main.go`
- `/home/knatte/Code/loomyard/wts/fabric-warp-binding-in-weft/internal/fabricengine/clone_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-warp-binding-in-weft/internal/fabricengine/clone_adopt_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-warp-binding-in-weft/internal/fabricengine/boardjunction_integration_test.go`

{"status":"success","commit_sha":"d62efbd204a2d8f8270a4ea4f824307552446750","session_id":"78df627b-9b79-45fa-84dc-c80ac42b76ac","cards_done":[3,4,5,6,7]}