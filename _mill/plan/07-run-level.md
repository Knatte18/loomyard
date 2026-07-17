# Batch: run-level

```yaml
task: 'Master Builder: new, parallel fork-based implementation module'
batch: 'run-level'
number: 7
cards: 5
verify: go test ./internal/websterengine/...
depends-on: [6]
```

## Batch Scope

The product verb's engine: `Run` takes the run-level lock, gates, reclaims,
archives, spawns the Master session (fork-authorized, both output files), and
blocks to a terminal outcome — plus the summary artifact's parser/archiver and
the run-exit whole-session audit cross-check. Mirrors builder's `runlevel.go`
discipline with webster's mechanism between the boundaries. External
interface: `Run`/`RunDeps`/`RunOptions`/`RunResult`, `ParseSummary`/
`ArchiveStaleSummary`, and the sentinel/typed errors webstercli maps to
envelopes.

## Cards

### Card 28: summary artifact

- **Context:**
  - `internal/builderengine/outcome.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/summary.go`
  - `internal/websterengine/summary_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `const SummaryFileName = "summary.md"`;
  `SummaryPath(websterDir string) string`.
  `type Summary struct { Title string; Body string }`.
  `ParseSummary(path string) (*Summary, error)` — fail-loud minimal
  validation per discussion.md `summary-artifact`: file must exist, be
  non-empty, and its first non-blank line must be a `# <title>` heading with
  non-empty title; everything after is the free-form narrative (never
  schema-validated — the consumer is PR prose).
  `ArchiveStaleSummary(websterDir string, now func() time.Time) (string, error)`
  — timestamp-rename with `builderengine.FirstFreeArchivePath` collision
  handling, absent-file → no-op (mirror `ArchiveStaleOutcome`'s shape).
  Tests: valid summary parses (title + body); missing file, empty file, no
  heading first line, empty title → distinct loud errors; archive renames and
  collision-suffixes.
- **Commit:** `webster: summary.md artifact parse and archive`

### Card 29: Run gates, reclaim, and Master spawn

- **Context:**
  - `internal/builderengine/runlevel.go`
  - `internal/builderengine/spawn.go`
  - `internal/builderengine/plan.go`
  - `internal/builderengine/validate.go`
  - `internal/builderengine/fingerprint.go`
  - `internal/builderengine/pause.go`
  - `internal/builderengine/outcome.go`
  - `internal/lock/lock.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/render.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/runlevel.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `type MasterStarter interface` +
  `MasterHandle { StrandGUID() string; Wait() (shuttleengine.Result, error) }`
  (builder's `OrchestratorStarter`/`OrchestratorHandle` shape, webster-named,
  satisfied by an adapter over `*shuttleengine.Runner` in webstercli).
  `RunDeps`: `Starter MasterStarter`, `Mux shuttleengine.MuxOps`,
  `Engine shuttleengine.Engine`, `ShuttleCfg shuttleengine.Config`,
  `Layout *hubgeometry.Layout`, `Roles`, `Config`,
  `PlanDir, WebsterDir, ReportsDir, PromptsDir, WorktreeRoot string`.
  `RunOptions{Fresh bool}`. `Run(deps, opts) (RunResult, error)` sequence:
  (1) `lock.TryAcquireWriteLock` on `run.lock` in `WebsterDir` for the WHOLE
  run — held busy → webster-local `ErrRunBusy` (caller touched nothing:
  webstercli skips its weft backstop on it);
  (2) validation gate: `builderengine.ParsePlan` + `builderengine.Validate`
  with caps from `Config`; any finding refuses loud; ADDITIONALLY refuse a
  plan with zero batches (`nothing-to-build is a malformed plan, never a
  vacuous done` — webster's own pre-flight, per discussion.md
  `run-verb-shape`);
  (3) state phase under the mutate lease: load state; entry-time reclaim —
  `builderengine.RemoveStrandIfLive` on a recorded `MasterStrand` and on
  every non-terminal `Kind: "recovery"` batch's `StrandGUID` (forks die with
  the Master; these two are the only reclaimable substrates, per
  discussion.md `crash-resume-re-drive-first-unreported`);
  (4) fingerprint: recompute vs `PlanFingerprint` — mismatch without `Fresh`
  → `ErrFingerprintMismatch` (pause left intact); with `Fresh` →
  `builderengine.ArchiveStateFile(WebsterDir, now)` +
  `builderengine.ArchiveReportsDir(ReportsDir, now)` + recreate reports dir +
  remove the prompts dir's rendered files (re-renderable, never archived) +
  fresh `RunGUID` + re-init state with the new fingerprint;
  (5) committed to spawning: `builderengine.ClearPause(WebsterDir)`;
  `ArchiveStaleOutcome(WebsterDir, now)` + `ArchiveStaleSummary(WebsterDir, now)`;
  (6) render the master prompt (`RenderMasterPrompt`); Spec:
  `OutputFiles: [outcomePath, summaryPath]` (both — shuttle classifies done
  only when the contract files land), `ForkSubagents: true`,
  `Model/Effort/Version` from `Roles[RoleMaster]`, `Role: "master"`,
  `Timeout: MasterTimeoutMin` minutes;
  (7) `Starter.StartMaster(spec)` → record `MasterStrand` from the handle,
  `MasterSessionID` via `shuttleengine.FindRun`, and
  `AssertedModel = Roles[RoleMaster].Model` (the launch model — the
  idempotent-assertion baseline); save state, release the lease (the Master's
  own verb calls need it free), THEN block on `handle.Wait()`.
- **Commit:** `webster: Run gates, reclaim, fresh archiving, and Master spawn`

### Card 30: outcome handling and run-exit audit cross-check

- **Context:**
  - `internal/builderengine/outcome.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/summary.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/forkaudit.go`
- **Edits:**
  - `internal/websterengine/runlevel.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Map the Master spawn's outcome exactly as builder maps its
  orchestrator's: `asking`/`died`/`timeout` → webster-local
  `MasterAskingError`/`MasterDiedError`/`MasterTimeoutError` (SessionID + kept
  RunDir; asking also carries LastAssistantMessage) — these never reach the
  outcome-file parse (the two failure classes are never conflated). `done` →
  `builderengine.ParseOutcome` (strict) + `ParseSummary` REQUIRED when
  `outcome: done` (missing/invalid summary = hard error; optional on
  `stuck`/`paused`) + the RUN-EXIT AUDIT CROSS-CHECK: `Result.ForkAudit` is a
  pointer populated only on fork-authorized done runs — a NIL `ForkAudit` on
  a `done` run of the Master's `ForkSubagents: true` spec is itself a hard
  error (the audit could not complete; fail-loud, never skipped); otherwise
  run `CheckParent` and `CheckFork` over it (the whole-session backstop
  behind the per-batch incremental audits) and verify the audited fork-transcript count
  is >= the number of begun batches with `Kind: "fork"` — a shortfall means a
  batch was recorded without its fork surviving audit; violations are hard
  errors carried on the run error, the outcome stays on disk for diagnosis.
  `paused` → `RunResult{Outcome: "paused"}` with the pause flag left as the
  operator's record; every non-`paused` terminal → `builderengine.ClearPause`.
  `RunResult{Outcome, StuckReason, BatchesDone, SummaryTitle}`.
- **Commit:** `webster: outcome, summary gate, and run-exit audit cross-check`

### Card 31: run-level tests

- **Context:**
  - `internal/websterengine/runlevel.go`
  - `internal/builderengine/runlevel.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/runlevel_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Fake `MasterStarter`/handle (scripted `Result` incl.
  `ForkAudit`), fake mux, temp dirs with seed plan files. Cover: `ErrRunBusy`
  when the lock is held; zero-batch plan refused loud; fingerprint mismatch
  without `--fresh` refused with pause flag left intact; `--fresh` archives
  state + reports (files exist under timestamped names) and clears rendered
  prompts; entry reclaim stops a recorded live master strand and a live
  non-terminal recovery strand (fake mux records removals) but not a
  cleanly-absent one; stale outcome AND summary archived before spawn;
  `AssertedModel` initialized to the master role's model; done outcome with
  valid summary + clean audit → `RunResult` populated; done with missing
  summary → hard error; done with a parent-write violation in `ForkAudit` →
  hard error; asking/died/timeout → the typed errors with kept RunDir;
  paused → flag intact; done → flag cleared. Integration-tagged, reusing the
  package `testmain_test.go`.
- **Commit:** `webster: run-level tests`

### Card 32: pause and validate pass-throughs

- **Context:**
  - `internal/builderengine/pause.go`
  - `internal/builderengine/validate.go`
  - `internal/websterengine/runlevel.go`
- **Edits:**
  - `internal/websterengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** No new pause/validate engine code — webster reuses
  `builderengine.RequestPause`/`PauseRequested`/`ClearPause` against
  `WebsterDir` and `builderengine.ParsePlan`+`Validate` directly (the cli
  wires them in batch 8). This card makes that explicit where the next reader
  looks: extend `doc.go`'s design statement with the reuse inventory (which
  builderengine functions webster calls and why there is no webster-side
  duplicate), including the pause-flag mechanics (checked at the `begin-batch`
  boundary, cleared at run commitment and non-paused terminals) and the
  zero-batch refusal.
- **Commit:** `webster: document the builderengine reuse inventory`

## Batch Tests

`go test ./internal/websterengine/...` — run-level tests are the heaviest in
the module (lock, archive, reclaim, outcome mapping, audit cross-check), all
against fakes and temp dirs, integration-tagged where git/fixture-backed under
the hermetic `TestMain`.
