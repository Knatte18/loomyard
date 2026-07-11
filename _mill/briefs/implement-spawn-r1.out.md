All 3 of 3 cards from the batch file are committed, matching their exact `Commit:` messages, and `go test ./internal/builderengine/...` (the batch's `verify:` command) passes cleanly. `git status --porcelain --untracked-files=no` shows no dirty tracked files.

{"status":"success","commit_sha":"81b207d","session_id":"32c2b427-b437-4883-a6be-d70ea2839623"}

Summary of work (3 of 3 cards committed):

- **Card 20** (`d54edf6`): Created `internal/builderengine/implementer-template.md`, extended `internal/builderengine/template.go` with `//go:embed implementer-template.md` + `ImplementerTemplate() []byte`, and extended `internal/builderengine/template_test.go` with property tests pinning the commit-per-card `NN.C:` subject shape, the bounded self-fix cap, report-as-final-action with the exact batch-report schema keys, and the never-touch-the-weft rule.
- **Card 21** (`f1de7e3`): Created `internal/builderengine/spawn.go` (`SpawnBatch`, `SpawnDeps`, `SpawnBatchOptions`, `SpawnResult`, `Starter`, `ErrPaused`, `selectRole`, `findBatch`, `batchReportFileName`) and `internal/builderengine/spawn_test.go` (tagged `//go:build integration`, mirroring `chain_test.go`/`gitquery_test.go`'s Tier 2 convention since `SpawnBatch` requires real git for `HeadSHA`), with local `spawnFakeMux`/`spawnFakeEngine` doubles wired into a real `*shuttleengine.Runner` so `Start` produces a genuine `*shuttleengine.Run` that `shuttleengine.FindRun` can resolve.
- **Card 22** (`81b207d`): Extended godoc on `SpawnBatch` and `internal/builderengine/doc.go`'s package comment to spell out the three-point weft commit boundary (spawn-batch commits state.json after a successful spawn; poll commits report+state at terminal classification; run backstops at exit); `SpawnResult` already carried the required fields (`BatchName`, `Role`, `StartSHA`, `StrandGUID`, `RunDir`, `ReportPath`).

Verified: `go build ./...`, `go test ./internal/builderengine/...` (Tier 1, the batch's declared verify), and `go test -tags integration ./internal/builderengine/...` (Tier 2, exercising the new spawn tests end-to-end) all pass. `gofmt -l` clean on all touched files.

Files touched:
- `internal/builderengine/implementer-template.md`
- `internal/builderengine/template.go`
- `internal/builderengine/template_test.go`
- `internal/builderengine/spawn.go`
- `internal/builderengine/spawn_test.go`
- `internal/builderengine/doc.go`
