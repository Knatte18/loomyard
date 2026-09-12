# Batch: loom-step-verb

```yaml
task: "lyx loom step + external supervisor skill"
batch: "loom-step-verb"
number: 4
cards: 4
verify: go test ./internal/loomcli/... ./cmd/lyx/... ./internal/lyxcwd/...
depends-on: [1, 2, 3]
```

## Batch Scope

This batch delivers the `lyx loom step` verb itself: the early run-lock probe, the idempotent bootstrap, the single `shed.Step` call, the ten-key success envelope, and the five-kind refusal vocabulary — plus its cobra registration, the three existing tests that pin loom's verb set, and the two doc surfaces that enumerate loom's verbs.
It is one batch because the verb, its registration, its tests, and the docs that describe it are a single shipped unit; splitting registration from the verb would leave a dead file at a batch boundary, and `CLAUDE.md` requires the doc surfaces to move in the same commit as the observable CLI change.
It consumes `shedengine.Step`/`StepResult` from batch 1, `loomshed.InterruptPolicyFor` from batch 2, and all three helpers from batch 3.

Batch-local decision: the envelope builder and the refusal-kind mapping are pure functions in `step.go`, tested directly. `step`'s success envelope cannot be reached in an untagged test without a real fabric and a real reed session, so per `## Shared Decisions`' new-tests-stay-untagged-and-pure rule the decisions are factored out and tested where they live, mirroring how `bootstrap.go` already holds `run`'s pure decisions apart from `run.go`'s assembly.

## Cards

### Card 10: the step verb

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/run.go`
  - `internal/loomcli/drive.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/seedinput.go`
  - `internal/loomcli/status.go`
  - `internal/loomshed/interruptpolicy.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/errors.go`
  - `internal/shedengine/status.go`
  - `internal/loomengine/config.go`
  - `internal/output/output.go`
  - `internal/clihelp/exec.go`
  - `internal/lock/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/step.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare the closed refusal-kind vocabulary as five unexported constants: `stepKindBusy = "busy"`, `stepKindUnseeded = "unseeded"`, `stepKindOwnership = "ownership"`, `stepKindBootstrap = "bootstrap"`, and `stepKindProducer = "producer"`. Add `var stepKinds = []string{...}` listing all five, so a test can assert the set is exactly this and no larger. Document that the skill's one-retry rule applies to `stepKindProducer` alone, and that every other kind is handed straight back to the operator with no retry because none of them can be fixed by running the same command again.

  Add a pure `func stepKindForBootstrapStage(stage bootstrapStage) string` mapping `bootstrapStageSeed` to `stepKindUnseeded`, `bootstrapStageOwnership` to `stepKindOwnership`, and both `bootstrapStageOrigin` and `bootstrapStageCommit` to `stepKindBootstrap`. Any other value, `bootstrapStageNone` included, also maps to `stepKindBootstrap`, so an unclassifiable failure still carries a kind rather than an empty one.

  Add a pure `func stepEnvelope(res shedengine.StepResult, nextPolicy, statusFile string) map[string]any` returning exactly ten keys: `producer` from `res.Producer`; `outcome` from `string(res.Outcome)`; `output` from `res.Output`; `next` from `res.Next`; `state` from `string(res.State)`; `reason` from `res.Reason`; `continue` derived as `res.State == shedengine.StateRunning`; `history_length` as `len(res.History)`; `next_interrupt_policy` from `nextPolicy`; and `status_file` from `statusFile`. Document that `continue` is derived here rather than left to the caller so a thin skill never carries a copy of the state vocabulary, and that the key set is closed — a key outside these ten has no test and no documented meaning.

  Add `func (c *loomCLI) stepCmd() *cobra.Command` with `Use: "step"`, a non-empty `Short` per the CLI/Cobra Invariant, and a `Long` describing what the verb does, what it never does (it spawns no driver, hands the terminal to nothing, and loops over nothing), and carrying an `Example` block with `lyx loom step` and `lyx loom step --parent main`. Register a `--parent` string flag with the same help text `run` uses.

  The `RunE` body, in this exact order:

  1. `if clihelp.ShouldAbort(cmd.Context()) { return nil }`, then bind `ctx := cmd.Context()` and `out := cmd.OutOrStdout()`.
  2. `slug := seedSlug(c.location.WorktreeName)`.
  3. The early run-lock probe, **before** the bootstrap. Call `os.MkdirAll(filepath.Dir(c.shedPaths.LockPath), 0o755)`; on error refuse with `stepKindBootstrap`. Then `lock.TryAcquireWriteLock(c.shedPaths.LockPath)`; on error refuse with `stepKindBootstrap`; when the lock was free, release it immediately; when it was held, refuse with `stepKindBusy` and a message naming `lyx loom pause` as the remedy. Comment that the `MkdirAll` is part of the probe rather than an accident of ordering: the run lock lives in the ephemeral tree, `internal/lock` opens with `O_CREATE` but never creates a parent, and `run` creates that same directory at its step 4 before its own step-5 probe — so hoisting only the probe would run it on a fresh worktree whose parent directory does not exist. Comment that treating a missing-parent error as "lock free" is explicitly rejected, because it converts a filesystem fault into a false green light on the one check guarding against a second driver. Comment that without this early probe, `step` would seed, commit, bring up reed, and churn the status strand against a task a live driver owns — and that the strand work in particular is a real side effect on a running session, since it can remove and re-add the pane a driver's operator is watching.
  4. `parent, stage, err := c.seedAndCommitBootstrap(slug, parentFlag)`; on error refuse with `stepKindForBootstrapStage(stage)`. Discard `parent` with the blank identifier — `step` needs the bootstrap's effects, not its return value.
  5. Resolve `loomengine.LoomBootstrapLock(c.location)`, `os.MkdirAll` its parent, and `lock.AcquireWriteLock` it; on either error refuse with `stepKindBootstrap`. Call `c.ensureStatusStrand()`; on error release the lock and refuse with `stepKindBootstrap`. Release the lock immediately after the helper returns, **before** the producer call. Comment that the lock must never be held across a minutes-long LLM row, which is why it is released here rather than deferred to `RunE`'s return.
  6. `shed, err := c.buildLoomShed()`; on error refuse with `stepKindBootstrap`.
  7. `res, err := shed.Step(ctx)`. On a non-nil error, refuse with `stepKindBusy` when `errors.Is(err, shedengine.ErrShedBusy)` and with `stepKindProducer` otherwise. Comment that the early probe is an optimisation and not the authority — `shedengine.Step`'s own acquisition is what guarantees mutual exclusion, so a driver starting in the window between the probe and that acquisition is benign: the run is still refused, and the only cost is bootstrap work that is idempotent. Comment that a producer hard error mirrors `drive`'s handling of the same condition and that the status file already records `state: "failed"` with the error text, so the supervisor loses nothing.
  8. On success, compute `nextPolicy := loomshed.InterruptPolicyFor(res.Next)` and emit `clihelp.SetExit(ctx, output.Ok(out, stepEnvelope(res, nextPolicy, c.shedPaths.StatusPath)))`.

  Every refusal goes out as `clihelp.SetExit(ctx, output.ErrFields(out, <message>, map[string]any{"kind": <kind>}))` followed by `return nil` — never `output.Err`, and never a bare `return err`. Do not change `output.Err` or anything else in `internal/output`.
