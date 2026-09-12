# Batch: loomcli-shared-helpers

```yaml
task: "lyx loom step + external supervisor skill"
batch: "loomcli-shared-helpers"
number: 3
cards: 4
verify: go test ./internal/loomcli/...
depends-on: []
```

## Batch Scope

This batch extracts three blocks `step` must reuse verbatim into shared helpers, and rewrites `run` and `drive` to call them.
It is one batch because all three extractions are behaviour-preserving refactors of the same package, verified by the same existing suites, and splitting them would leave `internal/loomcli` half-refactored at a batch boundary for no benefit.
It delivers no observable behaviour change: `run`'s and `drive`'s output, ordering, and exit codes are identical before and after.
The external interface batch 4 consumes is the three methods `seedAndCommitBootstrap`, `ensureStatusStrand`, and `buildLoomShed`.

Batch-local decision: `seedAndCommitBootstrap` returns a `bootstrapStage` alongside its error. `run` ignores the stage and writes `output.Err` exactly as today, so its behaviour is unchanged; `step` maps the stage onto its own `kind` vocabulary in batch 4. The alternative — having `step` re-run the sub-steps itself to learn which one failed — would duplicate the very block this batch exists to share.

## Cards

### Card 6: extract the seed-and-commit bootstrap block

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/seedinput.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomengine/config.go`
  - `internal/loomshed/seed.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/loomcli/run.go`
- **Creates:**
  - `internal/loomcli/sharedbootstrap.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/loomcli/sharedbootstrap.go` holding the blocks `run` and `step` share. Open it with a file comment stating that every helper here is lock-agnostic: none acquires or releases `loomengine.LoomBootstrapLock`, because the lock's position differs between the two calling verbs, and each verb wraps its own window around these calls.

  Declare an unexported `type bootstrapStage int` with four constants — `bootstrapStageOrigin`, `bootstrapStageSeed`, `bootstrapStageOwnership`, `bootstrapStageCommit` — plus a zero-valued `bootstrapStageNone` declared first so the zero value means "no failure". Document that the stage exists so a caller can classify a failure without re-running the sub-steps, that `run` deliberately ignores it, and that `step` maps it onto its own refusal-kind vocabulary.

  Add `func (c *loomCLI) seedAndCommitBootstrap(slug, parentFlag string) (string, bootstrapStage, error)` holding today's steps 1 through 3 from `run.go`'s `RunE`, verbatim and in today's order, with every load-bearing comment carried over: `fabricengine.ReadOrigin(c.location)`; `resolveParentBranch(recorded, found, parentFlag)`; the conditional `fabricengine.WriteOrigin` with its throwaway `fabricengine.NewMutations("")` recorder and the comment recording why loom's envelope gains no mutation keys; `loomshed.Seed(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug, parent)` tolerating exactly `loomshed.ErrSeedExists` via `errors.Is`, with the comment recording why a stat-then-seed probe would reintroduce the race the seeder's single lock closes; `loomengine.VerifySeedOwnership(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug)` with its crucible-round provenance comment; and the unconditional `fabricengine.CommitAnchoredPaths` over `[]string{loomengine.LoomStatusRel(), fabricengine.OriginRecordRel()}` with `fabricengine.NewMutations("")`, the message `fmt.Sprintf("loom: seed session bootstrap for %s", slug)`, and `fabricengine.EnvSyncOptions()` — carrying the comment explaining why the commit is unconditional on every invocation and why it must precede any producer call.

  Return the resolved `parent` on success with `bootstrapStageNone` and a nil error. On failure return the empty string, the stage that failed, and the error unwrapped: `bootstrapStageOrigin` for both the `ReadOrigin` and `resolveParentBranch` and `WriteOrigin` failures, `bootstrapStageSeed` for a `Seed` error that is not `ErrSeedExists`, `bootstrapStageOwnership` for a `VerifySeedOwnership` failure, and `bootstrapStageCommit` for a `CommitAnchoredPaths` failure.

  In `run.go`, replace today's steps 1 through 3 with a single call to this helper, discarding the stage with the blank identifier and keeping the existing envelope shape exactly: `clihelp.SetExit(ctx, output.Err(out, err.Error()))` followed by `return nil`. Keep `slug := seedSlug(c.location.WorktreeName)` in `run.go` and pass it in. Remove from `run.go` any import that the extraction leaves unused, and add none.
- **Commit:** `refactor(loomcli): extract run's seed-and-commit bootstrap into a shared helper`

