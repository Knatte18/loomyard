Both cards (10 and 11) match declared `Commit:` messages exactly. Build passes, verify passes, no tracked dirty files.

{"status":"success","commit_sha":"a1d0ebbcb6c79c7a05793c06bcdd5fd684117673","session_id":"e04be7f7-1591-47fc-a188-2e580aedd303","cards_done":[10,11]}

Summary: 2 of 2 cards committed (all complete).

- `/home/knatte/Code/loomyard/wts/fabric-commit-lock-coalescing/internal/boardengine/sync.go` — Card 10: `Sync` now builds a step closure (ensureLockfilesIgnored + commitDirty + optional PushWeftAt) and delegates the absorbing-lock loop to `fabricengine.CoalescePush`, acquiring `board.push.lock` at the unchanged path.
- `/home/knatte/Code/loomyard/wts/fabric-commit-lock-coalescing/internal/boardengine/testmain_test.go` — Card 11: new hermetic `TestMain` for package `boardengine`.
- `/home/knatte/Code/loomyard/wts/fabric-commit-lock-coalescing/internal/boardengine/sync_integration_test.go` — Card 11: new `//go:build integration` test covering Sync parity (dirty commit+push, .gitignore seeding, lock-path serialization, skipPush behavior), with a locally-reimplemented bare-remote/board fixture.

Commits: `7fa45dc8` (Card 10), `a1d0ebbc` (Card 11), both pushed to `fabric-commit-lock-coalescing`. Verify command `go test -tags integration ./internal/boardengine/...` passes; `go build ./...` passes; `git status --porcelain --untracked-files=no` is clean (only the pre-existing untracked brief file remains, which is outside batch scope).