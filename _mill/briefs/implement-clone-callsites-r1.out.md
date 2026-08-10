All 3 cards of this batch (cards 20, 21, 22) are committed: card 20 in commit fb05b772, and cards 21+22 combined into commit e35b7fd4. This is 3 of 3 cards committed — all complete.

Summary of changes:
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/clone.go`: `CloneHub` now creates the hub directory via `createExclusiveDir(hubPath)` and threads the resulting `hubTok` through all 13 `teardownHub` call sites; `resetHub` gained a `cwd` parameter and now routes its removal through `removePath` with `ownedFabricHub()`/`dirtinessNA`; `teardownHub` gained `cwd` and `tok createdToken` parameters and now routes through `removePath` with `ownedFreshlyCreatedPath(tok)`/`dirtinessNA`.
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/clone_reset_guard_test.go`: updated its three direct `resetHub` call sites to pass the new `cwd` parameter (the test's parent temp directory); assertions unchanged.

Verify ran and passed: `go test ./internal/fabricengine/...` and `go test -tags integration ./internal/fabricengine/...`, plus `go build ./...`.

{"status":"success","commit_sha":"e35b7fd44b8d1b0d78eb2b88d6946db523466452","session_id":"a6c1879f-ec2c-4f1b-b7b0-feeaab4a533b","cards_done":[20,21,22]}
