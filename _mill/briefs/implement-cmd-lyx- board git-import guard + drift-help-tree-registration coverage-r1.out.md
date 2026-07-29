All 3 cards (25, 26, 27) committed — 3 of 3 cards complete, matching the batch's declared `cards: 3`. Verify passed for both the untagged and integration-tagged runs. No commit-none cards in this batch.

{"status":"success","commit_sha":"cefa25c3","session_id":"ea5c9e41-95b7-47ff-9d5f-463a72a5e83f","cards_done":[25,26,27]}

Summary: 3 of 3 cards committed and verified.

- `/home/knatte/Code/loomyard/wts/board/cmd/lyx/boardguard_test.go` (created, Card 25) — new `TestBoardGuard_NoRawGitImportOrShellOut` guard.
- `/home/knatte/Code/loomyard/wts/board/cmd/lyx/tierpurity_test.go` (Card 25) — allowlisted the new guard test's `go env GOMOD` spawn.
- `/home/knatte/Code/loomyard/wts/board/cmd/lyx/helptree_test.go` (Card 26) — pinned `notes`/`promote-note` into board's `wantSubs`.
- `/home/knatte/Code/loomyard/wts/board/internal/fabriccli/cli_test.go` (Card 27) — new `TestRunCLI_CloneRequiresExactlyTwoArgs`.

Commits: 8a907d05, 9603e3a8, cefa25c3 — all pushed to `origin/board`. Batch verify (`go test ./cmd/lyx/... ./internal/fabriccli/...` and the `-tags integration` pass over the same paths) both passed. Working tree is clean.

{"status":"success","commit_sha":"cefa25c3","session_id":"ea5c9e41-95b7-47ff-9d5f-463a72a5e83f","cards_done":[25,26,27]}