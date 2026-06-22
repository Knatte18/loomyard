# Plan: Optimise and slim the rest of the test suite

```yaml
task: "Optimise and slim the rest of the test suite"
slug: optimize-remaining-test-suites
approved: true
started: "20260622-062759"
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: gate-offline
    file: 01-gate-offline.md
    depends-on: []
    verify: go test ./internal/board/... ./internal/git/... ./internal/ide/... && go test -tags integration ./internal/board/... ./internal/git/... ./internal/ide/...
  - number: 2
    name: board-fixtures
    file: 02-board-fixtures.md
    depends-on: [1]
    verify: go test -tags integration ./internal/board/... ./internal/lyxtest/...
  - number: 3
    name: ide-fixtures
    file: 03-ide-fixtures.md
    depends-on: [1]
    verify: go test -tags integration ./internal/ide/... ./internal/lyxtest/...
  - number: 4
    name: board-prune
    file: 04-board-prune.md
    depends-on: [1]
    verify: go test ./internal/board
  - number: 5
    name: docs
    file: 05-docs.md
    depends-on: [2, 3, 4]
    verify: go test ./... -count=1 && go test -tags integration ./... -count=1
```

## Shared Decisions

### Decision: two-tier model + Go verify shape

- **Decision:** The suite has two tiers — Tier 1 = default `go test ./...` (must spawn zero
  git/subprocesses); Tier 2 = `go test -tags integration ./...` (the git-spawning suite).
  This task moves the remaining spawners into Tier 2. Because this is a **Go** project,
  `verify:` commands use `go test` directly with **no** `PYTHONPATH=` prefix (the
  `verify-not-isolated` validator check is Python-only).
- **Rationale:** Established by the parent task (`docs/benchmarks/test-suite-timing.md`).
- **Applies to:** all batches.

### Decision: build tag is `//go:build integration`

- **Decision:** Every new gate uses `//go:build integration` as the first line, followed by
  one blank line, then the `package` clause (Go build-constraint placement). Leave
  `internal/muxpoc/muxpoc_smoke_test.go`'s existing `//go:build smoke` tag untouched.
- **Rationale:** `integration` is the repo convention (17 files already use it); muxpoc's
  smoke test is already excluded from Tier 1.
- **Applies to:** gate-offline (and any file re-touched later).

### Decision: keep `BOARD_SKIP_GIT` / `BOARD_SKIP_PUSH` env seam — no prod refactor

- **Decision:** Production code (`internal/board/board.go`, `internal/board/sync.go`) keeps
  reading the env vars. Do NOT thread a `SkipGit`/`SkipPush` option through `board.Config`.
  Only the **test** files change.
- **Rationale:** The env→option refactor is the prod-code risk that hurt the parent; it is
  out of scope (discussion: board-git-seam).
- **Applies to:** all batches.

### Decision: lyxtest reuse first; no per-test git spawns; CopyBoardRepo only as board fallback

- **Decision:** Migrate git **fixtures** onto existing `internal/lyxtest` fixtures
  (`CopyHostHub`, `CopyWeft`, `CopyPaired`) — template-built-once + per-test filesystem copy,
  zero per-test git spawns for the fixture build. Add a new exported `CopyBoardRepo` to
  lyxtest **only** if no existing fixture fits board (board-fixtures fallback). For ide, if no
  fixture fits, leave that test gated-but-unmigrated rather than force-fitting or adding a
  fixture. In-body git **verification** spawns (`rev-list`, `status`, `git log`,
  `git worktree add`) are inherent to some tests and acceptably remain in Tier 2.
- **Rationale:** Discussion: board-fixtures, ide-scope; "use lyxtest where it's worth it."
- **Applies to:** board-fixtures, ide-fixtures.

### Decision: tests stay serial where `t.Setenv`/`os.Chdir` present (parallelism best-effort)

- **Decision:** Migrated tests keep `t.Parallel()` ONLY where no process-global seam blocks
  it. Board moved tests use `t.Setenv` (serial); ide `menu_test` uses `t.Setenv` (serial);
  ide `cli_test` uses `os.Chdir` (serial). The headline offline win comes from gating, not
  parallelism — do NOT add `t.Parallel()` to any test that uses `t.Setenv`/`os.Chdir`.
- **Rationale:** Discussion: board-git-seam, ide-scope (Go forbids `t.Parallel` after
  `t.Setenv`; `os.Chdir` is process-global).
- **Applies to:** board-fixtures, ide-fixtures.

### Decision: equivalence guardrail — superset, union across board+boardtest (non-negotiable)

- **Decision:** The post-change test-name set must be a **superset** of the pre-change set,
  verified by diffing `go test -list` + `=== RUN` baselines. Because `git_test.go`/
  `sync_test.go` move from `internal/board` (Tier 1) into `internal/board/boardtest`
  (Tier 2), the superset check is computed against the **union across both final packages**
  (board untagged + boardtest under `-tags integration`). Table-driven folds are allowed
  ONLY when assertions are preserved; no named (sub)test may be silently dropped.
- **Rationale:** Discussion: Constraints (equivalence guardrail).
- **Applies to:** gate-offline (relocation), board-prune (folds), docs (record the note).

### Decision: Path Invariant does not constrain this task

- **Decision:** `CONSTRAINTS.md` bans raw `os.Getwd`/`git rev-parse --show-toplevel` outside
  `internal/paths` + `cmd/lyx/main.go`, but `internal/paths/enforcement_test.go` **skips
  `_test.go` files** (verified). This task edits only test files plus `internal/lyxtest`
  (which uses neither primitive). Do NOT introduce raw `os.Getwd`/`git rev-parse` into any
  non-`_test.go` production file.
- **Rationale:** Keeps the build-time enforcement green.
- **Applies to:** all batches.

## All Files Touched

- `docs/benchmarks/test-suite-timing.md`
- `internal/board/boardtest/git_test.go`
- `internal/board/boardtest/sync_test.go`
- `internal/board/render_test.go`
- `internal/board/store_test.go`
- `internal/git/git_test.go`
- `internal/ide/cli_test.go`
- `internal/ide/menu_test.go`
- `internal/lyxtest/lyxtest.go`
- `internal/lyxtest/lyxtest_test.go`
