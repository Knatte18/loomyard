# Plan: fabric: close the weft-visibility leak (slice 8)

```yaml
task: 'fabric: close the weft-visibility leak (slice 8)'
slug: fabric-weft-visibility-cleanup
approved: false
started: '20260806-185219'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: fabric API expand
    file: 01-fabric-api-expand.md
    depends-on: []
    verify: go test -tags integration ./internal/fabricengine/
  - number: 2
    name: typed Healthy reason and Clean reword
    file: 02-typed-health-and-clean.md
    depends-on: [1]
    verify: go test -tags integration ./internal/fabricengine/ ./internal/loomengine/
  - number: 3
    name: consumer call-site migration
    file: 03-consumer-migration.md
    depends-on: [1]
    verify: go test -tags integration ./internal/buildercli/ ./internal/webstercli/ ./internal/perchcli/ ./internal/configcli/ ./internal/builderengine/ ./internal/websterengine/ ./internal/fabriccli/ && go test ./cmd/lyx/
  - number: 4
    name: constructor contract (unexport)
    file: 04-constructor-contract.md
    depends-on: [2, 3]
    verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/
  - number: 5
    name: templates describe one repo
    file: 05-templates-one-repo.md
    depends-on: [3]
    verify: go test -tags integration ./internal/websterengine/ ./internal/builderengine/ ./internal/burlerengine/
  - number: 6
    name: comment and test vocabulary sweep
    file: 06-comment-sweep.md
    depends-on: [2, 3]
    verify: go vet -tags integration ./... && go test ./cmd/lyx/
  - number: 7
    name: vocabulary enforcement test
    file: 07-enforcement.md
    depends-on: [4, 5, 6]
    verify: go test ./internal/lyxcwd/
  - number: 8
    name: documentation
    file: 08-docs.md
    depends-on: [7]
    verify: go vet ./internal/fabricengine/
```

## Shared Decisions

### Decision: expand-migrate-contract sequencing

- **Decision:** the API change ships in three waves — batch 01 adds the new surfaces (`Open`, `Ready`, `Committed()`, `RefScanner`) as pure additions while `New`/`Fabric.Warp`/`Fabric.Weft` stay exported;
  batches 02-03 migrate every caller;
  batch 04 unexports.
  Every card-level commit compiles and passes its package tests — the repo is never left non-compiling between commits.
- **Rationale:** the discussion's sequencing note ("unexporting `New` breaks `fabriccli` in the same compile unit") forbids a one-shot flip across packages;
  expand-migrate-contract is the standard way to keep each commit green.
- **Applies to:** all batches

### Decision: comments ride with code edits

- **Decision:** every card that edits a file also rewords that file's `weft`/`warp`/fabric-sense-`host` comments per `comment-fidelity`;
  batch 06 sweeps only files no code card touches.
  A file appears in at most one batch, except `websterengine/integration.go` (card 12 identifier rename, card 20 residual comments — sequential via the DAG) and `fabricengine/doc.go`/`commit.go`/`open.go` (batches 01/04/08, all on one dependency chain).
- **Rationale:** avoids parallel-batch write conflicts and double-visits;
  the `parallel-modifies-overlap` validator only tolerates shared files on dependent batches.
- **Applies to:** all batches

### Decision: vocabulary owner set and carve-outs

- **Decision:** owners (vocabulary stays): `internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/lyxtest`, `internal/boardengine`, `internal/configsync` (string literals only), `tools/`, `sandbox/`.
  Verbatim carve-outs everywhere: `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`, the PowerShell cmdlet `Write-Host`, `lyxtest` owner-API selectors in tests, and the `lyx-test-weft`/`lyx-fabric-test-weft` GitHub URLs plus the identifiers naming them.
  `host` is policed only as a fabric-sense phrase (`host repo`, `host worktree`, `host HEAD`, `hostBranch`, …), never as a bare word.
- **Rationale:** decisions `fabric-vocabulary-rule` and the discussion's Out list;
  the carve-outs are external resources or owner API, not leaks.
- **Applies to:** all batches

### Decision: test tagging for new tests

- **Decision:** every new test that builds a git fixture (`lyxtest.CopyPaired` etc.) goes in a file whose first line is `//go:build integration`;
  fixture-free table tests stay untagged.
- **Rationale:** Test Tier Purity Invariant — untagged files must not spawn;
  the existing fabricengine integration suite follows the same split.
- **Applies to:** all batches

### Decision: scoped verify, full suite at the done gate

- **Decision:** per-batch `verify:` runs only the touched packages (with `-tags integration` where the batch touches tagged tests);
  the repo-wide `go test ./... && go test -tags integration ./...` runs once via the configured `pipeline.done_gate`, satisfying the discussion's full-suite gate.
- **Rationale:** verify runs after every implementer and fixer round;
  the full dual-suite is minutes-long and belongs at the boundary, not in the loop.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `README.md`
