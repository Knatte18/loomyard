# Batch: loomcli-core

```yaml
task: 'loom: session bootstrap'
batch: loomcli-core
number: 4
cards: 6
verify: go test ./internal/loomcli/
depends-on: []
```

## Batch Scope

This batch creates `internal/loomcli`, the thirteenth seam module, with three of its four verbs: `drive`, `status`, and `pause`.
It carries the cobra tree, the seam functions, the single cwd resolution in `PersistentPreRunE`, and the whole engine-stack wiring that turns a resolved `*lyxcwd.Location` into a `loomshed.Deps` with a real `websterengine.RunDeps` inside it.
It is one batch because the three verbs are meaningless without the shared receiver and the shared wiring, and because everything here is testable from one package's tier-1 suite.
`run` is deliberately held back to batch 5, which is where the bootstrap's own machinery — locks, spawn, handshake, terminal handover — lands.

This batch depends on nothing, so it can land alongside batches 1, 2 and 3.
Everything it touches already exists: the three loom path accessors it wires are the ones shipped before this task, whose values batch 3 leaves byte-identical, and it reaches none of batch 1's new fabric record functions and none of batch 3's new accessors or seed sentinel — every one of those is first used by batch 5, which is where the dependency on both batches genuinely sits.

The external interface batch 5 consumes: the `loomCLI` receiver and its populated fields, `(*loomCLI).runCmd`'s siblings on the same receiver, and `(*loomCLI).resolvePersistentPreRun`.
The external interface batch 6 consumes: `loomcli.Command`, `loomcli.RunCLI`, and `loomcli.RunCLIIn`.

Batch-local decisions beyond `## Shared Decisions` in the overview:

- The pre-run wires the full stack for every verb, including `status` and `pause`, matching how `internal/reedcli` and `internal/webstercli` already resolve everything once per invocation rather than per verb.
  The cost is that a broken loom config fails `status` too, which is the honest outcome in a hub where loom is meant to run.
- The status line's poll interval is a flag with a sub-second-capable default rather than a package constant, so a test drives it fast without a real wall-clock wait.

## Cards

### Card 15: the loomcli package skeleton and cobra seam