### Card 7: extract the status-strand block

- **Context:**
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/cli.go`
  - `internal/reedengine/render/types.go`
  - `internal/shell/shell.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/loomcli/run.go`
  - `internal/loomcli/sharedbootstrap.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func (c *loomCLI) ensureStatusStrand() error` to `internal/loomcli/sharedbootstrap.go`, holding `run.go`'s step-4 strand work verbatim and in today's order, starting after the bootstrap-lock acquisition and ending before the run-lock probe: `c.reed.Up()`; `c.reed.Status()`; `resolveStatusStrandAction(statusResult.Strands)`; the `statusStrandReplace` branch calling `c.reed.RemoveStrand(staleGUID, false)` and degrading a removal failure to `logger.Warn` with today's exact message, key names, and the follow-on assignment of `statusStrandKeep`; and the `statusStrandAdd` branch building today's `reedengine.AddSpec` with `NameOverride: statusStrandDisplayName`, `Cmd: statusStrandCmd(shell.ForGOOS(), exe)` over `os.Executable()`, and `Display: render.Display{Anchor: render.AnchorBelowParent, ShrinkWhenWaitingOnChild: true}`, then `c.reed.AddStrand(addSpec)`. Carry every existing comment, including the one recording why a dead entry is replaced rather than added over.

  Return the first error encountered, unwrapped, and nil on success. The helper must not acquire or release the bootstrap lock and must not call `bootstrapLock.Release()` — the caller owns that entirely.

  In `run.go`, replace the extracted block with `if err := c.ensureStatusStrand(); err != nil { _ = bootstrapLock.Release(); clihelp.SetExit(ctx, output.Err(out, err.Error())); return nil }`. This preserves today's behaviour exactly: every error path inside the old block released the bootstrap lock before writing its envelope, and the single call site now does that once. Leave the `bootstrapLockPath` resolution, its `os.MkdirAll`, the `lock.AcquireWriteLock` call, and the comment recording why the lock is released explicitly rather than deferred exactly where they are in `run.go`. Remove from `run.go` any import the extraction leaves unused, and add none.
- **Commit:** `refactor(loomcli): extract run's status-strand block into a shared helper`

### Card 8: extract drive's shed-construction block

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/landingdeps.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/fabricengine/open.go`
  - `internal/fabricengine/origin.go`
- **Edits:**
  - `internal/loomcli/drive.go`
  - `internal/loomcli/sharedbootstrap.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func (c *loomCLI) buildLoomShed() (*shedengine.Shed, error)` to `internal/loomcli/sharedbootstrap.go`, holding `drive.go`'s shed-construction block verbatim and in today's order: `fabricengine.Open(c.location)`; `handle.CurrentBranch()`; `handle.OriginURL()` with its existing degrade-to-empty-string-on-error behaviour and the comment recording the scalar-read-errors-refuse-or-defer-by-consumer rule; `fabricengine.ReadOrigin(c.location)`; `resolveLandingParent(recorded, found, taskBranch)`; `syncOpts := fabricengine.EnvSyncOptions()`; the `pushBranch` closure over `handle.PushBranch(syncOpts)`; the assignment of `landingDeps(c.location, c.runDeps.Geom, taskBranch, originURL, parentBranch, syncOpts.SkipPush, pushBranch, c.registry, c.runner, c.landingCfg)` to `c.env.Landing`; and `loomrecipe.New(c.env, c.shedPaths)`. Return the constructed shed and a nil error, or a nil shed and the first error encountered, unwrapped.

  In `drive.go`, replace the extracted block with a single call to this helper, keeping the existing envelope shape on error. `drive` keeps everything else unchanged: its two pre-flight refusals (the status-file stat naming `lyx loom run` as the remedy, and the `VerifySeedOwnership` check), its own `c.reed.Up()` call with the comment explaining why `drive` needs the substrate despite adding no strand, its `shed.Run(cmd.Context())` tail with the comment about `ErrShedBusy` being an ordinary error envelope, and its four-key success envelope. Only the block that builds the shed moves. Remove from `drive.go` any import the extraction leaves unused, and add none.

  This is behaviour-preserving for `drive` in exactly the sense the `run` extractions are for `run`: `drive` keeps its own pre-flight refusals and its `shed.Run(ctx)` tail unchanged.
- **Commit:** `refactor(loomcli): extract drive's shed construction into a shared helper`

