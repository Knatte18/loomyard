{"status":"success","commit_sha":"f82b0777b7a31ddf21451d322fabf73b11f38f6f","session_id":"ca339cd0-1dad-464d-99b2-c18114db8537","cards_done":[17,18,19,20,21,22,23,24]}

Summary: all 8 of 8 cards in batch `04-empty-pathspec-and-unreservation` are committed and verified. `go test -tags integration ./internal/fabricengine/ ./internal/configsync/ ./internal/configcli/ ./internal/lyxcwd/ ./cmd/lyx/` passes, as does the module-wide `go vet -tags integration ./...` boundary check, and `git status --porcelain` is clean.

Key files touched:
- `internal/fabricengine/template.yaml` (card 17)
- `internal/configsync/configsync_test.go`, `internal/configcli/configcli_integration_test.go`, `internal/fabricengine/template_test.go` (card 18)
- `internal/fabricengine/junctionnames.go` (card 19)
- `internal/fabricengine/junctionnames_test.go` (card 20)
- `internal/fabricengine/structuraldirs_test.go`, `add_test.go`, `config_test.go`, `fabric_test.go`, `hostjunction_test.go` (card 21)
- `internal/fabricengine/reconcile_stale_removal_test.go` (card 22)
- `internal/fabricengine/weftgit_pathspec_integration_test.go` (card 23)
- `internal/lyxcwd/lyxcwd_test.go` (card 24)

Notable deviation from the plan text: in card 22's `TestReconcile_NeverRemovesReservedHubName`, used `_portals` rather than `_board` as the still-reserved exemplar — `_board` collided with a real per-worktree board junction `Add` already wires at that same path (`fslink.CreateDirLink` failed with "link already exists"). `_portals`/`_launchers` are hub-level only, so `_portals` was substituted; this is noted in the test's own comment.

Post-check grep confirms `pattern.DirName` now appears only in `internal/fabricengine/junction_pattern_integration_test.go`, exactly as the batch's own guardrail requires.

Card-count self-check: 8 of 8 cards committed (17, 18, 19, 20, 21, 22, 23, 24) — all complete.
