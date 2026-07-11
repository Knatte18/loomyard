All 3 of 3 cards for batch `orchestrator-run` are committed, matching their exact `Commit:` messages from `_mill/plan/06-orchestrator-run.md`:

1. `65d8b6c` — Card 23: `feat(builder): embedded orchestrator prompt template`
2. `043c364` — Card 24: `feat(builder): outcome.yaml contract with stale-file archiving`
3. `44b0330` — Card 25: `feat(builder): engine Run with lock, gates, and outcome mapping`

Verify command `go test ./internal/builderengine/...` passes (confirmed both scoped and via the whole-repo Tier 1 `go test ./...`). Working tree is clean (no uncommitted tracked changes). All commits pushed to `origin/internal-builder`.

Files touched:
- `internal/builderengine/orchestrator-template.md` (new)
- `internal/builderengine/template.go` (edited — added `OrchestratorTemplate()`)
- `internal/builderengine/template_test.go` (edited — added orchestrator-template property tests)
- `internal/builderengine/outcome.go` (new — `Outcome`, `ParseOutcome`, `ArchiveStaleOutcome`)
- `internal/builderengine/outcome_test.go` (new)
- `internal/builderengine/runlevel.go` (new — `Run`, `RunDeps`, `RunOptions`, `RunResult`, `ErrRunBusy`, `ErrFingerprintMismatch`, `OrchestratorAskingError`/`DiedError`/`TimeoutError`)
- `internal/builderengine/runlevel_test.go` (new)

Note on tooling: `golangci-lint` is not installed in this environment and `go install` for it fails (an upstream transitive dependency, `github.com/tdakkota/asciicheck`, 404s on GitHub -- not something fixable from this repo). Ran `goimports -l` and `gofmt -l`/`-w` on all changed files instead (gofmt caught and fixed minor alignment drift in `runlevel_test.go`), plus `go vet ./...` and the full `go test ./...` Tier 1 suite, all clean.

{"status":"success","commit_sha":"44b0330","session_id":"0f724765-520e-4743-b30f-2ae63153d798"}
