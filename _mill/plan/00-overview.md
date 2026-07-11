# Plan: Build builder - the batch-implementation loop

```yaml
task: "Build builder - the batch-implementation loop"
slug: "internal-builder"
approved: true
started: "20260711-121453"
parent: "main"
root: ""
verify: go build ./...
```

## Batch Index

```yaml
batches:
  - number: 1
    name: plan-model
    file: 01-plan-model.md
    depends-on: []
    verify: go test ./internal/hubgeometry/... ./internal/builderengine/...
  - number: 2
    name: config-roles
    file: 02-config-roles.md
    depends-on: []
    verify: go test ./internal/builderengine/... ./internal/configreg/...
  - number: 3
    name: state-report-digest
    file: 03-state-report-digest.md
    depends-on: [1]
    verify: go test ./internal/builderengine/...
  - number: 4
    name: poll-pause
    file: 04-poll-pause.md
    depends-on: [3]
    verify: go test ./internal/builderengine/...
  - number: 5
    name: spawn
    file: 05-spawn.md
    depends-on: [2, 4]
    verify: go test ./internal/builderengine/...
  - number: 6
    name: orchestrator-run
    file: 06-orchestrator-run.md
    depends-on: [5]
    verify: go test ./internal/builderengine/...
  - number: 7
    name: buildercli
    file: 07-buildercli.md
    depends-on: [6]
    verify: go test ./internal/buildercli/... ./internal/builderengine/...
  - number: 8
    name: registration-docs
    file: 08-registration-docs.md
    depends-on: [7]
    verify: go test ./cmd/lyx/... ./internal/buildercli/...
```

## Shared Decisions

### Decision: authoritative design source

- **Decision:** `_mill/discussion.md` is the pinned design record for every decision in
  this plan (verb surface, digest contract, poll classification, chain rollback, pause,
  outcome contract, resume, config keys). `docs/modules/plan-format.md` is the pinned
  input/output contract (plan structure, batch-report schema, validation checks 1–6).
  When a card's Requirements seem ambiguous, those two documents break the tie — never
  invent a third reading.
- **Rationale:** the discussion went through 3 review rounds; re-deriving decisions
  mid-implementation is how designs rot.
- **Applies to:** all batches

### Decision: engine/cli split and weft ownership

- **Decision:** `internal/builderengine` is the domain kernel: no cobra, no
  `io.Writer`/exit codes, returns `(T, error)`. It is geometry-AWARE (it resolves
  `_lyx/plan` and `_lyx/builder` via the new `hubgeometry` helpers — the paths are part
  of the pinned plan contract) but weft-BLIND: every weft commit lives in
  `internal/buildercli`, mirroring `perchcli`'s block-exit `weftengine.Commit` + `Push`
  with the `:(exclude)*.lock` pathspec (lock files are machine-local, never committed).
- **Rationale:** CLI/Cobra Invariant package-naming rule; Weft Git Invariant (Go verbs
  commit `_lyx` artifacts; agents never run weft git); perchcli precedent at
  `internal/perchcli/run.go`.
- **Applies to:** all batches

### Decision: fail-loud parses, fail-safe never

- **Decision:** every machine-read artifact (plan overview frontmatter, batch
  frontmatter, batch-report YAML, outcome.yaml, state.json, builder.yaml, role
  model-specs) parses strictly and fails loud on anything unrecognized —
  `yaml.Decoder.KnownFields(true)` for YAML structs, explicit enum checks for
  vocabularies. builder has NO fail-safe LLM-utility surface (perch's judge/triage
  posture does not apply here — there is no builder evaluator).
- **Rationale:** plan-format.md's "fail loud, never misread" discipline; the burler
  verdict-parse precedent.
- **Applies to:** all batches

### Decision: provider invariance

- **Decision:** builder never references Claude specifics. Model selection maps
  `modelspec.Resolved` onto `shuttleengine.Spec` exactly as modelspec's package doc
  pins: `spec.Model = resolved.Model; spec.Effort = resolved.Params["effort"];
  spec.Version = resolved.Params["version"]`. Engine names outside `claude` fail loud at
  role resolution (modelspec already enforces the known-engine set).
- **Rationale:** Shuttle Provider-Seam Invariant.
- **Applies to:** all batches

### Decision: prompt templates are embedded stencils, co-versioned

- **Decision:** the orchestrator and implementer prompts are `.md` assets in
  `internal/builderengine`, `//go:embed`'d, filled via `stencil.Fill` (fail-loud on
  unfilled top-level markers), with property tests pinning their contract statements —
  the burler `template_test.go` pattern. Caller-required markers stay at template top
  level (never inside `{{if}}` branches), per `stencil.Fill`'s documented guarantee.
- **Rationale:** the prompt is half of a Go-parsed contract (digest fields, verb names,
  outcome/report schemas); independent versioning drifts silently.
- **Applies to:** spawn, orchestrator-run

### Decision: geometry only via hubgeometry

- **Decision:** every `_lyx/plan` / `_lyx/builder` path resolves through the new
  `hubgeometry.PlanDir` / `BuilderDir` / `BuilderReportsDir` helpers (batch 1). No other
  package — production OR test — constructs those paths; tests anchor fixtures by
  calling the helpers against a temp base dir, or point parsers at plain `testdata/`
  dirs (the parser takes an explicit dir argument).
