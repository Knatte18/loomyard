{"status":"success","commit_sha":"172c955af13e17f67e7e25371080cbe9553e5287","session_id":"aa9e6430-5b6f-42bf-94a8-88a0e3c24baf"}

All work is complete. The implementation:

- **Card 18** (`e148d08`): Used `git mv internal/muxpoc internal/muxpoccli` to rename the whole directory as renames in git history, then applied surgical `package muxpoc` → `package muxpoccli` edits to all 15 files. Build constraints on `spawnattach_windows.go` and `spawnattach_other.go` were preserved verbatim.

- **Card 19** (`172c955`): Updated `cmd/lyx/main.go` to import `internal/muxpoccli` and call `muxpoccli.Command()`. The `"muxpoc"` in the Long description string was left unchanged (it refers to the CLI command name, which stays `muxpoc` per the plan's Shared Decision).

Verify passed: `go build ./...`, `go test ./...`, and `go test -tags integration ./...` all green, with `internal/muxpoccli` appearing in test results.

{"status":"success","commit_sha":"172c955af13e17f67e7e25371080cbe9553e5287","session_id":"aa9e6430-5b6f-42bf-94a8-88a0e3c24baf"}