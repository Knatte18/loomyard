# Batch: network-tests-removal

```yaml
task: "Speed up and stabilize the integration test tier"
batch: network-tests-removal
number: 1
cards: 2
verify: go test -tags integration ./internal/board/boardtest -count=1
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

### Card 2: Drop the "real GitHub remote" claim from running-tests.md

- **Context:** none
- **Edits:**
  - `docs/benchmarks/running-tests.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/benchmarks/running-tests.md`, update the Tier 2 description
  so it no longer claims the integration tier pushes to a real GitHub remote. Specifically,
  in the "The two tiers" section, change the Tier 2 bullet that reads "the gated tests that
  spawn real `git` (worktrees, commits, pushes, junctions) and, in one case, push to a real
  GitHub remote" to drop the "and, in one case, push to a real GitHub remote" clause — Tier
  2 is now all-local git. Leave the rest of the file (commands, harness, two-tier rationale)
  intact. Do not edit `test-suite-timing.md` here (batch 4 rewrites its numbers).
- **Commit:** `docs(benchmarks): Tier 2 is now all-local git (no real remote)`

## Batch Tests

`verify: go test -tags integration ./internal/board/boardtest -count=1` proves the package
still compiles and passes after the deletions (a dangling `testRepoURL` reference would
fail the build). Card 2 is a doc edit with no runnable surface; it is covered by review,
not the verify command.
