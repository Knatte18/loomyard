Clean working tree, no uncommitted tracked changes. 4 commits cover all 5 cards (27+28 combined into one commit, 29, 30, 31 each their own). All 5 cards from the batch file are committed, and verify passes.

{"status":"success","commit_sha":"b25498bb6edb28f7979bd0ee9ac5f8492c50a61c","session_id":"50be3cfd-9742-424b-bed8-fcb1eddfa747","cards_done":[27,28,29,30,31]}

Summary: 5 of 5 cards committed (cards 27 and 28 combined into one commit since both touched `cmd/lyx/main.go`, per the batch's combining allowance; cards 29, 30, 31 each got their own commit). Batch verify (`go test -tags integration ./cmd/lyx/... ./internal/logger/...`) passes.

Files touched:
- `/home/knatte/Code/loomyard/wts/trace-logging/cmd/lyx/main.go` (root `PersistentPreRunE` mint/adopt/export + arm under `!testing.Testing()`; `main()`/`run()` capture exit code and call `logger.NotifyExit`)
- `/home/knatte/Code/loomyard/wts/trace-logging/cmd/lyx/main_test.go` (`TestRootHookSuppressedUnderTest`)
- `/home/knatte/Code/loomyard/wts/trace-logging/cmd/lyx/main_integration_test.go` (`TestRootHookWritesTraceFileOnNonZeroExit`, `buildLyxBinary`, `traceFilenamePattern`)
- `/home/knatte/Code/loomyard/wts/trace-logging/internal/logger/logger.go` (package doc rewrite: trace/span model, durable sink + retention, `LYX_TRACE_ID`/`LYX_TRACE=1`, level policy, dual-handler-duplication note)

{"status":"success","commit_sha":"b25498bb6edb28f7979bd0ee9ac5f8492c50a61c","session_id":"50be3cfd-9742-424b-bed8-fcb1eddfa747","cards_done":[27,28,29,30,31]}
