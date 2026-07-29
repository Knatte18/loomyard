All 4 cards (22, 23, 24, 25) declared in the batch have matching commits. 4 of 4 cards committed — complete.

Summary of work for batch `ensure-server-supervised` (round 1):

- **Card 22** (`529bb8f2`): Added `proc.DetachBreakaway` to `internal/proc/proc_linux.go` and `internal/proc/proc_windows.go` (with `createBreakawayFromJob` const), plus `TestDetachBreakaway`/`TestDetachBreakawayDoesNotAffectDetach` in `internal/proc/proc_windows_test.go`.
- **Card 23** (`1a7c9a64`): Added `ErrServerSpawnTimeoutSentinel`/`ErrServerSpawnTimeout` to `internal/codeintelengine/errors.go`.
- **Card 24** (`9ac37157`): Implemented `ensureSupervised` in `internal/codeintelengine/ensureserver.go` (spawn-race lock, deterministic socket path, dial-or-restart, retry-exhaustion).
- **Card 25** (`a288ba93`): Added `internal/codeintelengine/supervised_test.go` (offline retry-exhaustion + stale-socket-cleanup tests) and `internal/codeintelengine/supervised_integration_test.go` (spawn/reconnect/kill-respawn against real gopls, build-tag `integration`), plus the `tierpurity_test.go` allowlist entry.

Notable finding fixed along the way: `ensureSupervised` originally failed on a worktree's first-ever call because `lock.TryAcquireWriteLock` never creates the lock file's parent directory (`gofrs/flock` doesn't `MkdirAll`). Caught by running the real integration test against a locally-installed `gopls`; fixed by adding `os.MkdirAll(filepath.Dir(lockPath), 0o755)` before the lock attempt in `internal/codeintelengine/ensureserver.go`, matching the existing `goToolchainInstallLock` precedent in `toolchain.go`.

Verified: `go build ./...`, `go vet ./...`, `go build -tags integration ./...`, `go vet -tags integration ./...`, the batch's declared `verify:` command, and additionally `go test -race -tags integration ./internal/codeintelengine/...` — all clean.

{"status":"success","commit_sha":"a288ba93","session_id":"7c382855-0a49-4748-8d8b-1bef03bfad36","cards_done":[22,23,24,25]}
