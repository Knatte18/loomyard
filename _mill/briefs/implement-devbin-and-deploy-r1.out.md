All 4 of 4 cards from the batch's `## Cards` section are committed, matching each declared `Commit:` message exactly. Verify (`go test ./tools/internal/devbin/ ./tools/deploy/`) passed. Working tree is clean of tracked in-scope changes.

Summary: 4 of 4 cards committed.
- Card 1: `34c97ec4` — created `tools/internal/devbin/devbin.go` (`RepoRoot`, `Dir`, `BinPath`)
- Card 2: `7c97f6cd` — created `tools/internal/devbin/devbin_test.go`
- Card 3: `fff67689` — edited `tools/deploy/main.go` (added `-dev` flag, `resolveDest`, removed local `repoRoot()`, now uses `devbin.RepoRoot()`)
- Card 4: `78db2cfd` — created `tools/deploy/main_test.go` covering `resolveDest`

Relevant files (absolute paths):
- `/home/knatte/Code/loomyard/wts/dev-test-binary/tools/internal/devbin/devbin.go`
- `/home/knatte/Code/loomyard/wts/dev-test-binary/tools/internal/devbin/devbin_test.go`
- `/home/knatte/Code/loomyard/wts/dev-test-binary/tools/deploy/main.go`
- `/home/knatte/Code/loomyard/wts/dev-test-binary/tools/deploy/main_test.go`

{"status":"success","commit_sha":"78db2cfd","session_id":"4775aea4-5b47-494c-a9f8-e4ab5e7a7d54"}
