# Batch: webstercli-registration

```yaml
task: 'Master Builder: new, parallel fork-based implementation module'
batch: 'webstercli-registration'
number: 8
cards: 7
verify: go test ./internal/webstercli/... ./cmd/lyx/...
depends-on: [7]
```

## Batch Scope

The fat verbs: `internal/webstercli` wires the engine into cobra behind the
`Command()`/`RunCLI` seam, owns all four weft-commit points, and registers the
module in `cmd/lyx` (import + AddCommand + root Long + the two hardcoded
helptree pins). Mirrors buildercli file-for-file. External interface:
`webstercli.Command()`/`RunCLI` consumed by `cmd/lyx` and the sandbox suite.

## Cards

### Card 33: cli receiver and resolution seam

- **Context:**
  - `internal/buildercli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/websterengine/config.go`
  - `internal/websterengine/roles.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/webstercli/cli.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `websterCLI` receiver + `Command() *cobra.Command`
  (parent `webster`, `RunE = clihelp.GroupRunE`, non-empty `Short`, a `Long`
  with concrete examples naming all seven subcommands) +
  `RunCLI(out io.Writer, args []string) int = clihelp.Execute(Command(), out, args)`.
  `PersistentPreRunE` mirrors buildercli's exact resolution order
  (`hubgeometry.Getwd` → `Resolve` → `shuttleengine.LoadConfig(layout.Cwd, "shuttle")`
  → `muxengine.LoadConfig(layout.Cwd, "mux")` →
  `websterengine.LoadConfig(layout.Cwd, "webster")` →
  `modelspec.LoadRegistry(layout.Cwd)` → `websterengine.ResolveRoles` →
  `muxengine.New` → `claudeengine.New()` → `shuttleengine.NewRunner`), skipped
  for the bare parent command, storing on the receiver: runner (as
  `builderengine.Starter`, as `websterengine.Injector`, and behind a
  `runnerMasterStarter` adapter satisfying `websterengine.MasterStarter`),
  engine, mux, layout, shuttleCfg, cfg, roles, and the dirs —
  `planDir = hubgeometry.PlanDir(layout.Cwd)`,
  `websterDir = hubgeometry.WebsterDir(layout.Cwd)`,
  `reportsDir = hubgeometry.WebsterReportsDir(layout.Cwd)`,
  `promptsDir = hubgeometry.WebsterPromptsDir(layout.Cwd)` — all anchored at
  `layout.Cwd`, never `WorktreeRoot` (the weft-junction rule buildercli
  documents). Fields unexported; tests populate them directly.
- **Commit:** `webster: webstercli receiver and resolution seam`

### Card 34: weft commit helper

- **Context:**
  - `internal/buildercli/weft.go`
  - `internal/weftengine/sync.go`
  - `internal/weftengine/weft.go`
  - `internal/builderengine/pause.go`
- **Edits:** none
- **Creates:**
  - `internal/webstercli/weft.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `websterWeftPathspec(layout *hubgeometry.Layout) []string`
  = `weftengine.ScopedPathspec(layout.RelPath, []string{hubgeometry.LyxDirName})`
  plus `:(exclude)*.lock`, `:(exclude)*/webster/` + `builderengine.PauseFlagName`,
  and `:(exclude)*/webster/prompts/*` (rendered fork prompts are machine-local
  re-renderable artifacts — committing them would be weft noise and a
  cross-machine confusion, same class as the pause flag).
  `weftCommit(layout, label) (bool, error)` calling `weftengine.Commit` with
  message prefix `"webster: "` + `weftengine.Push`, `EnvSyncOptions()` —
  buildercli's shape verbatim. Copy buildercli's doc rationale (why excludes,
  why cli-layer-only) adapted to webster.
- **Commit:** `webster: weft commit helper with prompts/lock/pause excludes`

### Card 35: validate, status, pause verbs