### Card 9: direct unit tests for the three extracted helpers

- **Context:**
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/cli.go`
  - `internal/loomcli/cli_test.go`
  - `internal/loomcli/bootstrap_test.go`
  - `internal/loomcli/wiring_test.go`
  - `internal/loomcli/testmain_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomshed/seed.go`
  - `internal/loomengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/sharedbootstrap_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write untagged tests driven against a hand-populated `loomCLI` receiver, bypassing `wire` entirely — the idiom `cli_test.go` already uses for the `drive` and `pause` refusal paths. Per `## Shared Decisions`' new-tests-stay-untagged-and-pure rule, no test here may spawn git, build a real hub, or bring up a real reed session; anything needing those belongs to the existing tagged suites, which this card does not touch.

  Cover the `bootstrapStage` classification directly: assert that the four stage constants are distinct, that `bootstrapStageNone` is the zero value, and that every stage a caller can receive is one of the five declared constants. Assert the seed-tolerance contract by calling `seedAndCommitBootstrap` twice in a row against a receiver whose `shedPaths` point into a `t.TempDir()` and confirming the second call does not fail on `loomshed.ErrSeedExists` — stub the fabric-touching tail by asserting the stage reached is `bootstrapStageCommit` rather than `bootstrapStageSeed`, which proves seeding was idempotent and the failure came later, at the step that genuinely needs a real fabric.

  Cover `buildLoomShed`'s output shape with one direct test asserting the built `*shedengine.Shed`'s `StatusPath`, `LockPath`, and `StatusLockPath` equal the receiver's own `c.shedPaths` values, and that `len(shed.Producers)` is seventeen. Skip the test with `t.Skip` and a stated reason if a prerequisite this pure path cannot satisfy is absent, rather than reaching for a real fabric.

  Cover `ensureStatusStrand`'s three `resolveStatusStrandAction` branches through that pure function directly — keep, add, and replace-a-dead-entry — rather than through a real reed engine, since `resolveStatusStrandAction` is where the whole decision lives and `bootstrap_test.go` already establishes that idiom. Assert the removal-failure degradation is a warning rather than a hard failure by reading `ensureStatusStrand`'s own control flow requirement: a `RemoveStrand` error reassigns the action to `statusStrandKeep` and the helper returns nil.
- **Commit:** `test(loomcli): cover the three extracted bootstrap helpers directly`

## Batch Tests

`verify: go test ./internal/loomcli/...` runs the whole untagged `loomcli` package. That scope is deliberate and is the justification the `verify-full-suite` carve-out asks for: this batch rewrites the bodies of `run` and `drive`, and the package's existing suites — `bootstrap_test.go`, `cli_test.go`, `wiring_test.go`, `wiring_commitstatus_test.go`, `parity_test.go`, `seedinput_test.go`, `landingdeps_test.go`, `validate_test.go` — are collectively the only guarantee the extraction is behaviour-preserving. Narrowing to the one new file would skip every one of them. A single Go package's untagged tests run in seconds.

The tagged suites (`smoke_test.go`, `smoke_attachprobe_test.go` under `//go:build smoke`; `wiring_commitstatus_integration_test.go` under `//go:build integration`) are not in this batch's verify scope and are not edited by it. The repo-wide `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) covers the integration tier before the task is marked done.

The existing suites must pass unedited. `sharedbootstrap_test.go` is the one new file.
