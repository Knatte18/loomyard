All 5 of 5 cards committed (matching the batch file's `## Cards` exactly), verify command passed, and no uncommitted tracked changes remain.

{"status":"success","commit_sha":"9366159493d52ddacce24c3e25436d8b23ed481e","session_id":"4291fd05-52ba-438f-9d78-28cf7d95a1c6"}

Summary of work for batch `fabric-core` (round 1):

All 5 cards in `/home/knatte/Code/loomyard/wts/fabric/_mill/plan/02-fabric-core.md` were completed and committed in order, each verified with `go test` + `gofmt`/`goimports`/`golangci-lint` before commit:

1. `31b438ac` — Card 3: package skeleton, config, configreg registration
   - `/home/knatte/Code/loomyard/wts/fabric/internal/fabricengine/doc.go`, `config.go`, `config_test.go`, `template.go`, `template.yaml`, `template_test.go`, `testmain_test.go`
   - `/home/knatte/Code/loomyard/wts/fabric/internal/configreg/configreg.go`, `configreg_test.go` (registered `fabric` between `burler` and `loom`)
2. `c718b259` — Card 4: `/home/knatte/Code/loomyard/wts/fabric/internal/fabricengine/branchname.go`, `branchname_test.go` (`WeftBranchName`, the sole `-weft` composition site)
3. `2c3e96d7` — Card 5: `/home/knatte/Code/loomyard/wts/fabric/internal/fabricengine/trailer.go`, `trailer_test.go` (`WarpSHATrailerKey`, `appendWarpSHATrailer`, `parseWarpSHATrailer`)
4. `53eba580` — Card 6: `/home/knatte/Code/loomyard/wts/fabric/internal/fabricengine/corrindex.go`, `corrindex_test.go` (git-free `corrIndex`/`corrEntry`)
5. `93661594` — Card 7: `/home/knatte/Code/loomyard/wts/fabric/internal/fabricengine/fabric.go`, `fabric_test.go` (`Fabric`, `New`, `SyncOptions`, `EnvSyncOptions`, `DefaultCommitMessage`, `ScopedPathspec`)

Batch verify `go test ./internal/fabricengine ./internal/configreg` passes. `go build ./...` and `go vet ./...` for the whole repo are also clean. Working tree has no uncommitted tracked changes (only the pre-existing untracked brief file `_mill/briefs/implement-fabric-core-r1.md`, which is out of scope). All 5 commits were pushed to the `fabric` branch on origin.
