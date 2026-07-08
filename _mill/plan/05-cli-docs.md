# Batch: cli-docs

```yaml
task: "Build perch - the review gate loop"
batch: "cli-docs"
number: 5
cards: 5
verify: go test ./cmd/lyx/ ./internal/perchcli/ ./internal/perchengine/
depends-on: [4]
```

## Batch Scope

The product surface and the landing obligations: `internal/perchcli` (`lyx perch run`,
`lyx perch pause`) with the full wiring chain, the standalone weft commit at block exit,
root registration with every pinned-set update, the sandbox scenario, and the Documentation
Lifecycle moves (delete `docs/modules/perch.md`, fold the durable design into the package
header, flip overview/roadmap). After this batch the module is registered, covered, and
documented — the repo's guards (`registration_test`, `longlist_test`, `sandbox_coverage_test`,
`drift_test`, `helptree_test`, `configreg_test`) all pass by construction.

## Cards

### Card 14: perchcli command tree and wiring

- **Context:**
  - `internal/burlercli/cli.go`
  - `internal/burlercli/cli_test.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/perchengine/engine.go`
  - `internal/perchengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/perchcli/cli.go`
  - `internal/perchcli/cli_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** cli.go mirrors burlercli/cli.go with one deliberate difference: because
  the pause seam closes over a per-run dir, `PersistentPreRunE` resolves and stores the
  engine's INGREDIENTS rather than the engine itself — `type perchCLI struct { burlerEngine
  *burlerengine.Engine; runner *shuttleengine.Runner; perchCfg perchengine.Config; layout
  *hubgeometry.Layout; runDirBase string }` — and the run verb (card 15) calls
  `perchengine.New` per invocation. The PreRunE chain (guard-skipped when `cmd.Name() ==
  "perch"`): `hubgeometry.Getwd` → `hubgeometry.Resolve` → `shuttleengine.LoadConfig(
  layout.Cwd, "shuttle")` → `muxengine.LoadConfig(layout.Cwd, "mux")` →
  `perchengine.LoadConfig(layout.Cwd, "perch")` → `muxengine.New` →
  `shuttleengine.NewRunner(muxEngine, claudeengine.New(), layout, shuttleCfg)` →
  `burlerengine.New(runner, layout)` — perchcli is the module's claudeengine wiring point
  (Provider-Seam Invariant); `runDirBase = hubgeometry.PerchRunsDir(layout.WorktreeRoot)`. Parent command: `Use: "perch"`, a `Short` naming the gate loop,
  a `Long` with a concrete `lyx perch run --profile` example, `RunE: clihelp.GroupRunE`.
  `func RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out, args)`.
  cli_test.go mirrors burlercli/cli_test.go: `TestRunCLI_NoArgs` (bare `lyx perch` lists
  `run` and `pause`, exit 0, no git repo needed), `TestRunCLI_UnknownSubcommand` (exit 1,
  `"ok":false` envelope), `TestRunCLI_GroupGuard_OutsideGitRepo`,
  `TestCommand_EveryCommandHasShort`.
- **Commit:** `perch: add perchcli command tree with engine wiring`

### Card 15: run verb — profile decode, run identity, engine call, weft commit

- **Context:**
  - `internal/burlercli/run.go`
  - `internal/initengine/undo.go`
  - `internal/weftengine/sync.go`
  - `internal/weftengine/weft.go`
  - `internal/perchengine/profile.go`
  - `internal/perchengine/state.go`
  - `internal/perchengine/run.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/perchcli/cli.go`
- **Creates:**
  - `internal/perchcli/run.go`
  - `internal/perchcli/run_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** run.go implements `func (c *perchCLI) runCmd() *cobra.Command` following
  burlercli/run.go's structure. Flags: `--profile` (required, validated manually before
  touching c.engine — never `MarkFlagRequired`), `--run-id` (optional override), `--model`,
  `--effort`, `--timeout` (a `time.Duration` flag; the three tuning flags override the
  profile's `model`/`effort`/`timeout` values when non-zero/non-empty — burlercli's flag
  semantics, per the discussion Decisions "Run-tuning v1" and "Command tree v1").
  Strict profile decode: `profileYAML` struct with `KnownFields(true)`, kebab-case keys —
  `target`/`fasit` (paths+instructions, burler's fileSetYAML shape), `rubric`, `fix-scope`,
  `tool-use`, `cluster-n`, `gate` (nested: `mode`, `command` (string list), `timeout` (Go
  duration string parsed via `time.ParseDuration`)), `round-caps`, `judge-model`,
  `judge-effort`, `model`, `effort`, `timeout` (top-level: the burler-round timeout, Go
  duration string) — mapped 1:1 onto `perchengine.Profile`. The Long
  documents a complete example profile (llm-verdict prose review AND a commented command-gate
  variant). Run identity: resolve the profile, `perchengine.ProfileHash`, run-id =
  `--run-id` if set else `perchengine.DeriveRunID(profilePath, hash)`; `runDir =
  filepath.Join(c.runDirBase, runID)`. Pause seam wiring: the engine is
  constructed per-invocation inside the run verb's RunE, not in `PersistentPreRunE` — the
  PreRunE stores the resolved pieces (`burlerEngine`, `runner`, `perchCfg`, `layout`) on
  `perchCLI`, and RunE calls `perchengine.New(...)` with
  `Options{PauseRequested: func() bool { _, err :=
  os.Stat(perchengine.PauseFlagPath(runDir)); return err == nil }}` closing over the
  concrete runDir (card 14 shaped `perchCLI` for exactly this; the cli.go edit here is only
  registering `runCmd` on the parent).
  Call `c.engine.Run(profile, runDir)` (constructed engine); on hard error →
  `output.Err`. On success — any of the three outcomes — perform the weft sync per the Weft
  Git Invariant using initengine/undo.go's call shape:
  `weftengine.Commit(l.WeftWorktree(), weftengine.ScopedPathspec(l.RelPath,
  []string{hubgeometry.LyxDirName}), fmt.Sprintf("perch: %s %s", runID,
  string(result.Outcome)), weftengine.EnvSyncOptions())` then `weftengine.Push` with the
  same opts; a weft failure is reported via `output.Err` (the block result is already on
  disk; the message must say the review finished but the weft sync failed). Then
  `output.Ok` with fields: `outcome`, `stuckReason`, `roundsRun`, `runId`, `runDir`,
  `weftCommitted` (bool from Commit's first return). run_test.go: `TestRunCLI_Run_
  MissingProfile` (burlercli's pattern), `TestDecodeProfile` strict-decode table (full
  valid incl. gate mapping + duration parse, minimal valid, unknown key rejected, malformed
  YAML, bad gate duration), `TestDecodeProfile_FullValidFieldMapping` (every field incl.
  `gate.command` argv and `round-caps`), and a `DeriveRunID`-shape test via the exported
  perchengine helpers.
- **Commit:** `perch: add run verb with strict profile decode, run identity, and weft sync at exit`

### Card 16: pause verb

- **Context:**
  - `internal/perchcli/run.go`
  - `internal/perchengine/state.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/perchcli/cli.go`
- **Creates:**
  - `internal/perchcli/pause.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** pause.go: `func (c *perchCLI) pauseCmd() *cobra.Command` — `Use:
  "pause"`, required `--run-id` (manual validation), `Short`/`Long` explaining boundary
  semantics: writes the pause flag file at
  `perchengine.PauseFlagPath(filepath.Join(c.runDirBase, runID))`; the running block honors
  it at the NEXT round boundary (never mid-round) and exits `PAUSED`; re-running `lyx perch
  run` with the same profile resumes at the recorded round and clears the flag. Creating the
  flag: `os.WriteFile` of an empty file after `os.MkdirAll` of the run dir is NOT done — if
  the run dir does not exist, report `output.Err` ("no such run") instead of silently
  creating one. Idempotent when the flag already exists. `output.Ok` with `runId` and
  `pauseFile`. Register `pauseCmd` alongside `runCmd` in cli.go's `parent.AddCommand` (the
  cli.go edit). Tests for the verb (missing `--run-id`, no-such-run error, flag created,
  idempotent re-pause) go into the existing `internal/perchcli/cli_test.go` — card 14
  created it; extend it here (allowed: cli_test.go belongs to this batch).
- **Commit:** `perch: add pause verb writing the round-boundary pause flag`

### Card 17: root registration and pinned-set updates

- **Context:**
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/longlist_test.go`
  - `cmd/lyx/drift_test.go`
  - `internal/perchcli/cli.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** main.go `newRoot()`: import `internal/perchcli`; add `perchcli.Command()`
  to the `root.AddCommand(...)` block after `burlercli.Command()`; append `perch` to the
  root `Long`'s "Available modules:" list (after `burler`). helptree_test.go: add `"perch"`
  to `requiredModules` and a `wantSubs` table case `{module: "perch", subs: ["run",
  "pause"]}` matching the existing entries' shape. registration_test.go, longlist_test.go,
  drift_test.go need NO literal edits (AST-derived; listed as Context so the implementer
  verifies rather than guesses). sandbox_coverage_test.go passes only after card 18's
  `**Covers:** perch` tag — cards 17 and 18 land within the same batch, so the batch
  `verify:` sees both.
- **Commit:** `lyx: register perch module in root and helptree pinned sets`

### Card 18: docs lifecycle, sandbox scenario, durable package header

- **Context:**
  - `docs/modules/perch.md`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `_mill/discussion.md`
- **Edits:**
  - `docs/overview.md`
  - `docs/roadmap.md`
  - `docs/reviews/README.md`
  - `docs/modules/README.md`
  - `docs/modules/loom.md`
  - `docs/modules/hardener.md`
  - `tools/sandbox/SANDBOX-BURLER-SUITE.md`
  - `internal/perchengine/doc.go`
- **Creates:** none
- **Deletes:**
  - `docs/modules/perch.md`
- **Moves:** none
- **Requirements:** Documentation Lifecycle for a landed module. (1) Expand
  `internal/perchengine/doc.go` into the durable design header, folding in perch.md's
  surviving rationale as amended by `_mill/discussion.md`: the two-exit gate contract
  (APPROVED | STUCK, plus PAUSED as the operational exit), loop-until-dry, the milestone
  ladder (`round_caps`, last entry hard cap), the verdict-judge model (holistic Haiku judge,
  fail-safe uncertain→continue, themes overview — explicitly superseding the old
  key-canonicalization design), the pluggable gate and why the command runs in perch (the
  decider does not trust the worker), verdict-based judge triggering, non-done handling
  (retry/triage; infrastructure error ≠ STUCK), pause-at-round-boundary, weft-blindness,
  and the phase-knowledge-lives-in-loom config rule. (2) Delete `docs/modules/perch.md`. (3)
  Retarget every inbound reference: overview.md — flip the perch module bullet to
  `✅ Implemented` (`lyx perch run|pause`), drop the `modules/perch.md` link, keep the
  execution-stack lines accurate; roadmap.md — milestone 11's perch item flips to ✅ Done
  (pointing at the `internal/perchengine` package documentation, the burler precedent's
  wording), and the build-order line marks perch ✅; docs/reviews/README.md,
  docs/modules/README.md, docs/modules/loom.md, docs/modules/hardener.md — replace
  `perch.md` links with "see the `internal/perchengine` package documentation" (grep for
  both `modules/perch.md` and bare `perch.md` to catch all six files). (4) Sandbox: append a
  new `S`-numbered scenario to SANDBOX-BURLER-SUITE.md (its Notes section pins perch
  scenarios to this suite) in the existing block format with `**Covers:** perch`: operator
  writes a small deliberately-flawed fixture doc + a perch profile (llm-verdict gate,
  `round-caps: [2, 3]`), runs `lyx perch run --profile <file>`, watches convergence to
  `"outcome":"APPROVED"` within the cap, inspects the run dir (numbered round artifacts,
  state.json) and the weft commit; a second step runs `lyx perch pause --run-id <id>` during
  a longer run and confirms the `PAUSED` exit and that re-running `lyx perch run` resumes at
  the recorded round. `**Watch:**` names the JSON envelope fields and the run-dir contents;
  `**Verdict:**` line per the suite convention. (5) The roadmap edit marks a completed
  planned milestone — exactly the CLAUDE.md-permitted roadmap change, nothing else is added
  there.
- **Commit:** `docs(perch): land module docs — delete design doc, fold into package header, add sandbox scenario`

## Batch Tests

`verify:` runs `go test ./cmd/lyx/ ./internal/perchcli/ ./internal/perchengine/`: the root
guards (helptree/registration/longlist/drift/sandbox-coverage — proving registration and the
`**Covers:** perch` tag), the perchcli seam tests (envelope errors, strict decode, pause
verb), and the engine suite unchanged. Docs edits have no runnable surface beyond the
sandbox-coverage guard, which is exactly what enforces them.
