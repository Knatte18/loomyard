# Batch: burler-cli

```yaml
task: "Build burler - the review+fix round worker"
batch: "burler-cli"
number: 3
cards: 1
verify: go build ./... && go test ./internal/burlercli/...
depends-on: [2]
```

## Batch Scope

Creates `internal/burlercli` — the thin debug-CLI wrapper the discussion's debug-cli
decision un-deferred: `lyx burler run --profile <yaml>` plus run-tuning flags, wiring the
real `shuttleengine.Runner` (mux + claudeengine) exactly like shuttlecli, and emitting the
`internal/output` JSON envelope. Zero domain logic in the cli package. Root registration in
`cmd/lyx` is deliberately NOT in this batch — it lands in batch 4 together with the sandbox
suite, because the Sandbox Suite Coverage guard fails the moment a module is registered
without a `**Covers:**` tag. This batch's tests drive the module through its own
`RunCLI` seam, which needs no root registration. External interface for batch 4:
`burlercli.Command()`, `burlercli.RunCLI`.

## Cards

### Card 7: burlercli package — group, run verb, profile YAML decode

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttlecli/cli.go`
  - `internal/shuttlecli/run.go`
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/engine.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/burlercli/cli.go`
  - `internal/burlercli/run.go`
  - `internal/burlercli/cli_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Follow shuttlecli's shape file-for-file. `cli.go`: a `burlerCLI`
  receiver holding the `*burlerengine.Engine`; `Command() *cobra.Command` builds the
  parent group `Use: "burler"` with a non-empty `Short` ("run one review+fix round over an
  artifact (the burler round worker)" or equivalent), a `Long` that names the A-review →
  B-fix round, the profile-file contract, and a concrete `lyx burler run --profile
  profile.yaml` example, `RunE: clihelp.GroupRunE`, and a `PersistentPreRunE` that (skipping
  when `cmd.Name() == "burler"`, same guard as shuttlecli's) resolves
  `hubgeometry.Getwd()` → `hubgeometry.Resolve` → `shuttleengine.LoadConfig(layout.Cwd,
  "shuttle")` → `muxengine.LoadConfig(layout.Cwd, "mux")` → `muxengine.New` →
  `shuttleengine.NewRunner(muxEngine, claudeengine.New(), layout, shuttleCfg)` →
  `burlerengine.New(runner, layout)` into the receiver, with every failure going through
  `output.Err` + `clihelp.Abort(ctx, 1)` exactly as `internal/shuttlecli/cli.go` does
  (burlercli is the module's claudeengine wiring point, mirroring shuttlecli per the
  Provider-Seam Invariant). `RunCLI(out io.Writer, args []string) int` delegates to
  `clihelp.Execute(Command(), out, args)`. `run.go`: the `run` subcommand with non-empty
  `Short` and a `Long` holding a full example profile-YAML body; flags `--profile <path>`
  (required via `MarkFlagRequired`), `--model`, `--effort`, `--round` (strings), and
  `--timeout` (`time.Duration` via cobra's `DurationVar`; zero defers to the shuttle
  config default). Implement `func decodeProfile(data []byte) (burlerengine.Profile,
  error)` (exported only if the test package is external — keep the test same-package and
  the helper unexported): a `profileYAML` struct decoded with `yaml.v3`'s
  `Decoder.KnownFields(true)` (strict per the overview's yaml-strictness-split decision),
  keys `target:`/`fasit:` (each `{paths: [...], instructions: "..."}`), `rubric:`,
  `fix-scope:`, `tool-use:`, `cluster-n:`, `review-path:`, `fixer-report-path:`,
  `prior-reviews:`, `prior-fixer-reports:`; mapped 1:1 onto `burlerengine.Profile`
  (`fix-scope` string cast to `burlerengine.FixScope` verbatim — validation stays in the
  engine). The RunE reads the profile file (read error → `output.Err`), decodes, builds
  `burlerengine.RunOpts` from the four flags, calls `Engine.Run`, and on success emits
  `output.Ok` with fields `outcome`, `verdict`, `review_path`, `fixer_report_path`,
  `session_id`, `strand_guid`, `last_assistant_message`; any error (validation,
  `ErrClusterUnsupported`, shuttle, parse) goes through `output.Err` + non-zero exit via
  the `clihelp.Abort` pattern. `cli_test.go` (same package): bare `RunCLI(out,
  []string{})` lists subcommands exit 0; unknown subcommand → JSON error envelope +
  exit 1 (GroupRunE behavior); `run` without `--profile` → required-flag error; every
  command in the tree has a non-empty `Short`; `decodeProfile` table — full valid YAML
  (all fields land, `tool-use: true`, `cluster-n: 0`), minimal valid YAML, unknown key →
  error (strict decode), malformed YAML → error. Engine invocation is NOT exercised here
  (it needs live mux/claude); the PersistentPreRunE guard test asserts bare `burler`
  works outside a git repo, mirroring shuttlecli's guard rationale.
- **Commit:** `burler: add lyx burler CLI wrapper (profile YAML -> engine run)`

## Batch Tests

`verify:` runs `go test ./internal/burlercli/...` — the new `cli_test.go` covers the group
guard, help tree, strict profile decode, and flag surface. `cmd/lyx` is untouched until
batch 4, so no wider scope is needed.
