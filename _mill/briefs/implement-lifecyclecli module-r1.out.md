All 8 of 8 cards committed and verified: paths.go/paths_test.go (card 21), refusal.go/refusal_test.go (card 22), wire.go (card 23), cli.go (card 24), run.go (card 25), status.go (card 26), cli_test.go/run_test.go/wire_test.go/testmain_test.go (card 27), lifecycle_integration_test.go/testmain_integration_test.go (card 28). Both halves of the batch verify command pass. Working tree is clean.

Key files touched, all under /home/knatte/Code/loomyard/wts/worktree-lifecycle-shed-producers/internal/lifecyclecli/: paths.go, paths_test.go, refusal.go, refusal_test.go, wire.go, cli.go, run.go, status.go, cli_test.go, run_test.go, wire_test.go, testmain_test.go, lifecycle_integration_test.go, testmain_integration_test.go.

Notable implementation decisions worth flagging to the reviewer:
- run.go seeds the initial status row (state.UpdateJSON, idempotent) when none is persisted, since shedengine.Shed.Run never seeds one itself — this wasn't explicit in the card text but is required for the "absent status file is a fresh start" behavior to work.
- testmain_test.go needed a //go:build !integration constraint added after card 28 landed, to avoid two TestMain definitions colliding when -tags integration compiles both files into the same package.

{"status":"success","commit_sha":"510817abb13ba1634e5158ac330b3a2e9fe86610","session_id":"e1ec6b9a-d76e-48bc-add3-1cf51d0a847f","cards_done":[21,22,23,24,25,26,27,28]}
