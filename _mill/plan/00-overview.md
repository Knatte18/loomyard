# Plan: Build perch - the review gate loop

```yaml
task: "Build perch - the review gate loop"
slug: "internal-perch"
approved: true
started: "20260708-152618"
parent: "main"
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: foundations
    file: 01-foundations.md
    depends-on: []
    verify: go test ./internal/burlerengine/ ./internal/hubgeometry/ ./internal/perchengine/ ./internal/configreg/
  - number: 2
    name: profile-state
    file: 02-profile-state.md
    depends-on: [1]
    verify: go test ./internal/perchengine/
  - number: 3
    name: judge-triage
    file: 03-judge-triage.md
    depends-on: [2]
    verify: go test ./internal/perchengine/
  - number: 4
    name: gate-loop
    file: 04-gate-loop.md
    depends-on: [3]
    verify: go test ./internal/perchengine/
  - number: 5
    name: cli-docs
    file: 05-cli-docs.md
    depends-on: [4]
    verify: go test ./cmd/lyx/ ./internal/perchcli/ ./internal/perchengine/
```

## Shared Decisions

### Decision: mirror the burler module patterns exactly

- **Decision:** `internal/perchengine` / `internal/perchcli` copy the structural patterns of
  `internal/burlerengine` / `internal/burlercli` wherever a pattern exists: package split
  (engine never imports cobra/claudeengine; cli is the claudeengine wiring point), package-local
  seam interfaces with compile-time `var _ Iface = (*concrete)(nil)` proofs, `go:embed` +
  `internal/stencil` prompt templates, strict fail-loud parsers, `PersistentPreRunE` wiring
  chain with the bare-group guard, manual required-flag validation (never `MarkFlagRequired`),
  and JSON envelopes via `internal/output` + `internal/clihelp`.
- **Rationale:** burler is the reviewed, landed precedent one layer down the same spine;
  divergence would be rot, not design.
- **Applies to:** all batches

### Decision: YAML vocabulary split — config snake_case, profile kebab-case

- **Decision:** `perch.yaml` (the config module template) uses snake_case keys
  (`judge_model`, `judge_effort`, `round_caps`), matching the repo's config convention
  (`run_dir`, `poll_interval_ms` in shuttle/mux templates). The perch **profile** file uses
  kebab-case keys (`fix-scope`, `round-caps`, `judge-model`, `gate`), matching the burler
  profile vocabulary it embeds. The discussion wrote both as kebab-case; the config side is
  normalized here to the repo convention.
- **Rationale:** each file joins an existing vocabulary; consistency within each family beats
  consistency between them.
- **Applies to:** all batches

### Decision: error and fail-safe posture

- **Decision:** everything machine-read is parsed fail-loud with `"perch: "`-prefixed errors
  (mirroring `burlerengine.ParseReview`). The two LLM utility calls (progress judge,
  asking-triage) are the only fail-safe surface: any judge/triage infrastructure failure
  (spawn error, non-done outcome, unparseable verdict file) degrades to the safe default
  (judge → "progressing"/CONTINUE, triage → RETRY) with a `logger.Warn` carrying round, rung,
  and cause — never an error return, never STUCK. A *burler* round that reaches done with an
  invalid review file is a hard error (burler already returns it as one).
- **Rationale:** discussion Decisions "Verdict-judge model" and "Non-done burler outcomes";
  false-stuck is the costly error, the hard cap bounds a wrong "continue".
- **Applies to:** all batches

### Decision: round artifact naming and attempt suffixes

- **Decision:** all block artifacts live flat in the run dir: `state.json` (+
  `state.json.lock`), `round-<N>-review.md`, `round-<N>-fixer-report.md`,
  `round-<N>-judge.md`, `round-<N>-gate.md`, `round-<N>-triage.md`, `pause`. A retried
  attempt of round N appends a letter to the round token: `round-<N>b-review.md` (attempt 2).
  On resume, stale artifacts of an incomplete round are renamed with a `.stale` suffix before
  the round re-runs (shuttle rejects pre-existing output files).
- **Rationale:** one dir per block is the whole resume/judge-input/weft-commit surface;
  attempt letters keep every shuttle OutputFiles entry fresh without inventing subdirs.
- **Applies to:** profile-state, gate-loop, cli-docs

### Decision: the engine is weft-blind and geometry-blind

- **Decision:** `perchengine` never imports `weftengine`/`warpengine` and never constructs a
  `_lyx` path. It operates on a caller-supplied absolute `runDir`. `perchcli` resolves the
  run dir via the new `hubgeometry.PerchRunsDir` accessor and owns the weft commit+push at
  block exit (Weft Git Invariant; `internal/initengine/undo.go` is the call-shape precedent).
- **Rationale:** same split burler already enforces; loom later supplies its own run dirs and
  owns its own weft sync.
- **Applies to:** all batches

### Decision: test regime

- **Decision:** perchengine's suite is deterministic Go with scripted fakes for the three
  seams (Burler, Shuttle, gate CommandRunner) — no LLM, no psmux, standard `testing` package,
  table-driven where the discussion's Testing section lists scenario families. The only LLM
  touch is one opt-in smoke test gated by `//go:build smoke` (burlerengine's
  `smoke_round_test.go` pattern). perchcli tests mirror `burlercli/cli_test.go`.
- **Rationale:** discussion Testing section; the deterministic loop IS perch's strong test
  surface.
- **Applies to:** all batches

## All Files Touched

- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `docs/modules/README.md`
- `docs/modules/hardener.md`
- `docs/modules/loom.md`
- `docs/overview.md`
- `docs/reviews/README.md`
- `docs/roadmap.md`
- `internal/burlerengine/doc.go`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/engine_test.go`
- `internal/burlerengine/verdict.go`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_unit_test.go`
- `internal/perchcli/cli.go`
- `internal/perchcli/cli_test.go`
- `internal/perchcli/pause.go`
- `internal/perchcli/run.go`
- `internal/perchcli/run_test.go`
- `internal/perchengine/config.go`
- `internal/perchengine/config_test.go`
- `internal/perchengine/doc.go`
- `internal/perchengine/engine.go`
- `internal/perchengine/gate.go`
- `internal/perchengine/gate_test.go`
- `internal/perchengine/judge-circling-template.md`
- `internal/perchengine/judge-milestone-template.md`
- `internal/perchengine/judge.go`
- `internal/perchengine/judge_test.go`
- `internal/perchengine/judgeverdict.go`
- `internal/perchengine/judgeverdict_test.go`
- `internal/perchengine/profile.go`
- `internal/perchengine/profile_test.go`
- `internal/perchengine/result.go`
- `internal/perchengine/roundfiles.go`
- `internal/perchengine/roundfiles_test.go`
- `internal/perchengine/run.go`
- `internal/perchengine/run_test.go`
- `internal/perchengine/smoke_judge_test.go`
- `internal/perchengine/state.go`
- `internal/perchengine/state_test.go`
- `internal/perchengine/template.go`
- `internal/perchengine/template.yaml`
- `internal/perchengine/template_test.go`
- `internal/perchengine/triage-template.md`
- `tools/sandbox/SANDBOX-BURLER-SUITE.md`
