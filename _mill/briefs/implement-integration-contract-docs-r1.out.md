{"status":"success","commit_sha":"a2a2df62bf57991bf80573676a501575a81da56a","session_id":"886dd527-68d7-4adf-9e6f-482b1d604dbc","cards_done":[8,9,10,11]}

All 4 of 4 cards in this batch are committed and complete (cards 8, 9, 10, 11). Both `verify:` sub-commands pass:
- `go test ./internal/reedengine/...` — ok
- `go test -tags integration ./internal/reedengine/...` — ok (ran live against real tmux 3.6/pty, all new integration tests pass)

Files touched, all under `/home/knatte/Code/loomyard/wts/reed-attach-header-height-bug`:
- `internal/reedengine/attachgeometry_integration_test.go` (card 8, commit `224d7a6b`)
- `internal/reedengine/contract_integration_test.go` (card 9, commit `ee0b5c45`)
- `internal/reedengine/doc.go` (card 10, commit `bb210e0c`)
- `manifest/roadmap.md` (card 11, commit `a2a2df62`)

Notes on deviations from a literal read of the plan (both empirically necessary, no scope creep — same files, no new files touched):
- Card 9's `waitUntil` condition originally checked `sum of pane heights == 60`, which never becomes true because tmux reserves one row per inter-pane divider (verified live: two panes summed to 59, not 60, after a 60-row resize). Fixed to check `max(pane.Top+pane.Height) == 60` instead, which is what the requirement's own live-verified numbers (heights 1 and 58) actually describe.
- Card 9 also required a local variable rename (`live`→`afterResizeLive`) to avoid a `:=` redeclaration collision with a pre-existing `live` variable later in the same test function.

{"status":"success","commit_sha":"a2a2df62bf57991bf80573676a501575a81da56a","session_id":"886dd527-68d7-4adf-9e6f-482b1d604dbc","cards_done":[8,9,10,11]}