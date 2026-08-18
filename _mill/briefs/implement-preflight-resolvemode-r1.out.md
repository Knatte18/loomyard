3 of 3 cards committed — matches the batch's declared card count exactly (Cards 1, 2, 3, all with distinct commits). Full verify (`go test ./internal/preflight/...` and `go test -tags integration ./internal/preflight/...`) passed.

{"status":"success","commit_sha":"d6cbec56ad61898fdc77d13fd0396235aa59588a","session_id":"0162a819-a281-435e-8ab8-8104cb5b3dce","cards_done":[1,2,3]}

Summary: All 3 of 3 cards in batch 01-preflight-resolvemode were committed this turn.

- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/preflight/predicates.go` — added `Mode`/`ModeHub`/`ModeStandalone`, extracted `boardLyxPresent` (shared by `HubPresent` and the new `ResolveMode`), and implemented `ResolveMode(cwd) (*lyxcwd.Location, Mode, error)` per the four-step spec (commit `6b861365`).
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/preflight/doc.go` — rewrote the "why there are two predicates" section into a three-function section documenting `ResolveMode` alongside `Wired`/`HubPresent` (commit `cdec7072`).
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/preflight/preflight_integration_test.go` — added `TestResolveMode` pinning all seven rows of the hub/standalone/refuse table, using `gitkit.MustRun("git","init",...)` (never `gitkit.CopyRepo`, respecting the gitkit Leaf Invariant) for the plain-repo rows (commit `d6cbec56`).

Verify passed: `go test ./internal/preflight/...` and `go test -tags integration ./internal/preflight/...` both green, plus `internal/gitkit`'s `TestCopyRepoCallerSet_LyxcwdOnly` reconfirmed clean.

{"status":"success","commit_sha":"d6cbec56ad61898fdc77d13fd0396235aa59588a","session_id":"0162a819-a281-435e-8ab8-8104cb5b3dce","cards_done":[1,2,3]}