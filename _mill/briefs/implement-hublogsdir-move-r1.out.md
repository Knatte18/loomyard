No dirty tracked files. All 3 cards committed (3 of 3 cards). Verify passed fully.

Summary: All 3 cards for the hublogsdir-move batch are complete and committed (3 of 3 cards committed). Batch verify (`go test ./internal/fabricengine/... ./internal/reedengine/... ./cmd/lyx/... && go vet -tags smoke ./internal/reedcli/...`) passes.

Files touched:
- `/home/knatte/Code/loomyard/wts/shuttle-reed-told-geometry/internal/fabricengine/junctionnames.go` (added `HubLogsDir`)
- `/home/knatte/Code/loomyard/wts/shuttle-reed-told-geometry/internal/fabricengine/hubscratch_test.go` (new test, retargeted/renamed test)
- `/home/knatte/Code/loomyard/wts/shuttle-reed-told-geometry/internal/reedengine/lifecycle.go` (removed `HubLogsDir`, retargeted caller, dropped `lyxcwd` import)
- `/home/knatte/Code/loomyard/wts/shuttle-reed-told-geometry/internal/reedcli/smoke_debuglog_test.go` (retargeted both callers)
- `/home/knatte/Code/loomyard/wts/shuttle-reed-told-geometry/cmd/lyx/constructoranchoring_test.go` (retargeted both rows, import, header comment)
- `/home/knatte/Code/loomyard/wts/shuttle-reed-told-geometry/manifest/designs/producers-standalone.md` (corrected Location-consumption table row)

{"status":"success","commit_sha":"67975ffee9f06dcaecc617a64dda9e31e74d4f0e","session_id":"350689d7-d82d-4fff-8b26-dac267a54e9a","cards_done":[1,2,3]}
