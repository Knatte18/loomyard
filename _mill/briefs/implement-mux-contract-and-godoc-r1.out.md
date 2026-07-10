No tracked in-scope modifications remain uncommitted. Both cards are committed and pushed, and verify passes.

Summary of work:
- `internal/muxengine/doc.go` — appended a "Multiplexer contract surface" godoc section documenting the six `#{pane_*}` format vars, the `list-panes -F` string, `parsePaneList`'s parsing (`pane_dead=="1"`), the required subcommand set, and each load-bearing behavioral assumption (silent split failure, remain-on-exit dead-pane adoption, the `-l` leading-dash send-keys bug, empty-layout session destruction, async kill-server/probe-always-exits-0). Comment-only.
- `internal/muxengine/contract_integration_test.go` (new, `//go:build integration`) — `TestMultiplexerContract` loads `Config` via `LoadConfig`, skips cleanly if the configured binary is absent, spawns a real psmux server on a scratch `-L` socket, and asserts the exact wire contract from doc.go against the live binary. Torn down via `t.Cleanup`.

Both cards verified passing on this box, each run committed individually and pushed to `origin/facilitate-linux`.

{"status":"success","commit_sha":"3835a85","session_id":"c047ce6f-2be5-41e1-9250-cfef5999f67d"}
