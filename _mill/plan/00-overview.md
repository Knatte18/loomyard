# Plan: Diagnostic tracing (trace) on the logger module

```yaml
task: Diagnostic tracing (trace) on the logger module
slug: trace-logging
approved: false
started: 20260731-185537
parent: main
root: ""
verify: go vet ./...
```

## Batch Index

```yaml
batches:
  - number: 1
    name: hubgeometry-worktree-logs-dir
    file: 01-hubgeometry-worktree-logs-dir.md
    depends-on: []
    verify: go test ./internal/hubgeometry/...
  - number: 2
    name: logger-trace-identity
    file: 02-logger-trace-identity.md
    depends-on: []
    verify: go test ./internal/logger/...
  - number: 3
    name: logger-retention-sweep
    file: 03-logger-retention-sweep.md
    depends-on: []
    verify: go test ./internal/logger/...
  - number: 4
    name: logger-durable-sink
    file: 04-logger-durable-sink.md
    depends-on: [1, 2, 3]
    verify: go test ./internal/logger/...
  - number: 5
    name: logger-dual-handler-fanout
    file: 05-logger-dual-handler-fanout.md
    depends-on: [2, 4]
    verify: go test -race ./internal/logger/...
  - number: 6
    name: logger-spans
    file: 06-logger-spans.md
    depends-on: [2, 5]
    verify: go test ./internal/logger/...
  - number: 7
    name: cmd-lyx-root-wiring
    file: 07-cmd-lyx-root-wiring.md
    depends-on: [2, 4, 5, 6]
    verify: go test -tags integration ./cmd/lyx/... ./internal/logger/...
  - number: 8
    name: reedengine-trace-env-filter
    file: 08-reedengine-trace-env-filter.md
    depends-on: [2]
    verify: go test ./internal/reedengine/...
  - number: 9
    name: reedengine-adoption
    file: 09-reedengine-adoption.md
    depends-on: [5, 8]
    verify: go test ./internal/reedengine/...
  - number: 10
    name: treadleengine-adoption
    file: 10-treadleengine-adoption.md
    depends-on: [5]
    verify: go test ./internal/treadleengine/...
  - number: 11
    name: burler-fabric-shuttle-adoption
    file: 11-burler-fabric-shuttle-adoption.md
    depends-on: [5]
    verify: go test ./internal/burlerengine/... ./internal/fabricengine/... ./internal/shuttleengine/...
  - number: 12
    name: perchengine-adoption
    file: 12-perchengine-adoption.md
    depends-on: [5]
    verify: go test ./internal/perchengine/...
  - number: 13
    name: scoutengine-logger-conversion
    file: 13-scoutengine-logger-conversion.md
    depends-on: [5, 10]
    verify: go test ./internal/scoutengine/...
  - number: 14
    name: docs-and-constraints-wrapup
    file: 14-docs-and-constraints-wrapup.md
    depends-on: [7, 9, 10, 11, 12, 13]
    verify: null
```

## Shared Decisions

### Decision: Go project, native verify commands

- **Decision:** every `verify:` in this plan is a native `go test`/`go vet` invocation scoped to the package(s) the batch touches — never the `PYTHONPATH= ` prefix form (that convention is for the mill Python tooling repo, not this Go module).
- **Rationale:** `plugins/mill/skills/mill-plan/SKILL.md`'s verify-command-shape rule states non-Python projects use the native test runner directly.
- **Applies to:** all batches.

### Decision: `internal/logger` stays one package, no rename

- **Decision:** all trace/span/sink/retention machinery is added inside `internal/logger`, split across new files (`trace.go`, `span.go`, `sink.go`, `retention.go`) plus edits to the existing `logger.go`. No new package (`internal/trace`, `internal/traceengine`) is created, and `internal/logger` is not renamed.
- **Rationale:** see discussion.md's `one-package-not-two` and `keep-logger-name` decisions — every emitted line needs the trace/span state, so a package split would be a hot per-call dependency with no reuse gain.
- **Applies to:** batches 2-7.

### Decision: Hub Geometry Invariant compliance

