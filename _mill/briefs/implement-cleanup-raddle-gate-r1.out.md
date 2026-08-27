All 3 of 3 cards committed and match the batch's declared cards exactly (Cards 18, 19, 20). Verify passed in full.

{"status":"success","commit_sha":"b0bd510aa52878b2d39b6a880eb1aa42d4e506eb","session_id":"b2882614-b190-4d09-ab62-d67c393cbec4","cards_done":[18,19,20]}

Summary: 3 of 3 cards committed this turn (Card 18: removed `raddleFoldedBack` and its gate branch from `Topology.Cleanup` in `internal/fabricengine/cleanup.go`; Card 19: corrected the `_lyx/raddle/` clause in `CLAUDE.md` and the `cleanup` flag help/Long text in `internal/fabriccli/fabric.go`; Card 20: reworked `TestCleanup_DryRunMatchesApplyVerdict` in `internal/fabricengine/reconcile_stale_registration_test.go` and added `internal/fabricengine/cleanup_raddlegate_integration_test.go` with three new integration tests). The batch's full verify command (`go build ./cmd/lyx && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...`) passed. Working tree is clean at commit `b0bd510a`, pushed to `weft-local-only-files`.

{"status":"success","commit_sha":"b0bd510aa52878b2d39b6a880eb1aa42d4e506eb","session_id":"b2882614-b190-4d09-ab62-d67c393cbec4","cards_done":[18,19,20]}