- **Context:**
  - `internal/buildercli/validate.go`
  - `internal/buildercli/status.go`
  - `internal/buildercli/pause.go`
  - `internal/websterengine/state.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/webstercli/validate.go`
  - `internal/webstercli/status.go`
  - `internal/webstercli/pause.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `validate`: `builderengine.ParsePlan` + `Validate` with
  webster's caps → findings envelope (buildercli's shape) plus webster's
  zero-batch finding. `status`: side-effect-free snapshot — state.json +
  reports dir summary; never-started → `{"initialized": false}`; per-batch
  rows include `kind`, `status`, `terminal`, and whether a digest is
  persisted; never spawns, never weft-commits (builder-parity per
  discussion.md `run-verb-shape`). `pause`:
  `builderengine.RequestPause(websterDir)` → `{"paused": true}` envelope,
  idempotent. All errors via `output.Err`. Non-empty `Short` on each; `Long`
  with examples on `status` and `validate`.
- **Commit:** `webster: validate, status, and pause verbs`

### Card 36: begin-batch and record-batch verbs

- **Context:**
  - `internal/buildercli/spawnbatch.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/audit.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/webstercli/cli.go`
- **Creates:**
  - `internal/webstercli/beginbatch.go`
  - `internal/webstercli/recordbatch.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `begin-batch <NN> [--restart-chain]`: under
  `websterengine.AcquireStateMutation` — load state, `BeginBatch`, save state
  — release; then `weftCommit(layout, "begin-batch <BatchName>")`. Envelope:
  `{batch, prompt_path, start_sha, model}` (the prompt-file path is what the
  Master forwards to its fork verbatim). `ErrPaused` → the
  `{"paused": true}` refusal envelope (an operational signal, exit 0 —
  buildercli's `pausedEnvelope` pattern); `ErrFingerprintMismatch` → loud
  error naming `run --fresh`. `record-batch <NN>`: under the lease — load,
  `RecordBatch` (real `time.Sleep`-backed sleeper), save — release; then
  `weftCommit(layout, "record-batch <BatchName> <status>")`; envelope is the
  DIGEST verbatim (the pinned terse field set — the Master reads only this)
  plus `warnings`; `NoReport` → `{"no_report": true, batch}` envelope, exit 0
  (a ladder signal, not an error — Master re-forks once);
  `ErrNoBeginRecord` and audit violations → loud `output.Err`. Register both
  in `Command()`.
- **Commit:** `webster: begin-batch and record-batch bracket verbs`

### Card 37: run and recover-batch verbs

- **Context:**
  - `internal/buildercli/run.go`
  - `internal/buildercli/poll.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/webstercli/cli.go`
- **Creates:**
  - `internal/webstercli/run.go`
  - `internal/webstercli/recoverbatch.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `run [--fresh]`: `websterengine.Run` with the receiver's
  deps; exit-time weft backstop `weftCommit(layout, "run <outcome>")` on
  every path EXCEPT `ErrRunBusy` (the loser touched nothing); typed master
  errors and outcome results mapped to envelopes exactly as buildercli's run
  maps builder's. `recover-batch <NN> [--wait DURATION]` (default
  `poll_wait_s`): real-clock call to `RecoverBatch`; when the result carries
  `Spawned` → `weftCommit(layout, "recover-batch <BatchName> spawn")`
  immediately after the spawn-and-record phase; terminal →
  `weftCommit(layout, "recover-batch <BatchName> <status>")` and emit the
  digest envelope; `Running` → `{batch, status: "running", elapsed_s}`
  envelope touching neither git nor weft (the Master re-calls). Register
  both in `Command()`.
- **Commit:** `webster: run and recover-batch verbs`

### Card 38: cmd/lyx registration

- **Context:**
  - `internal/webstercli/cli.go`
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/longlist_test.go`
  - `cmd/lyx/drift_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:**
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `main.go`: import `internal/webstercli`, add
  `webstercli.Command()` to `root.AddCommand`, add `webster` to the root
  `Long` module list. In `helptree_test.go`: add `"webster"` to the
  `requiredModules` slice and a table entry naming all seven subcommands
  (`validate`, `run`, `status`, `pause`, `begin-batch`, `record-batch`,
  `recover-batch`). The registration/longlist/drift guards then pass without
  edits (they derive from the live tree). The sandbox-coverage guard
  (`TestSandboxCoverage_AllModulesCoveredOrExcluded`) fires the moment the
  module registers — so this card ALSO creates
  `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` as a MINIMAL stub in the existing
  suites' grammar (copied from `SANDBOX-BUILDER-SUITE.md`'s shape): the suite
  header plus one scenario shell `### W1 -- placeholder (authored in the
  sandbox-and-docs batch)` carrying the load-bearing `**Covers:** webster`
  line and placeholder `**Goal:**`/`**Watch:**`/`**Verdict:**` fields. The
  full W1/W2 scenario content is authored by card 40 (batch 9), which owns
  the scenario prose — this card only satisfies the coverage guard in the
  same commit that registers the module so `./cmd/lyx/...` stays green.
- **Commit:** `lyx: register the webster module with sandbox suite coverage`

### Card 39: cli tests

- **Context:**
  - `internal/buildercli/cli.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/weft.go`
- **Edits:** none
- **Creates:**
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/webstercli/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Drive everything through the `RunCLI` seam with fakes
  injected on the receiver (buildercli's documented pattern). `cli_test.go`
  (untagged, spawn-free): help tree — every subcommand present with non-empty
  `Short`; unknown subcommand rejected via `GroupRunE`; JSON envelope shape
  on validate/status/pause against temp dirs.
  `verbs_test.go` (integration-tagged where git-backed, with the package's
  hermetic `testmain_test.go`): begin-batch happy path (envelope carries
  prompt_path; state saved); paused refusal envelope; record-batch digest
  envelope and no_report envelope; recover-batch running vs terminal
  envelopes; run's `ErrRunBusy` skips the weft backstop (fake/recording weft
  seam or assert via `WEFT_SKIP_GIT` env per `weftengine.EnvSyncOptions`);
  `websterWeftPathspec` excludes `*.lock`, the pause flag, and
  `*/webster/prompts/*` (direct unit assertions on the returned pathspec).
- **Commit:** `webster: webstercli tests through the RunCLI seam`

## Batch Tests

`go test ./internal/webstercli/... ./cmd/lyx/...` — the cli package plus every
cmd/lyx guard this registration must satisfy: drift (Short on every command),
helptree (the two pins card 38 edits), registration (exists ⇒ registered),
longlist (named in root Long), and sandbox coverage (satisfied in the same
commit by card 38's suite file with `**Covers:** webster`). The cli tests run
untagged where spawn-free and integration-tagged where git-backed, under the
package's hermetic `TestMain`.
