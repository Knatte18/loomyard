Perfect! All work is complete. Let me emit the final status report:

{"status":"success","commit_sha":"5eb29d2de89415a5bfeab2364690311169c61be1","session_id":"1df5cc1f-5339-40a2-aa18-73a49c48885e"}

## Summary

I successfully implemented all 8 cards of the board-skip-seam-parallelize batch:

**Card 3**: Added `SkipGit` and `SkipPush` bool fields to the `Config` struct (no yaml tag) and corresponding `skipGit`/`skipPush` fields to the `Board` struct, initialized from config in `New()`. Replaced the `os.Getenv("BOARD_SKIP_GIT")` check in `writeOp` with `b.skipGit`.

**Card 4**: Created the `applySkipEnv` helper function that reads `BOARD_SKIP_GIT` and `BOARD_SKIP_PUSH` from environment variables exactly once at the `RunCLI` entry point (after config resolution), folding them into `cfg.SkipGit`/`cfg.SkipPush`.

**Card 5**: Updated the `Sync` function signature to accept `skipGit` and `skipPush` bool parameters, and updated `pushUnpushed` to accept `skipPush`. Updated `Board.Sync()` to pass the board's skip flags through.

**Card 6**: Updated `CommitPush` signature to include the `skipPush` bool parameter and removed the now-unused `os` import from git.go.

**Card 7**: Parallelized git_test.go by replacing `t.Setenv("BOARD_SKIP_PUSH", ...)` calls with the new `skipPush` parameter and adding `t.Parallel()` to all tests.

**Card 8**: Parallelized sync_test.go by removing the ambient-neutralizer `t.Setenv("BOARD_SKIP_GIT", "")` from `newSyncRepo`, replacing `t.Setenv("BOARD_SKIP_PUSH", "1")` with `cfg.SkipPush = true`, and adding `t.Parallel()` to all tests. Added `TestSkipSeam` with focused assertions on the skip behavior.

**Card 9**: Migrated all facade write-path tests in concurrency_test.go, board_test.go, and bench_test.go from `t.Setenv("BOARD_SKIP_GIT", "1")` to `cfg.SkipGit = true` (except CLI-driven benchmarks that properly use `RunCLI`).

**Card 10**: Created skipenv_internal_test.go as a white-box unit test for `applySkipEnv`, testing various combinations of environment variables and config state.

The verify command passed successfully: `go build ./...` and `go test -tags integration ./internal/board/... -count=1` both completed without errors. All 8 commits follow the established conventions with proper godoc comments and test coverage.
