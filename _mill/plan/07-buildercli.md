# Batch: buildercli

```yaml
task: "Build builder - the batch-implementation loop"
batch: "buildercli"
number: 7
cards: 4
verify: go test ./internal/buildercli/... ./internal/builderengine/...
depends-on: [6]
```

## Batch Scope

The cobra subtree: `lyx builder run | spawn-batch | poll | status | validate | pause`,
wired through the perchcli-shaped `builderCLI` receiver (PersistentPreRunE resolving
layout → configs → registry → engines → runner), every result and error on the JSON
envelope, weft commits at the pinned boundaries. Registration in `cmd/lyx` is batch 8.
External interface: `buildercli.Command()` / `RunCLI`.

## Cards

### Card 26: builderCLI receiver and command tree

- **Context:**
  - `internal/perchcli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/modelspec/load.go`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/buildercli/cli.go`
  - `internal/buildercli/cli_test.go`
  - `internal/buildercli/weft.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Mirror `perchcli/cli.go` exactly in shape: `builderCLI` struct
  holding resolved ingredients (`runner *shuttleengine.Runner`,
  `layout *hubgeometry.Layout`, `cfg builderengine.Config`,
  `roles map[builderengine.Role]modelspec.Resolved`, and the derived dirs `planDir`,
  `builderDir`, `reportsDir` — all anchored at `layout.Cwd` via
  `hubgeometry.PlanDir/BuilderDir/BuilderReportsDir`, matching perchcli's
  Cwd-anchoring rationale comment). `Command()` builds the parent `builder` command
  (`RunE: clihelp.GroupRunE`, the group-name guard skipping resolution for the bare
  group, `Short` + `Long` with concrete examples naming all six verbs) and a
  `PersistentPreRunE` resolving cwd → layout → shuttle cfg → mux cfg → builder cfg
  (`builderengine.LoadConfig(layout.Cwd, "builder")`) → `modelspec.LoadRegistry
  (layout.Cwd)` → `builderengine.ResolveRoles` (the fail-pre-flight surface: a typo'd
  role alias aborts every verb here) → mux engine → claude engine → Runner.
  `RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out,
  args)`. `weft.go`: one package-local helper `weftCommit(layout, label string)
  (bool, error)` wrapping `weftengine.Commit` + `Push` with
  `weftengine.ScopedPathspec(layout.RelPath, []string{hubgeometry.LyxDirName})` +
  `:(exclude)*.lock` and `weftengine.EnvSyncOptions()` — copied from perchcli's
  block-exit sync including its lock-exclusion rationale comment; commit message
  `builder: <label>`. Tests: `RunCLI` group listing works without a git repo; unknown
  subcommand yields the JSON error envelope.
- **Commit:** `feat(builder): buildercli command tree and weft helper`

### Card 27: validate and status verbs

