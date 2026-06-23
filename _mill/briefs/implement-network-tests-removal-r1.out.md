Excellent! The batch is complete and all verifications pass. Here's a summary of what was accomplished:

## Summary

**Card 1: Delete real-GitHub tests and benchmarks**
- Deleted `internal/board/boardtest/integration_test.go` (containing `TestIntegrationCommitPush`, `TestIntegrationPull`, and `testRepoURL` constant)
- Deleted `internal/board/boardtest/bench_git_test.go` (containing `BenchmarkSyncGit`, `BenchmarkSyncGitNoPush`, and `cloneBenchWiki` helper)
- Verified no remaining code references to deleted symbols

**Card 2: Update stale references**
- Updated `docs/benchmarks/running-tests.md` to remove "and, in one case, push to a real GitHub remote" from Tier 2 description
- Updated `internal/board/boardtest/doc.go` package comment to reference surviving gated files (`git_test.go`, `sync_test.go`)
- Updated `docs/benchmarks/board-performance.md` to remove all network benchmark references (How to run block, Background sync table, and Push access section)
- Updated `cmd/testtiming/main.go` to change output message from "(real git + network; ...)" to "(real local git; ...)"

**Verification:**
- `go build ./...` completed successfully
- `go test -tags integration ./internal/board/boardtest -count=1` passed
- All deleted symbols have no remaining references in the codebase

{"status":"success","commit_sha":"350e7f45803ef72a82333b6e3a799642def1d183","session_id":"e9b83f2c-ed6c-4ee9-8b6c-9a116fc7af69"}
