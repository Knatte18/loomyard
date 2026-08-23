1 of 1 card committed — all complete.

{"status":"success","commit_sha":"7e8aa7f22a2f349814780589d3bdabe993d11308","session_id":"c78f09e6-ad04-40bf-a3e1-b9ffd8509266","cards_done":[8]}

Summary: 1 of 1 card committed (Card 8, the only card in this batch). Verify passed for both `go test ./internal/loomcli/...` and `go test ./internal/lyxcwd/...`.

Files touched:
- `/home/knatte/Code/loomyard/wts/loom-self-checkable-mechanical-gates/internal/loomcli/parity_test.go` (new) — `TestGateParity_DiscussionValidate` and `TestGateParity_PlanValidate`, three-way verdict comparison (`verdictDone`/`verdictStuck`/`verdictError`) between the `loomshed` producer and the `loomcli` verb over shared fixtures reused from `validate_test.go`.
- `/home/knatte/Code/loomyard/wts/loom-self-checkable-mechanical-gates/CONSTRAINTS.md` — added `## Gate Self-Check Parity Invariant`, placed immediately after the Discussionparser Sole-Parser Invariant.

Commit: `7e8aa7f2` on branch `loom-self-checkable-mechanical-gates`, pushed to origin.

{"status":"success","commit_sha":"7e8aa7f22a2f349814780589d3bdabe993d11308","session_id":"c78f09e6-ad04-40bf-a3e1-b9ffd8509266","cards_done":[8]}