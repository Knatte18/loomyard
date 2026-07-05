# Plan: Build internal/shuttle: one LLM agent via a swappable engine

```yaml
task: 'Build internal/shuttle: one LLM agent via a swappable engine'
slug: internal-shuttle
approved: false
started: '20260705-144755'
parent: main
root: ""
verify: go test ./...
```

## Batch Index

```yaml
batches:
  - number: 1
    name: mux-extensions
    file: 01-mux-extensions.md
    depends-on: []
    verify: go test ./internal/muxengine/...
  - number: 2
    name: shuttle-foundation
    file: 02-shuttle-foundation.md
    depends-on: []
    verify: go test ./internal/shuttleengine/... ./internal/configreg/...
  - number: 3
    name: claude-engine
    file: 03-claude-engine.md
    depends-on: [2]
    verify: go test ./internal/shuttleengine/...
  - number: 4
    name: run-loop
    file: 04-run-loop.md
    depends-on: [1, 3]
    verify: go test ./internal/shuttleengine/...
  - number: 5
    name: cli-and-registration
    file: 05-cli-and-registration.md
    depends-on: [4]
    verify: go test ./cmd/lyx/... ./internal/shuttlecli/...
  - number: 6
    name: smoke-tests
    file: 06-smoke-tests.md
    depends-on: [5]
    verify: go vet -tags smoke ./internal/shuttlecli/... && go test ./internal/shuttleengine/... ./internal/shuttlecli/...
  - number: 7
    name: docs-lifecycle
    file: 07-docs-lifecycle.md
    depends-on: [6]
    verify: null
```

## Shared Decisions

### Decision: source of truth is discussion.md and the mux CODE

- **Decision:** `_mill/discussion.md` is the design authority for this task. For mux
  behaviour, the `internal/muxengine` source is authoritative — NOT `docs/modules/mux.md`
  (stale; deleted in batch 7). `docs/modules/shuttle.md` is design *intent* only and is also
  deleted in batch 7.
- **Rationale:** operator directive; module docs of built modules rot immediately.
- **Applies to:** all batches

### Decision: Interactive bool encodes the discussion's "Autonomous default true"

- **Decision:** the spec field is `Interactive bool` (Go zero value `false` = autonomous —
  the default). `Interactive == !Autonomous` from the discussion. Autonomous behaviour
  (`Interactive: false`): `--dangerously-skip-permissions` is added and the
  `AskUserQuestion` PreToolUse deny is included. Interactive behaviour (`Interactive:
  true`): neither. The `Agent` deny is included in BOTH modes (each deny still individually
  toggleable via the `claude_deny_agent_tool` / `claude_deny_ask_user_question` config keys,
  which gate whether the corresponding deny is ever emitted).
- **Rationale:** a Go bool cannot default to true; inverting the field name preserves the
  discussion's semantics with a safe zero value.
- **Applies to:** all batches

### Decision: provider-seam import rule (enforced)

- **Decision:** `internal/shuttleengine` NEVER imports
  `internal/shuttleengine/claudeengine`. The interface (`shuttleengine.Engine`) and all its
  value types live in `shuttleengine`; `claudeengine` imports `shuttleengine` and implements
  the interface; `internal/shuttlecli` imports both and injects the Claude engine into the
  runner. Enforced by `internal/shuttleengine/seam_enforcement_test.go` (import-scan test in
  the style of `internal/lyxtest/leaf_enforcement_test.go`).
- **Rationale:** the seam is the design point — provider specifics must be provably isolated
  so a second engine never requires touching the invariant machinery.
- **Applies to:** claude-engine, run-loop, cli-and-registration

### Decision: geometry and config discipline

- **Decision:** all cwd/geometry via `internal/hubgeometry` (`Getwd`/`Resolve`); the run-dir
  default is `filepath.Join(layout.DotLyxDir(), "shuttle")` — never a literal `.lyx`;
  config loads via `configengine.Load` through a `LoadConfig(baseDir, module)` that mirrors
  `muxengine.LoadConfig` exactly (same "not initialized" wrapping). Tests seed config with
  `lyxtest.SeedConfig(tb, dir, map[string]string{"shuttle": shuttleengine.ConfigTemplate(),
  "mux": muxengine.ConfigTemplate()})` at the test site (lyxtest Leaf Invariant).
