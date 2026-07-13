{"status":"success","commit_sha":"77393ab9a022e65473d67d00cb310ff7549bd8a5","session_id":"657648c2-1f2f-4e08-a54a-4c46b106e2f6"}

Summary: 1 of 1 card committed for batch `01-retier-builder-tests.md`. Added `//go:build integration` as the first non-empty line (before the file-doc comment and `package` clause) to the four builder test files flagged by the tier-purity guard:

- `C:\Code\loomyard\wts\faster-git-fixture-tests\internal\buildercli\spawnbatch_test.go`
- `C:\Code\loomyard\wts\faster-git-fixture-tests\internal\buildercli\validate_test.go`
- `C:\Code\loomyard\wts\faster-git-fixture-tests\internal\builderengine\config_test.go`
- `C:\Code\loomyard\wts\faster-git-fixture-tests\internal\builderengine\template_test.go`

No test code was otherwise changed. Verified Tier 1 (`go test ./cmd/lyx -run TestTierPurity -count=1`) passes, then the full batch verify command (`go test -tags integration -run TestTierPurity -count=1 ./cmd/lyx ./internal/buildercli ./internal/builderengine`) passes with all three packages green.

Note on formatting: `gofmt -l`/`goimports -l` flag these files, but confirmed this is a pre-existing, repo-wide artifact of the Windows checkout (`core.autocrlf=true` converts LF blobs to CRLF on checkout) — even the untouched `cmd/lyx/tierpurity_test.go` is flagged. `git diff --cached --stat` after staging showed exactly the intended `+2` lines per file (git normalizes CRLF back to LF automatically on add/commit), so no formatter-drift commit was needed.

Commit: `77393ab9a022e65473d67d00cb310ff7549bd8a5` — "test(builder): re-tier git-spawning tests behind the integration tag" — pushed to `faster-git-fixture-tests`.
