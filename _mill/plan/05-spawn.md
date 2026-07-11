# Batch: spawn

```yaml
task: "Build builder - the batch-implementation loop"
batch: "spawn"
number: 5
cards: 3
verify: go test ./internal/builderengine/...
depends-on: [2, 4]
```

## Batch Scope

The implementer side of the loop: the embedded implementer prompt template with its
property tests, and `SpawnBatch` — role selection from the batch's `oversized:` flag
(orchestrator overrides only for recovery), modelspec→shuttle mapping, start-SHA and
chain-anchor recording, pause gate, and the `--restart-chain` path. External interface
consumed later: `SpawnBatch`, `SpawnBatchOptions`.

## Cards

### Card 20: implementer prompt template

- **Context:**
  - `internal/burlerengine/template_test.go`
  - `internal/stencil/stencil.go`
  - `docs/modules/plan-format.md`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/builderengine/implementer-template.md`
- **Edits:**
  - `internal/builderengine/template.go`
  - `internal/builderengine/template_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `implementer-template.md` is the stencil-filled prompt one
  implementer session receives. Top-level markers (all caller-required, so all at
  template top level per `stencil.Fill`'s documented guarantee): `{{.batch_file}}`
  (absolute path to its `NN-<slug>.md`), `{{.batch_name}}` (`NN-<slug>`),
  `{{.report_path}}` (absolute batch-report path), `{{.self_fix_cap}}`,
  `{{.worktree_root}}`. The prompt instructs, in plan-format.md's exact vocabulary:
  read the batch file and only it (plus its Scope/Where files); implement cards in
  order; commit per card to the HOST repo with subject `NN.C: <short what>`; run the
  batch's `## verify:` command; after a red verify, at most `{{.self_fix_cap}}`
  in-session fix attempts, then write `status: stuck` with a `stuck_reason` naming
  BOTH the blocker and what was attempted; report `tests: skipped` when the batch
  frontmatter says `verify: deferred`; justify every out-of-scope edit in the
  report's `out_of_scope` field; NEVER run git against the weft repo or write outside
  the worktree; the FINAL action is writing the batch-report YAML (exact schema quoted
  from plan-format.md) to `{{.report_path}}`. Extend `template.go` with
  `//go:embed implementer-template.md` + `ImplementerTemplate() []byte`. Extend
  `template_test.go` with property tests (burler's `TestTemplate_StatesRoundDiscipline`
  pattern): stencil-fills clean with sample values; states commit-per-card with the
  `NN.C:` subject shape; states the bounded self-fix cap; states report-as-final-action
  and quotes the exact report schema keys; forbids weft git.
- **Commit:** `feat(builder): embedded implementer prompt template`

### Card 21: SpawnBatch

- **Context:**
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/stencil/stencil.go`
  - `internal/modelspec/modelspec.go`
  - `_mill/discussion.md`
  - `docs/modules/plan-format.md`
- **Creates:**
  - `internal/builderengine/spawn.go`
  - `internal/builderengine/spawn_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `SpawnBatch(deps SpawnDeps, opts SpawnBatchOptions) (*SpawnResult,
  error)`. `SpawnDeps` carries the seams so tests fake them: `Starter` (interface with
  `Start(shuttleengine.Spec) (*shuttleengine.Run, error)` satisfied by
  `*shuttleengine.Runner`), the parsed `*Plan`, `*State`, resolved roles
  `map[Role]modelspec.Resolved`, `Config`, `worktreeRoot`, `builderDir`, `reportsDir`,
  plus `ShuttleCfg shuttleengine.Config` and `Layout *hubgeometry.Layout` — the pair
  `shuttleengine.FindRun(cfg, layout, guid) (RunState, string, error)` needs to
  resolve the just-started run's identity cross-process (`*shuttleengine.Run` exposes
  only `StrandGUID()`; there is no run-dir accessor).
  `SpawnBatchOptions`: `BatchNumber int`, `RoleOverride Role` (empty or
  `RoleRecovery` only — any other override is an error), `RestartChain bool`.
  Sequence, per the discussion: (1) pause gate — `PauseRequested(builderDir)` →
  sentinel error `ErrPaused` (exported, `errors.Is`-matchable); (2) role selection —
  batch's `Oversized` flag picks `RoleImplementer` vs `RoleImplementerOversized`;
  `RoleOverride == RoleRecovery` wins (Go picks from the batch, the orchestrator
  overrides only for recovery); (3) `RestartChain` → resolve the batch's chain via
  `ChainEndFor`, call `RestartChain` (error if the batch is chainless); (4) capture
  `HeadSHA` as the batch start-SHA; if the batch is a chain member and
  `st.ChainStartSHAs[chainEnd]` is unset, record the anchor = this HEAD (the host
  commit immediately before the lowest member's first card commit); (5) fill
  `ImplementerTemplate()` via `stencil.Fill`; (6) build `shuttleengine.Spec`: Prompt,
  `OutputFiles = [reportPath]`, the modelspec mapping (`Model = resolved.Model`,
  `Effort = resolved.Params["effort"]`, `Version = resolved.Params["version"]`),
  `Role = string(role)`, `Round = batch name`, `Timeout =
  cfg.BatchTimeoutMin` minutes, `KeepPane = true` (poll owns cleanup semantics;
  panes/run dirs are kept for diagnosis on dead outcomes); (7) `deps.Starter.Start` —
  NOT `Run`: spawn-batch returns immediately (the tool-call cap is poll's problem);
  (8) resolve the run via `shuttleengine.FindRun(deps.ShuttleCfg, deps.Layout,
  run.StrandGUID())` — the returned `(RunState, runDir)` supply the run dir and
  `RunState.EventsPath`; record `BatchState` (slug, start-SHA, role, the strand guid,
  run dir, events path, SpawnedAt) + `CurrentBatch`, `SaveState`. A pre-existing report file for
  the batch is an error before any spawn (shuttle's OutputFiles rule would reject it
  anyway — surface it as builder's own named error first). Tests with a fake Starter:
  role selection matrix (plain, oversized, recovery override, invalid override),
  pause sentinel, chain-anchor recorded once, state persisted, spec fields mapped per
  the modelspec package-doc mapping, stale-report refusal.
- **Commit:** `feat(builder): SpawnBatch with role selection and chain anchors`

### Card 22: spawn-side weft note and state commit boundary

- **Context:**
  - `internal/perchcli/run.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/builderengine/spawn.go`
  - `internal/builderengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Document (godoc on `SpawnBatch` + the package doc's weft section)
  the commit boundary the discussion pinned: the CLI verb weft-commits `state.json`
  after a successful spawn (start-SHA recorded), `poll` weft-commits report + state at
  terminal classification, `run` backstop-commits at exit; builderengine itself never
  touches weft (weft-blind engine, perchcli precedent). Ensure `SpawnResult` exposes
  what the CLI needs to do that without re-deriving: `BatchName string`,
  `Role Role`, `StartSHA string`, `StrandGUID string`, `RunDir string`,
  `ReportPath string`. No behavioural change beyond the result struct — this card is
  the seam-tightening pass that keeps batch 7's CLI thin.
- **Commit:** `feat(builder): pin spawn result seam and weft commit boundary`

## Batch Tests

`verify:` runs the builderengine suite; this batch adds template property tests and the
fake-Starter SpawnBatch matrix.
