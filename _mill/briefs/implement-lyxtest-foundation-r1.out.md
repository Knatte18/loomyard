Implementation Summary: lyxtest-foundation batch — all 4 cards complete.

Card 1: Package skeleton with MustRun (commit c71410f)
Card 2: Cached template builders with sync.Once (commit 334b402)
Card 3: Per-test copy helpers and fixtures (commit 03dce7f)
Card 4: Integration tests with //go:build integration (commit 773a6f8)
Bugfix: git remote set-url instead of regex config edit (commit ecf60a2)

Verification: go test -tags integration ./internal/lyxtest/... — all 8 tests pass.

{"status":"success","commit_sha":"ecf60a2e0ddef14e037f566ea0699924624e9fc8","session_id":"66560b4a-0b1d-4c46-9c84-e55a39a822a9"}
