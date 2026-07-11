# Batch: orchestrator-run

```yaml
task: "Build builder - the batch-implementation loop"
batch: "orchestrator-run"
number: 6
cards: 3
verify: go test ./internal/builderengine/...
depends-on: [5]
```

## Batch Scope

The orchestrator side: the embedded orchestrator prompt template (the judgment core
over the verbs, co-versioned with the digest contract), the fail-loud outcome.yaml
parser with stale-file archiving, and the engine-level `Run` — lock, fingerprint gate,
validation gate, orchestrator spawn, shuttle-outcome mapping. External interface
consumed later: `Run`, `RunResult`, `ParseOutcome`, `ArchiveStaleOutcome`,
`ErrRunBusy`, `ErrFingerprintMismatch`.

## Cards

### Card 23: orchestrator prompt template

- **Context:**
  - `internal/burlerengine/template_test.go`
  - `internal/stencil/stencil.go`
  - `_mill/discussion.md`
  - `docs/modules/plan-format.md`
- **Creates:**
  - `internal/builderengine/orchestrator-template.md`
- **Edits:**
  - `internal/builderengine/template.go`
  - `internal/builderengine/template_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `orchestrator-template.md` is the long-lived orchestrator
  session's prompt — the judgment core; the machine lives in the verbs. Top-level
  markers: `{{.batch_index}}` (Go-rendered ordered list: number, slug, intent, plus
  `oversized`/chain annotations per batch — the discussion's pinned navigation
  source), `{{.progress}}` (Go-rendered per-batch state summary for resume; the word
  `none` for a fresh run), `{{.outcome_path}}` (absolute outcome.yaml path),
  `{{.self_fix_cap}}`, `{{.poll_wait_s}}`. The prompt instructs: drive batches
  STRICTLY in order via `lyx builder spawn-batch <NN>`; after each spawn call
  `lyx builder poll` and re-poll on a `running` return (the call blocks — that is the
  notification); read ONLY the digest fields, quoted exactly as the pinned contract
  (batch, status, tests, stuck_reason, out_of_scope, drift_unreported, files_changed,
  dirty, dead_reason, elapsed_s) — never raw session output, never files in the
  shuttle run dir; judge out_of_scope justifications and unreported drift (demand
  revert or accept — plan-format.md's honest-limitation note); on `stuck`/`dead`,
  recovery is YOUR judgment: respawn fresh once for `dead`, escalate via
  `spawn-batch <NN> --role recovery` after the implementer's in-session self-fix cap
  is exhausted, and for a stuck chain member restart the whole chain via
  `spawn-batch <NN> --restart-chain` (Go performs the reset); a `paused` refusal from
  spawn-batch → write the outcome file with `outcome: paused` and stop; NEVER run git
  against the weft, NEVER edit code yourself, NEVER use a `/model` switch; the FINAL
  action — after the last batch is green, or on stuck/paused — is writing
  `{{.outcome_path}}` with exactly `outcome: done | stuck | paused`,
  `stuck_reason: null | "<one line>"`, `batches_done: <int>`. Extend `template.go`
  with `//go:embed orchestrator-template.md` + `OrchestratorTemplate() []byte`.
  Property tests: fills clean; names all three verbs it drives (`spawn-batch`,
  `poll`, `status`); quotes every digest field name and no others; quotes the outcome
  schema keys; forbids weft git and self-editing; states strict batch order and the
  recovery ladder.
- **Commit:** `feat(builder): embedded orchestrator prompt template`

### Card 24: outcome contract

- **Context:**
  - `_mill/discussion.md`
