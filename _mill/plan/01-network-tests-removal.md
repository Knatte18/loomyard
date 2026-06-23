# Batch: network-tests-removal

```yaml
task: "Speed up and stabilize the integration test tier"
batch: network-tests-removal
number: 1
cards: 2
verify: go build ./... && go test -tags integration ./internal/board/boardtest -count=1
depends-on: []
```

## Batch Scope

Delete the only two tests that hit a real GitHub remote, plus the network benchmarks
that share their remote URL, so a plain `-tags integration` run is fully local and
deterministic. This is the "stabilize" half of the task — it removes all run-to-run
network noise. No production code changes; pure test/doc deletion. After this batch the
`boardtest` package compiles and passes under `-tags integration` with zero network
access, and `internal/git`/other integration tests are unaffected (verified: no other
test references a real `https://` remote). The external interface consumed by later
batches: none — this batch only removes.

## Cards

### Card 1: Delete the real-GitHub tests and benchmarks

- **Context:**
  - `internal/board/boardtest/doc.go`
  - `internal/board/boardtest/git_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/board/boardtest/integration_test.go`
  - `internal/board/boardtest/bench_git_test.go`
- **Requirements:** Delete `internal/board/boardtest/integration_test.go` (the
  `TestIntegrationCommitPush` and `TestIntegrationPull` tests and the `testRepoURL`
  constant) and `internal/board/boardtest/bench_git_test.go` (the `BenchmarkSyncGit` and
  `BenchmarkSyncGitNoPush` benchmarks, which clone `testRepoURL`). These two files are the
  only references to `testRepoURL`, so after deletion the package must still compile under
  `-tags integration`. Do not touch `git_test.go`, `sync_test.go`, or `concurrency_test.go`
  — they use local bare-repo fixtures and stay. Confirm no remaining code symbol references
  `testRepoURL`, `setupIntegrationRepo`, `cloneBenchWiki`, `BenchmarkSyncGit`, or
  `BenchmarkSyncGitNoPush`.
- **Commit:** `test(board): delete real-GitHub integration tests and network benchmarks`

### Card 2: Update stale references to the deleted network tests

- **Context:** none
- **Edits:**
  - `docs/benchmarks/running-tests.md`
  - `internal/board/boardtest/doc.go`
  - `docs/benchmarks/board-performance.md`
  - `cmd/testtiming/main.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Remove every reference to the now-deleted network tests/benchmarks so no
  doc or comment dangles:
  - `docs/benchmarks/running-tests.md` — in the "The two tiers" section, change the Tier 2
    bullet "the gated tests that spawn real `git` (worktrees, commits, pushes, junctions)
    and, in one case, push to a real GitHub remote" to drop the "and, in one case, push to a
    real GitHub remote" clause; Tier 2 is now all-local git.
  - `internal/board/boardtest/doc.go` — the package comment (line ~9) says "the
    git/integration suites are gated behind the `integration` build tag (see
    integration_test.go and bench_git_test.go)"; update it to reference the surviving gated
    files (`git_test.go`, `sync_test.go`) and drop the deleted ones.
  - `docs/benchmarks/board-performance.md` — remove **every** reference to the deleted
    `BenchmarkSyncGit` / `BenchmarkSyncGitNoPush` / `TestIntegrationCommitPush`, not just one
    section: (a) the "How to run" block (~lines 18-19) with the `-bench SyncGit` invocation and
    its "Network + push access … required" note; (b) the "Background sync
    (`-tags integration -bench SyncGit …`)" results table (~lines 86-91, the `SyncGit` /
    `SyncGitNoPush` rows); (c) any inline mention (e.g. the `SyncGit` reference ~line 109); and
    (d) the "## Push access" section (~line 140). Replace with nothing, or a one-line note that
    the integration tier no longer benchmarks against a real remote. Grep the file for `SyncGit`
    afterward to confirm zero remaining hits.
  - `cmd/testtiming/main.go` — line ~93 prints "(real git + network; this can take ~a
    minute)"; change it to drop "+ network" (the full tier is real local git only). Leave the
    `-full` flag help text intact.
  Do not edit `test-suite-timing.md` here (batch 4 rewrites its numbers).
- **Commit:** `docs(benchmarks): drop references to deleted network tests`

## Batch Tests

`verify: go build ./... && go test -tags integration ./internal/board/boardtest -count=1` —
the `go build ./...` leg compiles `cmd/testtiming` (card 2 edits its `main.go` string) and
catches any dangling symbol; the `go test` leg proves `boardtest` still compiles and passes
after the deletions (a dangling `testRepoURL` reference would fail the build). The other card-2
edits (running-tests.md, doc.go comment, board-performance.md) have no runnable surface and are
covered by review.