- **Commit:** `feat(loomcli): add the step verb driving exactly one producer per invocation`

### Card 11: register step and update the three tests that pin loom's verb set

- **Context:**
  - `internal/loomcli/step.go`
  - `internal/loomcli/wiring.go`
- **Edits:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/cli_test.go`
  - `internal/loomcli/wiring_test.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/loomcli/cli.go`, add `c.stepCmd()` to the `parent.AddCommand(...)` call. Add a `step` sentence to the parent command's `Long` prose describing it as the single-producer primitive an external supervisor drives, and add `lyx loom step` to the `Long`'s `Example` block. Do **not** add `"step"` to `verbUsesLightweightWiring` — `step` drives producers and so takes the full `wire()` path, including its early config refusal; add a sentence to `verbUsesLightweightWiring`'s doc comment recording that exclusion and its reason.

  In `internal/loomcli/cli_test.go`, `TestCommand_RegisteredVerbs_ExactSet` pins the registered set as an exact set. Add `"step"` to its `want` slice, keeping the slice sorted. Update the test's own doc comment where it says "exactly loom's six verbs" to say seven.

  In `internal/loomcli/wiring_test.go`, `TestVerbUsesLightweightWiring` pins the exact set of verbs that skip the full engine stack. Add a `{"Step", "step", false}` row to its table, so the exclusion is asserted rather than merely left to the `UnknownVerb` catch-all.

  In `cmd/lyx/helptree_test.go`, add `"step"` to the loom case's `wantSubs` slice.

  These three test edits are not accommodation of a refactor — each pins an exact, hand-maintained set that a genuinely new verb must join. They are distinct from `## Shared Decisions`' rule that batch 1's and batch 3's existing suites stay unedited, which governs behaviour-preserving extractions only.
- **Commit:** `feat(loomcli): register the step verb and pin it in loom's verb-set tests`

### Card 12: tests for step's envelope and refusal-kind decisions

