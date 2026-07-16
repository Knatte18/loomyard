# Batch: shell-env-and-shuttle-fork-types

```yaml
task: "Fork-based cluster review in burler"
batch: "shell-env-and-shuttle-fork-types"
number: 1
cards: 2
verify: go test ./internal/shell/ ./internal/shuttleengine/
depends-on: []
```

## Batch Scope

Foundation batch: the two provider-invariant seams every later batch builds on. Card 1
gives `internal/shell` an env-assignment prefix primitive (the Shell Mechanics Seam's
answer to "set one env var on a pane command line"). Card 2 gives `internal/shuttleengine`
the fork-mode vocabulary: the `Spec.ForkSubagents` knob, the `ForkAudit`/`ForkReport`
value types, and the `Result.ForkAudit` carrier field. Everything added here is inert —
no existing behavior changes, nothing populates the new field yet — so the batch is safe
to land alone and batch 2 (claudeengine) consumes the external interface: `shell.Shell.WithEnv`,
`shuttleengine.Spec.ForkSubagents`, `shuttleengine.ForkAudit`.

## Cards

### Card 1: shell.WithEnv env-prefix primitive

- **Context:**
  - `internal/shell/shell_test.go`
- **Edits:**
  - `internal/shell/shell.go`
  - `internal/shell/pwsh.go`
  - `internal/shell/posix.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `WithEnv(key, value, cmd string) string` to the `Shell` interface
  in `shell.go` with a doc comment following the existing interface style. Semantics:
  return `cmd` prefixed so that the environment variable `key=value` is set for the
  command when the returned line is typed into a pane shell. `posixShell.WithEnv`
  returns `key + "=" + p.Quote(value) + " " + cmd` (POSIX command-scoped assignment).
  `pwshShell.WithEnv` returns `"$env:" + key + " = " + p.Quote(value) + "; " + cmd`
  (pwsh has no command-scoped form; the assignment persists for the pane session, which
  is acceptable because shuttle panes are per-run — state this asymmetry in the
  interface doc comment). Document on the interface that `key` must be a plain
  identifier (`[A-Za-z_][A-Za-z0-9_]*`); callers pass compile-time constants and
  `WithEnv` performs no key validation (mirrors how `Invoke` trusts `bin`). `value` is
  always routed through `Quote` — never interpolated raw — for the same
  injection-hardening reason documented on `buildLaunchCmd` in
  `internal/shuttleengine/claudeengine/command.go`. Extend `shell_test.go` with cases
  for both shells: a plain value, a value containing a space, and a value containing a
  quote character, asserting the exact composed line.
- **Commit:** `shell: add WithEnv env-prefix primitive`

### Card 2: shuttleengine fork-mode Spec knob and audit value types

- **Context:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/spec_test.go`
- **Edits:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/run.go`
- **Creates:**
  - `internal/shuttleengine/forkaudit.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** (1) Add `ForkSubagents bool` to `Spec` in `spec.go`, after `Effort`/
  `Version`, with a doc comment stating: when true, the run is authorized to spawn
  in-session fork subagents; the engine realizes it (env flag, hook shape) or must
  hard-error if it cannot; `validate` does not inspect the field (engine vocabulary,
  same posture as `Effort`/`Version`). (2) Create `forkaudit.go` defining the
  provider-invariant audit types with package-style doc comments:
  `type ForkAudit struct { Forks []ForkReport; SpawnCalls int; NamedSpawns int }` —
  `SpawnCalls` counts Agent tool invocations observed in the parent session's
  transcript, `NamedSpawns` counts those carrying a `name` parameter (named forks
  silently lose inherited context in Claude Code ≤2.1.206, so a named spawn is a
  defect signal). `type ForkReport struct { TranscriptPath string; AgentCalls int;
  WriteCalls int; BashCommands []string; ToolCalls map[string]int; ReportReturned
  bool }` — `AgentCalls` counts Agent tool_use attempts inside the fork (attempts
  count even if denied), `WriteCalls` counts Write/Edit/NotebookEdit tool_use,
  `BashCommands` carries every Bash tool_use command string verbatim (policy over
  these strings — e.g. what counts as git-mutating — is the caller's job, not the
  engine's), `ReportReturned` is whether the fork produced a final assistant message.
  (3) Add `ForkAudit *ForkAudit` to `Result` in `run.go` with a doc comment: populated
  only when the Spec set `ForkSubagents` and the run classified done; nil otherwise.
  Nothing populates it in this batch. No test changes required beyond compilation;
  do not add validate rules (the bool needs none).
- **Commit:** `shuttle: add ForkSubagents spec knob and ForkAudit value types`

## Batch Tests

`go test ./internal/shell/ ./internal/shuttleengine/` — covers the extended
`shell_test.go` table (both shells × three value shapes) and recompiles shuttleengine
with the new types. No shuttleengine behavior changes to test yet: the knob and types
are consumed in batches 2 and 4.
