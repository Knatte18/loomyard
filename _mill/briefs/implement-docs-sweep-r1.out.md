Correcting the commit_sha (I had guessed before confirming it) — the actual HEAD is `755ea02e7d3a1f4a6e02d5fed7b799e95b5a3441`.

Summary: all 6 cards (15-20) of the docs-sweep batch are committed. Files touched: `manifest/designs/shed.md`, `manifest/designs/loom.md`, `internal/shedengine/doc.go`, `contracts/specs/loom-status-spec.md`, `manifest/roadmap.md`, `manifest/parallel-work.md`, `internal/loomcli/wiring.go`. Verify (`go build ./...` and `go test ./internal/lyxcwd/...`) both passed. Working tree is clean.

{"status":"success","commit_sha":"755ea02e7d3a1f4a6e02d5fed7b799e95b5a3441","session_id":"e9832e99-5ae8-43ad-8864-81dbeb6158f3","cards_done":[15,16,17,18,19,20]}