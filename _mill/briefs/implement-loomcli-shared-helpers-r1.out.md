All 4 cards in the loomcli-shared-helpers batch are committed (cards 6, 7, 8, 9), each with its own commit, and `go test ./internal/loomcli/...` passes cleanly with no uncommitted tracked changes.

Files touched:
- `/home/knatte/Code/loomyard/wts/loom-step/internal/loomcli/sharedbootstrap.go` (created)
- `/home/knatte/Code/loomyard/wts/loom-step/internal/loomcli/run.go` (edited)
- `/home/knatte/Code/loomyard/wts/loom-step/internal/loomcli/drive.go` (edited)
- `/home/knatte/Code/loomyard/wts/loom-step/internal/loomcli/sharedbootstrap_test.go` (created)

One note for the reviewer: in `sharedbootstrap_test.go`, `TestSeedAndCommitBootstrap_SecondCallDoesNotDivergeOnErrSeedExists` cannot reach `bootstrapStageSeed`/`bootstrapStageCommit` as the card's prose describes, because `seedAndCommitBootstrap`'s first sub-step (`fabricengine.ReadOrigin`) always spawns a real `git` subprocess even to fail fast on a non-repo directory — reaching further requires a real git-backed fabric, which the `new-tests-stay-untagged-and-pure` Shared Decision bars from this untagged suite. I adapted the test to assert idempotency at the stage actually reachable (`bootstrapStageOrigin`) rather than fabricate the deeper stage, and documented this in the test's own comment. `TestBuildLoomShed_OutputShape` uses the card's own explicitly-granted `t.Skip` escape hatch for the same underlying reason (`fabricengine.Open` needs a real fabric).

{"status":"success","commit_sha":"7bfe016b568fc02b0f805dad364d23799aa057f3","session_id":"b25c7dde-f2b4-4d17-a5f2-3efa160b6695","cards_done":[6,7,8,9]}