- **Rationale:** Hub Geometry Invariant + lyxtest Leaf Invariant are machine-enforced on
  every `go test`.
- **Applies to:** all batches

### Decision: test posture

- **Decision:** hermetic unit tests by default — pure decision helpers extracted and tested
  without psmux/claude, following muxengine's `*Locked`-helper pattern. Anything that spawns
  psmux or claude lives under `-tags smoke` (opt-in, subscription-consuming), following
  `internal/muxcli/smoke_*.go` patterns including `deferHubRelease`. Style: mirror
  muxengine's naming, godoc density, and per-file header comments; every new package gets a
  `doc.go` package header carrying the durable design notes (documentation lifecycle).
- **Rationale:** matches the repo's established layering; smoke tests are the live proof for
  hook/guardrail behaviour that hermetic tests cannot give.
- **Applies to:** all batches

### Decision: CLI envelope posture

- **Decision:** `lyx shuttle run` blocks until an outcome and then prints ONE
  `output.Ok` JSON envelope with fields `{outcome, sessionId, guid, lastAssistantMessage,
  runDir}` and exits 0 for every classified outcome (`done`/`asking`/`died`/`timeout`) —
  outcome is data, not an error. Mechanism failures (spec validation, config, launch
  errors) go through `output.Err` with exit 1. No envelope exception is needed (run is not
  a terminal handover). Parent command uses `clihelp.GroupRunE`; every command has `Short`;
  `run` carries a `Long` with examples.
- **Rationale:** CLI/Cobra Invariant; a classified outcome is the machine result callers
  branch on.
- **Applies to:** cli-and-registration, smoke-tests

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `docs/modules/README.md`
- `docs/modules/loom.md`
- `docs/modules/review.md`
- `docs/overview.md`
- `docs/research/mux-exploration.md`
- `docs/research/mux-hooks-exploration.md`
- `docs/research/mux-proposal.md`
- `docs/reviews/README.md`
- `docs/reviews/mux-review-prompt.md`
- `docs/roadmap.md`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/configsync/configsync_test.go`
- `internal/muxengine/config.go`
- `internal/muxengine/config_test.go`
- `internal/muxengine/io.go`
- `internal/muxengine/io_test.go`
- `internal/muxengine/strand.go`
- `internal/muxengine/strand_test.go`
- `internal/muxengine/template.yaml`
- `internal/shuttlecli/cli.go`
- `internal/shuttlecli/cli_test.go`
- `internal/shuttlecli/interrupt.go`
- `internal/shuttlecli/run.go`
- `internal/shuttlecli/send.go`
- `internal/shuttlecli/smoke_guardrail_test.go`
- `internal/shuttlecli/smoke_interrupt_test.go`
- `internal/shuttlecli/smoke_run_test.go`
- `internal/shuttleengine/claudeengine/claudeengine.go`
- `internal/shuttleengine/claudeengine/command.go`
- `internal/shuttleengine/claudeengine/command_test.go`
- `internal/shuttleengine/claudeengine/doc.go`
- `internal/shuttleengine/claudeengine/events.go`
- `internal/shuttleengine/claudeengine/events_test.go`
- `internal/shuttleengine/claudeengine/settings.go`
- `internal/shuttleengine/claudeengine/settings_test.go`
- `internal/shuttleengine/claudeengine/startup.go`
- `internal/shuttleengine/claudeengine/startup_test.go`
- `internal/shuttleengine/config.go`
- `internal/shuttleengine/config_test.go`
- `internal/shuttleengine/doc.go`
- `internal/shuttleengine/engine.go`
- `internal/shuttleengine/fakes_test.go`
- `internal/shuttleengine/mux.go`
- `internal/shuttleengine/posix.go`
- `internal/shuttleengine/posix_test.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/run_test.go`
- `internal/shuttleengine/rundir.go`
- `internal/shuttleengine/rundir_test.go`
- `internal/shuttleengine/seam_enforcement_test.go`
- `internal/shuttleengine/spec.go`
- `internal/shuttleengine/spec_test.go`
- `internal/shuttleengine/template.go`
- `internal/shuttleengine/template.yaml`
- `internal/shuttleengine/wait.go`
- `internal/shuttleengine/wait_test.go`
- `sandbox-shuttle-suite.cmd`
- `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
