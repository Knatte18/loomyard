Good, 40 hex chars — confirmed. All 5 cards (6, 7, 8, 9, 10) of the batch are committed, verify passed in full (untagged suites, integration tests, and smoke vet-checks), and the working tree is clean.

{"status":"success","commit_sha":"297d582be2b8187de8c82866d409a5c3077506f7","session_id":"4a15a59c-f2e9-4fc4-b2cc-f5163c5043f3","cards_done":[6,7,8,9,10]}

Summary: implemented all 5 cards of batch `reed-geometry-hubgeom` (03-reed-geometry-hubgeom.md) — 5 of 5 cards committed, matching the batch's declared card count exactly.

Files touched:
- Created `/home/knatte/Code/loomyard/wts/shuttle-reed-told-geometry/internal/reedengine/geometry.go` (Card 6)
- Created `/home/knatte/Code/loomyard/wts/shuttle-reed-told-geometry/internal/hubgeom/{hubgeom.go,doc.go,hubgeom_test.go}` (Card 7)
- Edited `internal/reedengine/{lock.go,lifecycle.go,strand.go,header.go,doc.go,lock_test.go,header_test.go,contract_integration_test.go,mouse_boot_integration_test.go}`, the five CLI construction sites (`internal/{burlercli,perchcli,webstercli,shuttlecli,reedcli}/cli.go`), and the four smoke fixtures (`internal/shuttlecli/smoke_interrupt_test.go`, `internal/treadleengine/smoke_judge_test.go`, `internal/burlerengine/{smoke_cluster_test.go,smoke_round_test.go}`) (Card 8)
- Edited `CONSTRAINTS.md` (Card 9)
- Edited `docs/overview.md` and `manifest/designs/producers-standalone.md` (Card 10)

Verify ran and passed in full: untagged suites for `hubgeom`, `reedengine`, all five CLI packages and `cmd/lyx`; `go test -tags integration ./internal/reedengine/...` (107 real-tmux tests, 0 failures); `go vet -tags smoke` over the four smoke packages.

{"status":"success","commit_sha":"297d582be2b8187de8c82866d409a5c3077506f7","session_id":"4a15a59c-f2e9-4fc4-b2cc-f5163c5043f3","cards_done":[6,7,8,9,10]}
