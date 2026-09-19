All 7 declared cards match commits (plus one extra self-fix commit `68fefe6f1` addressing a tier-purity bug introduced in card 1, not a separate card). 7 of 7 cards committed. Full verify passes, no dirty tracked files.

{"status":"success","commit_sha":"be5c8f98e3b0a2beb740ef9705f2b478aa245dda","session_id":"7225ef42-5c92-4e6f-b500-eac9a3f48fc5","cards_done":[1,2,3,4,5,6,7]}

All 7 of 7 cards in batch `01-go-cli-rename.md` are committed (commits `737622e06` through `be5c8f98e`, plus one interstitial self-fix commit `68fefe6f1` correcting a tier-purity token-match bug my own card 1 introduced). Both verify sub-invocations (`go test ./internal/loomcli/... ./cmd/lyx/...` and `go test -tags smoke -run XXX_NONE ./internal/loomcli/...`) pass, gofmt reports no drift on any touched file, and `git status --porcelain` is clean.

Key files touched:
- `/home/knatte/Code/loomyard/wts/loom-cli-rename/cmd/lyx/helptree_test.go`, `retiredverbs_test.go` (new), `sandbox_coverage_test.go`, `main.go`
- `/home/knatte/Code/loomyard/wts/loom-cli-rename/internal/loomcli/cli.go`, `cli_test.go`, `start.go` (was `run.go`), `run.go` (was `drive.go`), `pause.go`, `status.go`, `step.go`
- `/home/knatte/Code/loomyard/wts/loom-cli-rename/internal/loomcli/smoke_test.go`, `smoke_bootstrapwiring_test.go`
- `/home/knatte/Code/loomyard/wts/loom-cli-rename/internal/loomcli/bootstrap.go`, `bootstrap_test.go`, `friction_test.go`, `wiring.go`, `sharedbootstrap.go`, `seedinput.go`

{"status":"success","commit_sha":"be5c8f98e3b0a2beb740ef9705f2b478aa245dda","session_id":"7225ef42-5c92-4e6f-b500-eac9a3f48fc5","cards_done":[1,2,3,4,5,6,7]}