- **Context:**
  - `internal/loomcli/step.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/cli_test.go`
  - `internal/loomcli/testmain_test.go`
  - `internal/loomshed/interruptpolicy.go`
  - `internal/loomshed/loomshed.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/status.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/step_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write untagged tests, per `## Shared Decisions`' new-tests-stay-untagged-and-pure rule.

  Assert the full envelope key set: call `stepEnvelope` with a populated `shedengine.StepResult` and confirm the returned map's key set is exactly the ten documented keys — build the expected set from a literal slice and compare both directions, so a key added without a test fails here. Assert `continue` is `true` exactly when `res.State` is `shedengine.StateRunning`, driving it across all five `State` values.

  Assert each field's mapping with a table over the routing shapes batch 1 pins: a `Done`-with-`OnDone` result (non-empty `producer`, `outcome` `"done"`, `next` naming the `OnDone` target, `state` `"running"`, `continue` true); a `Done`-terminal result (`state` `"done"`, `next` the row's own name, `continue` false); a `Stuck`-within-budget result; a blocked result carrying a non-empty `reason`; and an already-done short-circuit result (empty `producer`, empty `outcome`). Assert `history_length` equals `len(res.History)` and `status_file` is passed through verbatim; both are envelope key names, signature inlined, no file read needed.

  Assert `next_interrupt_policy` matches `loomshed.InterruptPolicyFor` for the row `next` names — checking at least one `reinvoke` row and `loomshed.NameWebster` for `handback` — and that it is the empty string when `res.Next` is empty.

  Assert the closed refusal vocabulary: `stepKinds` contains exactly the five declared values, every entry is non-empty and distinct, and each of the five constants appears in it. Assert `stepKindForBootstrapStage` maps `bootstrapStageSeed` to `stepKindUnseeded`, `bootstrapStageOwnership` to `stepKindOwnership`, `bootstrapStageOrigin` and `bootstrapStageCommit` to `stepKindBootstrap`, and `bootstrapStageNone` to `stepKindBootstrap` — so a new refusal cannot ship without a kind.

  Assert the `busy` refusal reaches the envelope with its remedy text: drive `stepCmd()`'s `RunE` against a hand-populated receiver whose `shedPaths.LockPath` points into a `t.TempDir()` and whose lock this test acquires first, then confirm the emitted JSON carries `"ok": false`, `"kind": "busy"`, and an `error` string mentioning `lyx loom pause`. Confirm in the same test that the refusal happened before the bootstrap, by asserting no status file exists at the receiver's `shedPaths.StatusPath` afterwards — the bootstrap would have seeded one. Use the in-process capture idiom `cli_test.go` already uses for the `drive` and `pause` refusal paths.
- **Commit:** `test(loomcli): cover step's envelope keys and closed refusal-kind vocabulary`

### Card 13: document the step verb in loom's two verb-enumerating doc surfaces

- **Context:**
  - `internal/loomcli/step.go`
  - `internal/loomcli/cli.go`
  - `manifest/designs/loom-step.md`
- **Edits:**
  - `docs/overview.md`
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `docs/overview.md`, the loom module bullet enumerates the verbs as `lyx loom run|drive|status|pause|validate-discussion|validate-plan`. Add `step` to that enumeration. Add a sentence in the same bullet's per-verb prose, placed after the `drive` sentence, describing `step` as the verb that bootstraps idempotently, drives exactly one producer, and emits a JSON envelope — never spawning the detached driver and never handing the terminal over. Leave the line listing loom's two registered interactive-handoff exceptions unchanged: `step` is **not** one of them, because it emits a JSON envelope and hands the terminal to nothing.

  In `manifest/designs/loom.md`, add one new row to the Module decomposition table — the `| Piece | Form | Notes |` table whose existing rows include `` `loom` (`lyx loom run`) ``, `` `lyx loom status` ``, and `/ly-*` skills. `loom.md` has no unified verb list to append to, unlike `docs/overview.md`'s module bullet, so this table row is the edit site rather than prose. Place the new row immediately after the `` `lyx loom status` `` row, with Piece `` `lyx loom step` ``, Form `a loom subcommand`, and a Notes cell describing it as the single-producer primitive an external supervisor drives: it bootstraps idempotently, drives exactly one producer, emits a JSON envelope, and never spawns the detached driver. Keep the cell on one line, per the repo's table convention. Do not touch the `/ly-*` skills row in this card — batch 6 owns that edit, and both cards editing it would collide.

  Write both edits in semantic line breaks per `CLAUDE.md`: one sentence per line, breaking inside a long sentence only at an internal independent-clause boundary, using plain newlines and never trailing double-spaces or a backslash. Every inline link added must resolve, file part and `#anchor` alike, per the Markdown Link Integrity invariant — `internal/lyxcwd`'s `docslink_test.go` is what enforces it and is in this batch's verify scope.
- **Commit:** `docs(loom): enumerate the step verb in overview.md and loom.md`

## Batch Tests

`verify: go test ./internal/loomcli/... ./cmd/lyx/... ./internal/lyxcwd/...` runs exactly the three packages this batch touches. `internal/loomcli` carries the new `step_test.go` plus the two edited verb-set tests; `cmd/lyx` carries the edited `helptree_test.go`; `internal/lyxcwd` carries `docslink_test.go`, the Markdown Link Integrity guard that covers card 13's doc edits.

Running all three packages rather than named test functions is the right scope here: card 11 changes the registered command tree, which `cli_test.go`'s `TestCommand_EveryCommandHasShort` walks in full, and card 10 adds a file to a package whose compile must stay clean for every other test in it.

The tagged `smoke` and `integration` suites are not in scope and are not edited. The repo-wide `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) covers the integration tier and every package outside these three before the task is marked done.