- **Context:**
  - `internal/perchcli/run.go`
  - `internal/output/output.go`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/buildercli/validate.go`
  - `internal/buildercli/validate_test.go`
  - `internal/buildercli/status.go`
  - `internal/buildercli/status_test.go`
- **Edits:**
  - `internal/buildercli/cli.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `validate` (the standalone pre-flight half of the
  validate-both decision): `ParsePlan` + `Validate` with caps from cfg; zero findings
  → `output.Ok` with `{"valid": true, "batches": <n>}`; findings → `output.Err`
  carrying the findings array (check, batch, detail per entry) — exit non-zero, one
  JSON object, never plain text. `status` (instant snapshot, human- and loom-facing —
  the discussion's navigation refresher): `LoadState` + scan reports dir; `output.Ok`
  with `{"run_guid", "current_batch", "plan_fingerprint", "batches": [{number, slug,
  status, role, start_sha, terminal}...], "paused": <bool>}`; absent state →
  `{"initialized": false}`. Both verbs carry `Short` + example-bearing `Long`.
  Register both on the parent in `cli.go`. Tests drive `RunCLI` against a scratch
  worktree (`lyxtest.SeedConfig` + fixture plan copied under
  `hubgeometry.PlanDir(tmp)`) asserting envelope shapes for the valid, invalid,
  initialized, and uninitialized cases.
- **Commit:** `feat(builder): validate and status verbs`

### Card 28: spawn-batch and poll verbs

- **Context:**
  - `internal/perchcli/run.go`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/buildercli/spawnbatch.go`
  - `internal/buildercli/spawnbatch_test.go`
  - `internal/buildercli/poll.go`
  - `internal/buildercli/poll_test.go`
- **Edits:**
  - `internal/buildercli/cli.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `spawn-batch <NN>`: positional batch number; flags
  `--role recovery` (string; only `recovery` accepted — reject others in the verb
  before touching the engine) and `--restart-chain` (bool). Gate: `ParsePlan` +
  `Validate` refuse on findings (the automatic-gate half — same refusal envelope as
  the validate verb). Call `builderengine.SpawnBatch`; `ErrPaused` → `output.Err`
  with a `"paused": true` field (the orchestrator's pause signal); success →
  weft-commit state (`weftCommit(layout, "spawn-batch <NN>")`) then `output.Ok` with
  the `SpawnResult` fields (batch_name, role, start_sha, strand_guid, run_dir,
  report_path). `poll`: flag `--wait <duration>` (default from
  `cfg.PollWaitS` seconds); assembles the gather closure from state's current batch
  (error envelope when nothing is in flight), wiring `Classify`'s inputs —
  report parse, `turnEnded` from the recorded shuttle run dir, `strandLive` from
  `layout` state, elapsed from `SpawnedAt`, diff/dirty via the gitquery helpers —
  and calls `PollUntilTerminal`. Terminal digest → mark the batch terminal in state,
  `SaveState`, weft-commit report + state (`weftCommit(layout, "poll <batch>
  <status>")`), envelope the digest via `output.Ok`; deadline `running` snapshot →
  `output.Ok` with the snapshot (no weft commit, no git diff — terminal-only
  computation per the discussion). Tests: fake-free where possible (scratch worktree,
  hand-written report files to drive terminal classification); spawn-batch tests
  stub the Starter seam via a package-local injection point mirroring how
  builderengine's own spawn tests fake it (add the seam to builderCLI if card 26's
  struct needs it — keep the production default `runner`).
- **Commit:** `feat(builder): spawn-batch and poll verbs with weft commits`

### Card 29: run and pause verbs

- **Context:**
  - `internal/perchcli/run.go`
  - `internal/perchcli/pause.go`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/buildercli/run.go`
  - `internal/buildercli/run_test.go`
  - `internal/buildercli/pause.go`
  - `internal/buildercli/pause_test.go`
- **Edits:**
  - `internal/buildercli/cli.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `run`: flag `--fresh` (bool). Calls `builderengine.Run` with the
  resolved deps; `ErrRunBusy` → error envelope, NO weft sync (the losing call touched
  nothing — perchcli's ErrBlockBusy exemption comment applies verbatim);
  `ErrFingerprintMismatch` → error envelope naming `run --fresh`; any other exit —
  success OR error — runs the backstop `weftCommit(layout, "run <outcome-label>")`
  before the envelope (perchcli's commit-even-on-error rationale: completed batches'
  artifacts must not strand uncommitted). Success envelope: `{"outcome",
  "stuck_reason", "batches_done", "run_dir", "session_id", "weftCommitted"}`. The
  distinct asking/died/timeout error envelopes surface the engine's error text
  unchanged. `pause`: `builderengine.RequestPause(builderDir)` + `output.Ok` with
  `{"pause_requested": true}` and a Long documenting the batch-boundary semantics
  (spawn-batch refuses; the in-flight batch finishes; resume = `lyx builder run`).
  `run`'s Long documents the full lifecycle including `--fresh` and the paused exit.
  Tests: fake BlockingRunner injection (same seam pattern as card 28); busy path
  skips weft; fingerprint mismatch message; pause writes the flag a subsequent
  spawn-batch test observes.
- **Commit:** `feat(builder): run and pause verbs`

## Batch Tests

`verify:` runs buildercli (envelope shapes for all six verbs, seam-injected
spawn/run paths, weft-boundary behaviour) plus builderengine (unchanged, regression
guard for the seams the CLI consumes).
