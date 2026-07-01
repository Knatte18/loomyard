# Plan: Fix lyx CLI defects + host-commit gap from the sandbox run

```yaml
task: Fix lyx CLI defects + host-commit gap from the sandbox run
slug: lyx-sandbox-fixes
approved: false
started: 20260630-193602
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: raw-git-error-message-cleanup
    file: 01-raw-git-error-cleanup.md
    depends-on: []
    verify: go build ./... && go test -tags integration ./internal/hubgeometry/... ./internal/idecli/... ./internal/initcli/... && go test ./internal/configcli/... ./internal/muxpoccli/...
  - number: 2
    name: warp-path-separator-fix
    file: 02-warp-path-separator-fix.md
    depends-on: []
    verify: go build ./... && go test -tags integration ./internal/warpengine/...
  - number: 3
    name: sandbox-suite-doc-hardening
    file: 03-sandbox-suite-doc-hardening.md
    depends-on: []
    verify: null
```

## Shared Decisions

### Decision: Bare error pass-through at CLI call sites

- **Decision:** Every CLI call site that surfaces a `hubgeometry.Resolve()` failure
  passes the error straight through via `err.Error()` (or wraps it with `%w` only when
  combined with other context per existing convention) — no call site adds its own
  prefix restating "not a git repository" or "failed to resolve layout" on top of it.
- **Rationale:** Once `hubgeometry.go`'s `ErrNotAGitRepo` wrapping is cleaned up (Batch
  1, Card 1), the sentinel's own message is self-describing. A prefix added on top is
  redundant at best (e.g. "resolve layout: not a git repository") and a literal
  duplicate at worst (the `muxpoccli` case before this fix: "not a git repository: not
  a git repository: ...").
- **Applies to:** Batch 1 (`raw-git-error-message-cleanup`).

### Decision: No new error-format surface

- **Decision:** This task does not add a `--json`-vs-plain-text branch to any error
  path, and does not add a `hint`/guidance field to the JSON error envelope.
- **Rationale:** Per `discussion.md`, the JSON-always error envelope (`CONSTRAINTS.md`'s
  "Errors are JSON" rule) stays exactly as it is today — declined as a non-issue for
  this task's actual filer (an agent, not a human at a terminal). Only the literal
  raw-git-stderr leak (a genuine implementation defect, not a format question) is fixed.
- **Applies to:** all batches.

### Decision: Path-separator fix is JSON-boundary-only

- **Decision:** `filepath.ToSlash` is applied only at the point a `HostWorktree`/
  `WeftWorktree` JSON-tagged struct field is assigned. The OS-native `hostPath`/
  `weftPath` local variables used internally for `os.Stat`, git subprocess calls, and
  junction-health checks are never converted and must keep their current
  `filepath.FromSlash` + `filepath.Clean` construction unchanged.
- **Rationale:** `warp list` already emits forward-slash paths because it never calls
  `FromSlash` on git's (always-forward-slash) porcelain output. `status.go`, `prune.go`,
  and `reconcile.go` independently normalize to OS-native for internal use (correct and
  required on Windows) but then leak that OS-native form into JSON output (the #37 bug).
  Converting internally instead of at the JSON boundary would risk breaking
  `os.Stat`/git-subprocess calls on Windows.
- **Applies to:** Batch 2 (`warp-path-separator-fix`).

## All Files Touched

- `internal/configcli/configcli.go`
- `internal/configcli/reconcile_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_test.go`
- `internal/idecli/cli.go`
- `internal/idecli/cli_test.go`
- `internal/initcli/initcli.go`
- `internal/initcli/initcli_test.go`
- `internal/muxpoccli/cli.go`
- `internal/muxpoccli/cli_test.go`
- `internal/warpengine/prune.go`
- `internal/warpengine/prune_test.go`
- `internal/warpengine/reconcile.go`
- `internal/warpengine/reconcile_test.go`
- `internal/warpengine/status.go`
- `internal/warpengine/status_test.go`
- `tools/sandbox/SANDBOX-SUITE.md`