- `cmd/lyx/boardguard_test.go`
- `cmd/lyx/rawgitmutation_test.go`
- `cmd/lyx/tierpurity_test.go`
- `docs/benchmarks/test-suite-timing.md`
- `docs/overview.md`
- `docs/reference/builder-contract.md`
- `docs/skills.md`
- `internal/batcher/doc.go`
- `internal/buildercli/cli.go`
- `internal/buildercli/gitfixture_test.go`
- `internal/buildercli/pause_spawnbatch_test.go`
- `internal/buildercli/poll.go`
- `internal/buildercli/poll_test.go`
- `internal/buildercli/run.go`
- `internal/buildercli/run_test.go`
- `internal/buildercli/smoke_test.go`
- `internal/buildercli/spawnbatch.go`
- `internal/buildercli/spawnbatch_test.go`
- `internal/buildercli/status.go`
- `internal/buildercli/sync.go`
- `internal/buildercli/sync_integration_test.go`
- `internal/buildercli/sync_test.go`
- `internal/buildercli/validate_test.go`
- `internal/builderengine/chain.go`
- `internal/builderengine/config_test.go`
- `internal/builderengine/doc.go`
- `internal/builderengine/gitquery_test.go`
- `internal/builderengine/implementer-template.md`
- `internal/builderengine/orchestrator-template.md`
- `internal/builderengine/runlevel.go`
- `internal/builderengine/spawn.go`
- `internal/builderengine/spawn_test.go`
- `internal/builderengine/state.go`
- `internal/builderengine/template_test.go`
- `internal/burlercli/cli.go`
- `internal/burlerengine/doc.go`
- `internal/burlerengine/instruction-3-fix-template.md`
- `internal/burlerengine/profile.go`
- `internal/burlerengine/prompt.go`
- `internal/burlerengine/template_test.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configcli/configcli_test.go`
- `internal/configengine/config.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/boardjunction_integration_test.go`
- `internal/fabricengine/checkout_index_refresh_test.go`
- `internal/fabricengine/cleanreason_integration_test.go`
- `internal/fabricengine/commit.go`
- `internal/fabricengine/commit_gating_integration_test.go`
- `internal/fabricengine/commit_integration_test.go`
- `internal/fabricengine/commit_partial_integration_test.go`
- `internal/fabricengine/committed_lyxonly_integration_test.go`
- `internal/fabricengine/committed_test.go`
- `internal/fabricengine/config_driven_junctions_integration_test.go`
- `internal/fabricengine/diff.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/drift.go`
- `internal/fabricengine/export_test.go`
- `internal/fabricengine/fabric.go`
- `internal/fabricengine/fabric_test.go`
- `internal/fabricengine/healthreason_integration_test.go`
- `internal/fabricengine/hostclean.go`
- `internal/fabricengine/index.go`
- `internal/fabricengine/index_integration_test.go`
- `internal/fabricengine/junction_pattern_integration_test.go`
- `internal/fabricengine/open.go`
- `internal/fabricengine/open_integration_test.go`
- `internal/fabricengine/pull.go`
- `internal/fabricengine/pull_integration_test.go`
- `internal/fabricengine/ready.go`
- `internal/fabricengine/ready_integration_test.go`
- `internal/fabricengine/reconcile_stale_registration_test.go`
- `internal/fabricengine/reconcile_stale_removal_test.go`
- `internal/fabricengine/refscanner.go`
- `internal/fabricengine/refscanner_test.go`
- `internal/fabricengine/revert.go`
- `internal/fabricengine/snapshot_integration_test.go`
- `internal/fabricengine/unwire.go`
- `internal/fabricengine/warpforward.go`
- `internal/fabricengine/warpforward_integration_test.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabricengine/weftgit_exclude_test.go`
- `internal/fabricengine/weftgit_pathspec_integration_test.go`
- `internal/gitrepo/commitempty_integration_test.go`
- `internal/gitrepo/doc.go`
- `internal/logger/sink.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/preflight.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/loomengine/report.go`
- `internal/loomengine/status.go`
- `internal/lyxcwd/anchor.go`
- `internal/lyxcwd/enforcement_test.go`
- `internal/lyxcwd/geometry_test.go`
- `internal/lyxcwd/lyxcwd.go`
- `internal/pattern/patternpath_test.go`
- `internal/perchcli/cli.go`
- `internal/perchcli/cli_integration_test.go`
- `internal/perchcli/run.go`
- `internal/perchcli/run_integration_test.go`
- `internal/perchcli/run_test.go`
- `internal/perchengine/doc.go`
- `internal/perchengine/engine.go`
- `internal/perchengine/identity.go`
- `internal/perchengine/run_test.go`
- `internal/reedcli/cli.go`
- `internal/reedengine/lifecycle.go`
- `internal/scoutengine/daemonstate.go`
- `internal/selfreportcli/cli.go`
- `internal/shuttlecli/cli.go`
- `internal/treadleengine/doc.go`
- `internal/treadleengine/engine.go`
- `internal/treadleengine/run.go`
- `internal/webstercli/awaitbatch.go`
- `internal/webstercli/beginbatch.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/recordbatch.go`
- `internal/webstercli/recoverbatch.go`
- `internal/webstercli/run.go`
- `internal/webstercli/status.go`
- `internal/webstercli/sync.go`
- `internal/webstercli/sync_integration_test.go`
- `internal/webstercli/verbs_test.go`
- `internal/websterengine/audit.go`
- `internal/websterengine/audit_test.go`
- `internal/websterengine/awaitbatch.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/beginbatch_test.go`
- `internal/websterengine/config_test.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/fork-prefix.md`
- `internal/websterengine/implementer-body.md`
- `internal/websterengine/integration-template.md`
- `internal/websterengine/integration.go`
- `internal/websterengine/integration_test.go`
- `internal/websterengine/master-template.md`
- `internal/websterengine/pause.go`
- `internal/websterengine/recordbatch.go`
- `internal/websterengine/recordbatch_test.go`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/recoverbatch_test.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/state.go`
- `internal/websterengine/template_test.go`
- `manifest/designs/fabric-unified-view.md`
- `tools/sandbox/SANDBOX-PERCH-SUITE.md`
