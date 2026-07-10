Both cards' commit subjects match exactly. 2 of 2 cards committed. Final status check confirms working tree is clean and verify passes.

{"status":"success","commit_sha":"580b83e","session_id":"a8a15d5e-3393-49e2-8f42-fab32068cf7b"}

Summary: Both cards in batch `06-crosscompile-gate-and-roadmap` are committed.

- `cmd/lyx/crosscompile_test.go` (new) — `TestCrossCompileLinux` shells `GOOS=linux go build ./...` from module root, skips if `go` absent, `t.Fatalf`s with combined output on failure. Commit `e71ba04`.
- `docs/roadmap.md` — appended milestone 25 (🚧 planned), deferred real-Linux validation checklist. Commit `580b83e`.

Verify `go test ./cmd/lyx/ -run TestCrossCompileLinux` passes. Working tree clean.
