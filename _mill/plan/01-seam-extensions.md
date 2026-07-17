# Batch: seam-extensions

```yaml
task: 'Master Builder: new, parallel fork-based implementation module'
batch: 'seam-extensions'
number: 1
cards: 5
verify: go test ./internal/shuttleengine/... ./internal/builderengine/...
depends-on: []
```

## Batch Scope

Everything webster needs from the layers below it, landed first so the new
module builds against a stable seam: (a) richer fork-audit facts — parent-session
Write/Edit calls **with file paths** and parent-session Bash commands, both
provider-invariant fields on `shuttleengine.ForkAudit`; (b) an incremental audit
entry point on the `Engine` seam so a long-lived caller can audit only fork
transcripts it has not seen yet; (c) the `/model` switch choreography as a new
`Engine` seam method plus a generic `Runner.Inject` player that delivers
`[]PaneInput` to a live strand's pane without the asking-pane readiness guard;
(d) exporting four already-generic builderengine helpers webster will import.
External interface consumed by later batches: `shuttleengine.Engine` (two new
methods), `shuttleengine.ForkAudit` (three new fields), `(*Runner).Inject`,
and `builderengine.ArchiveStateFile`/`ArchiveReportsDir`/`FirstFreeArchivePath`/
`RemoveStrandIfLive`.

## Cards

### Card 1: Parent-session facts on ForkAudit

- **Context:**
  - `internal/shuttleengine/wait.go`
- **Edits:**
  - `internal/shuttleengine/forkaudit.go`
  - `internal/shuttleengine/claudeengine/audit.go`
- **Creates:**
  - `internal/shuttleengine/claudeengine/audit_parentfacts_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three provider-invariant fields to
  `shuttleengine.ForkAudit`: `ParentWriteCalls int`, `ParentWrites []string`
  (file paths of every parent-session `Write`/`Edit`/`NotebookEdit` tool_use
  block, in transcript order; path read from the block's `file_path` input key,
  falling back to `notebook_path` for `NotebookEdit`), and
  `ParentBashCommands []string` (verbatim `command` input of every parent
  `Bash` tool_use block). Extend `auditParentTranscript` in
  `claudeengine/audit.go` to extract them (mirroring how fork parsing pulls
  `Input["command"]`), extend its return values, and update its single caller
  `AuditForks`. Document on each new field that interpreting the facts is the
  caller's policy, matching the existing `ForkAudit` doc posture. New test file
  covers: parent transcript with writes/bash → fields populated in order; a
  missing path input key does not panic (skipped with the write still
  counted); existing zero-fork and fork-fact behaviour unchanged.
- **Commit:** `shuttle: collect parent write/bash facts in ForkAudit`

### Card 2: Engine.AuditForksIncremental

- **Context:**
  - `internal/shuttleengine/forkaudit.go`
  - `internal/shuttleengine/wait.go`
- **Edits:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/claudeengine/audit.go`
  - `internal/shuttleengine/fakes_test.go`
- **Creates:**
  - `internal/shuttleengine/claudeengine/audit_incremental_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add to the `shuttleengine.Engine` interface:
  `AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (ForkAudit, error)`.
  Contract: parent facts (SpawnCalls, NamedSpawns, ParentWriteCalls,
  ParentWrites, ParentBashCommands) are always full/cumulative; `Forks`
  contains one `ForkReport` per subagent transcript whose `TranscriptPath` is
  NOT in `seenTranscripts` (nil map == report everything). Implement in
  `claudeengine/audit.go` by refactoring the body of `AuditForks` into the new
  method and re-expressing `AuditForks(sessionID, workdir)` as
  `AuditForksIncremental(sessionID, workdir, nil)` — one parsing path, no
  duplication. A missing `subagents/` dir stays a legitimate zero-fork result;
  a missing parent transcript stays a hard error. Update the engine fake in
  `shuttleengine/fakes_test.go` to satisfy the widened interface. New test
  file covers: seen-set filtering (only unseen transcripts reported), nil-map
  equivalence with `AuditForks`, parent facts unaffected by the seen set.
- **Commit:** `shuttle: add AuditForksIncremental to the Engine seam`

### Card 3: Engine.ModelSwitchSequence

- **Context:**
  - `internal/shuttleengine/claudeengine/claudeengine.go`
- **Edits:**
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/claudeengine/startup.go`
  - `internal/shuttleengine/fakes_test.go`
