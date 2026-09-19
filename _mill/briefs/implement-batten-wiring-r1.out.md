All 6 cards (21-26) committed successfully, verify passed, working tree clean.

{"status":"success","commit_sha":"78c64550acac55aa497481868c6f24002729b8fb","session_id":"c07581ee-2c45-4d91-82e1-2adb29df2066","cards_done":[21,22,23,24,25,26]}

Summary: all 6 of 6 cards in batch `06-batten-wiring.md` are committed and verified. Two plan-scope extensions were made along the way (both committed as separate `plan:` commits before the corresponding code fix, per protocol): `internal/battencli/run_test.go` (card 23 -- pre-existing `SeedChild` gap surfaced by batch 5's recipe registration) and `internal/battencli/cli_test.go` (card 25 -- the exact-verb-set assertion predating step registration).

Key files touched: `internal/battencli/paths.go`, `wire.go`, `commitstatus.go` (+`commitstatus_test.go`), `arm.go` (+`arm_seed_test.go`), `cli.go` (+`cli_test.go`), `step_test.go`, `run_test.go`, `wire_test.go`, `paths_test.go`, and `CONSTRAINTS.md`. Plan file `_mill/plan/06-batten-wiring.md` was amended twice for the scope extensions above.

{"status":"success","commit_sha":"78c64550acac55aa497481868c6f24002729b8fb","session_id":"c07581ee-2c45-4d91-82e1-2adb29df2066","cards_done":[21,22,23,24,25,26]}