- **Context:**
  - `internal/reedcli/cli.go`
  - `internal/perchcli/cli.go`
  - `internal/webstercli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxcwd/cwdcontext.go`
  - `internal/shuttleengine/run.go`
  - `internal/websterengine/runlevel.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/cli.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `package loomcli` with a file-header comment stating that the parent `loom` command carries a `PersistentPreRunE` resolving cwd once into a `*lyxcwd.Location` and delegating the whole engine-stack construction to `wire`, that loom is hub-only so it resolves through `lyxcwd.Resolve` the way `internal/reedcli` does rather than through the degrade-to-standalone mode probe, and that the resolved value is named `location` throughout.
  Declare an unexported `loomCLI` struct holding the fields `wire` populates — at minimum the resolved location, the seam cwd, the loaded loom config, the constructed `*reedengine.Engine`, the assembled `loomshed.Deps`, and the assembled `websterengine.RunDeps` — each field carrying a one-line comment saying who reads it.
  Declare an unexported `runnerMasterStarter` struct wrapping a `*shuttleengine.Runner` with a `StartMaster` method satisfying `websterengine.MasterStarter`, and a comment naming it as this plan's deliberate duplication of the identical adapter in `internal/webstercli`, with the reason from the overview's `duplicated-cli-adapters-over-a-cli-to-cli-import` decision.
  Add `func (c *loomCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error`: return nil immediately when `cmd.Name()` is the group name, so a bare group listing and the unknown-subcommand path need no git repository; otherwise read cwd via `lyxcwd.CwdFrom(cmd.Context())`, resolve via `lyxcwd.Resolve(cwd)`, and call `c.wire`, each failure emitting `output.Err` and calling `clihelp.Abort(ctx, 1)` then returning nil, exactly as the three precedent files do.
  Add `func Command() *cobra.Command` building the parent `loom` command with a non-empty `Short`, a `Long` carrying concrete examples for all four verbs, `RunE: clihelp.GroupRunE`, the `PersistentPreRunE` above, and `parent.AddCommand` for the verbs.
  Register all four verb constructors — `c.runCmd()`, `c.driveCmd()`, `c.statusCmd()`, `c.pauseCmd()` — so the tree is complete; batch 5 supplies `runCmd`'s body, and this card supplies a compiling placeholder for it that emits an `output.Err` saying the verb is not wired yet, to be replaced wholesale in batch 5.
  Add `func RunCLI(out io.Writer, args []string) int` returning `RunCLIIn("", out, args)`, and `func RunCLIIn(cwd string, out io.Writer, args []string) int` branching on an empty cwd between `clihelp.Execute` and `clihelp.ExecuteIn`, both with the doc comments the other twelve modules carry, including the note that an empty directory would panic a uniform delegation.
- **Commit:** `feat(loomcli): add the loom cobra seam and its single cwd resolution`

### Card 16: wire builds the loomshed and webster dependency stacks

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/preflight.go`
  - `internal/loomengine/config.go`
  - `internal/webstercli/wiring.go`
  - `internal/webstercli/run.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/hubgeom/webstergeom.go`
  - `internal/shedadapters/webster.go`
  - `internal/websterengine/runlevel.go`
  - `internal/reedengine/config.go`
  - `internal/shuttleengine/run.go`
  - `internal/batcher/batcher.go`
  - `internal/modelspec/modelspec.go`
  - `internal/fabricengine/refscanner.go`
  - `internal/fabricengine/open.go`
- **Edits:**
  - `internal/loomcli/cli.go`
- **Creates:**
  - `internal/loomcli/wiring.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (c *loomCLI) wire(location *lyxcwd.Location, cwd string) error` in the new file, with a file-header comment saying it is extracted from the pre-run so a test can drive it against a hand-built location and stay tier 1, and that it resolves no cwd and spawns no process — every path it touches is caller-supplied or a plain config read.
  Anchor every config load at `location.AnchorPath()`, matching the three precedent CLIs: the loom config, the reed config, the shuttle config, the webster config, the model registry, the resolved webster roles, and the active batchifier.
  Build the reed geometry and the reed engine from `hubgeom`, build the claude engine, and build the shuttle runner from them exactly as the webster hub wiring does.
  Assemble a `websterengine.RunDeps` mirroring `webstercli`'s own `runDeps` field-for-field: the master starter through `runnerMasterStarter`, the reed ops, the claude engine, the shuttle config, the resolved roles, the webster config, the batchifier, the webster geometry from `hubgeom`, and the reference matcher.
  Pin the reference matcher to `fabricengine.NewRefScanner(location)`, built eagerly because that constructor only compiles a regexp and cannot fail; add a comment stating it must never be the never-matching stand-in, because that stand-in is permitted only in standalone, where there is no weft worktree for the guard to protect, and loom is hub-only.
  Set the bisector opener to a closure over `fabricengine.Open(location)`, and add a comment that the fabric handle stays lazy and must not be opened during the pre-run because opening stat-checks the paired sibling.
  Leave the webster runner seam nil so the adapter defaults to the production entry point.
  Assemble a `loomshed.Deps` whose status, run-lock, and status-lock paths come from the three `loomengine` accessors, whose anchor and worktree-root come from the location's two derived accessors, whose decision-record and support-log paths come from the two `loomengine` discussion accessors, whose bounce budget is left zero so the engine's own default applies, whose preflight producer is `loomshed.NewPreflightProducer(cwd)` — the adapter, never the bare validator function — and whose webster deps are the value just assembled.
  Store both assembled values plus the reed engine, the location, the cwd, and the loom config on the receiver.
- **Commit:** `feat(loomcli): wire the loomshed and webster dependency stacks from one resolved location`

### Card 17: the drive verb runs the phase machine in the foreground

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/errors.go`
  - `internal/loomengine/config.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/drive.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (c *loomCLI) driveCmd() *cobra.Command` with a non-empty `Short` and a `Long` stating that `drive` runs loom's phase machine in the foreground with no tmux and no strand, that it is the no-tmux escape hatch for debugging and CI, and that it never seeds and never commits.
  Its `RunE` checks `clihelp.ShouldAbort` first, then runs a pre-flight that refuses on the envelope when the status file named by the `loomengine` status accessor does not exist, with a message telling the operator to run the two-word bootstrap verb instead — add a comment saying this refusal exists so the operator is told on the envelope rather than discovering it as the phase machine's own seed-missing precondition failure buried in the detached driver's log, and that only the bootstrap verb may seed because only it owns the commit-before-precondition ordering.
  On a pre-flight pass, build the machine with `loomshed.New(c.deps)` and call its `Run` with the command's context, mapping the outcome onto one envelope: a nil error emits `output.Ok` carrying the outcome, the halted producer, the blocked reason, and the history length; a non-nil error emits `output.Err` with the error text.
  Treat the engine's already-running sentinel as an ordinary error envelope rather than a special case — a second driver is a real refusal here.
- **Commit:** `feat(loomcli): add the drive verb, the no-tmux phase-machine runner`

### Card 18: the status verb and its one-line watch renderer

- **Context:**
  - `internal/shedengine/status.go`
  - `internal/shedengine/activity.go`
  - `internal/loomengine/status.go`
  - `internal/loomengine/config.go`
  - `internal/state/state.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/reedcli/attach.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/status.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a pure `func renderStatusLine(st shedengine.Status) string` producing exactly one line from the composed activity: the literal prefix `loom`, then the state, then ` | now ` and the activity's now field, then ` | last ` and the last field only when it is non-empty, then ` | wait ` and the wait field only when it is non-empty.
  Its doc comment must state that the format is pinned to an exact string rather than left to judgment because a test asserts it, mirroring how the engine pins its own composed last field.
  Add `func (c *loomCLI) statusCmd() *cobra.Command` with a non-empty `Short`, a `Long` documenting both modes, a `--watch` bool flag, and an `--interval` duration flag whose default is one second and whose help text says it exists so the poll can be driven fast.
  Its `RunE` checks `clihelp.ShouldAbort` first.
  Without `--watch` it reads the status file through `state.ReadJSONStrict` under the two `loomengine` accessors and emits one `output.Ok` envelope carrying the current producer, the state, the error text, the pause flag, the composed activity, the history length, and the decoded loom product's slug and parent; a missing file and a decode failure each emit `output.Err` with a message naming the file.
  With `--watch` it performs the same read once as a pre-flight and refuses on the envelope if it fails, then enters the terminal keepalive: print `renderStatusLine` for each successful read, sleep the interval, and loop forever, emitting no further JSON.
  Add a comment recording that this tail is the CLI/Cobra Invariant's self-displays-then-blocks-forever exception, taken explicitly and narrowly, and that everything fallible stays in the pre-flight above it.
  A read failure inside the loop must not terminate the loop and must not write an envelope; render a one-line unavailable marker instead, since the pane is expected to survive the driver rewriting the file underneath it.
- **Commit:** `feat(loomcli): add the status verb and its one-line watch renderer`

### Card 19: the pause verb sets the flag and nothing else

- **Context:**
  - `internal/shedengine/status.go`
  - `internal/state/state.go`
  - `internal/loomengine/config.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/pause.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (c *loomCLI) pauseCmd() *cobra.Command` with a non-empty `Short` and a `Long` stating that pause sets a request the running phase machine consumes at its next producer boundary, that it does not kill anything, and that the machine clears the flag itself in the persist that records the paused state.
  Its `RunE` checks `clihelp.ShouldAbort` first, then calls `state.UpdateJSON` over the shed status shape under the two `loomengine` accessors with a mutate closure that returns an error naming the file when the record was not found — pausing an absent status file must error rather than create one — and otherwise returns the current value with only its pause-requested field set true, leaving every other field untouched.
  Emit one `output.Ok` envelope naming the file on success and one `output.Err` on failure.
- **Commit:** `feat(loomcli): add the pause verb`

### Card 20: tier-1 coverage for the wiring, the renderer, and the three verbs

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/status.go`
  - `internal/loomcli/pause.go`
  - `internal/loomcli/drive.go`
  - `internal/loomengine/config.go`
  - `internal/loomshed/loomshed.go`
  - `internal/websterengine/runlevel.go`
  - `internal/fabricengine/refscanner.go`
  - `internal/shedengine/status.go`
  - `internal/reedcli/testmain_test.go`
  - `internal/perchcli/wiring_test.go`
  - `internal/gitkit/hermetic.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/testmain_test.go`
  - `internal/loomcli/cli_test.go`
  - `internal/loomcli/wiring_test.go`
  - `internal/loomcli/status_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** All four files are untagged and must spawn nothing.
  `testmain_test.go` declares a `TestMain` calling the hermetic git environment helper before running the suite, so the package satisfies the Hermetic Git Test Environment Invariant once batch 7's tagged tests spawn git; it is untagged so one declaration covers every build of the package.
  `wiring_test.go` drives `wire` directly against a hand-built `*lyxcwd.Location` over a temporary directory seeded with the module config files the loads require, and asserts: every path field of the assembled `loomshed.Deps` equals the corresponding `loomengine` accessor's output for the same location; the run-lock path differs from the status-lock path, which the phase machine rejects outright when equal; the preflight field is non-nil and is the adapter type rather than a bare function; every field the webster hub wiring fills is non-zero in the assembled webster deps; the reference matcher is a non-nil scanner value and not the never-matching stand-in; and the bisector opener is non-nil in this hub-only mode.
  `status_test.go` is a table over `renderStatusLine` covering an empty last and wait, a populated last only, and both populated, each asserting the exact expected line.
  `cli_test.go` asserts the built tree: the parent carries a non-empty `Short`, every one of the four subcommands is registered and carries a non-empty `Short`, and a bare group invocation through `RunCLI` succeeds without needing a git repository — the guard for the group-name short-circuit in the pre-run.
  Add a table covering the drive verb's seed-missing pre-flight and the pause verb's absent-file refusal by invoking each through `RunCLIIn` against a temporary directory and asserting the emitted envelope reports failure with a message naming the expected remedy, skipping any assertion that would require a wired hub.
- **Commit:** `test(loomcli): cover the dependency wiring, the status line, and the verb tree`

## Batch Tests

`verify: go test ./internal/loomcli/` runs the one package this batch creates.
Every card's subject is inside it: cards 15 and 16's wiring is asserted by `wiring_test.go` and `cli_test.go`, card 18's renderer by `status_test.go`, and cards 17 and 19's refusal paths by `cli_test.go`'s envelope table.
No other package changes, so a wider scope would only re-run suites this batch cannot affect.
The suite is untagged and spawns nothing, so it stays inside the Test Tier Purity Invariant.
