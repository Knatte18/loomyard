All work for this batch is committed and pushed, and verify passes.

Card-count self-check: the batch declares 3 cards (8, 9, 10). I committed all 3 in a single combined commit (`501a2a55`), which is permitted per the brief's "one combined commit" allowance since cards 8-10 all touch the same two files (`write.go`, `write_test.go`) and card 10 has a hard symbol dependency on card 8/9's code living in the same file. Honest count: **3 of 3 cards committed** (combined into 1 commit, not 3 separate commits).

Summary of what was implemented:

- `/home/knatte/Code/loomyard/wts/git-native-library/internal/gitnativepoc/write.go` (new) — `StageAndCommit`/`StageAllAndCommit` (card 8, MIGRATE via a synthetic-index technique since go-git's `Worktree.Commit` has no pathspec option), `Push` (card 9, CLI-BOUND for rebase-retry — go-git v5.19.1 ships no rebase implementation at all, confirmed by grepping the module source), and `SetSnapshotSHA` (card 10, MIGRATE — fast-forward-only push + adopt-on-conflict reproduces gitrepo's contract exactly, reusing `isStrictDescendant`/`validSnapshotKey`/`validSHA` from batch 2's `read.go`).
- `/home/knatte/Code/loomyard/wts/git-native-library/internal/gitnativepoc/write_test.go` (new) — differential parity tests for all three write-surface ops, including the pivotal `TestPush_RebaseRetryOnNonFastForward` (asserts the CLI-BOUND verdict against `gitrepo.Push` as oracle) and `TestSetSnapshotSHA_AdoptOnConflict` (simulated concurrent writer via a second bare-remote clone, forcing the adopt path).

Both the full batch verify command (`go test -tags integration ./internal/gitnativepoc/...` plus the tier-purity/hermetic-env guard tests) and `go build ./...` pass cleanly. `gofmt`/`goimports`/`golangci-lint` show no drift or new findings from the two new files (the two golangci-lint findings in the package are pre-existing in `read.go` from batch 2, outside this batch's scope). Commit `501a2a55` is pushed to `origin/git-native-library`.

{"status":"success","commit_sha":"501a2a55","session_id":"1acd39c0-d181-4e83-9347-2caa0046303c","cards_done":[8,9,10]}
