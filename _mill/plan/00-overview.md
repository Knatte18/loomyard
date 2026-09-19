# Plan: fabric: no remote/GitHub branch deletion

```yaml
task: 'fabric: no remote/GitHub branch deletion'
slug: 'fabric-remote-branch-delete'
approved: true
started: '20260918-194549'
parent: 'main'
root: ""
verify: null
discussion_sha: 99971b735487208b4a4b1a7f0c168ebd7d18f526
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: gitrepo-remote-delete-primitive
    file: 01-gitrepo-remote-delete-primitive.md
    depends-on: []
    verify: go test ./internal/gitrepo/... && go test -tags integration ./internal/gitrepo/...
  - number: 2
    name: fabricengine-remote-executor
    file: 02-fabricengine-remote-executor.md
    depends-on: [1]
    verify: go test ./internal/fabricengine/... ./cmd/lyx/... && go test -tags integration ./internal/fabricengine/...
  - number: 3
    name: engine-wiring
    file: 03-engine-wiring.md
    depends-on: [2]
    verify: go vet ./... && go vet -tags integration ./... && go vet -tags smoke ./... && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 4
    name: cli-surface-and-docs
    file: 04-cli-surface-and-docs.md
    depends-on: [3]
    verify: go test ./internal/fabriccli/... ./cmd/lyx/... && go test -tags integration ./internal/fabriccli/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: discussion-is-authoritative

- **Decision:** `_mill/discussion.md`'s Decisions section is the settled design for this task.
  Every card below implements one of those decisions;
  no card re-opens one.
  Where a card's Requirements and the discussion disagree, the discussion wins and the card is a plan defect to be reported, never silently re-decided at implementation time.
- **Rationale:** the discussion went through seven review rounds and records the rejected alternatives alongside each decision, so a builder who re-derives a choice from the code alone will re-open a settled question.
- **Applies to:** all batches

### Decision: every-batch-compiles-and-passes

- **Decision:** each batch's own `verify:` must pass at that batch's end, so no batch may leave the module uncompilable for the next one.
  This is why batch 3 includes a compile-only update to `internal/fabriccli/fabric.go`'s two engine call sites — passing the new `remote` argument as the literal `false` — which batch 4 then rewrites into the real flag-threaded form.
  The interim churn is deliberate and is not a plan defect.
- **Rationale:** the two engine verb signatures widen in batch 3, which breaks every caller in the module including `internal/fabriccli` and `internal/loomcli`'s smoke test.
  Splitting the CLI's flag surface into its own batch keeps batch 3 reviewable as an engine change, but only works if batch 3 restores compilation itself.
- **Applies to:** batch 3, batch 4

### Decision: verify-scope-and-build-tags

- **Decision:** every batch's `verify:` runs both the untagged and the `integration`-tagged form of the package it touches, because this repo splits git-spawning tests into `//go:build integration` files under the Test Tier Purity Invariant.
  Batch 3 additionally runs `go vet` across the whole module in all three tag configurations (untagged, `integration`, `smoke`), because the two widened engine signatures have callers in test files under every one of those tags, and `go build` alone never type-checks a test file.
- **Rationale:** a scoped `go test` on one package cannot see a broken call site in another package's test file;
  `go vet ./...` type-checks the whole module including tests, is a compile-class check rather than a test suite, and runs in under two seconds here.
- **Applies to:** all batches

### Decision: constraints-md-is-not-edited

- **Decision:** `CONSTRAINTS.md` is not edited by this task.
- **Rationale:** the shipped Fabric Destruction Chokepoint Invariant names no primitive count and no individual executor — its bullets are "every executor checks, in order", "`--force` answers dirtiness only", "a gate refusal is never silently discarded", and "the `rec *Mutations` recorder is threaded into `destroy.go` only".
  Every one of those stays true verbatim with a sixth primitive.
  The counts that do move live in `destroy.go`'s own file header, which batch 2 updates.
- **Applies to:** all batches

### Decision: docs-overview-is-not-edited

- **Decision:** `docs/overview.md` is not edited by this task.
- **Rationale:** the project rule in `CLAUDE.md` requires a `docs/overview.md` update only when the module table or execution stack changes.
  That file's fabric line enumerates the verb surface (`status|commit|push|pull|sync|diff|merge-in|merge|merge-stage`) and does not name `cleanup` or `remove` at all, so adding a flag to two verbs it never lists changes nothing there.
  The observable-CLI-behaviour docs obligation is discharged by the `Use`/`Long`/usage-string rewrites in batch 4 and the two file headers in batches 2 and 3.
