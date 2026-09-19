# Batch: go-cli-rename

```yaml
task: "Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise"
batch: "go-cli-rename"
number: 1
cards: 7
verify: go test ./internal/loomcli/... ./cmd/lyx/... && go test -tags smoke -run XXX_NONE ./internal/loomcli/...
depends-on: []
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

**Move ordering is load-bearing in card 2.** `internal/loomcli/run.go` is both a move source and a move target, so the two `git mv` calls must run in this exact order:

```
git mv internal/loomcli/run.go internal/loomcli/start.go
git mv internal/loomcli/drive.go internal/loomcli/run.go
```

Running them in the other order clobbers `run.go` before it has been relocated.

## Per-file sweep rule

Every card in this batch follows the discussion's own Scope rule: a file listed in a card's `Edits:` is swept **whole**, and every hit is read and classified before it is touched.
The sites named in each card's `Requirements:` are landmarks that pin the hard judgment calls — they are never the boundary, and a hit that is not named still gets classified and fixed.

The hit classes to sweep each `Edits:` file for:

1. **Verb names** — `lyx loom run`, `lyx loom drive`, `loom run`, `loom drive`, and bare backticked `` `run` ``/`` `drive` `` where the word names a loom verb.
2. **Pre-move filename citations** — a comment naming `run.go` or `drive.go` to say where a block came from or where its caller lives.
   After card 2's moves these become `start.go` and `run.go` respectively.
3. **Identifiers** — `runCmd`, `driveCmd`, `RunAliasCommand` in prose or in Go code, including **test function names** that encode a retired verb.
4. **Skill name** — `ly-supervise`, which becomes `ly-drive`.

Leave the disposition-2 noun sense alone: "run" meaning "an execution" (a run's bookkeeping, an autonomous run, mid-run, the run lock, a driver process) is not a verb name and is untouched.
"driver" and "drives" as ordinary English describing what a process does are likewise untouched — only the verb *name* `drive` is retired.

## Batch Scope

This batch is the atomic Go rename: the two file moves, the three identifier renames, the cobra surface, the three runtime refusal strings, and every test that names a renamed identifier or verb literal.
It is one batch because it is one compile unit — `runCmd`, `driveCmd`, and `RunAliasCommand` are referenced across `internal/loomcli` and `cmd/lyx`, so splitting the rename across batches would leave the tree non-compiling at a batch boundary.
Card 1 deliberately lands first and is expected to fail on the pre-rename tree: it is the TDD pair the discussion's Testing section calls for.
The external interface the later batches consume is the final verb vocabulary itself — `lyx loom start`, `lyx loom run`, `lyx start` — which every subsequent batch's prose and launcher content must match.

Batch-local decision: the loom-subtree absence guard is NOT duplicated.
`TestCommand_RegisteredVerbs_ExactSet` already errors in both directions, so it fails on a surviving `drive` once its `want` literal is updated;
the new guard in card 1 is scoped to the root tree alone, which genuinely has no absence coverage.

## Cards

### Card 1: TDD guards for the new root and loom verb names

- **Context:**
  - `internal/loomcli/cli.go`
  - `cmd/lyx/main.go`
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:**
  - `cmd/lyx/helptree_test.go`
- **Creates:**
  - `cmd/lyx/retiredverbs_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/helptree_test.go`, change the `requiredModules` slice entry `"run"` to `"start"`, and change the `loom` test case's `wantSubs` literal from `{"run", "drive", "step", "status", "pause", "validate-discussion", "validate-plan"}` to `{"start", "run", "step", "status", "pause", "validate-discussion", "validate-plan"}`.
  Both assertions are per-item `strings.Contains` sweeps, so they confirm the new names reached cobra but prove nothing about the old ones being gone.
  Create `cmd/lyx/retiredverbs_test.go` holding one untagged Tier 1 test that builds the root command via `newRoot()` and asserts the root tree carries no bare child named `run` — walk `root.Commands()`, skip `help` and `completion` the way `sandbox_coverage_test.go` already does, and fail if any child's `Name()` equals `"run"`.
  Scope the new test to the root tree only;
  do not assert anything about the loom subtree, which card 4's `TestCommand_RegisteredVerbs_ExactSet` update already guards in both directions.
  The test is a pure `*cobra.Command` walk: no `exec.Command`, no `gitexec`, no `hubforge.NewHub`, per the Test Tier Purity Invariant.
  Both assertions are expected to fail until card 2 lands, which is the intended TDD signal.
- **Commit:** `test(lyx): assert the renamed loom and root verb names before the rename`

### Card 2: rename the files, identifiers, cobra surface, and driver spawn

- **Context:**
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/seedinput.go`
- **Edits:**
  - `internal/loomcli/cli.go`
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/loomcli/run.go` -> `internal/loomcli/start.go`
  - `internal/loomcli/drive.go` -> `internal/loomcli/run.go`
- **Requirements:** Perform the two `git mv` calls in the order the `## Rename mechanic` section above pins, then make surgical edits only.
  In the file relocated by the first move pair above, formerly the bootstrap's own source file: rename the method `runCmd` to `startCmd`, set its `cobra.Command` `Use` to `"start"`, rename `RunAliasCommand` to `StartAliasCommand` and change its body's `c.runCmd()` call to `c.startCmd()`, rename the local variable `driveCmd` (the detached driver's `*exec.Cmd`, distinct from the method of the same name) to `runCmd` across all of its uses, and change `exec.Command(exe, "loom", "drive")` to `exec.Command(exe, "loom", "run")`.
  Rewrite the file's header comment, `startCmd`'s `Short` and `Long`, its `Example:` block, and `StartAliasCommand`'s doc comment so every reference names `start` rather than `run`;
  `StartAliasCommand`'s doc comment currently says the alias's own `Name()` is `"run"` and that it resolves "exactly as `lyx loom run` does" — both become `"start"` and `lyx loom start`.
  The constants `bootstrapHandshakePollInterval` and `bootstrapHandshakeAttempts` are unchanged.
  In the relocated `internal/loomcli/run.go` (formerly `drive.go`): rename the method `driveCmd` to `runCmd`, set its `Use` to `"run"`, and rewrite the file header comment, `Short`, `Long`, `Example:` block, the `shouldReflectFriction` doc comment, and every inline comment so the verb is named `run` and its contrast partner is named `lyx loom start`.
  These contrast sentences must be rewritten as sentences, not token-substituted: the `Long`'s "drive ensures that session itself, exactly as \"lyx loom run\" does" becomes a statement that `run` ensures the session itself exactly as `lyx loom start` does, and "drive never seeds a status file ... only \"lyx loom run\" seeds" becomes the same assertion naming `lyx loom start` as the seeder.
  A mechanical replace would turn each into a self-reference.
  Also in that file, change the seed-missing refusal string so it reads `loom: no status file at <path>; run "lyx loom start" first to bootstrap this task`, and update the two inline comments above it and above the `c.reed.Up()` call that name `"lyx loom run"` as the seeder or the `Up` caller.
  In `internal/loomcli/cli.go`: change `parent.AddCommand(c.runCmd(), c.driveCmd(), ...)` to `parent.AddCommand(c.startCmd(), c.runCmd(), ...)`, keeping the remaining five verbs and their order;
  rewrite the parent's `Long` so the sentences describing each verb's role name `"start"` as the bootstrap and `"run"` as the no-tmux foreground escape hatch, and update its `Example:` block's first two lines to `lyx loom start` and `lyx loom run`;
  sweep the whole `loomCLI` struct and rewrite **every** field comment naming a renamed verb, a renamed identifier, or a pre-move filename — at minimum `cfg`, `env`, `shedPaths`, `registry`, `runner`, `frictionDir`, and `landingCfg`, which between them name `driveCmd`, `runCmd`, `drive.go`, `run.go`, the "run/bootstrap verb", and `drive`'s detached driver log — so each names the post-rename identifier or filename;
  and rewrite `verbUsesLightweightWiring`'s doc comment, whose last sentence names `"run"` and `"drive"` as the comparison for why `"step"` is excluded, so it names `"start"` and `"run"` instead.
  The `verbUsesLightweightWiring` switch's own case list is untouched — none of the renamed verbs appear in it.
  Also in the relocated file, rewrite `reflectFriction`'s own doc comment, whose sentence about the run lock reading as free so that a second `"lyx loom run"` spawns a second driver is a bootstrap-sense hit and becomes `"lyx loom start"`;
  it is a separate comment from `shouldReflectFriction`'s and is easy to miss because the two read almost identically.
  In `cmd/lyx/main.go`: change `loomcli.RunAliasCommand()` to `loomcli.StartAliasCommand()` and rewrite the four-line comment above it so it names the `"start"` verb rather than the `"run"` verb.
  Also change the root command's `Long` string, whose closing sentence `Available modules: board, config, ide, reed, fabric, selfreport, shuttle, burler, webster, stencil, loom, run, quarry.` names the bare root alias as `run`;
  that entry becomes `start`, matching the alias's own `Use` after this card.
  Leaving it would keep the retired name in the root help text, against the no-back-compat-aliases decision.
  Do not touch the `runCmd` methods in `internal/burlercli`, `internal/shuttlecli`, or `internal/webstercli`.
- **Commit:** `refactor(loomcli): rename the drive verb to run and the run verb to start`

### Card 3: the remaining runtime refusal strings and step's Long

- **Context:**
  - `internal/loomcli/run.go`
- **Edits:**
  - `internal/loomcli/pause.go`
  - `internal/loomcli/status.go`
  - `internal/loomcli/step.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** These three files name the bootstrap verb in operator-facing text and must follow card 2's rename.
  In `internal/loomcli/pause.go`, change the absent-status-file error so its remedy reads `run "lyx loom start" first to bootstrap this task`, keeping the rest of the message (including the "there is nothing running to pause" clause) byte-identical.
  In `internal/loomcli/status.go`, change the `!found` branch's refusal so its remedy likewise names `"lyx loom start"`, keeping the rest of the message unchanged.
  In `internal/loomcli/step.go`, sweep the whole file per the batch's `## Per-file sweep rule`.
  Three landmarks: the `Long` string's opening sentence, which says step bootstraps the worktree's loom task exactly as `"lyx loom run"` does, and becomes `"lyx loom start"`;
  the file's own header comment, which says step bootstraps idempotently exactly as `` `run` `` does and that unlike `` `run` `` it spawns no detached driver — both of those name the bootstrap and become `` `start` ``;
  and the two `RunE`-body comments naming the foreground verb, one saying a producer hard error mirrors `drive`'s handling of the same condition and one about the next `drive`'s Tier-1 entry observation, which both become `run`.
  `step`'s own `Use`, `Short`, and behaviour are unchanged — the verb keeps its name because it already matches `shedengine.Shed.Step` — and the many uses of "driver" and "drives" as ordinary English are untouched.
- **Commit:** `refactor(loomcli): name lyx loom start in the pause, status, and step operator text`

### Card 4: update cli_test.go's verb set, alias guard, and refusal table

- **Context:**
  - `internal/loomcli/run.go`
  - `internal/loomcli/start.go`
  - `internal/loomcli/status.go`
  - `internal/loomcli/pause.go`
- **Edits:**
  - `internal/loomcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `TestCommand_RegisteredVerbs_ExactSet`, change the `want` literal to `[]string{"pause", "run", "start", "status", "step", "validate-discussion", "validate-plan"}`.
  This is a mandatory edit, not optional — the test fails the moment `start` is registered — and because it errors in both directions it is already the loom subtree's surviving-`drive` guard, so no additional loom-subtree absence test is written.
  Rename `TestRunAliasCommand_StaysOneCommandWithSubtreeVerb` to `TestStartAliasCommand_StaysOneCommandWithSubtreeVerb`, repoint its body at `StartAliasCommand()`, update its doc comment to name `StartAliasCommand` and `startCmd`, and flip the expected `alias.Use` from `"run"` to `"start"`;
  its non-empty-`Short` and `--parent`-flag assertions carry over unchanged.
  Add one assertion that test currently lacks: build the subtree verb directly from a fresh `&loomCLI{}` receiver via `startCmd()` and assert the alias's `Use` equals that command's `Use`, so the alias and the subtree verb can never drift apart again.
  This assertion encodes the root-alias-becomes-start decision structurally rather than as a string literal.
  In `TestVerbRefusals`, rename the `Drive_SeedMissing` row to `Run_SeedMissing`, repoint its `buildCmd` from `(*loomCLI).driveCmd` to `(*loomCLI).runCmd`, and change both existing rows' `wantRemedy` from `` `lyx loom run` `` to `` `lyx loom start` ``.
  Add a third row named `Status_NotFound` covering `status.go`'s `!found` refusal, whose `buildCmd` is `(*loomCLI).statusCmd` and whose `wantRemedy` is the same `lyx loom start` string the other two rows now carry;
  it must follow the existing rows' shape exactly — a hand-populated `*loomCLI` whose `shedPaths.StatusPath`/`StatusLockPath` point into a `t.TempDir()` that never receives a `status.json` — so the suite stays Tier 1 and spawns nothing.
  Update the `TestVerbRefusals` doc comment, which names the drive verb and `driveCmd/pauseCmd`, to name the renamed verbs and the third row's coverage.
- **Commit:** `test(loomcli): update the verb set, alias, and refusal guards for the rename`

### Card 5: update the sandbox coverage allowlist key

- **Context:**
  - `CONSTRAINTS.md`
  - `cmd/lyx/main.go`
- **Edits:**
  - `cmd/lyx/sandbox_coverage_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `excludedModules` map, change the key `"run"` to `"start"`, keeping its reason string `"alias of loom's own bootstrap verb; covered by the loom module's scenario"` byte-identical.
  This is a mandatory edit: `TestSandboxCoverage_AllModulesCoveredOrExcluded` asserts every excluded name is actually a registered module, so a stale `"run"` key fails outright with "excludedModules names %q but no such module is registered".
  No suite doc changes accompany this — no `**Covers:**` tag names `run`.
- **Commit:** `test(lyx): rename the sandbox coverage exclusion key to start`

### Card 6: smoke-tagged argv sites, the driver probe, and the refusal assertion

- **Context:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/run.go`
- **Edits:**
  - `internal/loomcli/smoke_test.go`
  - `internal/loomcli/smoke_bootstrapwiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** These are the most dangerous hits in the task, because `run` is reused rather than retired: a missed `"loom", "run"` does not fail as an unknown verb, it silently invokes the new foreground driver against an unbootstrapped worktree.
  In `internal/loomcli/smoke_test.go`, change every `runLoomCLINoFatal(exe, worktree, ..., "loom", "run")` invocation — the ones meaning the bootstrap — to `"loom", "start"`;
  change every `"loom", "drive"` argv pair, including the `exec.CommandContext(ctx, exe, "loom", "drive")` site, to `"loom", "run"`.
  Read each call site before changing it and pick the replacement from what the surrounding test actually asserts, rather than substituting by pattern.
  Rewrite `findDriverPIDs` so it no longer matches a lone argv element equal to `"drive"`: it must instead require an **adjacent `"loom"` followed by `"run"` argv pair** in the split `/proc/<pid>/cmdline` slice, keeping the existing cwd filter and the `runtime.GOOS != "linux"` early return unchanged.
  A lone `"run"` probe is wrong in the other direction — `run` is a verb on `shuttle`, `burler`, and `webster` too, so it would over-match any such process sharing the worktree cwd and the function would silently over-report;
  left unchanged it matches an argv element no process carries any more and returns nil forever, making every assertion built on it pass vacuously.
  Update `findDriverPIDs`' doc comment, which names "the detached `lyx loom drive` process" and the `"drive"` argv element, to describe the adjacent-pair discriminator and the new verb name.
  Beyond the argv and probe work, sweep both files whole per the batch's `## Per-file sweep rule`.
  In `smoke_test.go` that means at least: the file's own header doc comment, which narrates `"lyx loom run"` spawning its detached driver and a binary that knows how to dispatch `"loom drive"`;
  `newWiredPairFixture`'s doc comment, which mentions a caller's own first `"loom run"` call;
  and the three test function names `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed`, `TestSmokeDriveStandalone_RefusesOnNeverSeededPair`, and `TestSmokeDriveStandalone_FailureBeforeFirstPersistLeavesNonEmptyLog`, which encode the retired verb in Go identifiers and are renamed to their `Run` forms.
  In `internal/loomcli/smoke_bootstrapwiring_test.go`, change the assertion `strings.Contains(envelope.Error, "loom run")` to `"loom start"` — this asserts on the refusal text card 2 and card 3 changed, and is an assertion rather than a comment.
  Also rewrite that file's comments naming `lyx loom run`, `lyx loom drive`, or the `ly-supervise` skill so they name `lyx loom start`, `lyx loom run`, and `ly-drive` respectively, and retarget its stale citation saying `ensureFrictionDirAfterSeed` was written, documented and unit-tested in run.go and reached by neither `runCmd`'s `RunE` nor `seedAndCommitBootstrap` — after card 2 that file is start.go and that method is `startCmd`.
  The three `runLoomCLINoFatal(..., "loom", "step")` invocations are unchanged, because `step` keeps its name.
- **Commit:** `test(loomcli): retarget the smoke argv sites, driver probe, and refusal assertion`

### Card 7: loomcli comment and filename-citation sweep

- **Context:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/run.go`
- **Edits:**
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/bootstrap_test.go`
  - `internal/loomcli/friction_test.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/seedinput.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** These six files name the verbs in comments and doc strings, so they are almost entirely compile-safe, but they are retrospective records describing the tree as it stands and are rewritten outright rather than glossed.
  Two classes of hit appear here that the rest of the batch does not have: stale *filename* citations, where a comment names `run.go` or `drive.go` to say where a block came from or where its caller lives, and which must be retargeted to `start.go` and `run.go` respectively after card 2's moves;
  and a single test-function identifier, handled at the end of this card.
  In `internal/loomcli/bootstrap.go`, rewrite the comments naming `lyx loom drive` (the `shedengine.Run` lock-release narrative and the friction-timeout narrative) to name `lyx loom run`, and the comment naming `lyx loom run` as the repeated bootstrap invocation to name `lyx loom start`.
  Also rewrite that file's own header comment, whose closing clause says the verb body in run.go is assembly over judgment already under test: after card 2's moves that body lives in start.go, so the citation is retargeted.
  In `internal/loomcli/bootstrap_test.go`, apply the same rule: the handshake comment naming `lyx loom run` becomes `lyx loom start`, the friction-timeout comment naming `lyx loom drive` becomes `lyx loom run`, and the "every subsequent \"lyx loom run\"" comment becomes `lyx loom start`.
  In `internal/loomcli/friction_test.go`, rewrite the file header and the four comments naming `driveCmd`, `runCmd`, or `drive`'s own `RunE` so they name the post-rename identifiers — the former `driveCmd` is now `runCmd` and the former `runCmd` is now `startCmd`, so every one of these needs reading rather than substituting.
  Also change the comment naming "a second \"lyx loom run\" spawns a second driver" to name `lyx loom start`, and rename the test function `TestDriveEnsuresAbsentFrictionDir` to `TestRunEnsuresAbsentFrictionDir`, so the retired verb does not survive in a Go identifier.
  That rename is the one executable change in this card;
  it is a package-local test function with no other caller, so it is safe on its own.
  In `internal/loomcli/wiring.go`, rewrite the doc comment whose sentence says the verbs that actually build producers — naming the two renamed verbs — deliberately keep the full `wire()`, so it names the post-rename verbs;
  also retarget its two stale filename citations, one saying `CommitDiscussion` mirrors the seed commit the bootstrap source file already performs, and one saying `Env.Landing` is assembled in the foreground driver's source file.
  Its many other uses of the word "run" are the noun sense meaning "an execution" and must be left alone.
  In `internal/loomcli/sharedbootstrap.go`, rewrite the file header naming the two verbs whose shared blocks it holds, the lock-position sentences contrasting those two verbs, the failure-classification sentence naming them again, the comment saying the bootstrap verb spawns the foreground driver which ensures the directory itself, the quoted refusal text `run \"lyx loom ...\"`, and the four stale filename citations naming the pre-move source files.
  The verb this file calls `step` is unchanged throughout;
  the contrast sentences here name both renamed verbs against each other and must be rewritten as sentences rather than substituted, since a token swap makes each name one verb twice.
  In `internal/loomcli/seedinput.go`, retarget the two filename citations naming the foreground driver's pre-move source file as the seeder of `Env.Landing` and as the verb with no `--parent` flag, and rename the `drive-refuses-an-unrecorded-parent` decision name in its comment to match the renamed verb.
- **Commit:** `docs(loomcli): rewrite the verb and filename citations across the package comments`

## Batch Tests

`verify:` runs `go test ./internal/loomcli/... ./cmd/lyx/...`, which covers every untagged test this batch edits or creates: `cli_test.go` (the exact-set verb guard, the alias guard, and the three-row refusal table), `friction_test.go`, `bootstrap_test.go`, `helptree_test.go`, `sandbox_coverage_test.go`, and the new `retiredverbs_test.go`.
Both packages run in roughly two seconds on this tree, so the scope is per-batch as the default expects.

It then chains `go test -tags smoke -run XXX_NONE ./internal/loomcli/...`.
This batch edits `smoke_test.go` and `smoke_bootstrapwiring_test.go`, both `//go:build smoke`, which neither the untagged command above nor `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) ever compiles.
Without this chained invocation the argv retargeting, the rewritten `findDriverPIDs` probe, and the changed refusal assertion would ship with no compile coverage at all.
`-run XXX_NONE` matches no test name, so the files are type-checked and linked but the multi-minute, process-spawning smoke suite does not execute — the right trade for a gate that runs after every implementer and fixer round.

`CGO_ENABLED=1` and a C compiler are required for any build in this repo, per CLAUDE.md;
they are already the developer-machine default.
