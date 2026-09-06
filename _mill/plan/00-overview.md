# Plan: webster standalone mode: run refuses to start Master; logs write untracked into target repo

```yaml
task: 'webster standalone mode: run refuses to start Master; logs write untracked into target repo'
slug: 'standalonegeom-webster-run-and-log-hygiene'
approved: true
started: '20260906-181015'
parent: 'crucible-loom-glyph-hardening'
root: ""
verify: go build ./...
discussion_sha: fae55f73b4292d000f96d5edeacabdc7e47010a2
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shuttle-detached-runner
    file: 01-shuttle-detached-runner.md
    depends-on: []
    verify: go test ./internal/shuttleengine/...
  - number: 2
    name: logs-dir-and-sink-api
    file: 02-logs-dir-and-sink-api.md
    depends-on: []
    verify: go test ./internal/standalonegeom/... ./internal/logger/...
  - number: 3
    name: standalone-wiring-and-docs
    file: 03-standalone-wiring-and-docs.md
    depends-on: [1, 2]
    verify: go test ./internal/webstercli/... ./internal/burlercli/... ./cmd/lyx/... && go test -tags integration ./internal/webstercli/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: names-fixed-here

- **Decision:** the three new exported identifiers this task introduces are named once, here, and every batch uses exactly these spellings: `shuttleengine.NewDetachedRunner`, `standalonegeom.LogsDir`, and `logger.SetDurableSinkDirWithWorktreeRoot`.
  The one new unexported identifier fixed here is `shuttleengine.validateDetachedToldPaths`.
- **Rationale:** batch 3 calls all three across two packages it does not otherwise change the shape of, and batches 1 and 2 run in parallel;
  a name settled at plan time is what lets batch 3's cards be written against a surface that does not exist yet.
- **Applies to:** all batches

### Decision: hub-mode-is-byte-identical

- **Decision:** no card changes hub-mode behaviour.
  `NewRunner` keeps its exact signature, its exact containment assertion, and its exact error strings;
  `SetDurableSinkDir` keeps its exact name, its exact signature, and its exact reset semantics — the observable call-site contract, not its literal body text, which batch 2 card 4 deliberately refactors into a shared `resetDurableSinkLocked` helper it then delegates to;
  neither `wireHub` gains a sink call.
  Every existing test that pins hub behaviour must keep passing without being edited, and the plan names those tests explicitly so an edit to one is visible as a plan violation rather than as ordinary churn.
- **Rationale:** the task is two standalone-mode defects.
  The existing containment clause is a swap detector four hub call sites depend on, and the shipped `SetDurableSinkDir` has twenty-one call sites.
  Making hub-mode invariance a plan-level rule rather than a per-card aside is what keeps a reviewer able to check it in one pass.
- **Applies to:** all batches

### Decision: sink-override-is-process-global

- **Decision:** every new or edited test that causes `logger.SetDurableSinkDir` or `logger.SetDurableSinkDirWithWorktreeRoot` to be called — directly, or indirectly by driving a `wireStandalone` — registers `t.Cleanup(func() { logger.SetDurableSinkDir("") })`.
- **Rationale:** the override is process-global, and a non-empty override bypasses the `testing.Testing()` sink suppression, so a leaked override arms a live trace sink for every later test in the same binary.
  `cmd/lyx/main_test.go` already establishes this cleanup pattern.
- **Applies to:** all batches

### Decision: go-verify-no-python-prefix

- **Decision:** this is a Go repository, so no `verify:` command carries the `PYTHONPATH= ` prefix;
  batch verifies name Go package patterns directly, and the batch that adds an `integration`-tagged test appends a second, separately-tagged `go test` invocation rather than folding the tag into the untagged one.
- **Rationale:** the `PYTHONPATH= ` shape rule is Python-project-specific.
  Appending a chained `-tags integration` invocation rather than editing the untagged one keeps the untagged run's package set unchanged.
- **Applies to:** all batches

### Decision: docs-land-with-the-change

- **Decision:** every doc comment the fix falsifies moves in the same commit as the code that falsifies it, and `CONSTRAINTS.md`'s new clause lands in batch 3 alongside the two call sites it describes.
  No `manifest/designs/` file and no `manifest/roadmap.md` entry moves.
- **Rationale:** the project's own task-completion rule.
  No module is added, removed, or re-layered, so `docs/overview.md` is unchanged, and the roadmap does not move for a bugfix.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `cmd/lyx/prerunlogging_test.go`
- `internal/burlercli/wiring.go`
- `internal/burlercli/wiring_test.go`
- `internal/logger/sink.go`
- `internal/logger/sink_test.go`
- `internal/shuttleengine/doc.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/run_test.go`
- `internal/shuttleengine/wait.go`
- `internal/shuttleengine/wait_test.go`
- `internal/standalonegeom/doc.go`
- `internal/standalonegeom/logsdir.go`
- `internal/standalonegeom/standalonegeom_test.go`
- `internal/webstercli/cli_integration_test.go`
- `internal/webstercli/wiring.go`
- `internal/webstercli/wiring_test.go`
