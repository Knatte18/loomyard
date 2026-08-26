41 chars including newline = 40-char SHA. Confirmed correct full SHA.

Summary: All 4 cards of batch `01-planparser-split-and-writer.md` were implemented and committed:

- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/planparser/validate.go` — split into `ValidateFormat`/`Validate`/`validate` (card 1)
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/planparser/approve.go` (new) and `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/planparser/doc.go` — added `SetApproved` (card 2)
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/planparser/validate_test.go` — split test coverage (card 3)
- `/home/knatte/Code/loomyard/wts/loom-plan-approval-gate/internal/planparser/approve_test.go` (new) — table test for `SetApproved` (card 4)

4 of 4 cards committed this turn. `go test ./internal/planparser/...` passes in full, working tree is clean.

{"status":"success","commit_sha":"4a930326f626d60b2b42493de656f5a4224047f2","session_id":"c600e100-6d9e-4742-bb68-0fb4359b3f9b","cards_done":[1,2,3,4]}
