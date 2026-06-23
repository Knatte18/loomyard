All `internal/board` test files are external (`package board_test`). So there is no existing white-box test file, and card 10 as written (calling unexported `applySkipEnv` from `package board_test`) is a genuine compile failure. The fix is either to make the test white-box (`package board` in a new/separate file — but cli_test.go is the named edit target and is `board_test`), or export the helper, or have the card test through `RunCLI`'s observable effect. This is BLOCKING for batch 2's `verify` (which runs `go test ./internal/board/...` and would fail to compile).

I have completed my verification. Rendering the review.

MILL_REVIEW_BEGIN
# Review: Speed up and stabilize the integration test tier — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-23
```

## Findings

### [BLOCKING] Card 10 tests unexported applySkipEnv from external pkg
**Location:** Batch 2 / Card 10 (cli_test.go)
**Issue:** `applySkipEnv` is unexported (added in `package board`, card 4), but `cli_test.go` is `package board_test` (verified — all `internal/board/*_test.go` are external; no white-box file exists), so `applySkipEnv(Config{})` will not compile, failing batch 2's `verify` (`go test ./internal/board/...`).
**Fix:** Either place the helper test in a new white-box `package board` file, export the helper, or assert env→cfg behaviour through `RunCLI`'s observable effect; update card 10's Edits/Requirements accordingly.

### [NIT] Two surviving BOARD_SKIP_* env-setters not acknowledged
**Location:** Overview / All Files Touched; Batch 2 card 9
**Issue:** `cmd/lyx/main_test.go` and `internal/ide/menu_test.go` set `BOARD_SKIP_GIT=1` but appear in neither "All Files Touched" nor any card. Both are in fact safe — `main_test.go` reaches the board via `RunCLI`, so env is folded by `applySkipEnv`; `menu_test.go` only calls `board.HealthCheck()`, which never spawns a sync regardless of `skipGit` — but the plan's blast-radius/equivalence reasoning never states this.
**Fix:** Add a one-line note (e.g. in card 9 or the overview) confirming these two files are intentionally untouched and why they do not regress; no code change required.

## Verdict

REQUEST_CHANGES
Plan is sound and well-grounded; only card 10's external-package compile error blocks it.
MILL_REVIEW_END