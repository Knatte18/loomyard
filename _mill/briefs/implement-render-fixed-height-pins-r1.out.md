Both card commits are present, matching the batch's declared 2 cards exactly (card 1's `feat(reedengine/render): add FixedHeightPins...` and card 2's `test(reedengine/render): pin FixedHeightPins...`). 2 of 2 cards committed — all complete.

{"status":"success","commit_sha":"67724ebda18296ddc15a226c289f63c807c4a3f2","session_id":"d9dc07e3-f1ef-4b36-9ea7-63603cb42dbb","cards_done":[1,2]}

Summary: Both cards of batch `01-render-fixed-height-pins.md` are committed (2 of 2), and `go test ./internal/reedengine/render/...` passes.

Files touched:
- `/home/knatte/Code/loomyard/wts/reed-attach-header-height-bug/internal/reedengine/render/layout.go` — added `strip bool` field to `placement`.
- `/home/knatte/Code/loomyard/wts/reed-attach-header-height-bug/internal/reedengine/render/height.go` — set `strip` from `isStrip[i]` in `stackHeights`.
- `/home/knatte/Code/loomyard/wts/reed-attach-header-height-bug/internal/reedengine/render/rules.go` — extracted `planCells`/`cellPlan`, rewrote `Rules` to consume it, added exported `Pin` and `FixedHeightPins`.
- `/home/knatte/Code/loomyard/wts/reed-attach-header-height-bug/internal/reedengine/render/pins_test.go` — new table-driven drift-guard test plus an ordering test.

{"status":"success","commit_sha":"67724ebda18296ddc15a226c289f63c807c4a3f2","session_id":"d9dc07e3-f1ef-4b36-9ea7-63603cb42dbb","cards_done":[1,2]}