- **Applies to:** all batches

### Decision: no-module-design-doc-exists

- **Decision:** no file under `manifest/designs/` is created or edited.
- **Rationale:** there is no `manifest/designs/fabric.md`;
  `manifest/designs/fabric-windows-verification.md` is about Windows verification and is not this module's doc.
  The durable design record for the destructive gate is `destroy.go`'s own file header.
- **Applies to:** all batches

### Decision: fabric-vocabulary-stops-at-gitrepo

- **Decision:** the new `internal/gitrepo` method, its doc comment, its error strings, and its test file use only vocabulary-neutral words — `remote`, `branch`, `repo`.
  The words `weft` and `warp` appear nowhere in batch 1.
- **Rationale:** the Fabric Vocabulary Invariant's owner set includes `fabricengine`, `fabriccli`, `weftname`, `gitkit`, `hubforge`, and `boardengine`, but not `internal/gitrepo`.
  `TestEnforcement_FabricVocabulary` fails the build on a violation.
- **Applies to:** batch 1

### Decision: local-first-then-remote-everywhere

- **Decision:** at both real call sites the remote deletion is attempted only after the local `git branch -D` for the same branch returned success.
  No call site ever attempts the remote deletion independently of, or ahead of, the local one.
- **Rationale:** the gate's ownership and dirtiness answers are read from local state, so a branch the gate refuses to delete locally must not lose its copy on a shared remote.
  The reverse order is unrecoverable.
- **Applies to:** batch 3

### Decision: remote-failure-non-fatal-in-engine-fatal-in-cli

- **Decision:** a remote deletion failure never makes an engine verb return a non-nil `error` and never feeds `removeWeftWorktree`'s `firstErr` accumulator;
  it is recorded on a result field.
  At the `internal/fabriccli` layer the same condition IS a failure and exits non-zero through `errWithRecordFields`.
- **Rationale:** an offline laptop must not abort a cleanup sweep or strand a teardown mid-way, but a scripted caller reading only `ok` and the exit code must not be told an unqualified success for work that did not happen.
  The two layers answer two different questions.
- **Applies to:** batch 3, batch 4

### Decision: card-commits-are-conventional-and-standalone

- **Decision:** every card carries its own `Commit:` message in this repo's existing conventional-commit style with a `fabric`, `gitrepo`, or `docs` scope, and each card's diff stands alone as one commit.
  No card's Requirements instructs the implementer to fold its diff into another card's commit.
- **Rationale:** mill-go commits once per card and the harness never amends a pushed commit, so a cross-card "same commit" instruction cannot be honoured.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `cmd/lyx/destructiveguard_test.go`
- `cmd/lyx/gitrepoboundary_test.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/remoteenvelope_integration_test.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/cleanup.go`
- `internal/fabricengine/cleanup_primary_integration_test.go`
- `internal/fabricengine/cleanupremote_integration_test.go`
- `internal/fabricengine/destroy.go`
- `internal/fabricengine/destroy_containment_toctou_integration_test.go`
- `internal/fabricengine/destroy_test.go`
- `internal/fabricengine/destroyremote_integration_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/export_test.go`
- `internal/fabricengine/livestate_mutationoracle_test.go`
- `internal/fabricengine/livestate_refusal_selftest_test.go`
- `internal/fabricengine/livestate_verbs_test.go`
- `internal/fabricengine/mergecrucible_integration_test.go`
- `internal/fabricengine/mergesiblings_integration_test.go`
- `internal/fabricengine/mutation.go`
- `internal/fabricengine/mutation_record_integration_test.go`
- `internal/fabricengine/origin_integration_test.go`
- `internal/fabricengine/reconcile_stale_registration_test.go`
- `internal/fabricengine/reconcile_stale_removal_test.go`
- `internal/fabricengine/remove.go`
- `internal/fabricengine/remove_guard_integration_test.go`
- `internal/fabricengine/remove_junctions_integration_test.go`
- `internal/fabricengine/remove_refusal_remedy_integration_test.go`
- `internal/fabricengine/remove_reserved_integration_test.go`
- `internal/fabricengine/removeremote_integration_test.go`
- `internal/fabricengine/weftwiring.go`
- `internal/gitrepo/deleteremotebranch_integration_test.go`
- `internal/gitrepo/push.go`
- `internal/loomcli/smoke_test.go`
- `manifest/roadmap.md`
