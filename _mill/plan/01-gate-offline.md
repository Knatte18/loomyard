# Batch: gate-offline

```yaml
task: "Optimise and slim the rest of the test suite"
batch: gate-offline
number: 1
cards: 5
verify: go test ./internal/board/... ./internal/git/... ./internal/ide/... && go test -tags integration ./internal/board/... ./internal/git/... ./internal/ide/...
depends-on: []
```

## Batch Scope

Make the default `go test ./...` loop offline for the four remaining git-spawning packages,
by gating their subprocess-spawning test files behind `//go:build integration`. Two board
files (`git_test.go`, `sync_test.go`) are **moved** into the existing `internal/board/boardtest`
package (rewriting `package board_test` → `package boardtest` and adding the build tag); the
`internal/git` and `internal/ide` files are gated **in place**. No fixture or assertion
changes here — this batch is purely mechanical relocation + tagging. After it lands, Tier 1
is offline repo-wide and Tier 2 (`-tags integration`) runs every moved/gated test. The
fixture migrations (batches 2, 3) and pruning (batch 4) build on this.

This batch defines no new external interface; later batches consume the relocated files.

## Cards

### Card 1: Move `board/git_test.go` into `boardtest`, gated

- **Context:**
  - `internal/board/boardtest/doc.go`
  - `internal/board/boardtest/integration_test.go`
- **Edits:** none
- **Creates:**
  - `internal/board/boardtest/git_test.go`
- **Deletes:**
  - `internal/board/git_test.go`
- **Requirements:** Move the full content of the deleted `internal/board/git_test.go` into
  the new `internal/board/boardtest/git_test.go`. Make the new file's first line
  `//go:build integration`, then one blank line, then `package boardtest` (replacing
  `package board_test`). Keep all imports and both funcs `TestPull` and `TestCommitPush`
  byte-for-byte otherwise — they reference only the exported `board.Pull` / `board.CommitPush`,
  so the package rename compiles. Delete the original `internal/board/git_test.go`.
- **Commit:** `test(board): gate git_test behind integration in boardtest`

### Card 2: Move `board/sync_test.go` into `boardtest`, gated

- **Context:**
  - `internal/board/boardtest/doc.go`
  - `internal/board/boardtest/integration_test.go`
- **Edits:** none
- **Creates:**
  - `internal/board/boardtest/sync_test.go`
- **Deletes:**
  - `internal/board/sync_test.go`
- **Requirements:** Move the full content of the deleted `internal/board/sync_test.go` into
  the new `internal/board/boardtest/sync_test.go`, including the package-level helpers
  `newSyncRepo` and `dirty` and all five `TestSync*` funcs. First line `//go:build integration`,
  blank line, then `package boardtest` (replacing `package board_test`). Keep imports and
  bodies otherwise unchanged (it uses only exported `board.*`). Confirm no helper-name clash
  with the existing `boardtest` package (`cloneBenchWiki`, `benchmarkSync`, `seedWiki`,
  `setupIntegrationRepo` — none collide with `newSyncRepo`/`dirty`). Delete the original
  `internal/board/sync_test.go`.
- **Commit:** `test(board): gate sync_test behind integration in boardtest`

### Card 3: Gate `internal/git/git_test.go` in place

- **Context:** none
- **Edits:**
  - `internal/git/git_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Prepend `//go:build integration` as the first line, then one blank line,
  before the existing `package git_test` clause. No other change. The three `TestRunGit_*`
  funcs fundamentally need real git; after this, `internal/git` has zero Tier-1 tests (the
  package still compiles), which is intended.
- **Commit:** `test(git): gate git_test behind integration`

### Card 4: Gate `internal/ide/cli_test.go` in place

- **Context:** none
- **Edits:**
  - `internal/ide/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Prepend `//go:build integration` as the first line, then one blank line,
  before the existing `package ide` clause (keep the file's leading comment after the tag, or
  move the tag above it — the build constraint must be the first line and separated from
  `package` by a blank line). No fixture or assertion change. The local helpers
  `mustRun`/`newTestGitRepo` are used only within this file, so gating is self-contained.
- **Commit:** `test(ide): gate cli_test behind integration`

### Card 5: Gate `internal/ide/menu_test.go` in place

- **Context:** none
- **Edits:**
  - `internal/ide/menu_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Prepend `//go:build integration` as the first line, then one blank line,
  before the existing `package ide` clause. No fixture or assertion change. The local helpers
  `mustRunMenu`/`newTestGitRepoWithWorktrees` are used only within this file.
- **Commit:** `test(ide): gate menu_test behind integration`

## Batch Tests

`verify` runs both tiers for the three affected package trees:
`go test ./internal/board/... ./internal/git/... ./internal/ide/...` (Tier 1 — must pass with
the moved/gated tests **absent**) `&& go test -tags integration ./internal/board/... ./internal/git/... ./internal/ide/...`
(Tier 2 — must pass with every moved/gated test **present**). `./internal/board/...` includes
the `boardtest` subpackage.

**Equivalence guardrail (relocation):** before editing, capture
`go test -list '.*' ./internal/board` and `go test -tags integration -list '.*' ./internal/board/boardtest ./internal/git ./internal/ide`.
After the move, the **union** of board (untagged) + boardtest/git/ide (integration) `-list`
sets must equal the pre-change union — names are only relocated, none added or dropped. Also
confirm `go test -list '.*' ./internal/ide` (Tier 1) no longer lists `TestRunCLI*` /
`TestMenu*`, and `go test -list '.*' ./internal/git` (Tier 1) lists no `TestRunGit_*` —
demonstrating the offline guarantee structurally.
