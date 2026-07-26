# Plan: Treadle: shared round-loop engine + perch rewrite

```yaml
task: 'Treadle: shared round-loop engine + perch rewrite'
slug: treadle
approved: false
started: '20260726-143105'
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
    name: treadle-extraction
    file: 01-treadle-extraction.md
    depends-on: []
    verify: go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./cmd/lyx/...
  - number: 2
    name: judge-handoff
    file: 02-judge-handoff.md
    depends-on: [1]
    verify: go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./cmd/lyx/...
  - number: 3
    name: preround-targeting
    file: 03-preround-targeting.md
    depends-on: [2]
    verify: go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./cmd/lyx/...
  - number: 4
    name: modelspec-migration
    file: 04-modelspec-migration.md
    depends-on: [3]
    verify: go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./internal/configreg/... ./cmd/lyx/...
  - number: 5
    name: docs-lifecycle
    file: 05-docs-lifecycle.md
    depends-on: [4]
    verify: go test ./...
```

The DAG is a deliberate linear chain: batches 2–5 each edit files their
predecessor also touches (`internal/treadleengine/run.go`, `judge.go`,
`state.go`, the judge templates, `internal/perchengine/doc.go`), so no two
batches are safe to run in parallel.

## Shared Decisions

### Decision: name-parameterized-diagnostics

- **Decision:** `treadleengine.New` takes a `name string` (perch passes
  `"perch"`). Every error and `logger.Warn` string moved out of perchengine
  keeps its exact current wording with the literal `perch` prefix produced
  from that name (e.g. the composed busy error must remain byte-identical:
  `perch: block is already running: %q (run.lock held); wait for it to
  finish or use a different --run-id`). The `ErrBlockBusy` sentinel's own
  message is the un-prefixed `block is already running`; the name prefix is
  applied at wrap time.
- **Rationale:** the differential bar — `internal/perchengine/run_test.go`
  and `state_test.go` assert error substrings; a future Tenter gets
  `tenter: `-prefixed diagnostics for free.
- **Applies to:** all batches

### Decision: differential-test-bar

- **Decision:** the existing `perchengine`/`perchcli` test suites are the
  acceptance test. Only three kinds of edits to existing test files are
  sanctioned: (a) mechanical package-split fallout in
  `internal/perchengine/run_test.go` helpers (card 3 — a test-local
  state-schema mirror struct and inlined artifact-path joins; assertion
  bodies untouched), (b) the judge read-set pins updated to the handoff
  contract (card 9), (c) config/profile fixtures migrated to model-spec
  strings (cards 12–13). Test files that move to `internal/treadleengine`
  move via `git mv` with surgical edits only. Everything else — burler
  hydration pins, ladder exactness, gate modes, pause, locking, resume, CLI
  behavior — must pass without modification.
- **Rationale:** the task's stated acceptance criterion: perch's unchanged
  behavior proves Treadle carries everything perch needs.
- **Applies to:** all batches

### Decision: byte-identical-perch-api

- **Decision:** `perchengine` keeps its full exported surface. Types that
  stay perch-owned: `Profile`, `Config`, `Engine`, `Options`, `Burler`,
  `Result`, `RoundSummary`, `Outcome`, `StuckReason`, `Gate`, `GateMode`
  and their constants (perch keeps its own `Gate`/`GateMode` declarations;
  the adapter converts to treadle's identically-spelled vocabulary — no
  aliasing of these, so `ProfileHash`'s JSON encoding provably cannot
  drift). Re-exported via alias/delegation in `internal/perchengine/identity.go`:
  `ProfileHash`, `DeriveRunID`, `ValidRunID`, `TerminalOutcome`,
  `PauseFlagPath`, `PauseFlagName`, `ErrBlockBusy` (aliased to treadle's
  sentinel so `errors.Is` matches across packages), and the
  `JudgeVerdict`/`TriageVerdict` types and constants (`JudgeProgressing`,
  `JudgeCircling`, `JudgeContinue`, `JudgeStop`, `JudgeUncertain`,
  `TriageRetry`, `TriageGiveUp`) as type aliases + aliased constants, so
  `run_test.go`'s references compile unchanged. `ParseJudgeVerdict`/
  `ParseTriageVerdict` move to treadleengine and are NOT re-exported (their
  `judgeFraming` parameter type was unexported, so no external caller can
  exist; `perchcli` consumes neither).
- **Rationale:** discussion decision perch-api-and-identity-stability;
  `perchcli` compiles untouched against the engine-facing API.
- **Applies to:** treadle-extraction, judge-handoff

### Decision: no-burler-import

