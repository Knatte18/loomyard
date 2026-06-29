All four cards complete. Verify passes: `go build ./...`, `go test ./...`, and `go test -tags integration ./...` all green.

Summary of what was done across cards 9-11:

**Card 9** (committed `c897f69`): Git-moved all 18 domain test files from `internal/warp/` to `internal/warpengine/`. Updated package declarations (`warp` → `warpengine`, `warp_test` → `warpengine_test`). Retargeted imports and symbols in `config_test.go` and `list_test.go`. Exported `DeriveHostName` in `clone_test.go`. Removed `TestCloneHub_TeardownFailure` and updated `CloneHub` calls in `clone_integration_test.go`.

**Card 10** (committed `bfa0c48`): Git-moved `warp.go` → `internal/warpcli/warp.go` (package `warpcli`, all `LoadConfig`/`New` calls prefixed `warpengine.`, local `addOptionsFromEnv` helper). Created `internal/warpcli/clone.go` (`runClone`, `runCloneWithReset` calling into `warpengine`). Git-moved `warp_test.go` → `internal/warpcli/warp_test.go` (package `warpcli_test`, import retargeted to `warpcli`). Created `internal/warpcli/clone_cli_test.go` (`package warpcli`, integration-tagged, `TestCloneHub_TeardownFailure` swapping `warpengine.RemoveAll` cross-package).

**Card 11** (committed `9dc8c8c`): Retargeted all five importers (`cmd/lyx/main.go` → `warpcli`, `internal/configreg/configreg.go` + `internal/initcli/initcli.go` + `internal/initcli/initcli_test.go` + `internal/configcli/configcli_integration_test.go` → `warpengine`). `internal/warp/` was already empty after the git mv operations.

{"status":"success","commit_sha":"9dc8c8cbbe2745433c14fe04d88e0f364cdec40a","session_id":"c9f84ec5-699a-4862-9385-bf8c94366fd7"}