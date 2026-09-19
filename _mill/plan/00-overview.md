# Plan: Seeded driver choice: ly-drive strand as the child's driver

```yaml
task: 'Seeded driver choice: ly-drive strand as the child''s driver'
slug: seeded-driver-choice
approved: false
started: '20260919-174938'
parent: main
root: ""
verify: go build ./...
discussion_sha: 8203a2d5a96a7f432bade23c3f67daee901ead70
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shuttle-seam
    file: 01-shuttle-seam.md
    depends-on: []
    verify: go build ./... && go test ./internal/shuttleengine/...
  - number: 2
    name: loom-driver-config
    file: 02-loom-driver-config.md
    depends-on: []
    verify: go build ./... && go test ./internal/loomengine/...
  - number: 3
    name: bootstrap-verb-capability
    file: 03-bootstrap-verb-capability.md
    depends-on: []
    verify: go build ./... && go test ./internal/loomcli/... ./internal/battencli/... ./internal/shedcli/...
  - number: 4
    name: driver-launch
    file: 04-driver-launch.md
    depends-on: [1, 2, 3]
    verify: go build ./... && go test ./internal/loomcli/... ./cmd/lyx/...
  - number: 5
    name: lift-llm-refusals
    file: 05-lift-llm-refusals.md
    depends-on: [4]
    verify: go build ./... && go test ./internal/shedrun/... ./internal/shedcli/... ./internal/battencli/... && go test -tags integration ./internal/shedcli/...
  - number: 6
    name: ly-drive-autonomous
    file: 06-ly-drive-autonomous.md
    depends-on: [4]
    verify: go build ./... && go test ./cmd/lyx/...
  - number: 7
    name: smoke-and-integration
    file: 07-smoke-and-integration.md
    depends-on: [5]
    verify: go build ./... && go test -tags smoke ./internal/loomcli/... && go test -tags integration ./internal/loomcli/...
  - number: 8
    name: docs-sweep
    file: 08-docs-sweep.md
    depends-on: [5, 6, 7]
    verify: go build ./... && go test ./cmd/lyx/... ./internal/loomcli/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: go-verify-commands-carry-no-PYTHONPATH-prefix

- **Decision:** every `verify:` in this plan is a bare `go build`/`go test` invocation with no `PYTHONPATH= ` prefix.
- **Rationale:** the prefix exists to stop a Python test subprocess inheriting mill's cache scripts directory.
  This is a Go module; the `verify-not-isolated` validator check is language-conditional and does not apply.
  `CGO_ENABLED=1` is required for every build here per the Quarry CGO Requirement Invariant, and is already the native default when a C compiler is on `PATH`, so no command sets it explicitly.
- **Applies to:** all batches

### Decision: mechanism-ships-before-the-gate-is-lifted

- **Decision:** `shedrun.ValidateDriver` keeps refusing `DriverLLM` until batch 5, which is the last behavioural batch.
  Batches 1-4 build the whole `llm` path — the spec seam, the config key, the capability constants, the bootstrap branch — while no seed on disk can legally hold `llm`.
- **Rationale:** the alternative orderings both produce a commit that is honestly broken.
  Lifting the refusal first would let an operator seed `driver: llm` and silently get the `go` driver, which is exactly the "what a run was seeded as diverges from what it is doing" failure the new invariant exists to prevent — shipped deliberately, for the span of several commits.
  Lifting it in the middle would do the same for a shorter window.
  Building the branch first costs nothing, because every piece of it is a pure function or sits behind an injected seam and is therefore directly unit-testable against a synthetic `shedrun.Seed` value without any seed on disk ever holding `llm`.
  The branch is unreachable-in-production, not untested, for the three commits between batch 4 and batch 5.
- **Applies to:** all batches

### Decision: per-module-docs-ride-their-own-card

- **Decision:** a `CONSTRAINTS.md` amendment lands in the card whose change makes it true or false, not in the final docs batch.
  The new Driver Choice Single-Site Invariant therefore lands in batch 4, the batch that creates the single read site it governs.
  Batch 8 carries only the cross-cutting prose sweep that cannot be written until the whole shape exists: `docs/overview.md`, `manifest/roadmap.md`, `manifest/designs/seeded-shed.md`, and the sandbox suite document.
- **Rationale:** CLAUDE.md requires docs in the same commit as the change they describe.
  This mirrors the predecessor task's decision of the same name, so a reader moving between the two plans meets one convention rather than two.
- **Applies to:** all batches

### Decision: driver-seams-are-interface-fields-on-loomCLI

- **Decision:** every piece of the `llm` path that touches a live substrate reaches it through an interface field on `loomCLI`, with the production implementation a thin adapter over the concrete engine value the struct already carries.
  Two such fields are added: `driverStarter` over `*shuttleengine.Runner`, and `driverPaneProbe` over `*reedengine.Engine`.
- **Rationale:** the Test Tier Purity Invariant bars `exec.Command` and real spawns from untagged files, and `loomCLI.runner`/`loomCLI.reed` are concrete types, so without a seam the branch could not be exercised without a real tmux and a real provider binary.
  `runnerMasterStarter` in `internal/loomcli/cli.go` is the shape to copy — it already adapts that same `*shuttleengine.Runner` to `websterengine.MasterStarter` for exactly this reason.
- **Applies to:** driver-launch, smoke-and-integration

### Decision: pure-decisions-live-in-bootstrap-go

- **Decision:** every new predicate, name constant, path composer and prompt composer this task adds to `internal/loomcli` is a pure function or takes injected seams, and lives in `internal/loomcli/bootstrap.go` or a new sibling beside it — never inline in `start.go`'s verb body.
- **Rationale:** `bootstrap.go`'s own file doc comment states this rule for the package, and `start.go`'s body is already assembly over judgment that is under test.
  The `llm` branch roughly doubles that body's decision count, so following the existing rule is what keeps the verb readable and the decisions testable with no real lock, process or clock.
- **Applies to:** driver-launch

### Decision: the-bootstrap-lock-is-released-on-every-llm-error-path

- **Decision:** the `llm` branch releases the bootstrap lock explicitly on every one of its own failure returns, exactly as the `go` branch does, and never by `defer`.
- **Rationale:** `start.go` holds the bootstrap lock across steps 5 and 6 and releases it explicitly at step 7, because it must stay held across the spawn and the handshake.
  A `defer` would release it at `RunE` return, far too late to be a substitute and far too early to be correct.
  A leaked bootstrap lock wedges every subsequent `lyx loom start` in that worktree and is invisible until the second invocation, which is why the error paths get their own card and their own test list rather than being left to review.
- **Applies to:** driver-launch

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens)._

- `CONSTRAINTS.md`
- `cmd/lyx/drivercap_test.go`
- `cmd/lyx/helptree_test.go`
- `docs/overview.md`
- `internal/battencli/bootstrapverb.go`
- `internal/battencli/bootstrapverb_test.go`
- `internal/battencli/cli.go`
- `internal/battencli/cli_test.go`
- `internal/battencli/refusal.go`
- `internal/battencli/refusal_test.go`
- `internal/loomcli/bootstrap.go`
- `internal/loomcli/bootstrap_test.go`
- `internal/loomcli/bootstrapverb.go`
- `internal/loomcli/bootstrapverb_test.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/driverlaunch.go`
- `internal/loomcli/driverlaunch_test.go`
- `internal/loomcli/driverprompt.go`
- `internal/loomcli/driverprompt_test.go`
- `internal/loomcli/driverreport.go`
- `internal/loomcli/driverreport_test.go`
- `internal/loomcli/driverspec.go`
- `internal/loomcli/driverspec_test.go`
- `internal/loomcli/integration_driverbootstrap_test.go`
- `internal/loomcli/smoke_driverstrand_test.go`
- `internal/loomcli/start.go`
- `internal/loomcli/start_driver_test.go`
- `internal/loomcli/testmain_integration_test.go`
- `internal/loomcli/wiring.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/driver.go`
- `internal/loomengine/driver_test.go`
- `internal/shedcli/seed.go`
- `internal/shedcli/seed_test.go`
- `internal/shedcli/table.go`
- `internal/shedcli/table_test.go`
- `internal/shedrun/seed.go`
- `internal/shedrun/seed_test.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/run_test.go`
- `internal/shuttleengine/spec.go`
- `internal/shuttleengine/spec_test.go`
- `manifest/designs/seeded-shed.md`
- `manifest/roadmap.md`
- `plugins/ly/skills/ly-drive/SKILL.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
