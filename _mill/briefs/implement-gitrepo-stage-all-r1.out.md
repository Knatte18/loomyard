All 3 commits match the 3 cards' `Commit:` messages exactly (`feat(gitrepo): add StageAllAndCommit wildcard-stage method`, `docs(gitrepo): document StageAllAndCommit; drop stale board cross-refs`, `test(gitrepo): cover StageAllAndCommit wildcard staging`). 3 of 3 cards committed. Verify passed (`go test -tags integration ./internal/gitrepo/...` → ok).

{"status":"success","commit_sha":"924a722b","session_id":"4898d553-19cc-42cd-8bae-b0906b2e9bca"}

Summary: 3 of 3 cards committed for batch `01-gitrepo-stage-all`.

- `/home/knatte/Code/loomyard/wts/board-use-gitrepo/internal/gitrepo/gitrepo.go` (Card 1) — added `StageAllAndCommit(msg string) (sha string, committed bool, err error)` immediately after `StageAndCommit`, plus a file-header update.
- `/home/knatte/Code/loomyard/wts/board-use-gitrepo/internal/gitrepo/doc.go` and `/home/knatte/Code/loomyard/wts/board-use-gitrepo/internal/gitrepo/push.go` (Card 2) — documented `StageAllAndCommit` as board's opt-in wildcard exception, reworded the Push-surface section, and dropped dangling cross-references to board's soon-to-be-deleted `pushUnpushed`/`hasUnpushed`.
- `/home/knatte/Code/loomyard/wts/board-use-gitrepo/internal/gitrepo/gitrepo_test.go` (Card 3) — added `TestStageAllAndCommit_CommitsBothUntrackedAndModifiedFiles`, `TestStageAllAndCommit_NothingToCommit_WhenTreeClean`, and `TestStageAllAndCommit_CapturesFileExplicitListWouldMiss`.

Verify command `go test -tags integration ./internal/gitrepo/...` passed. Working tree is clean of tracked changes; only the untracked brief file `_mill/briefs/implement-gitrepo-stage-all-r1.md` remains (out of scope). Final HEAD is `924a722b`, pushed to `origin/board-use-gitrepo`.
