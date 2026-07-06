# Plan: Add Effort to shuttle's run Spec

```yaml
task: Add Effort to shuttle's run Spec
slug: shuttle-spec-effort
approved: false
started: 20260706-163844
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: effort
    file: 01-effort.md
    depends-on: []
    verify: go test ./internal/shuttleengine/... ./internal/shuttlecli/...
  - number: 2
    name: ask-signal
    file: 02-ask-signal.md
    depends-on: [1]
    verify: go test ./internal/shuttleengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: provider-seam containment

- **Decision:** All Claude specifics — the `--effort`/`--model` flags, the effort
  vocabulary set, the `settings.json` hook schema, `hook_event_name` / `tool_name`
  / `tool_input` payload shapes, and the literal `AskUserQuestion` tool name — live
  ONLY under `internal/shuttleengine/claudeengine`. `internal/shuttleengine` stays
  provider-invariant: the renamed `Event` type and its `Kind` constants
  (`EventStop` / `EventAsk`) are provider-neutral; no Claude marker string appears
  in `shuttleengine`.
- **Rationale:** Shuttle Provider-Seam Invariant (`CONSTRAINTS.md`). The import-graph
  half is machine-checked by `seam_enforcement_test.go`; the neutral-`Kind`-naming is
  the semantic half (a review obligation).
- **Applies to:** all batches

### Decision: effort is engine-validated, vocabulary-only

- **Decision:** `shuttleengine.Spec.Effort` is a plain pass-through string (empty =
  provider default); `Spec.validate` never inspects it. `claudeengine` owns
  validation: `Prepare` hard-errors on any non-empty value that is not an
  exact-lowercase member of `{low, medium, high, xhigh, max}`, before writing any
  artifact. Per-model support is NOT policed (it is invisible from the CLI — proven
  live: `claude --model haiku --effort high` returns success with no signal).
- **Rationale:** Discussion decisions `effort-validation` and
  `per-model-realizability`. claude only warns-and-ignores a bad value, so shuttle
  must hard-error itself; it cannot detect model incompatibility, so it does not
  pretend to.
- **Applies to:** effort

### Decision: effort carrier is the `--effort` launch flag

- **Decision:** `claudeengine` realizes `Effort` via the `--effort <value>` CLI flag
  appended in `buildLaunchCmd` next to `--model`, single-quoted, launch-only
  (`buildResumeCmd` unchanged). Not the `effortLevel` settings key.
- **Rationale:** Discussion decisions `effort-carrier` and resume behavior — the flag
  supports the full range incl. `max` (the settings key lacks it) and mirrors the
  existing `--model` handling exactly.
- **Applies to:** effort

### Decision: live-ask is a real-time `OutcomeAsking` via a non-denying marker hook

- **Decision:** Interactive runs get a non-denying `PreToolUse(AskUserQuestion)` hook
  whose command is the SAME append command the `Stop` hook uses; the payload
  self-describes, so `ParseEvents` classifies it. `pollEventsTick` maps a live-ask
  event to the existing `OutcomeAsking` (no new `Outcome` value) the instant it
  appears (done-first preserved), keeping the pane for attach. This single-`Wait`
  termination for every interactive run at the first question is the intended
  contract, not a regression (answer-and-continue is orchestration-layer, out of
  scope).
- **Rationale:** Discussion decisions `ask-signal-mechanism` and
  `ask-signal-outcome`.
- **Applies to:** ask-signal

### Decision: verify scopes cover all touched packages

- **Decision:** Batch verify commands use the native Go runner (no `PYTHONPATH=`
  prefix — this is a Go repo). No external package references the renamed `StopEvent`
  type (grep-verified: only the three shuttle packages use it), so the per-batch
  scopes cover every affected package and no repo-wide `verify:` / `done_gate` is
  added.
- **Rationale:** Go project; containment verified during planning.
- **Applies to:** all batches

## All Files Touched

- `docs/overview.md`
- `internal/shuttlecli/cli_test.go`
- `internal/shuttlecli/run.go`
- `internal/shuttleengine/claudeengine/claudeengine.go`
- `internal/shuttleengine/claudeengine/command.go`
- `internal/shuttleengine/claudeengine/command_test.go`
- `internal/shuttleengine/claudeengine/events.go`
- `internal/shuttleengine/claudeengine/events_test.go`
- `internal/shuttleengine/claudeengine/prepare_test.go`
- `internal/shuttleengine/claudeengine/settings.go`
- `internal/shuttleengine/claudeengine/settings_test.go`
- `internal/shuttleengine/engine.go`
- `internal/shuttleengine/fakes_test.go`
- `internal/shuttleengine/spec.go`
- `internal/shuttleengine/spec_test.go`
- `internal/shuttleengine/wait.go`
- `internal/shuttleengine/wait_test.go`