- **Decision:** `internal/treadleengine` production code never imports
  `internal/burlerengine` (nor any `*cli` package). Its import allowlist:
  stdlib, `internal/hubgeometry`, `internal/lock`, `internal/logger`,
  `internal/state`, `internal/stencil`, `internal/shuttleengine`,
  `gopkg.in/yaml.v3`. `shuttleengine` is deliberately allowed: the judge/
  triage calls ride the Shuttle seam and `AttemptResult` reuses
  `shuttleengine.Outcome` (the discussion's "shuttle-style outcome").
  Enforced by `internal/treadleengine/seam_enforcement_test.go` (card 4)
  and recorded in `CONSTRAINTS.md` in the same commit.
- **Rationale:** discussion decision no-burler-import; anything genuinely
  shared gets extracted out of burler, never imported downward.
- **Applies to:** all batches

### Decision: treadle-owns-no-config

- **Decision:** treadleengine reads no config files and does no model-spec
  parsing. `treadleengine.Profile` carries resolved plain data only:
  `ProfileHash` (caller-computed identity), `Gate`+`GateDir`, resolved
  `RoundCaps`, `(JudgeModel, JudgeEffort)`, `(Model, Effort)`, `Timeout`,
  and (batch 3) `PreRoundTargeting`. All `perch.yaml`/profile loading,
  default resolution (`profile > perch.yaml > built-in`), and model-spec
  resolution stay in perchengine/perchcli.
- **Rationale:** discussion decision config-and-modelspec-migration;
  geometry-blindness (Hub Geometry Invariant) — treadle takes a
  caller-supplied absolute `runDir` and `GateDir`, constructs no `_lyx`
  paths.
- **Applies to:** all batches

### Decision: state-json-compatibility

- **Decision:** treadle's persisted `state.json` schema is byte-compatible
  with today's: identical JSON key spellings (`profileHash`, `roundCaps`,
  `rounds`, `round`, `attempts`, `shuttleOutcome`, `verdict`,
  `blockingCount`, `reviewPath`, `fixerReportPath`, `judgePath`,
  `gatePath`, `triagePath`, `judgeVerdict`, `gatePassed`, `sessionId`,
  `outcome`, `stuckReason`). New fields are strictly additive with
  `omitempty`: `handoffPath` (batch 2), `seedPath` (batch 3). A block
  written by the old binary resumes with zero migration; its records simply
  lack handoff coverage, which the fallback read-set already handles.
- **Rationale:** discussion decision perch-api-and-identity-stability.
- **Applies to:** all batches

### Decision: commit-style

- **Decision:** commit messages follow the repo's `<area>: <summary>`
  convention (`treadle: ...` for treadleengine work, `perch: ...` for
  perchengine/perchcli work, `docs: ...`/`manifest: ...` for doc-only
  cards), lowercase summary, no conventional-commit type prefixes.
- **Rationale:** matches recent history (`webster: rewrite for flat card
  list`, `manifest: ...`).
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `docs/overview.md`
- `internal/perchcli/cli.go`
- `internal/perchcli/run.go`
- `internal/perchcli/run_test.go`
- `internal/perchengine/adapter.go`
- `internal/perchengine/config.go`
- `internal/perchengine/config_test.go`
- `internal/perchengine/doc.go`
- `internal/perchengine/engine.go`
- `internal/perchengine/identity.go`
- `internal/perchengine/run_test.go`
- `internal/perchengine/template.go`
- `internal/perchengine/template.yaml`
- `internal/treadleengine/doc.go`
- `internal/treadleengine/engine.go`
- `internal/treadleengine/engine_test.go`
- `internal/treadleengine/gate.go`
- `internal/treadleengine/gate_lingering_test.go`
- `internal/treadleengine/gate_test.go`
- `internal/treadleengine/handoff.go`
- `internal/treadleengine/handoff_test.go`
- `internal/treadleengine/judge-circling-template.md`
- `internal/treadleengine/judge-milestone-template.md`
- `internal/treadleengine/judge.go`
- `internal/treadleengine/judge_test.go`
- `internal/treadleengine/judgeverdict.go`
- `internal/treadleengine/judgeverdict_test.go`
- `internal/treadleengine/profile.go`
- `internal/treadleengine/result.go`
- `internal/treadleengine/roundfiles.go`
- `internal/treadleengine/roundfiles_test.go`
- `internal/treadleengine/run.go`
- `internal/treadleengine/runner.go`
- `internal/treadleengine/seam_enforcement_test.go`
- `internal/treadleengine/smoke_judge_test.go`
- `internal/treadleengine/state.go`
- `internal/treadleengine/state_test.go`
- `internal/treadleengine/targeting-template.md`
- `internal/treadleengine/targeting.go`
- `internal/treadleengine/template.go`
- `internal/treadleengine/template_test.go`
- `internal/treadleengine/testmain_test.go`
- `internal/treadleengine/triage-template.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-PERCH-SUITE.md`
