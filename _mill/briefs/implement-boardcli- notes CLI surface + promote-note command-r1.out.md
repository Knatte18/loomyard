All 5 cards (20-24) are committed and complete. 3 content commits were made, with cards 20-22 combined into one commit (named after card 22's `Commit:` message, per the batch's combining rule since all three touch `internal/boardcli/cli.go`), and cards 23-24 each with their own commit. Verify passes: `go test ./internal/boardcli/... && go test -tags integration ./internal/boardcli/...` — all green. Working tree is clean (only the untracked out-of-scope brief file remains, which is not part of this batch's declared scope).

Files touched:
- `/home/knatte/Code/loomyard/wts/board/internal/boardcli/cli.go` (Cards 20, 21, 22)
- `/home/knatte/Code/loomyard/wts/board/internal/boardcli/cli_test.go` (Card 23)
- `/home/knatte/Code/loomyard/wts/board/internal/boardcli/help_test.go` (Card 24)
- `/home/knatte/Code/loomyard/wts/board/internal/boardcli/notes_test.go` (Card 24, new)
- `/home/knatte/Code/loomyard/wts/board/internal/boardcli/promotenote_test.go` (Card 24, new)

Card-count self-check: 5 of 5 cards committed/completed (20, 21, 22, 23, 24) — all done.

{"status":"success","commit_sha":"37b7b316","session_id":"ebff4b01-8ad5-4a7b-9011-0866f310b9e2","cards_done":[20,21,22,23,24]}