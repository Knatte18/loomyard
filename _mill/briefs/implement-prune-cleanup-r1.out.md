All three cards have been successfully implemented and verified:

- **Card 11**: Merged `TestPrune_DryRunReportsStaleWeft` and `TestPrune_ApplyRemovesStaleWeft` into `TestPrune_StaleWeft`, which runs sequentially on a shared fixture to test both dry-run reporting and apply removal.

- **Card 12**: Merged `TestCleanup_DryRunReportsOrphanBranch` and `TestCleanup_ForceAloneReportsOnly` into `TestCleanup_ReportOnlyModes`, which tests both report-only flag combinations on a shared fixture.

- **Card 13**: Combined `TestCleanup_LiveBranchNeverDeleted` and `TestCleanup_LiveBranchNeverDeleted_NonEmptyBranchPrefix` into a single `TestCleanup_LiveBranchNeverDeleted`, preserving the prefix-mismatch regression coverage by testing both no-prefix and prefixed branch cases sequentially.

The verify command confirmed all tests pass:
- `go test -tags integration -run TestPrune ./internal/warp/` ✓
- `go test -tags integration -run TestCleanup ./internal/warp/` ✓

Files modified:
- `/internal/warp/prune_test.go` (Card 11)
- `/internal/warp/cleanup_test.go` (Cards 12 and 13)

{"status":"success","commit_sha":"0d66609c2b2c8688f4a35cfd2175c70edbda0beb","session_id":"a519ca6a-7624-468d-a779-f1d2573630b4"}
