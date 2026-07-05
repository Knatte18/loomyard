# Batch: cli-and-registration

```yaml
task: 'Build internal/shuttle: one LLM agent via a swappable engine'
batch: cli-and-registration
number: 5
cards: 4
verify: go test ./cmd/lyx/... ./internal/shuttlecli/...
depends-on: [4]
```

## Batch Scope

The `lyx shuttle` cobra module (`run`, `interrupt`, `send`), its registration in the lyx
root per the CLI/Cobra Invariant (pinned help-tree sets updated in the same batch), and
the sandbox-suite coverage the Sandbox Coverage invariant demands. This is where
`claudeengine` gets injected into the runner — the only place the two sides of the seam
meet.

Batch-local note: `cmd/lyx/sandbox_coverage_test.go` asserts BOTH directions
(registered ⊆ covered and covered ⊆ registered), so card 20 (registration) and card 21
(suite tag) each leave that one guard transiently red on their own commit — no card order
fixes it. The guard is green at the batch-end `verify:`, which is the gate that counts.

## Cards

### Card 18: shuttlecli package with the run verb

- **Context:**
  - `internal/muxcli/cli.go`
  - `internal/muxcli/add.go`
  - `internal/muxcli/status.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/config.go`
  - `internal/muxengine/config.go`
  - `internal/muxengine/lock.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/muxengine/render/types.go`
- **Edits:** none
- **Creates:**
  - `internal/shuttlecli/cli.go`
  - `internal/shuttlecli/run.go`
  - `internal/shuttlecli/cli_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `cli.go` mirrors `muxcli/cli.go`: `Command() *cobra.Command` (Use
  `shuttle`, non-empty `Short`, `RunE: clihelp.GroupRunE`) with a `PersistentPreRunE`
  that resolves `hubgeometry.Getwd()` → `hubgeometry.Resolve` →
  `shuttleengine.LoadConfig(layout.Cwd, "shuttle")` → `muxengine.LoadConfig(layout.Cwd,
  "mux")` → `muxengine.New(muxCfg, layout)` →
  `shuttleengine.NewRunner(muxEngine, claudeengine.New(), layout, shuttleCfg)`, storing
  the runner on the cli struct; errors go through `output.Err` + `clihelp.Abort` exactly
  like muxcli. `RunCLI(out io.Writer, args []string) int` =
  `clihelp.Execute(Command(), out, args)`. `run.go`: `lyx shuttle run` flags:
  `--prompt string` XOR `--prompt-file string` (exactly one required; prompt-file reads
  the file into `Spec.Prompt`), `--output-file` (repeatable `StringArray`, required ≥1),
  `--model string`, `--interactive bool`, `--role`/`--round`/`--parent string`,
  `--anchor string` (default `below-parent`), `--focus bool` (default true), `--shrink
  bool` (default true), `--timeout duration` (0 = config default), `--keep-pane bool`.
  Build `shuttleengine.Spec` (Display from anchor/focus/shrink), call `runner.Run(spec)`,
  and print `output.Ok` with fields `{outcome, sessionId, guid, lastAssistantMessage,
  runDir}` — exit 0 for every classified outcome; mechanism errors → `output.Err` (see
  overview Shared Decision "CLI envelope posture"). `Short` on every command; `run`
  carries a `Long` with two concrete examples (autonomous run with two output files;
  interactive run). `cli_test.go`: help output names run/interrupt/send; unknown
  subcommand rejected via GroupRunE; flag validation (missing output-file, both
  prompt+prompt-file, neither) exercised through `RunCLI` against an uninitialized dir
  is acceptable where config loading would abort first — structure validation so flag
  errors surface before config resolution (validate flags in the verb's RunE before
  touching the runner).
- **Commit:** `feat(shuttle): lyx shuttle run — one agent over the file contract`

### Card 19: interrupt and send verbs

- **Context:**
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/mux.go`
  - `internal/shuttleengine/claudeengine/startup.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/shuttlecli/cli.go`
  - `internal/shuttlecli/cli_test.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/rundir.go`
