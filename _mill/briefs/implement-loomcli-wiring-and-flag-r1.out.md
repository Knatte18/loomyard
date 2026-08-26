All 4 of 4 cards (17, 18, 19, 20) are committed. Verify (`go test ./internal/loomcli/... ./cmd/lyx/...`) passes cleanly. No uncommitted tracked changes.

{"status":"success","commit_sha":"0443f13ea6991a60857c337667f3120d284afc00","session_id":"7ce79937-f8b7-4eb8-96d6-f5f2eb186ff7","cards_done":[17,18,19,20]}

Summary: 4 of 4 cards committed this turn (17, 18, 19, 20), matching the batch's declared card count.

Files touched:
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/loomcli/wiring.go` — filled `Env.ApprovePlan` closure.
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/loomcli/validate.go` — added `--require-approved` flag to `validate-plan`, rewrote `Long`.
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/loomcli/parity_test.go` — re-keyed `TestGateParity_PlanValidate` into a fixture x mode table.
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/loomcli/validate_test.go` — extended `TestValidatePlanCmd` to both modes, added flag-registration test.

{"status":"success","commit_sha":"0443f13ea6991a60857c337667f3120d284afc00","session_id":"7ce79937-f8b7-4eb8-96d6-f5f2eb186ff7","cards_done":[17,18,19,20]}
