Card-count self-check: 3 commits map to the 4 declared cards — Card 12 (commit `65f7f006`), Card 13 (`5dcdaa74`), and Cards 14+15 combined into one commit (`46785dfd`), matching the batch's stated batch-local decision to combine 14/15 into one file/commit. All 4 of 4 cards are complete.

{"status":"success","commit_sha":"46785dfdbed4b10c4c28974fefc04750cb8a366c","session_id":"9211818a-ceaa-44a7-915e-0a014d59f884","cards_done":[12,13,14,15]}

Summary of work done in `/home/knatte/Code/loomyard/wts/reed-header-pane-boot-noise`:

- `internal/reedcli/header.go` — Card 12: added `headerBlockingPayload(text string) string`, an unexported pure helper returning `"\x1b[2J\x1b[3J\x1b[H"` + trimmed text; wired the `--blocking` branch through it; updated file header comment and command `Long` text.
- `internal/reedcli/header_test.go` — Card 13: added `TestHeaderBlockingPayload`, pinning the helper's output byte-for-byte (trailing CRLF trim, no-trailing-newline passthrough, interior-newline preservation), plus an updated file header comment.
- `internal/reedcli/smoke_headerscrollback_test.go` (new) — Cards 14 and 15 combined: `TestSmokeHeaderPayloadClearsPaneScrollback` (direct ED 3 efficacy proof against a real tmux pane) and `TestSmokeHeaderPaneScrollbackIsClean` (composite backstop B across boot/resume/heal).

All 4 cards of 4 committed. Verify passed: `go test ./internal/reedcli/` and `go test -tags smoke -run 'TestSmokeHeaderPayloadClearsPaneScrollback|TestSmokeHeaderPaneScrollbackIsClean' ./internal/reedcli/` both green, plus a `go vet ./...` sanity check with no output. Working tree is clean (no uncommitted tracked changes).

{"status":"success","commit_sha":"46785dfdbed4b10c4c28974fefc04750cb8a366c","session_id":"9211818a-ceaa-44a7-915e-0a014d59f884","cards_done":[12,13,14,15]}