- **Creates:**
  - `internal/shuttlecli/interrupt.go`
  - `internal/shuttlecli/send.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `lyx shuttle interrupt <guid>` and `lyx shuttle send <guid> <text>`
  (exact-args cobra validation; non-empty `Short` each). Both verify the guid belongs to
  a shuttle run via the run-dir lookup — a miss errors with "not a shuttle strand"
  through `output.Err`. Expose the pieces this needs from shuttleengine: in `rundir.go`,
  export `func FindRun(cfg Config, layout *hubgeometry.Layout, guid string) (RunState,
  string, error)` composing `runDirRoot` + `findRunByStrand`; in `run.go`, add
  `func (r *Runner) Interrupt(guid string) error` and `func (r *Runner) Send(guid, text
  string) error` — the same sequence playback as card 17's handle methods (share the
  unexported input-playback helper; Send enforces the same single-line rule). CLI
  success prints `output.Ok` with `{guid, action}`. Update `cli_test.go` for the new
  help entries and arg validation.
- **Commit:** `feat(shuttle): lyx shuttle interrupt/send — cross-terminal agent poke`

### Card 20: root registration and pinned help-tree sets

- **Context:**
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/longlist_test.go`
  - `cmd/lyx/drift_test.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `newRoot()`: import `internal/shuttlecli`, append
  `shuttlecli.Command()` to the `root.AddCommand(...)` list, and extend the root `Long`
  module list to `init, board, config, ide, mux, weft, warp, selfreport, shuttle`. In
  `helptree_test.go`: add `shuttle` to `requiredModules` in
  `TestHelpTree_RootNamesAllModules` and add a `TestHelpTree_VerbModuleSubcommands`
  entry `{name: "shuttle", module: "shuttle", wantSubs: ["run", "interrupt", "send"]}`.
  Run the four registration guards (`drift`, `helptree`, `registration`, `longlist`) and
  fix any further pinned expectations they surface (CONSTRAINTS: update pinned sets in
  the same commit as the module registration).
- **Commit:** `feat(shuttle): register lyx shuttle in the root (pinned help sets updated)`

### Card 21: sandbox suite coverage

- **Context:**
  - `tools/sandbox/SANDBOX-MUX-SUITE.md`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `sandbox-mux-suite.cmd`
  - `docs/sandbox-howto.md`
- **Edits:**
  - `tools/sandbox/main.go`
  - `tools/sandbox/suite.go`
  - `tools/sandbox/main_test.go`
  - `tools/sandbox/suite_test.go`
- **Creates:**
  - `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
  - `sandbox-shuttle-suite.cmd`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Wire the new suite into the launcher exactly as `mux-suite` is wired:
  add a `shuttleSuite` `suiteSpec` to `suite.go` (embed `SANDBOX-SHUTTLE-SUITE.md` via
  `//go:embed`, default instruction `"Read ./SANDBOX-SHUTTLE-SUITE.md and follow the
  instructions in it exactly."`) and a `"shuttle-suite"` case to `main.go`'s subcommand
  switch mirroring the `"mux-suite"` case exactly (own flagset, `-claude`/`-prompt`
  flags, `runSuite(absParent, *claudeFlag, *promptFlag, shuttleSuite)`). Add dispatch
  tests to `main_test.go` mirroring `TestRun_MuxSuiteRoutesToLaunch` /
  `TestRun_MuxSuiteFlagsRoutedAfterToken` / `TestRun_MuxSuiteErrorPropagation` for
  `shuttle-suite`, and spec tests to `suite_test.go` mirroring the `TestRunSuite_MuxSpec_*`
  family for `shuttleSuite`. New suite file modeled structurally on `SANDBOX-MUX-SUITE.md`
  (same bold-label scenario grammar: `**Goal:**`/`**Watch:**`/`**Verdict:**` plus
  `**Covers:** shuttle` on scenarios that drive the module). Minimum scenarios:
  (S1) autonomous `lyx shuttle run` happy path — prompt instructing the agent to write a
  given output file; watch the strand appear, the outcome JSON report `done`, pane and
  run dir cleaned; (S2) `asking` path — prompt that requires a decision the agent cannot
  make; verdict JSON reports `asking` with the question, strand kept, operator attaches
  and answers; (S3) interrupt/send — start a long-running run, `lyx shuttle interrupt
  <guid>`, `lyx shuttle send <guid> "<one-line update>"`, watch the agent continue.
  `sandbox-shuttle-suite.cmd` at the repo root mirrors `sandbox-mux-suite.cmd` (same
  launcher mechanics, pointed at the new suite file). This satisfies the Sandbox
  Coverage invariant's "exists ⇒ covered" for the new registered module (the guard
  unions `**Covers:**` tags across `tools/sandbox/*SUITE.md`).
- **Commit:** `test(shuttle): sandbox suite + launcher (Covers: shuttle)`

## Batch Tests

`verify: go test ./cmd/lyx/... ./internal/shuttlecli/...` — the four registration guards
(drift/helptree/registration/longlist) plus `sandbox_coverage_test.go` (now satisfied by
the new suite file) and shuttlecli's own help/flag-validation tests. The verbs' live
behaviour against a real session is batch 6 (smoke) and the sandbox suite (operator-run).
