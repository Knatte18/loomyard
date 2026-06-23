# Plan: Speed up and stabilize the integration test tier

```yaml
task: "Speed up and stabilize the integration test tier"
slug: optimize-integration-tier
approved: false
started: 20260623-111128
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: network-tests-removal
    file: 01-network-tests-removal.md
    depends-on: []
    verify: go test -tags integration ./internal/board/boardtest -count=1
  - number: 2
    name: board-skip-seam-parallelize
    file: 02-board-skip-seam-parallelize.md
    depends-on: [1]
    verify: go build ./... && go test -tags integration ./internal/board/... -count=1
  - number: 3
    name: lean-fixture-worktree
    file: 03-lean-fixture-worktree.md
    depends-on: []
    verify: go test -tags integration ./internal/worktree ./internal/lyxtest -count=1
  - number: 4
    name: record-timing
    file: 04-record-timing.md
    depends-on: [1, 2, 3]
    verify: null
```

## Shared Decisions

### Decision: Single production env read; internal sites use explicit flags

- **Decision:** `BOARD_SKIP_GIT` / `BOARD_SKIP_PUSH` are read from `os.Getenv` in exactly
  **one** production location — `RunCLI` in `internal/board/cli.go`, immediately after
  `cfg` is resolved (after the `--board-path` / `LoadConfig` branch, ~cli.go:83) — where
  the env values are folded into `cfg.SkipGit` / `cfg.SkipPush`. Every other production
  site (`writeOp`, `Sync`, `pushUnpushed`, `CommitPush`) reads the explicit flag/param,
  never env. The four current `os.Getenv("BOARD_SKIP_*")` reads (board.go:83, sync.go:32,
  sync.go:103, git.go:69) are replaced by flag/param checks.
- **Rationale:** The only production path that relies on the env override is the detached
  `lyx board sync` child (it inherits the parent's env, re-enters `RunCLI`, and must honor
  `BOARD_SKIP_PUSH`). Resolving env→cfg once at that single entry point preserves the
  operational override while making every internal site deterministic. Because no test
  invokes `RunCLI` for these paths, tests never read env at all — which structurally
  eliminates the ambient-`BOARD_SKIP_*` leakage the discussion flagged (a parallel test
  can no longer be silently no-op'd by an inherited env var).
- **Applies to:** all batches (defines the seam batch 2 implements and batch-2 tests rely on).

### Decision: Plain bool flags/params, not functional options

- **Decision:** Add `SkipGit bool` / `SkipPush bool` to `board.Config`; give package
  `Sync` the signature `Sync(boardPath string, skipGit, skipPush bool)` and `CommitPush`
  the signature `CommitPush(boardPath string, relPaths []string, message string, skipPush bool)`.
  No functional-options / tri-state mechanism.
- **Rationale:** The discussion offered functional options + tri-state as one option to
  defeat env leakage at the *consumption* site. With env resolved once at the CLI entry
  (decision above), consumption sites never consult env, so the tri-state is unnecessary
  complexity. `CommitPush`/`Pull` have **no production callers** (only `boardtest`), so a
  plain param carries no ambient-env risk. This is the simpler, YAGNI-correct realization
  of the discussion's chosen "resolve env at entry" direction.
- **Applies to:** board-skip-seam-parallelize.

### Decision: Equivalence guardrail — justified subset

- **Decision:** The only intentionally-removed test names are `TestIntegrationCommitPush`
  and `TestIntegrationPull` (deleted in batch 1), each covered by `git_test.go:TestCommitPush`
  / `git_test.go:TestPull`. Every other change preserves test names (adds `t.Parallel()`,
  swaps fixtures/flags). One new name is **added** (`TestWeftSpawnPushesWeftBranch`, batch 3).
  Batch 4 records the subset-drop justification in the timing doc and verifies via a
  `-list` diff that no other name vanished.
- **Rationale:** Mirrors the `prune-board-tests` (2026-06-22) subset-with-justification
  relaxation; strict superset is impossible once the network tests are deleted.
- **Applies to:** all batches.

### Decision: Go test + timing conventions

- **Decision:** Follow `golang-testing` conventions. `verify:` uses the native `go test`
  runner (no `PYTHONPATH=` prefix — this is a Go project). Tier 2 numbers are measured
  **warm, `-count=1`, median of ≥3 runs**; any coverage profile writes to `.scratch/`.
- **Rationale:** Windows timing is noisy; a single run is not truth. Native runner is the
  only correct invocation for Go.
- **Applies to:** all batches.

## All Files Touched

- `docs/benchmarks/running-tests.md`
- `docs/benchmarks/test-suite-timing.md`
- `internal/board/board.go`
- `internal/board/boardtest/concurrency_test.go`
- `internal/board/boardtest/git_test.go`
- `internal/board/boardtest/sync_test.go`
- `internal/board/cli.go`
- `internal/board/cli_test.go`
- `internal/board/config.go`
- `internal/board/git.go`
- `internal/board/sync.go`
- `internal/lyxtest/lyxtest.go`
- `internal/worktree/add_test.go`
- `internal/worktree/remove_test.go`
- `internal/worktree/weft_test.go`
