# Plan: Fork-based cluster review in burler

```yaml
task: "Fork-based cluster review in burler"
slug: "burler-fork-cluster"
approved: false
started: "20260716-194307"
parent: "main"
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: shell-env-and-shuttle-fork-types
    file: 01-shell-env-shuttle-fork-types.md
    depends-on: []
    verify: go test ./internal/shell/ ./internal/shuttleengine/
  - number: 2
    name: claudeengine-fork-mode
    file: 02-claudeengine-fork-mode.md
    depends-on: [1]
    verify: go test ./internal/shuttleengine/...
  - number: 3
    name: burler-config-module
    file: 03-burler-config-module.md
    depends-on: []
    verify: go test ./internal/burlerengine/ ./internal/configreg/
  - number: 4
    name: burler-cluster-round
    file: 04-burler-cluster-round.md
    depends-on: [2, 3]
    verify: go test ./internal/burlerengine/ ./internal/burlercli/ ./internal/perchengine/ ./internal/perchcli/
  - number: 5
    name: docs-and-smoke
    file: 05-docs-and-smoke.md
    depends-on: [4]
    verify: go test ./...
```

## Shared Decisions

### Decision: fan vocabulary and pinned identifiers

- **Decision:** The cluster interface is a single profile key `cluster-fan` (YAML) /
  `ClusterFan string` (Go) — naming a fan activates clustering, N = the fan's entry
  count, absent/empty = no clustering. Pinned identifiers used across batches:
  `Spec.ForkSubagents bool` (shuttleengine), `ForkAudit` / `ForkReport`
  (shuttleengine value types), `Engine.AuditForks` (provider seam method),
  `ErrClusterForksMissing` (burlerengine sentinel), `Config` / `Lens` / `LoadConfig` /
  `ResolveFan` (burlerengine config), `WithEnv` (shell seam method). The config file
  vocabulary is `lenses:` (name → emphasis prose) and `fans:` (name → list of lens
  names, repeats allowed); seeded fans are named `standard` and `full` — deliberately
  NOT `default`, because no fan is ever active unless a profile names it.
- **Rationale:** One mechanism, no count/list reconciliation; "profile" is already
  double-booked (`--profile` round YAML, `burlerengine.Profile`).
- **Applies to:** all batches

### Decision: fail-loud posture, no silent defaults

- **Decision:** Every new validation follows the module's existing posture: unknown fan
  name, unknown lens name inside a fan, empty fan, fan length > 16 (`maxClusterN = 16`),
  fork-count shortfall, Agent-call-in-fork, file-mutation-in-fork, and named fork spawns
  are all hard errors with burler-prefixed messages. No degrade-to-solo, no silent
  truncation, no advisory-only treatment of mechanism failures. Sloppiness that no
  mechanism can prevent (a fork returning no report, unusual tool-call volume) is a
  warning surfaced on the Result, never a round failure.
- **Rationale:** Operator decision recorded in `_mill/discussion.md` (Q8/Q12/Q13): a
  cluster round that cannot deliver exactly what the profile demanded is an
  infrastructure defect that gets fixed, not accepted.
- **Applies to:** all batches

### Decision: provider-seam and shell-seam placement

- **Decision:** Claude-specific knowledge stays in `internal/shuttleengine/claudeengine`:
  the `~/.claude/projects/<encoded-cwd>/<session-id>/subagents/*.jsonl` transcript
  layout, the PreToolUse hook JSON, and the `CLAUDE_CODE_FORK_SUBAGENT=1` env flag.
  `internal/shuttleengine` carries only the provider-invariant `Spec.ForkSubagents`
  knob, the `ForkAudit`/`ForkReport` value types, and the `AuditForks` seam method.
  All shell syntax (the env-assignment prefix) goes through `internal/shell` — never
  raw pwsh/posix tokens in claudeengine.
- **Rationale:** Shuttle Provider-Seam Invariant and Shell Mechanics Seam
  (CONSTRAINTS.md).
- **Applies to:** shell-env-and-shuttle-fork-types, claudeengine-fork-mode

### Decision: test-tier discipline

- **Decision:** All new unit tests are untagged Tier-1 files: no process spawns, no
  `lyxtest.Copy*`, fixtures as plain `testdata/` files read with `os.ReadFile`. The new
  cluster smoke test file carries the same build tag and TestMain/hermetic-env pattern
  as the existing `internal/burlerengine/smoke_round_test.go`. Tests are table-driven
  where the existing package tests are.
- **Rationale:** Test Tier Purity Invariant and Hermetic Git Test Environment Invariant
  (CONSTRAINTS.md).
- **Applies to:** all batches

### Decision: commit-message shape

- **Decision:** Card commits use the repo's `<module>: <summary>` shape (e.g.
  `shell: add WithEnv env-prefix primitive`), matching recent history.
- **Rationale:** Consistency with `git log` conventions; no conventional-commit tooling
  in this repo.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `docs/overview.md`
- `docs/roadmap.md`
- `internal/burlercli/cli.go`
- `internal/burlercli/cli_test.go`
- `internal/burlercli/run.go`
- `internal/burlerengine/cluster.go`
- `internal/burlerengine/cluster_test.go`
- `internal/burlerengine/config.go`
- `internal/burlerengine/config_test.go`
- `internal/burlerengine/doc.go`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/engine_test.go`
- `internal/burlerengine/profile.go`
- `internal/burlerengine/profile_test.go`
- `internal/burlerengine/prompt.go`
- `internal/burlerengine/prompt_test.go`
- `internal/burlerengine/review-prompt-template.md`
- `internal/burlerengine/smoke_cluster_test.go`
- `internal/burlerengine/smoke_round_test.go`
- `internal/burlerengine/template.yaml`
- `internal/burlerengine/template_test.go`
- `internal/burlerengine/verdict.go`
- `internal/burlerengine/verdict_test.go`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/perchcli/cli.go`
- `internal/perchcli/run.go`
- `internal/perchcli/run_test.go`
- `internal/perchengine/profile.go`
- `internal/perchengine/roundfiles.go`
- `internal/perchengine/roundfiles_test.go`
- `internal/shell/posix.go`
- `internal/shell/pwsh.go`
- `internal/shell/shell.go`
- `internal/shell/shell_test.go`
- `internal/shuttleengine/claudeengine/audit.go`
- `internal/shuttleengine/claudeengine/audit_test.go`
- `internal/shuttleengine/claudeengine/claudeengine.go`
- `internal/shuttleengine/claudeengine/command.go`
- `internal/shuttleengine/claudeengine/command_test.go`
- `internal/shuttleengine/claudeengine/prepare_test.go`
- `internal/shuttleengine/claudeengine/settings.go`
- `internal/shuttleengine/claudeengine/settings_test.go`
- `internal/shuttleengine/claudeengine/testdata/fork-clean.jsonl`
- `internal/shuttleengine/claudeengine/testdata/fork-mutating.jsonl`
- `internal/shuttleengine/claudeengine/testdata/fork-nested-agent.jsonl`
- `internal/shuttleengine/claudeengine/testdata/parent-spawns.jsonl`
- `internal/shuttleengine/engine.go`
- `internal/shuttleengine/fakes_test.go`
- `internal/shuttleengine/forkaudit.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/spec.go`
- `internal/shuttleengine/wait.go`
- `internal/shuttleengine/wait_test.go`
- `tools/sandbox/SANDBOX-BURLER-SUITE.md`
