# Plan: Sandbox test-suite launcher and task harvester

```yaml
task: "Sandbox test-suite launcher and task harvester"
slug: "sandbox-suite"
approved: false
started: "20260628-195210"
parent: "main"
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: sandbox-suite-launcher
    file: 01-sandbox-suite-launcher.md
    depends-on: []
    verify: go test ./tools/sandbox/...
```

## Shared Decisions

### Decision: subcommand dispatch on `tools/sandbox`

- **Decision:** `tools/sandbox` grows a subcommand layer. The first positional arg
  selects the action: `build` (default when absent) runs the existing clone/reset
  behaviour; `suite` runs the new agent launcher. The shared `-parent` flag stays on
  the top-level flagset (the `sandbox.cmd` wrapper supplies `-parent C:\Code`). `-reset`
  stays recognised at the top level so the existing `sandbox.cmd` and `sandbox.cmd -reset`
  invocations (no subcommand → `build`) keep working unchanged.
- **Rationale:** One tool owns the one Hub; a sibling `tools/sandbox-suite/` next to
  `tools/sandbox/` would be vague. Top-level `-reset` preserves back-compat.
- **Applies to:** all batches

### Decision: black-box isolation, never headless

- **Decision:** The launched agent runs interactively (`claude --dangerously-skip-permissions
  "<instruction>"`) with **cwd = the Hub host repo** and inherited terminal stdio/env.
  Never `claude -p`. The launcher hands the agent no path into the lyx source tree — only
  `lyx` (on PATH) and the copied scheme. No psmux and no `CLAUDECODE`/`CLAUDE_CODE_*` env
  stripping (deferred until the `mux` module exists).
- **Rationale:** CLAUDE.md economic stance (headless bills as API); the whole point is to
  exercise `lyx.exe` as a black box a real user would face.
- **Applies to:** all batches

### Decision: `tools/` stays self-contained, no path-invariant primitives

- **Decision:** `tools/sandbox` derives every filesystem path from the `-parent` flag via
  `path/filepath`. It MUST NOT call `os.Getwd` or shell `git rev-parse` (CONSTRAINTS.md
  Path Invariant, enforced by `internal/paths/enforcement_test.go` scanning the whole
  tree). Writing `.git/info/exclude` is plain file IO, not a git primitive.
- **Rationale:** `tools/` is outside `internal/paths`; the enforcement scan still covers it.
- **Applies to:** all batches

### Decision: Go style + error posture

- **Decision:** Match existing `tools/sandbox/main.go`: package-level `var` function
  seams for testability (mirroring `cloneRun` / `removeAll`), `fmt.Errorf("...: %w", err)`
  wrapping, errors to `os.Stderr` prefixed `sandbox:`, `os.Exit(1)` on failure. Godoc per
  `golang-comments`. Tests follow `main_test.go`: `t.TempDir()`, seam stubs, no network,
  no real `claude`/`lyx` launch.
- **Rationale:** Consistency with the file being edited.
- **Applies to:** all batches

## All Files Touched

- `docs/sandbox-hub.md`
- `tools/sandbox/main.go`
- `tools/sandbox/main_test.go`
- `tools/sandbox/suite.go`
- `tools/sandbox/suite_test.go`
- `tools/sandbox/test-scheme.md`
