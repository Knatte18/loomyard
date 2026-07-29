Both commit subjects match the batch file's Card 26 and Card 27 `Commit:` messages exactly. 2 of 2 cards committed — all complete.

{"status":"success","commit_sha":"87eee1ad","session_id":"a3971197-76c8-41e4-8e2e-85064f469251","cards_done":[26,27]}

Summary of work completed (2 of 2 cards committed, verified against the range start `a99805b6`):

- **Card 26** (commit `7d056fb3`): Added `connKindLegacy` to the `connKind` const block in `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/ensureserver.go`. In `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/refs.go`, added `WorktreeRoot string` to `Options`, added `acquireConnection`, `teardownConnection`, and `lookup`, and rewrote `References` as a two-line wrapper over `lookup`. `resolvePosition` and `toSortedReferences` are unchanged.
- **Card 27** (commit `87eee1ad`): Added `TestReferences_HasNativeDaemonRoutesThroughEnsureServer` to `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/refs_test.go`, reusing `withTempUserCacheDir`/`withFakeInstaller` from `toolchain_test.go` (same package) to prove the `HasNativeDaemon: true` branch routes through `ensureServer` → `ensureNative` → `resolveGoToolchain` without spawning a real `gopls`. `TestReferences_NonExistentServerBinaryYieldsErrServerNotFound` was left byte-for-byte unmodified.

Verify (`go test -count=1 ./internal/codeintelengine/...`) passes. `goimports`/`gofmt`/`go vet` are clean on all touched files. `golangci-lint run ./internal/codeintelengine/...` shows only 3 pre-existing `errcheck` findings in `ensureserver.go`'s `ensureSupervised` (from batch 6, unrelated to this batch's edits — not touched here). Working tree is clean aside from the untracked brief file at `_mill/briefs/implement-wire-ensure-server-into-refs-r1.md`, which is outside this batch's scope.
