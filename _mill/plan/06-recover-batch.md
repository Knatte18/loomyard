# Batch: recover-batch

```yaml
task: 'Master Builder: new, parallel fork-based implementation module'
batch: 'recover-batch'
number: 6
cards: 3
verify: go test ./internal/websterengine/...
depends-on: [5]
```

## Batch Scope

The exception path: a re-entrant, bounded long-poll verb that escalates a
stuck/report-less batch to a cold implementer strand at the `recovery` model —
the only place webster spawns a separate process. First call spawns and
records; every call (first included) blocks at most one `poll_wait_s` window
and returns either the terminal digest or a running snapshot; the Master
re-calls until terminal. Reuses builder's classification machinery
(`Classify`/`PollUntilTerminal`/`TurnEnded`/`StrandLive`) and builder's
implementer template (a cold session needs cold orientation — exactly what
that template is). External interface: `RecoverBatch`/`RecoverResult` consumed
by webstercli (batch 8).

## Cards

### Card 25: RecoverBatch spawn-or-attach

- **Context:**
  - `internal/builderengine/spawn.go`
  - `internal/builderengine/template.go`
  - `internal/builderengine/implementer-template.md`
  - `internal/websterengine/state.go`
  - `internal/websterengine/roles.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir.go`
  - `internal/stencil/stencil.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/recoverbatch.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `type RecoverDeps struct`: `Starter builderengine.Starter`,
  `Plan`, `State`, `Roles map[Role]modelspec.Resolved`, `Config Config`,
  `Engine shuttleengine.Engine`, `Mux shuttleengine.MuxOps`,
  `ShuttleCfg shuttleengine.Config`, `Layout *hubgeometry.Layout`,
  `WorktreeRoot, WebsterDir, ReportsDir string`.
  `RecoverBatch(deps RecoverDeps, batchNumber int, wait time.Duration, clk Clock) (*RecoverResult, error)`
  where `Clock` is a webster-local `{Now() time.Time; Sleep(time.Duration)}`
  interface (structurally satisfies builderengine's unexported clock — the
  documented reuse path). Spawn-or-attach decision from state:
  `Batches[batchNumber]` with `Kind == "recovery"`, non-`Terminal`, and a
  recorded `StrandGUID` → ATTACH (skip spawn, go straight to the bounded
  wait). Otherwise SPAWN:
  (1) archive any stale report at
  `builderengine.BatchReportFileName(number, slug)` via a webster-local
  `archiveStaleReport` built on `builderengine.FirstFreeArchivePath`
  (archive-never-refuse — the stuck report is the recovery spawn's own output
  path);
  (2) stop any prior recorded strand for this batch still live
  (`builderengine.RemoveStrandIfLive`);
  (3) fill `builderengine.ImplementerTemplate()` via `stencil.Fill` with the
  batch's markers (`batch_file`, `batch_name`, `report_path`,
  `self_fix_cap`, `worktree_root`) — a cold recovery session gets builder's
  full implementer orientation, per discussion.md
  `single-model-forks-and-cold-recovery`;
  (4) build the `shuttleengine.Spec`: `Prompt` from the filled template,
  `OutputFiles: [reportPath]`, `Model/Effort/Version` from
  `deps.Roles[RoleRecovery]`, `Role: "recovery"`, `Round` = the batch name,
  `Timeout` = `RecoveryTimeoutMin` minutes; `Starter.Start`;
  (5) record via `shuttleengine.FindRun`: `BatchState{Slug, StartSHA (fresh
  HeadSHA), Kind: "recovery", SpawnedAt, StrandGUID, ShuttleRunDir,
  EventsPath}`, `CurrentBatch = batchNumber`.
  The caller (cli) holds the lease around the spawn-and-record phase and
  weft-commits state immediately after it — return a
  `Spawned bool` on the result so the cli knows this call spawned.
- **Commit:** `webster: RecoverBatch spawn-or-attach for cold recovery strands`

### Card 26: RecoverBatch bounded wait and terminal persistence

- **Context:**
  - `internal/builderengine/poll.go`
  - `internal/builderengine/digest.go`
  - `internal/builderengine/gitquery.go`
  - `internal/websterengine/state.go`
- **Edits:**
  - `internal/websterengine/recoverbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** After spawn-or-attach, the bounded wait: a `gather`
  closure assembling `builderengine.ClassifyInputs` (report path + parse via
  the report-presence branch of `Classify`; `TurnEnded(EventsPath,
  deps.Engine)`; `StrandLive(deps.Mux, StrandGUID)`; `Elapsed` from
  `SpawnedAt` — evaluated ACROSS re-entrant calls so `RecoveryTimeoutMin`
  measures since spawn, not since this call; `Changed`/`Dirty` via gitquery;
  `BatchTimeout` from `RecoveryTimeoutMin`), passed to
  `builderengine.PollUntilTerminal(gather, wait, clk)`. Non-terminal after
  `wait` → `RecoverResult{Running: true, ElapsedS}` — state untouched, caller
  re-calls. Terminal → persist exactly as `RecordBatch` does
  (`Digest`, `Terminal`, `Status`, clear `CurrentBatch`) and release the
  substrate with builder's parity rules: `done` → `RemoveStrandIfLive` +
  remove the run dir; `stuck` → remove strand, KEEP the run dir (the stuck
  trail); `dead` → keep both for diagnosis. Cleanup failures are logged
  warnings on the result, never fatal. Return
  `RecoverResult{Digest, Running, Spawned, ElapsedS, Warnings}`.
- **Commit:** `webster: bounded long-poll wait and terminal persistence for recovery`

### Card 27: RecoverBatch tests

- **Context:**
  - `internal/websterengine/recoverbatch.go`
  - `internal/builderengine/poll.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/recoverbatch_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Fake `Starter` (returns a scripted `*shuttleengine.Run` —
  reuse builderengine's established fake-starter approach), stub engine
  (scripted `ParseEvents`), fake mux (scripted `Status`), fake `Clock`
  (virtual time, no real sleeps). Cover: first call spawns (stale report
  archived — file renamed with timestamp suffix, prior live strand stopped),
  records strand fields, and with no report within the window returns
  `Running` with `Spawned: true`; second call ATTACHES (no second spawn —
  fake starter records call count) and, when the report appears, returns the
  terminal digest with state persisted and `done`-substrate released;
  timeout: virtual elapsed since `SpawnedAt` crossing `RecoveryTimeoutMin`
  across two calls classifies `dead`/`timeout`; unrecorded/terminal batch
  spawns fresh rather than attaching. Integration-tagged (gitquery), reusing
  the package `testmain_test.go`.
- **Commit:** `webster: RecoverBatch tests`

## Batch Tests

`go test ./internal/websterengine/...` (Tier 1), with the new tests in the
integration tier under the hermetic `TestMain`. The re-entrancy contract
(spawn-once, attach-thereafter, elapsed-across-calls) is the test centre —
it is what makes the bounded long-poll safe against the tool-call ceiling
(external review r2's finding).
