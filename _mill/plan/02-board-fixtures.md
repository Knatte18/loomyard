# Batch: board-fixtures

```yaml
task: "Optimise and slim the rest of the test suite"
batch: board-fixtures
number: 2
cards: 2
verify: go test -tags integration ./internal/board/... ./internal/lyxtest/...
depends-on: [1]
```

## Batch Scope

Replace the per-test `git init`/`clone`/`config`/`commit`/`push` **fixture builds** in the
now-relocated `boardtest/git_test.go` and `boardtest/sync_test.go` with `internal/lyxtest`
shared fixtures (template-built-once + per-test filesystem copy → zero per-test fixture-build
git spawns). In-body git **verification** (`git rev-list`, `git status --porcelain`,
`git ls-files`, `git log`) is inherent to these tests' assertions and stays — only repo
construction changes; every assertion is preserved. These are Tier-2 (integration) tests and
stay **serial** (`t.Setenv`). Default path is reuse of `CopyWeft`/`CopyHostHub`; a dedicated
`CopyBoardRepo` is added to lyxtest ONLY if `CopyWeft` genuinely cannot satisfy a board.Sync
assertion.

Depends on batch 1 (the files now live in `internal/board/boardtest` as `package boardtest`).

## Cards

### Card 6: Migrate `sync_test` fixtures onto lyxtest

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/board/sync.go`
  - `internal/board/board.go`
- **Edits:**
  - `internal/board/boardtest/sync_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/lyxtest_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `newSyncRepo`, replace the manual `git init --bare` + `git clone` +
  `git config` + seed-commit + `git push -u origin HEAD` with `lyxtest.CopyWeft(t)`: use the
  returned `WeftFixture.WeftPath` as the working tree (`work`) and `.Bare` as the remote.
  Keep the `count`/`@{u}`/`HEAD` commit-counting closures and the `dirty` helper unchanged
  (in-body verification stays). All five `TestSync*` assertions must pass unchanged
  (`TestSyncCommitsAndPushes`, `TestSyncCoalescesBurstIntoOneCommit`,
  `TestSyncSkipPushCommitsLocallyOnly`, `TestSyncCleanTreeIsNoOp`, `TestSyncIgnoresLockfiles`).
  Keep `t.Setenv("BOARD_SKIP_GIT","")` and the per-test `t.Setenv("BOARD_SKIP_PUSH",…)`; do
  NOT add `t.Parallel()`. **Fallback (likely needed — evaluate first):** `CopyWeft` seeds
  `_lyx/config.yaml` but **not** `tasks.json`, while these tests write and commit `tasks.json`,
  and `TestSyncCleanTreeIsNoOp` assumes the working tree is clean after the initial
  board-managed `.gitignore` commit. If `CopyWeft`'s pre-seeded weft contents cause any
  `board.Sync` assertion to diverge (most likely the clean-tree no-op or the commit counts),
  add an exported `CopyBoardRepo(t)` to `internal/lyxtest/lyxtest.go` mirroring `CopyWeft`'s
  bare+clone+upstream but seeded with a `tasks.json` only (no `_lyx`), with a matching fixture
  test in `internal/lyxtest/lyxtest_test.go` (follow the existing `lyxtest_test.go`
  `//go:build integration` + per-fixture-test pattern), and use it instead. Prefer `CopyWeft`
  reuse if it passes cleanly; otherwise take the `CopyBoardRepo` path rather than forcing
  `CopyWeft`.
- **Commit:** `test(board): migrate sync_test fixtures onto lyxtest`

### Card 7: Migrate `git_test` fixtures onto lyxtest

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/board/board.go`
- **Edits:**
  - `internal/board/boardtest/git_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestPull` — replace its manual bare/clone/config/initial-commit and
  `git push -u origin master` with `lyxtest.CopyWeft(t)` (a clone with upstream history on
  `main` to pull from); drop the hard-coded `master` and rely on the fixture's `main`.
  `TestCommitPush` — its two `BOARD_SKIP_PUSH=1` subtests need only a local committed repo:
  use `lyxtest.CopyHostHub(t).Hub`; the rebase-retry subtest (`BOARD_SKIP_PUSH=""`, which
  pushes) needs a working upstream: use `lyxtest.CopyWeft(t)`. Preserve every assertion and
  the in-body git verification (`git log --oneline`, `git rev-list --count`). Keep the tests
  serial (the subtests use `t.Setenv`). Do not edit lyxtest in this card — reuse only.
- **Commit:** `test(board): migrate git_test fixtures onto lyxtest`

## Batch Tests

`verify: go test -tags integration ./internal/board/... ./internal/lyxtest/...` — runs the
migrated board integration tests plus the (integration-gated) lyxtest fixture tests, covering
the `CopyBoardRepo` fallback if it was added. Confirm the seven moved board test names are
unchanged from batch 1's post-move baseline (the migration changes fixture construction only,
not test names or assertions) via `go test -tags integration -list '.*' ./internal/board/boardtest`.
