# Plan: llm-driven child can park forever on Claude Code's own workspace-trust dialog

```yaml
task: llm-driven child can park forever on Claude Code's own workspace-trust dialog
slug: llm-driver-trust-dialog-hang
approved: false
discussion_sha: f9f8296b7272ac1ef9c6d5a5758b2b1e0341d50c
started: 20260923-080231
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shuttle-await-started
    file: 01-shuttle-await-started.md
    depends-on: []
    verify: go test ./internal/shuttleengine/
  - number: 2
    name: loom-llm-arm-readiness
    file: 02-loom-llm-arm-readiness.md
    depends-on: [1]
    verify: go test ./internal/loomcli/ && go test -tags smoke -run TestSmokeDriverStrand ./internal/loomcli/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: the AwaitStarted contract batch 2 consumes

- **Decision:** batch 1 adds exactly one exported method, `func (run *Run) AwaitStarted() (bool, error)`, in `internal/shuttleengine/wait.go`.
  `(true, nil)` means the provider reached `StartupReady` (with `run.state.Started` persisted to run.json), or the run's file contract is already satisfied.
  `(false, nil)` means the pane died or the startup window (`startup_timeout_s`, verbatim, 0 included) closed without `StartupReady`, or the tick-count cap was hit with the file contract unsatisfied.
  A non-nil error means `checkLivenessTick` failed `maxStatusRetries` consecutive times with the file contract unsatisfied.
  It never calls `finalize` and never writes a terminal `Outcome`.
- **Rationale:** the discussion's "Mechanism" and "Result mapping and bookkeeping" decisions; batch 2's `driverHandle` interface names this exact signature.
- **Applies to:** all batches

### Decision: tests stay untagged and hermetic, except the one smoke file

- **Decision:** every new or changed test in `internal/shuttleengine` and `internal/loomcli` is untagged and uses fakes plus a fake clock, with no real spawn and no `time.Sleep` of 1s or more (Test Tier Purity Invariant).
  The one exception is `internal/loomcli/smoke_driverstrand_test.go`, which is already `smoke`-tagged and is adapted rather than created.
- **Rationale:** the discussion's Testing section and the Test Tier Purity Invariant.
- **Applies to:** all batches

### Decision: docs/overview.md is not edited

- **Decision:** `docs/overview.md`'s `lyx loom start` bootstrap bullet names what the bootstrap spawns but not the readiness signal either arm waits on, so the discussion's conditional ("if it names the readiness signal") does not fire and the file is left unchanged.
  The observable readiness change is documented where it is stated today: the `start` command's `Long` text and the `--no-attach` usage string, both edited in the same card and commit as the behavior change (batch 2, card 3).
  `CONSTRAINTS.md` and `manifest/roadmap.md` are unchanged: no new cross-cutting invariant, and this is a bugfix.
- **Rationale:** CLAUDE.md's "docs land in the same commit" rule binds the docs that describe the changed behavior, and only those.
- **Applies to:** loom-llm-arm-readiness

### Decision: the live reproduction recipe is an operator step, not a card

- **Decision:** the discussion's "Reproduction recipe (live verification)" is run by the operator after mill-go completes, not by a mill-go implementer card.
  mill-go's completion report must restate it as an outstanding verification item, citing `_mill/discussion.md`'s recipe section, and the operator records the `jq` before/after output and the pane capture in the task's completion notes.
- **Rationale:** the recipe launches a real, billed interactive Claude session that goes on to drive a loom run, builds a fixture hub outside this worktree, and mutates the operator's own `~/.claude.json`.
  None of those belong in an unattended implementer's verify loop, and the discussion itself classifies the recipe as manual and not automated.
  The smoke test (batch 2, card 4) is the automated live-substrate check of `AwaitStarted`'s success path.
- **Applies to:** all batches

### Decision: done_gate stays as configured

- **Decision:** `pipeline.done_gate` is already `go test ./... && go test -tags integration ./...` in this hub's config, which covers every package outside the two batch verify scopes; no change.
  `golangci-lint` is not installed on this host, so no lint command is added.
- **Rationale:** mill-plan's done-gate guidance; the configured gate already exceeds the batch scopes.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `internal/loomcli/driverlaunch.go`
- `internal/loomcli/driverlaunch_test.go`
- `internal/loomcli/driverspec.go`
- `internal/loomcli/smoke_driverstrand_test.go`
- `internal/loomcli/start.go`
- `internal/loomcli/start_driver_test.go`
- `internal/shuttleengine/awaitstarted_test.go`
- `internal/shuttleengine/completionsignal_enforcement_test.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/wait.go`