- **Decision:** the only path-construction call for the new durable-sink directory is `hubgeometry.Layout.WorktreeLogsDir()` (batch 1). `internal/logger` never joins `.lyx`/`logs` from literals itself, in production or test code.
- **Rationale:** CONSTRAINTS.md's Hub Geometry Invariant — `internal/hubgeometry` owns all such path construction, machine-enforced by `TestEnforcement_GeometryLiterals`.
- **Applies to:** batches 1, 4.

### Decision: Test Tier Purity — sink/retention/span/fanout unit tests never spawn git

- **Decision:** every untagged unit test added for the sink, retention, spans, and fan-out points the durable sink at a `t.TempDir()` via the new test seam (batch 4) rather than calling `hubgeometry.Resolve`. Only the one `//go:build integration` test (batch 7) spawns a real binary and touches real geometry.
- **Rationale:** CONSTRAINTS.md's Test Tier Purity Invariant bans `gitexec.RunGit`/`exec.Command`/`lyxtest.Copy*` as raw substrings in untagged test files.
- **Applies to:** batches 3, 4, 5, 6, 7 (the untagged test cards specifically).

### Decision: `level-policy` governs every adoption-pass call

- **Decision:** every new `logger` call added by the adoption-pass batches (8, 9, 10, 11, 12, 13) follows discussion.md's `level-policy`: Warn for a notable-but-recoverable failure (retry, unconfirmed teardown, swallowed error on a fallback path); Info for real-OS-process spawn/teardown lifecycle; Debug for everything else worth a line; nothing at Warn inside a loop body that can iterate more than ~10 times without a state change.
- **Rationale:** stated once here so no adoption-pass batch needs to restate it; discussion.md's `level-policy` and `adoption-scope` decisions are the source.
- **Applies to:** batches 8, 9, 10, 11, 12, 13.

### Decision: Adoption is an audited stop-rule, not a fixed site list

- **Decision:** in batches 9, 10, 11, 12, "adopt logger calls" means applying discussion.md's `adoption-scope` done-criterion to every error-return path in the listed files: a site gets a call when (a) its error is discarded/swallowed-by-fallback/retried, OR propagates without already naming what failed, AND (b) it touches a real OS process, a lock, a file, or a subprocess result. A site that already wraps and propagates identifying context gets nothing. Concrete candidate line numbers are given per card as a research-verified floor, not an exhaustive ceiling — the implementer re-reads each listed file in full against the stop-rule.
- **Rationale:** discussion.md's `adoption-scope` explicitly frames this as an audited stop-rule ("Audit the error paths is not a boundary a plan writer can size"), not a pre-enumerable list.
- **Applies to:** batches 9, 10, 11, 12.

### Decision: `internal/proc.IsAlive`/`Detach`/`DetachBreakaway` untouched

- **Decision:** no batch modifies `internal/proc`. `retention.go` (batch 3) and `internal/reedengine`'s env filter (batch 8) call its existing exported surface only.
- **Rationale:** `internal/proc` is stdlib-only (`os/exec`, `syscall`); widening it is out of scope and unnecessary — confirmed by research that it has no `cmd.Env` references at all.
- **Applies to:** batches 3, 8.

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/main.go`
- `docs/shared-libs/README.md`
- `internal/burlerengine/engine.go`
- `internal/fabricengine/spawn.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/worktreelogs_test.go`
- `internal/logger/logger.go`
- `internal/logger/logger_test.go`
- `internal/logger/retention.go`
- `internal/logger/retention_test.go`
- `internal/logger/sink.go`
- `internal/logger/sink_test.go`
- `internal/logger/span.go`
- `internal/logger/span_test.go`
- `internal/logger/trace.go`
- `internal/logger/trace_test.go`
- `cmd/lyx/main_test.go`
- `cmd/lyx/main_integration_test.go`
- `internal/perchengine/config.go`
- `internal/perchengine/engine.go`
- `internal/perchengine/identity.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/lifecycle_test.go`
- `internal/scoutengine/ensureserver.go`
- `internal/scoutengine/leaf_enforcement_test.go`
- `internal/scoutengine/lspclient.go`
- `internal/shuttleengine/run.go`
- `internal/treadleengine/run.go`
- `internal/treadleengine/seam_enforcement_test.go`
</content>
