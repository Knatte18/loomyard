40 chars + newline = 41, correct full SHA: `1204b1567f0ed3567ec1a506a2d7177074eb3d1d`.

{"status":"success","commit_sha":"1204b1567f0ed3567ec1a506a2d7177074eb3d1d","session_id":"c24e1594-960c-4ea0-82e4-0d7dfef85729","cards_done":[6,7,8]}

All 3 of 3 cards in this batch (Cards 6, 7, 8) were committed and verified. Summary:

- Card 6 (commit `edaea777`): `internal/batcher/config.go` — `Active` now uses `configengine.LoadOrTemplate` instead of `Load`, dropped the unreachable "not initialized" rewrap branch and unused `strings` import, rewrote the doc comment to match `websterengine`/`reedengine`'s `LoadOrTemplate` shape.
- Card 7 (commit `3683bc7e`): `internal/batcher/config_test.go` — replaced the two strict-error tests with degrading-fallback tests (absent `_lyx/`, absent `batcher.yaml`) asserting the identity batchifier resolves with nil error; updated stale comments referencing the old `configengine.Load` path.
- Card 8 (commit `1204b156`): `CONSTRAINTS.md` — moved `batcher` into the Config Strictness Invariant's degrading pinned set and out of the strict set; rewrote the "watch item for T7/T10" bullet to record the move as fired by this task.

Verify (`go test ./internal/batcher/...`) passed. `go build ./...` also passed. Working tree is clean (no uncommitted tracked changes).

{"status":"success","commit_sha":"1204b1567f0ed3567ec1a506a2d7177074eb3d1d","session_id":"c24e1594-960c-4ea0-82e4-0d7dfef85729","cards_done":[6,7,8]}
