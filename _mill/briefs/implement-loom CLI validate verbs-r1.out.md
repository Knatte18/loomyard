All 3 cards for this batch (cards 5, 6, 7) are committed and match the batch file's Commit: messages exactly. Verify (`go test ./internal/loomcli/... ./cmd/lyx/...`) passes. No uncommitted tracked changes remain.

Summary of work:
- `/home/knatte/Code/loomyard/wts/loom-self-checkable-mechanical-gates/internal/loomcli/validate.go` (new) — `validateDiscussionCmd`/`validatePlanCmd` verbs plus the shared generic `renderFindings` helper.
- `/home/knatte/Code/loomyard/wts/loom-self-checkable-mechanical-gates/internal/loomcli/cli.go` — registered both verbs and extended the parent's `Long`/`Example`.
- `/home/knatte/Code/loomyard/wts/loom-self-checkable-mechanical-gates/internal/loomcli/validate_test.go` (new) — tier-1 coverage of both verbs' clean/findings/fault cases.
- `/home/knatte/Code/loomyard/wts/loom-self-checkable-mechanical-gates/internal/loomcli/cli_test.go` — replaced the subset check with `TestCommand_RegisteredVerbs_ExactSet`.
- `/home/knatte/Code/loomyard/wts/loom-self-checkable-mechanical-gates/cmd/lyx/helptree_test.go` — extended the `loom` entry's `wantSubs`.

3 of 3 cards committed this turn (cards 5, 6, 7 — all with real content commits, none were `Commit: none`).

{"status":"success","commit_sha":"338c8dccc095e70caf076368d4beaeb2fe38e0fe","session_id":"9d34a226-2ddb-416c-b303-df84965a192d","cards_done":[5,6,7]}