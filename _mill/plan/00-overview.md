# Plan: shed: the LLM driver as a generic stepper and mender

```yaml
task: 'shed: the LLM driver as a generic stepper and mender'
slug: shed-llm-driver
approved: true
started: '20260926-113342'
parent_branch: main
root: ""
verify: null
discussion_sha: 2c758e58031f604a30263d9088bb0ed1d4e48be0
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: logger-trace-accessors
    file: 01-logger-trace-accessors.md
    depends-on: []
    verify: go test ./internal/logger/ -run 'TestTraceFile|TestTraceDir|TestEnsureDurableSink|TestNotifyExit' && go test -tags integration ./internal/logger/ -run 'TestDurableSink_CwdFallback'
  - number: 2
    name: fabric-mutation-trace
    file: 02-fabric-mutation-trace.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/fabricengine/ -run 'TestMutations|TestRefDetail' && go test -tags integration ./internal/fabricengine/ -run 'TestPushAnchored_PushesAndRecordsBranchPush|TestPushWarpRebaseFreeAt_PushesAndRecordsBranchPush'
  - number: 3
    name: shed-envelope-trace
    file: 03-shed-envelope-trace.md
    depends-on: [1, 2]
    verify: go build ./... && go test ./internal/shedverbs/ ./internal/loomcli/ ./internal/battencli/ ./internal/landingshed/ -run 'TestStep|TestStatus|TestNoResolverNoModuleCLIInvariant|TestSpecFor|TestBattenPreStep|TestPublish|TestVerbRefusals|TestRun' && go test -tags integration ./internal/battencli/ -run 'TestBattenIntegration_RealReadStatus|TestBattenIntegration_NonPrimeRefusal' && go test -tags integration ./internal/landingshed/ -run 'TestPublish_MergesInCleanlyBeforeCreatingPullRequest' && go test -tags smoke ./internal/loomcli/ -run 'TestSmokeStatusAndPause_OnNeverBootstrappedPairNameTheRemedy|TestSmokeStep_RecordsCleanHandoffMarkerMatchingPersistedStatus'
  - number: 4
    name: step-entry-point-integration
    file: 04-step-entry-point-integration.md
    depends-on: [3]
    verify: go test -tags integration ./internal/shedcli/ -run 'TestParity_'
  - number: 5
    name: ly-drive-recipe-blind
    file: 05-ly-drive-recipe-blind.md
    depends-on: [3, 4]
    verify: go build ./... && go test ./cmd/lyx/ -run 'TestLyDriveSkill|TestHelpTree|TestDriftGuard' && go test ./internal/loomcli/ -run 'TestDriverPrompt|TestStartLLMDriverArm' && go test -tags integration ./internal/loomcli/ -run 'TestIntegrationDriverBootstrap' && go test -tags smoke ./internal/loomcli/ -run 'TestSmokeStatusAndPause_OnNeverBootstrappedPairNameTheRemedy' && go test ./internal/shedadapters/ -run 'TestBouncer_ReBounceProbesForALiveSeed'
```

## Shared Decisions

### Decision: trace-record-message-vocabulary

- **Decision:** the new durable-sink records use four fixed message strings, and the skill and tests match on them verbatim:
  `"fabric: mutation"` (attrs `kind`, `target`, `detail`) from `Mutations.Append`/`AppendRef`;
  `"shed: step"` (attr `status_file`) at step entry;
  `"shed: step done"` (attrs `producer`, `outcome`, `state`, `next`, `reason`) after `shed.Step` returns without error;
  `"shed: step refused"` (attrs `kind`, `error`) at `Warn` on each step error envelope;
  and `"landingshed: pull request created"` (attrs `owner`, `repo`, `number`) at `Info` after a successful `PullRequests.Create`.
- **Rationale:** the driver reads trace lines by message; one vocabulary stated once keeps the skill, the Go and the tests from drifting.
- **Applies to:** fabric-mutation-trace, shed-envelope-trace, step-entry-point-integration, ly-drive-recipe-blind

### Decision: ref-detail-format

- **Decision:** every `branch_created`/`branch_pushed` entry's `detail` is `side=<warp|weft> repo=<abs path>` for a create and `side=<warp|weft> repo=<abs path> remote=<name>` for a push, built by one helper `refDetail(side, repo, remote string) string` in `internal/fabricengine/mutation.go`.
  `remote=` is omitted when the remote is unknown (empty), which the skill treats as failing the stranded-branch rule.
  `branch_deleted`/`remote_branch_deleted` detail is unchanged.
- **Rationale:** Decision `repair-scope` in the discussion; one helper means one grammar.
- **Applies to:** fabric-mutation-trace, ly-drive-recipe-blind

### Decision: trace-tests-use-sink-override

- **Decision:** every new test that reads the durable trace arms it with `logger.SetDurableSinkDir(t.TempDir())` and registers `t.Cleanup(func() { logger.SetDurableSinkDir("") })`, then globs `trace-*.log` in that directory.
  No test sets `LYX_TRACE=1`.
  Such a test never calls `t.Parallel()`, even in a file whose other tests all do (e.g. `internal/fabricengine/mutation_test.go`): the sink's override, path and header are package-level state one test's `SetDurableSinkDir` resets out from under another, the hazard `internal/burlercli/cli_test.go` already documents.
  Its doc comment says so in one line.
- **Rationale:** the sink refuses to arm under `testing.Testing()` without an override; the override keeps trace files out of fixtures and the empty reset returns the process to the no-sink state other tests expect.
- **Applies to:** all batches with Go tests

### Decision: lint-not-recommended-for-done-gate

- **Decision:** recommend no lint command in `pipeline.done_gate`.
  `golangci-lint run` exits 1 against the current tip (pre-existing findings unrelated to this task, e.g. `cmd/lyx/drift_test.go` S1025).
  The effective `done_gate` (`go test ./... && go test -tags integration ./...`) is already repo-wide, so no test-command recommendation differs from it either.
- **Rationale:** mill-plan's done-gate guidance: lint debt unrelated to the task is recorded, not pushed onto it.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/lydriveskill_test.go`
- `docs/overview.md`
- `internal/battencli/arm.go`
- `internal/battencli/step_test.go`
- `internal/battenshed/doc.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/coalesce.go`
- `internal/fabricengine/mutation.go`
- `internal/fabricengine/mutation_test.go`
- `internal/fabricengine/pushanchored.go`
- `internal/fabricengine/pushanchored_integration_test.go`
- `internal/fabricengine/spawn.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabricengine/weftwiring.go`
- `internal/landingshed/publish.go`
- `internal/landingshed/publish_test.go`
- `internal/logger/logger.go`
- `internal/logger/sink.go`
- `internal/logger/sink_test.go`
- `internal/loomcli/arm.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/cli_test.go`
- `internal/loomcli/driverprompt.go`
- `internal/loomcli/driverprompt_test.go`
- `internal/loomcli/smoke_bootstrapwiring_test.go`
- `internal/loomcli/start.go`
- `internal/loomcli/status_test.go`
- `internal/loomcli/step_test.go`
- `internal/shedadapters/bouncer_seed_test.go`
- `internal/shedcli/parity_test.go`
- `internal/shedverbs/doc.go`
- `internal/shedverbs/seam_enforcement_test.go`
- `internal/shedverbs/spec.go`
- `internal/shedverbs/status.go`
- `internal/shedverbs/status_test.go`
- `internal/shedverbs/step.go`
- `internal/shedverbs/step_test.go`
- `manifest/roadmap.md`
- `plugins/ly/skills/INDEX.md`
- `plugins/ly/skills/ly-drive/SKILL.md`
