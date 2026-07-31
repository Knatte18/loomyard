All 4 cards (6, 7, 8, 9) committed, matching the batch's declared 4 cards exactly. Verify passed. Working tree is clean.

{"status":"success","commit_sha":"301a0f301bc3de1de002a390afe404d0072c5a5b","session_id":"dd6f1bc5-c8dd-4c96-b9d4-fee9b4c0ce14","cards_done":[6,7,8,9]}

Summary: 4 of 4 cards committed for batch `commit-empty-primitive`.

- Card 6 (`3da55913`): added `gitrepo.ErrIndexNotEmpty` and `Repo.CommitEmpty` to `internal/gitrepo/gitrepo.go`.
- Card 7 (`1c826b30`): added `internal/gitrepo/commitempty_integration_test.go` with the five required cases.
- Card 8 (`d1bab249`): pinned `CommitEmpty` in `cmd/lyx/gitrepoboundary_test.go` and updated `CONSTRAINTS.md`'s gitrepo Client Boundary Invariant.
- Card 9 (`301a0f30`): documented `CommitEmpty` in `internal/gitrepo/doc.go`.

Verify command `go test -tags integration -count=1 ./internal/gitrepo/... ./cmd/lyx/...` passed. Working tree is clean (no uncommitted tracked changes).

{"status":"success","commit_sha":"301a0f301bc3de1de002a390afe404d0072c5a5b","session_id":"dd6f1bc5-c8dd-4c96-b9d4-fee9b4c0ce14","cards_done":[6,7,8,9]}
