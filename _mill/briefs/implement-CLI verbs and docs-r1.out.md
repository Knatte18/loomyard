All 3 of 3 cards committed and verified.

{"status":"success","commit_sha":"9d0f02914f269baaa3ed492b6d7a638092ee5ca6","session_id":"16649a4c-d72c-4fd6-afde-c785e0fe68b8","cards_done":[16,17,18]}

Summary: implemented batch "CLI verbs and docs" (3 of 3 cards committed):

- Card 16 (`3ba784e7`): `internal/fabriccli/merge_verbs.go` (new), `internal/fabriccli/envelope.go`, `internal/fabriccli/weft_verbs.go` — added `lyx fabric merge`/`merge-in` verbs and the `errConflictsWithRecord` envelope helper.
- Card 17 (`d97695b5`): `cmd/lyx/helptree_test.go`, `internal/fabriccli/argsarity_test.go`, `internal/fabriccli/envelope_test.go`, `internal/fabriccli/cli_test.go`, `internal/fabriccli/envelopecontract_integration_test.go`, `internal/fabriccli/merge_cli_integration_test.go` (new) — help-tree/arity/envelope-contract coverage plus end-to-end integration tests.
- Card 18 (`9d0f0291`): `internal/fabricengine/doc.go`, `manifest/designs/finalize.md`, `manifest/roadmap.md`, `docs/overview.md` — the merge-surface godoc section, finalize.md reword, roadmap move + Someday entries, and overview.md CLI-surface updates.

Verify (`go test ./cmd/lyx/ ./internal/fabriccli/ ./internal/lyxcwd/ && go test -tags integration -run Merge ./internal/fabriccli/`) passes. Working tree is clean (no uncommitted tracked changes).
