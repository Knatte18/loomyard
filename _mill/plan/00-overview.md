# Plan: Add a local-only file category to weft

```yaml
task: "Add a local-only file category to weft"
slug: "weft-local-only-files"
approved: false
started: "20260827-073054"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: merge-drops-weft
    file: 01-merge-drops-weft.md
    depends-on: []
    verify: go build ./cmd/lyx && go test ./internal/fabricengine/... ./internal/lyxcwd/... && go test -tags integration ./internal/fabricengine/...
  - number: 2
    name: weft-guards-drop
    file: 02-weft-guards-drop.md
    depends-on: [1]
    verify: go build ./cmd/lyx && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 3
    name: pull-non-fatal-weft
    file: 03-pull-non-fatal-weft.md
    depends-on: [2]
    verify: go build ./cmd/lyx && go test ./internal/fabricengine/... ./internal/fabriccli/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
  - number: 4
    name: cleanup-raddle-gate
    file: 04-cleanup-raddle-gate.md
    depends-on: []
    verify: go build ./cmd/lyx && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 5
    name: push-and-mergestate-probe
    file: 05-push-and-mergestate-probe.md
    depends-on: [3]
    verify: go build ./cmd/lyx && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 6
    name: shed-commitstatus-seam
    file: 06-shed-commitstatus-seam.md
    depends-on: []
    verify: go build ./cmd/lyx && go test ./internal/shedengine/... ./internal/loomrecipe/... ./internal/lyxcwd/...
  - number: 7
    name: loom-transition-commit
    file: 07-loom-transition-commit.md
    depends-on: [5, 6]
    verify: go build ./cmd/lyx && go test ./internal/loomcli/... ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: no-caller-facing-signature-change-in-fabric

- **Decision:** `Fabric.Merge`, `Fabric.MergeIn`, `Fabric.MergeAbort`, `Fabric.MergeContinue`, `Fabric.Pull` and `Topology.Cleanup` all keep their exported signatures exactly as written.
  The two new exported functions, `PushAnchored` and `MergeStateActive`, take a `*lyxcwd.Location` and return no path, mirroring `CommitAnchoredPaths`.
  Unexported helpers (`resolveMergeSources`, `resetMergeSides`) DO change signature, because they are package-private and their dropped parameters would otherwise be dead.
- **Rationale:** the `no-api-change` Decision in `_mill/discussion.md`, plus the Fabric Vocabulary Invariant, which forbids `Weft`/`Warp` in any caller-facing identifier — hence `PushAnchored`, never `PushAnchoredWeft`.
- **Applies to:** all batches

### Decision: weft-fields-are-filled-as-unmoved-never-dropped

- **Decision:** `mergeState`'s `WeftStart`, `WeftSource`, `WeftOutcome` and `WeftCommitted` stay in the struct and stay filled.
  `WeftStart` and `WeftSource` take the current weft SHA and the resolved weft counterpart SHA (best-effort, possibly empty);
  `WeftOutcome` is written as the literal `mergeOutcomeAlreadyUpToDate` constant at the point the weft `MergeStart` call used to be made;
  `WeftCommitted` is left to whatever the conclude path leaves in it (empty on every path this change produces).
- **Rationale:** `mergeAttemptIncompleteReason` refuses a resume when `WeftOutcome == ""`, `mergeguards.go` reads `WeftCommitted`/`WeftOutcome`/`WeftSource`, and filling rather than dropping keeps the persisted JSON schema byte-compatible in both directions across the binary change.
- **Applies to:** merge-drops-weft, weft-guards-drop

### Decision: conclude-and-conflict-plumbing-is-retained

- **Decision:** `concludeMergeSides`' weft arm, `unifyConflictPaths`' weft parameter, `mergeresolve`'s conflict-resolution session, `MergeStageResolved`, and `fabriccli/merge_verbs.go`'s junction-staging path are all left in place.
  They become unreachable-in-practice rather than wrong, and only their doc comments are corrected.
- **Rationale:** the `merge-plumbing-stays` Decision.
  `concludeMergeSides` additionally still has to work for a `fabric-merge.json` record written by a pre-change binary, which can legitimately carry `WeftOutcome: "staged"`.
- **Applies to:** merge-drops-weft

### Decision: commitstatus-seam-carries-the-transition

- **Decision:** the injected seam is `shedengine.Shed.CommitStatus func(producer, state string) error`, not the no-argument `func() error` spelled out in the discussion's `commit-hook-lives-in-persist` Decision.
  `persist` passes `nextCurrentProducer` and `string(nextState)`.
  Nil stays the absent value and stays a silent no-op, exactly as `shedadapters.BouncerConfig.Commit`.
- **Rationale:** `_mill/discussion.md` is internally inconsistent on this point.
  `commit-hook-lives-in-persist` names the field type `func() error`, while `commit-and-push-every-transition` requires the commit message to be "a fixed prefix plus the transition's own producer and state, built by the `loomcli` closure that already holds both values" — and the `loomcli` closure, built once at `wire()` time, does not hold them: they are per-transition values only `persist` has.
  The message requirement is the more specific and more load-bearing of the two (the discussion calls an unreadable stream of identical messages "the log a resuming operator has to read"), and the only alternative that preserves `func() error` is a second locked read of the status file immediately after `persist` released its lock — a redundant read of the value the caller already holds.
  Two plain `string` parameters rather than `shedengine.State` keep `internal/loomcli`'s closure free of the engine's enum type.
- **Applies to:** shed-commitstatus-seam, loom-transition-commit

### Decision: foreign-merge-state-probe-stays-two-sided

- **Decision:** `foreignMergeStatePresent` (`internal/fabricengine/mergestate.go`) keeps both its weft probes and keeps refusing every mutating merge verb on weft-side foreign merge state.
  It is NOT narrowed to warp alone.
- **Rationale:** the `weft-guards-drop-with-it` Decision enumerates exactly six sites — `pairDirtyReason`, `detachedHeadReason`, `syncedToUpstreamReason`, `resolveMergeSources`' weft arm, `syncSideBeforeMerge`'s weft call, and (via `abort-does-not-reset-weft`) `resetMergeSides`' weft arm — and `foreignMergeStatePresent` is not among them.
  Narrowing it is a behaviour change the discussion neither decided nor rejected, so it stays out of scope.
  This is worth naming rather than leaving implicit: a weft carrying foreign merge state still refuses a warp-only merge under this plan, which a reviewer may reasonably read as a residual instance of the same class of problem `weft-guards-drop-with-it` addresses.
- **Applies to:** weft-guards-drop

### Decision: cleanup-keeps-its-force-parameter

- **Decision:** `Topology.Cleanup(l, apply, force bool)` keeps the `force` parameter and `lyx fabric cleanup` keeps its `--force` flag, even though removing `raddleFoldedBack` leaves `force` answering no gate inside `Cleanup`.
  The parameter is documented as reserved and currently unconsulted;
  the flag's help text and the package flag matrix are corrected to say so.
- **Rationale:** `raddle-gate-removed` decides that the gate is removed;
  it decides nothing about the CLI surface.
  Deleting a shipped flag is an observable CLI change the discussion does not authorize, and the CLI / Cobra Invariant makes the flag set part of the module's contract.
  A no-op `force` parameter compiles and vets cleanly (this repo has no `golangci-lint` configuration, so no `unparam`-class check fires on it).
  The alternative — dropping both the parameter and the flag — is a smaller residual but a larger, unplanned blast radius, and belongs in its own task.
- **Applies to:** cleanup-raddle-gate

### Decision: done-gate-and-lint-left-as-configured

- **Decision:** `pipeline.done_gate` in `mill-config.yaml` is left exactly as it stands — `go test ./... && go test -tags integration ./...` — and no lint command is appended.
- **Rationale:** the configured gate is already a repo-wide test command covering both tiers, which is what the done-gate exists for.
  This repo ships no `.golangci.yml` and no `golangci-lint` invocation anywhere in its build or CI surface, so there is no lint command to default to.
  `README.md:123-124`'s task-wide verify (`go build ./cmd/lyx && go test ./...`) is a strict subset of the configured gate.
- **Applies to:** all batches

### Decision: go-verify-commands-carry-no-pythonpath-prefix

- **Decision:** every `verify:` command in this plan is a native `go` invocation with no `PYTHONPATH= ` prefix, and every batch that edits an `integration`-tagged test file appends a second, ` && `-chained `go test -tags integration ...` invocation over the same package set rather than comma-joining tags onto one command.
- **Rationale:** this is a Go repo, so mill-plan's `PYTHONPATH= ` rule does not apply.
  Batches touching `manifest/designs/*.md` or `CONSTRAINTS.md` additionally run `./internal/lyxcwd/...`, which is where `docslink_test.go` enforces the Markdown Link Integrity invariant.
- **Applies to:** all batches

### Decision: tests-follow-the-repo-tier-rules

- **Decision:** every new test that spawns git or builds a `hubforge` fixture goes in an `//go:build integration`-tagged file whose `TestMain` reaches `gitkit.HermeticGitEnv()` through the package's existing `testmain_test.go`.
  Pure in-process tests (the `shedengine` persist-hook tests) stay untagged.
- **Rationale:** the Test Tier Purity Invariant and the Hermetic Git Test Environment Invariant.
- **Applies to:** all batches

## All Files Touched

- `CLAUDE.md`
- `CONSTRAINTS.md`
- `internal/fabricengine/cleanup.go`
- `internal/fabricengine/cleanup_raddlegate_integration_test.go`
- `internal/fabricengine/destroy.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/export_test.go`
- `internal/fabricengine/merge.go`
- `internal/fabricengine/merge_target_integration_test.go`
- `internal/fabricengine/mergeguards.go`
- `internal/fabricengine/mergelifecycle.go`
- `internal/fabricengine/mergestate.go`
- `internal/fabricengine/mergestate_integration_test.go`
- `internal/fabricengine/mergestateactive.go`
- `internal/fabricengine/mergestateactive_integration_test.go`
- `internal/fabricengine/mergeweftlocal_integration_test.go`
- `internal/fabricengine/pull.go`
- `internal/fabricengine/pull_integration_test.go`
- `internal/fabricengine/pushanchored.go`
- `internal/fabricengine/pushanchored_integration_test.go`
- `internal/fabricengine/reconcile_stale_registration_test.go`
- `internal/fabricengine/weftguards_integration_test.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_commitstatus_test.go`
- `internal/loomrecipe/loomrecipe.go`
- `internal/shedengine/run.go`
- `internal/shedengine/run_commitstatus_test.go`
- `internal/shedengine/shed.go`
- `manifest/designs/loom.md`
- `manifest/designs/shed.md`
