No unexported references — the move to `boardtest` (black-box `package boardtest`) is clean as claimed. I have verified all the load-bearing claims. Compiling my review.

MILL_REVIEW_BEGIN
# Review: Optimise and slim the rest of the test suite

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\optimize-remaining-test-suites\_mill\discussion.md
date: 2026-06-22
```

## Findings

### [GAP] git_test.go move target package not type-checked
**Section:** Decisions: board-test-home / Technical context (files-to-gate)
**Issue:** The discussion says `git_test.go` is `package board_test` and moves cleanly to `boardtest`, but `boardtest` is `package boardtest` (verified `doc.go`), not `package board_test`; the moved file's `package` clause must be rewritten, not just relocated, and the discussion only mentions adding a build tag.
**Fix:** State explicitly that both moved files get `package boardtest` (both are already black-box, verified no unexported `board.*` refs) plus `//go:build integration` plus the blank-line placement.

### [GAP] boardtest already has a TestPull/CommitPush — name-collision risk
**Section:** Decisions: board-test-home; Constraints (equivalence guardrail)
**Issue:** `boardtest/integration_test.go` already exercises `board.Pull`/`CommitPush`/`AtomicWrite` against a network remote; moving `git_test.go`'s `TestPull`/`TestCommitPush` into the same package risks duplicate top-level func names (compile failure) and muddies the superset diff (two packages' `-list` sets merge into one).
**Fix:** Confirm no func-name collision in `boardtest` and define the equivalence baseline per-final-package (board, boardtest) so the superset check accounts for the cross-package move.

### [NOTE] git_test.go uses BOARD_SKIP_PUSH, not BOARD_SKIP_GIT
**Section:** Technical context (board git seam)
**Issue:** The seam discussion centers on `BOARD_SKIP_GIT`, but `git_test.go` toggles `BOARD_SKIP_PUSH` (and one subtest sets it to `""`); the env-contention note in board-test-home only addresses `BOARD_SKIP_GIT`.
**Fix:** Note that the moved git tests interact with `BOARD_SKIP_PUSH` (Setenv-scoped, no contention with boardtest's `BOARD_SKIP_GIT=1`), so they stay serial for the same reason.

### [NOTE] board-fixtures reuse left to implementation-time
**Section:** Decisions: board-fixtures
**Issue:** Whether `newSyncRepo` maps onto `CopyWeft` is deferred ("decide concretely during implementation"); `newSyncRepo` pushes `HEAD` (default branch) with upstream while `CopyWeft`'s template pushes `-u origin main` — a branch-name mismatch the plan writer cannot resolve from the discussion.
**Fix:** Flag the `HEAD`-vs-`main` default-branch detail as the concrete fit risk to settle, so reuse-vs-new-fixture is decidable.

## Verdict
GAPS_FOUND
Package-clause rewrite and boardtest name/baseline collision must be resolved before planning.
MILL_REVIEW_END