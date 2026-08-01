All 8 of 8 cards committed and verified (confirmed by counting commits since batch-start commit `3884571...` against the batch file's `## Cards` — cards 6 through 13, one commit each, all present).

Summary of work:
- Card 6: `internal/scoutengine/ensureserver_integration_test.go` retagged `integration` → `scout`
- Card 7: `internal/scoutengine/refs_integration_test.go` retagged `integration` → `scout`
- Card 8: `internal/scoutengine/supervised_integration_test.go` retagged `integration` → `scout`
- Card 9: `internal/scoutengine/toolchain_integration_test.go` retagged `integration` → `scout`
- Card 10: split `internal/scoutengine/supervised_test.go`'s two gopls-gated subtests (`TestEnsureSupervised_StaleSocketCleanupAllowsRebind`, `TestEnsureSupervised_DaemonLogsToOwnFileNotCallersStderr`) into new `internal/scoutengine/supervised_scout_test.go` (`//go:build scout`, runtime skip-gate dropped); rewrote `supervised_test.go`'s header comment
- Card 11: `cmd/lyx/tierpurity_test.go`'s `allowedSpawners["internal/scoutengine/supervised_test.go"]` reason string updated to describe the three remaining subtests
- Card 12: `cmd/lyx/sandbox_coverage_test.go`'s `excludedModules["scout"]` reason string updated (`integration` → `scout`)
- Card 13: `CONSTRAINTS.md`'s Sandbox Suite Coverage Allowlist bullet updated to match

Batch verify command passed in full:
`go build -tags scout ./cmd/lyx/... ./internal/scoutengine/...`, `go vet -tags scout ...`, `go test ./cmd/lyx/... ./internal/scoutengine/... -count=1`, `go test -tags integration ./cmd/lyx/... ./internal/scoutengine/... -count=1` — all green.

`go test -tags scout` itself was not run (per the plan's explicit Decision: `gopls` is not installed in this environment, and Card 10 removed the runtime skip-gate, so that run would hard-fail here rather than skip). This is documented in the batch's `## Batch Tests` section as a manual, post-merge, gopls-equipped-machine confirmation step — not part of this batch's machine-enforced gate.

No tracked files are dirty; final HEAD is `9ce859554f47231a8163500d40d7e0d06021e2c1`.

{"status":"success","commit_sha":"9ce859554f47231a8163500d40d7e0d06021e2c1","session_id":"e082c301-7f61-433d-8d9a-8e993d909bc0","cards_done":[6,7,8,9,10,11,12,13]}