- **Creates:**
  - `internal/shuttleengine/claudeengine/modelswitch_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add to the `shuttleengine.Engine` interface:
  `ModelSwitchSequence(model string) []PaneInput` — the provider choreography
  that switches a live interactive session's model. Claude implementation in
  `claudeengine/startup.go`, mirroring `ComposeSend`:
  `[]PaneInput{{Key: "Escape", SettleMS: composeSendSettleMS}, {Text: "/model " + model, Submit: true}}`.
  The literal `/model` string appears ONLY in `claudeengine` (Provider-Seam
  Invariant — shuttleengine must not contain it). Update the engine fake in
  `fakes_test.go`. New test pins the exact sequence shape (Escape first with
  settle, then `/model <name>` submitted) and that the model string is passed
  through verbatim.
- **Commit:** `shuttle: add ModelSwitchSequence choreography to the Engine seam`

### Card 4: Runner.Inject

- **Context:**
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/mux.go`
  - `internal/shuttleengine/engine.go`
- **Edits:**
  - `internal/shuttleengine/run.go`
- **Creates:**
  - `internal/shuttleengine/run_inject_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (r *Runner) Inject(guid string, inputs []PaneInput) error`
  to `run.go`, following the out-of-process `Runner.Send(guid, ...)` shape:
  resolve the strand via `FindRun` (confirming it is a shuttle strand), require
  the strand live via the same liveness check `Send` uses, then play `inputs`
  through `playInputs`. Deliberately do NOT apply the ready/asking-pane guard
  (`requireReadyAgentPane`): `Inject` exists to reach a session that is
  mid-turn, busy executing a foreground tool subprocess — document this
  contrast with `Send` in the method's doc comment, including that delivery
  into a busy TUI is validated by webster's sandbox scenario rather than
  guarded here. Empty `inputs` is a no-op error. New test drives `Inject` with
  the existing fake mux/engine plumbing from `fakes_test.go`: happy path plays
  every input in order; dead strand errors; unknown guid errors.
- **Commit:** `shuttle: add Runner.Inject for mid-turn pane input delivery`

### Card 5: Export builderengine archive/strand helpers

- **Context:**
  - `internal/builderengine/outcome.go`
- **Edits:**
  - `internal/builderengine/runlevel.go`
  - `internal/builderengine/spawn.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename four unexported, already-generic helpers for
  import by webster, updating every builderengine call site:
  `archiveStateFile` → `ArchiveStateFile(dir string, now func() time.Time) (string, error)`,
  `archiveReportsDir` → `ArchiveReportsDir(reportsDir string, now func() time.Time) error`,
  `firstFreeArchivePath` → `FirstFreeArchivePath(candidate func(suffix string) string) (string, error)`,
  `removeStrandIfLive` → `RemoveStrandIfLive(mux shuttleengine.MuxOps, guid string) error`.
  No signature or behaviour changes — pure export renames plus godoc stating
  each is shared infrastructure with a second consumer (webster). Keep
  `archiveTimestampFormat` unexported (both callers live inside packages that
  can reach it through the exported functions).
- **Commit:** `builder: export archive and strand-reclaim helpers for webster`

## Batch Tests

`go test ./internal/shuttleengine/... ./internal/builderengine/...` — the two
packages this batch touches. New tests: parent-fact extraction, incremental
seen-set filtering, model-switch choreography shape, `Inject` liveness/ordering.
Existing suites guard the refactors (AuditForks delegation, export renames).
All new tests are untagged and spawn nothing (fixture transcripts are plain
files written with `os.WriteFile`; mux/engine are the package's existing
fakes) — Tier 1 clean.