- **Creates:**
  - `internal/builderengine/outcome.go`
  - `internal/builderengine/outcome_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `Outcome` struct: `Outcome string` (`done|stuck|paused`),
  `StuckReason string` (yaml `stuck_reason`), `BatchesDone int` (yaml
  `batches_done`). `ParseOutcome(path string) (*Outcome, error)` —
  `KnownFields(true)`, vocabulary check on `Outcome`, `stuck` requires a non-empty
  `stuck_reason`; fail loud, never guessed. `ArchiveStaleOutcome(builderDir string,
  now func() time.Time) (archivedTo string, err error)`: when `outcome.yaml` exists,
  rename it in place to `outcome-<UTC compact timestamp>.yaml` (the discussion's
  archive-never-refuse decision — resume must not be blocked; the prior run's
  judgment stays auditable); absent file → `("", nil)`. Tests: parse accept/reject
  table; archive renames and preserves content; absent no-op; a second archive in the
  same second does not clobber (append a numeric suffix on collision).
- **Commit:** `feat(builder): outcome.yaml contract with stale-file archiving`

### Card 25: engine Run

- **Context:**
  - `internal/lock/lock.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/spec.go`
  - `internal/perchengine/run.go`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/builderengine/runlevel.go`
  - `internal/builderengine/runlevel_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `Run(deps RunDeps, opts RunOptions) (RunResult, error)` in
  `runlevel.go` (named to avoid colliding with the poll/spawn files). `RunDeps`: a
  `BlockingRunner` seam (`Run(shuttleengine.Spec) (shuttleengine.Result, error)`
  satisfied by `*shuttleengine.Runner`), plan dir, builder dir, reports dir, worktree
  root, `Config`, resolved roles. `RunOptions`: `Fresh bool`. Sequence, each step a
  named, testable failure: (1) acquire `lock.TryAcquireWriteLock` on `run.lock` in
  builderDir — busy → exported sentinel `ErrRunBusy` (fail fast, perch's
  ErrBlockBusy pattern; the losing call must not touch state); release via defer;
  (2) `ClearPause` (the never-instantly-re-pause rule); (3) `ParsePlan` +
  `Validate(plan, worktreeRoot, caps)` with caps from Config and the deps' worktree
  root — any finding refuses the run (the automatic gate half of the discussion's
  validate-both decision); (4) `Fingerprint` vs `LoadState`: state
  exists and fingerprints differ → `ErrFingerprintMismatch` (exported sentinel; the
  error text names both fingerprints and instructs `run --fresh`) UNLESS
  `opts.Fresh`, which archives the stale state (rename `state.json` →
  `state-<timestamp>.json`), clears the reports dir the same way, and re-inits; state
  absent → init fresh `State` with a new `RunGUID` and the fingerprint;
  (5) `ArchiveStaleOutcome`; (6) render `{{.batch_index}}` and `{{.progress}}` from
  the plan + state (resume-on-files: reports present are summarized done; the
  always-fresh-orchestrator decision — never a session resume), fill
  `OrchestratorTemplate()`, build the Spec: `OutputFiles = [outcome path]`,
  orchestrator role's modelspec mapping, `Interactive = false`,
  `Timeout = cfg.OrchestratorTimeoutMin` minutes; (7) `deps.Runner.Run` (blocking);
  (8) map the shuttle Result per the discussion's distinct-envelope decision:
  `OutcomeDone` → `ParseOutcome` (a parse failure here is the reserved malformed-file
  error) → `RunResult{Outcome, StuckReason, BatchesDone}`; `asking`/`died`/`timeout`
  → three DISTINCT wrapped errors, each carrying SessionID and the kept RunDir, and
  for asking the LastAssistantMessage — never entering the outcome-file parse.
  (9) terminal outcome (done or stuck, not paused) → `ClearPause`. `RunResult` also
  carries `SessionID` and `RunDir` for the CLI envelope. Tests with a fake
  BlockingRunner: lock busy; validation refusal; fingerprint mismatch + `--fresh`
  archive/re-init; fresh init; outcome mapping for all four shuttle outcomes;
  progress rendering for a partially-reported state.
- **Commit:** `feat(builder): engine Run with lock, gates, and outcome mapping`

## Batch Tests

`verify:` runs the builderengine suite; this batch adds orchestrator template property
tests, the outcome parse/archive table, and the fake-runner Run sequence tests.
