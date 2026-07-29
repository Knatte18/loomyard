All 6 cards declared in this batch (4, 5, 6, 7, 8, 9) are committed and addressed — 5 commits total, since Card 4's signature change was folded into Card 6's commit (both touch `clone.go`, and separating them would have left a non-compiling intermediate state, which the brief explicitly permits combining for). Verify passed (both tagged and untagged `go test`), and a full-repo `go build ./...` / `go build -tags integration ./...` also succeeded. Working tree is clean of tracked modifications.

{"status":"success","commit_sha":"b5bbb1a6","session_id":"a6fa497d-4b95-4297-a377-0a1cb65193bc","cards_done":[4,5,6,7,8,9]}

Summary of work:
- `internal/fabricengine/boardweft.go` (new): `ensureBoardWorktree` — adopt-or-orphan `_board` worktree-add logic (Card 5, commit `7bcad0a2`).
- `internal/fabricengine/clone.go`: `suffixWeftPrimaryBranch` now returns the host branch it read; `CloneHub` dropped `boardURL`/`resolvedBoardURL`, now materializes `_board` via `ensureBoardWorktree`; `deriveBoardURL` deleted (Cards 4+6 combined, commit `401d9367`).
- `internal/fabriccli/clone.go`: `runCloneWithReset` now 2-arg, envelope returns only `{"hub": ...}` (Card 7, commit `ddc272a6`).
- `internal/fabriccli/fabric.go`: `clone` command help text updated for `_board`'s new provenance (Card 8, commit `e5a0e993`).
- `internal/fabricengine/clone_test.go` / `clone_adopt_test.go`: removed `TestDeriveBoardURL`, updated all `CloneHub` call sites to 2-arg/2-return, added `assertBoardIsWeftWorktree`/`resolveGitCommonDir`/`hasNoCommits`/`makeEmptyBareRemote` helpers and a new `TestCloneHub_BoardWorktreeOrphanBranchOnEmptyWeftRemote` (Card 9, commit `b5bbb1a6`).

All 6 declared cards (4, 5, 6, 7, 8, 9) were committed this turn — full completion, no cards remaining.