- **Rationale:** Hub Geometry Invariant, machine-enforced by
  `internal/hubgeometry/enforcement_test.go` (`TestEnforcement_GeometryLiterals`).
- **Applies to:** all batches

### Decision: git operations via gitexec

- **Decision:** all host-repo git queries and mutations (batch start-SHA capture, diff
  vs start-SHA, dirty check, chain reset) go through `gitexec.RunGit(args, cwd)`.
  Chain reset (`git reset --hard <recorded SHA>`) happens ONLY inside
  `builderengine`'s restart-chain path, from the SHA recorded in state.json — never
  from a caller-supplied SHA string.
- **Rationale:** discussion's chain-rollback decision (correctness-by-tool-design);
  existing `internal/gitexec` seam.
- **Applies to:** state-report-digest, spawn, orchestrator-run

### Decision: test conventions

- **Decision:** table-driven Go tests colocated per package; scratch git repos built
  with `git init` + commits via `gitexec.RunGit` in `t.TempDir()`; config seeded via
  `lyxtest.SeedConfig` (lyxtest Leaf Invariant — never import configreg from lyxtest
  call sites' fixtures); shuttle interaction tested against fake `MuxOps`/`Engine`
  implementations modeled on `internal/shuttleengine/fakes_test.go` (builderengine
  defines its own local fakes; test helpers are package-local, not exported).
  No real agent spawns anywhere in `go test`.
- **Rationale:** golang-testing conventions; CI determinism; subscription economics.
- **Applies to:** all batches

### Decision: commit message convention

- **Decision:** conventional-commit subjects, scope = module: `feat(builder): …`,
  `feat(hubgeometry): …`, `test(builder): …`, `docs(builder): …`. One commit per card.
- **Rationale:** matches recent repo history (`chore(gitignore): …`, `docs(skills): …`).
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `docs/modules/builder.md`
- `docs/modules/loom.md`
- `docs/modules/plan-format.md`
- `docs/overview.md`
- `docs/reference/model-spec.md`
- `docs/roadmap.md`
- `internal/buildercli/cli.go`
- `internal/buildercli/cli_test.go`
- `internal/buildercli/pause.go`
- `internal/buildercli/pause_test.go`
- `internal/buildercli/poll.go`
- `internal/buildercli/poll_test.go`
- `internal/buildercli/run.go`
- `internal/buildercli/run_test.go`
- `internal/buildercli/spawnbatch.go`
- `internal/buildercli/spawnbatch_test.go`
- `internal/buildercli/status.go`
- `internal/buildercli/status_test.go`
- `internal/buildercli/validate.go`
- `internal/buildercli/validate_test.go`
- `internal/buildercli/weft.go`
- `internal/builderengine/chain.go`
- `internal/builderengine/chain_test.go`
- `internal/builderengine/config.go`
- `internal/builderengine/config_test.go`
- `internal/builderengine/digest.go`
- `internal/builderengine/digest_test.go`
- `internal/builderengine/doc.go`
- `internal/builderengine/fingerprint.go`
- `internal/builderengine/fingerprint_test.go`
- `internal/builderengine/gitquery.go`
- `internal/builderengine/gitquery_test.go`
- `internal/builderengine/implementer-template.md`
- `internal/builderengine/orchestrator-template.md`
- `internal/builderengine/outcome.go`
- `internal/builderengine/outcome_test.go`
- `internal/builderengine/pause.go`
- `internal/builderengine/pause_test.go`
- `internal/builderengine/plan.go`
- `internal/builderengine/plan_test.go`
- `internal/builderengine/poll.go`
- `internal/builderengine/poll_test.go`
- `internal/builderengine/report.go`
- `internal/builderengine/report_test.go`
- `internal/builderengine/roles.go`
- `internal/builderengine/roles_test.go`
- `internal/builderengine/runlevel.go`
- `internal/builderengine/runlevel_test.go`
- `internal/builderengine/spawn.go`
- `internal/builderengine/spawn_test.go`
- `internal/builderengine/state.go`
- `internal/builderengine/state_test.go`
- `internal/builderengine/template.go`
- `internal/builderengine/template.yaml`
- `internal/builderengine/template_test.go`
- `internal/builderengine/testdata/plan-broken-chain/00-overview.md`
- `internal/builderengine/testdata/plan-broken-chain/01-first.md`
- `internal/builderengine/testdata/plan-broken-chain/02-second.md`
- `internal/builderengine/testdata/plan-unapproved/00-overview.md`
- `internal/builderengine/testdata/plan-unapproved/01-only.md`
- `internal/builderengine/testdata/plan-valid/00-overview.md`
- `internal/builderengine/testdata/plan-valid/01-json-flag.md`
- `internal/builderengine/testdata/plan-valid/02-list-tests.md`
- `internal/builderengine/testdata/plan-valid/03-refactor-a.md`
- `internal/builderengine/testdata/plan-valid/04-refactor-b.md`
- `internal/builderengine/testdata/plan-valid/05-oversized.md`
- `internal/builderengine/validate.go`
- `internal/builderengine/validate_test.go`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_test.go`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
