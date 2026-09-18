All 4 of 4 cards committed and verified this turn (cards 24, 25, 26, 27 — card 27's commit set includes two commits since a discovery mid-card required extending scope to `attach_test.go` and `watchdog_integration_test.go`, both added to the plan file first). Both verify sub-invocations (`go test ./internal/reedengine/` and `go test -tags integration ./internal/reedengine/`) pass. Working tree is clean.

Relevant files:
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/internal/reedengine/windowsize.go`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/internal/reedengine/windowsize_test.go`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/internal/reedengine/attach.go`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/internal/reedengine/attach_test.go`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/internal/reedengine/attachgeometry_integration_test.go`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/internal/reedengine/watchdog_integration_test.go`
- `/home/knatte/Code/loomyard/wts/reed-header-selvage/_mill/plan/04-status-line-pins.md`

{"status":"success","commit_sha":"c2608520b44c0552475c5b9236e69fe731436a59","session_id":"e660438e-bc71-4acd-95e4-6eb8123352b5","cards_done":[24,25,26,27]}
