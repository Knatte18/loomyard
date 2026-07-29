All 5 cards (Card 5, Card 6, Card 7, Card 8, Card 9) are committed and match the batch's declared 5 cards exactly — 5 of 5 cards complete. Verify (`go test -count=1 ./internal/codeintelengine/... ./cmd/lyx/...`) passes. Working tree is clean.

{"status":"success","commit_sha":"81240b4e","session_id":"642c181e-c7c4-4417-b9a3-2495cbad37bd","cards_done":[5,6,7,8,9]}

Summary of work (all files under `/home/knatte/Code/loomyard/wts/codeintel-v1`):

- `internal/codeintelengine/toolchain.go` (created, then extended twice): `goToolchainCacheDir`/`goToolchainInstallLock` path helpers over a `userCacheDir` seam var; `toolchainInstaller` type, `installGoToolchain`/`runGoInstall` production seam; `resolveGoToolchain` with the fast path / blocking-lock / double-check / install sequence. Found and fixed a real bug during test-writing: `lock.AcquireWriteLock` (gofrs/flock) never creates the lock file's parent directory, so a genuinely cold cache needed an explicit `os.MkdirAll` before acquiring the install lock — fixed to match `internal/state`'s/`internal/reedengine`'s existing MkdirAll-before-lock pattern.
- `internal/codeintelengine/leaf_enforcement_test.go`: allowlisted `internal/lock`.
- `CONSTRAINTS.md`: Codeintelengine Leaf Invariant amended for `internal/lock` (card 6); Sandbox Suite Coverage backport adding the `codeintel` exclusion line verbatim (card 7).
- `internal/codeintelengine/toolchain_test.go` (created): offline, mocked-installer coverage of already-installed / cold-install / concurrent-install-serialization paths.
- `internal/codeintelengine/toolchain_integration_test.go` (created): `//go:build integration`-tagged, unconditional (no skip), real `go install` + `gopls version` smoke test.
- `cmd/lyx/hermeticenv_test.go`: added the `internal/codeintelengine` package-level entry to `allowedNonHermetic`.

Card count check: 5 of 5 cards in this batch committed (commits `9e5ede55`, `e148c514`, `579ed6e2`, `cd9c46e9`, `81240b4e`), matching every `### Card N:` heading's `Commit:` message.
