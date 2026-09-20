# Batch: driver-launch

```yaml
task: 'Seeded driver choice: ly-drive strand as the child''s driver'
batch: driver-launch
number: 4
cards: 10
verify: go build ./... && go test ./internal/loomcli/... ./cmd/lyx/...
depends-on: [1, 2, 3]
```

## Batch Scope

This batch is the task: `lyx loom start` reads its own run's seed and, on `driver: llm`, boots a Claude strand running ly-drive instead of spawning the detached Go runner.
It delivers the widened spawn predicate, the driver-strand name, the per-attempt report path, the launch prompt, the spec composition, the two engine seams, the bounded pane-liveness probe, the branch in the verb body, and the new invariant that governs the read site it creates.
It is one batch because all of it is one branch and the decisions that branch is assembled from: every piece is reached only from step 5 of one command, none is independently useful, and splitting them would leave a half-wired seam that no test could exercise end to end.

The `llm` arm is unreachable in production until batch 5 lifts `shedrun.ValidateDriver`'s refusal, per the overview's `mechanism-ships-before-the-gate-is-lifted` decision.
Every card here is nonetheless fully tested: the predicates and composers are pure, and the branch itself is driven through the two injected seams against a synthetic `shedrun.Seed` value.

The external interface batch 6 consumes is `loomcli.AutonomousDriveStepCap`; batch 7 consumes the two seam fields.

Batch-local decision beyond the overview's: **the handshake stays the `go` path's alone.**
`awaitRunLock`, `bootstrapHandshakeAttempts` and `dispositionForHandshake` are not called, not widened and not refactored on the `llm` path, and card 16 asserts that absence explicitly.
An ly-drive session takes the run lock only inside each `lyx shed step` and releases it between steps, so a handshake on it would either race the Claude boot or observe a free lock between two perfectly healthy steps — converting a working driver into a refused bootstrap.

## Cards

### Card 8: the driver strand name and the widened spawn predicate

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/loomcli/start.go`
- **Edits:**
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/bootstrap_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `driverStrandDisplayName = "loom-driver"` to `internal/loomcli/bootstrap.go` beside `statusStrandDisplayName` and `operatorStrandDisplayName`, with a doc comment giving the same reason those two carry: reed's add has no upsert semantics, so every add and every lookup must agree on one byte-stable literal or a re-entrant bootstrap stacks a second pane instead of matching the first.
  Widen `mustSpawnDriver` to take a second parameter, `driverStrandLive bool`, returning true only when the run lock is free **and** no live driver strand exists.
  Amend its doc comment to state why the conjunction is required on **both** paths rather than one signal per path: an operator may run `lyx loom run` by hand against an `llm`-seeded run, so a bootstrap consulting the strand table alone would find no strand and launch a Claude driver alongside the live Go one; and a live ly-drive session between two steps holds no run lock, so a bootstrap consulting the lock alone would spawn a second driver into a worktree that already has one.
  Each signal is blind to exactly the driver kind the other sees.
  Add `driverStrandAction`, a three-value type with constants `driverStrandNone`, `driverStrandLive` and `driverStrandDead`, and `resolveDriverStrandAction(strands []reedengine.StrandStatus) (driverStrandAction, string)` returning the action and the matched strand's GUID, modelled on `resolveStatusStrandAction` and reusing `findStatusStrand` for the lookup.
  A dead strand resolves to `driverStrandDead` with its GUID so the caller can remove the corpse before relaunching; nothing here removes anything.
  In `bootstrap_test.go` give `mustSpawnDriver` a four-row truth table, and name the two mixed rows in the test names after the cases they encode: lock held with no strand is the hand-started `lyx loom run` case and must not spawn; lock free with a live strand is the ly-drive-between-steps case and must not spawn.
  Those two rows are the finding the predicate was widened for — a table covering only the two pure rows passes against the narrow single-argument version.
  Cover `resolveDriverStrandAction`'s three arms, and add a table test pinning that the name used to add a driver strand and the name looked up are the same constant, in the shape the status and operator strand names are already pinned.
- **Commit:** `feat(loomcli): widen the spawn predicate with the driver strand signal`

### Card 9: the per-attempt drive report path

- **Context:**
  - `internal/shedrun/paths.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/shuttleengine/spec.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/driverreport.go`
  - `internal/loomcli/driverreport_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomcli/driverreport.go` with a pure composer for the drive report's path, taking a clock and a random source as injected seams rather than reading either directly: `driverReportPath(l *lyxcwd.Location, runID string, now func() time.Time, rand func() string) string`.
  It returns `shedrun.ScratchDir(l, runID)` joined with a filename of the form `drive-report-<compact-timestamp>-<4-hex>.md`, where the compact timestamp comes from `now()` and the four hex characters from `rand()`.
  Compose the directory through `shedrun.ScratchDir` and never by spelling `.lyx` or `shed` in this package — the Lyxdirs Single-Declarer Invariant and the Shed Run-Directory Invariant both make that a `shedrun` obligation, and `ScratchDir` is the shipped accessor for exactly this placement.
  Add a second function, `newDriverReportRand() func() string`, returning the production random source; it is the only place in this package that reads a real entropy source, so the composer itself stays pure.
  The per-attempt suffix is what keeps a relaunch legal: `Spec.validate` rejects an output file that already exists, so a second bootstrap after a driver stopped must not name the report the first one wrote.
  Both halves of the suffix are load-bearing and neither is decoration — the timestamp is what makes a directory listing of past attempts readable in order, and the random component is what makes a same-second relaunch legal, which is not hypothetical: corpse removal plus a fresh start inside one second is exactly what batch 7's instantly-exiting stub pane produces.
  In `driverreport_test.go` the load-bearing case is **two calls under a frozen clock producing two different paths** — a test that advances the clock between calls proves only that the timestamp varies, which is the half that already worked, and would pass against a composer with no random component at all.
  Also assert the path lands under the run's ephemeral scratch directory, that the timestamp appears ahead of the random component in the filename, and that a fixed stub random source makes the whole path deterministic.
- **Commit:** `feat(loomcli): compose a per-attempt drive report path`

### Card 10: the autonomous step cap and the launch prompt

- **Context:**
  - `internal/shuttleengine/claudeengine/claudeengine.go`
  - `internal/shuttleengine/claudeengine/command.go`
  - `internal/shedrun/runid.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/driverprompt.go`
  - `internal/loomcli/driverprompt_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomcli/driverprompt.go` declaring the exported constant `AutonomousDriveStepCap = 120` and a pure composer `driverPrompt(runID string, reportPath string) string`.
  The constant is exported deliberately and the reason belongs in its doc comment: batch 6 pins the same number in the ly-drive skill's own autonomous section, no package under the plugins tree compiles Go, and a test outside this package cannot see an unexported identifier — so the constant is the single source the prompt interpolates and that test reads.
  State the derivation too, so the number is not a bare magic value: it is loom's own worst case as the skill already computes it, plus a margin that keeps an unlucky-but-legitimate run inside the budget.
  `driverPrompt` composes a **short pointer**, never a copy of the skill: the ly-drive skill invocation, the run-id, the report path the session must write at every stop condition, and an explicit statement that this session runs in autonomous mode with no operator to ask, carrying `AutonomousDriveStepCap` as the number of steps it runs under.
  Keep it short on purpose — the Claude engine caps a prompt at `maxLaunchPromptBytes`, declared in `internal/shuttleengine/claudeengine/command.go` and enforced in `internal/shuttleengine/claudeengine/claudeengine.go`, because the whole prompt expands into one command-line argument, and a prompt that grew into a copy of the skill would fail only at launch, after a bootstrap has already seeded and committed.
  Name no Claude flag and compose no command line here: this file produces prompt text alone, and the Shuttle Provider-Seam Invariant keeps provider specifics under the claude engine package.
  In `driverprompt_test.go` assert the prompt names the run-id, names the report path verbatim, states autonomous mode, and carries `AutonomousDriveStepCap`'s value interpolated rather than a hard-coded `120`.
  Assert its length stays well under that cap's value for a realistic run-id and report path — a bound check, not an exact-length pin.
- **Commit:** `feat(loomcli): compose the autonomous driver launch prompt`

### Card 11: the driver spec composition

- **Context:**
  - `internal/shuttleengine/spec.go`
  - `internal/loomengine/driver.go`
  - `internal/reedengine/render/types.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/driverprompt.go`
  - `internal/loomcli/driverreport.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/driverspec.go`
  - `internal/loomcli/driverspec_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomcli/driverspec.go` with a pure composer `driverSpec(prompt string, reportPath string, settings loomengine.DriverSettings) shuttleengine.Spec`.
  Give it a doc comment in the shape `operatorStrandAddSpec`'s already has — every non-default field pinned with its own reason, which is the standard this package holds.
  Pin these fields and no others: `Prompt` from the argument; `OutputFiles` a single-entry slice holding `reportPath`; `Model`, `Effort` and `Version` from `settings`, which carries the **resolved** triple; `NameOverride` set to `driverStrandDisplayName`, which is what the next bootstrap looks the strand up by; `Interactive` false, which is shuttle's autonomous posture and what adds the skip-permissions flag and the operator-prompt deny an unattended session needs; `ForkSubagents` false, because the driving loop reads envelopes and invokes a CLI and has no research fan-out to delegate; `Role` the literal `"driver"`, which is what the run directory records and what makes a driver run distinguishable from a producer round; `Round` empty, because a driver is not one round of anything; `Parent` empty, because the driver is top-level and the panes loom's producers spawn while it runs are its siblings; `Display.Anchor` below-parent; `Display.Focus` false; `Display.ShrinkWhenWaitingOnChild` false.
  Leave `Timeout`, `KeepPane` and `AwaitOperator` at their zero values and say in the doc comment that this is a statement that nothing reads them rather than a choice about pane retention or deadlines — all three are read only by the wait loop, which this path never enters.
  `Display.Anchor` must be below-parent and never hidden: the driver is the session an operator attaches to watch, and a hidden pane would make a run that may last hours legible only through its log file.
  `Display.Focus` is false for the reason `operatorStrandAddSpec` pins it false — the flag is persisted on the strand and re-evaluated on every subsequent add, so a true value would re-capture focus on every agent pane the run spawns afterwards.
  In `driverspec_test.go` pin every field listed above, each as its own named assertion.
  Assert the resolved triple reaches the spec as a provider model id plus its effort and version rather than a raw alias, by driving the composer with a `DriverSettings` value whose fields differ from any config string.
  Pin `KeepPane` and `AwaitOperator` at their zero values with the "nothing reads these, the wait loop is never entered" reason in the test's own comment, so a later reader does not mistake them for a pane-retention decision.
  Assert `Timeout` is left at zero and do **not** assert any resolved deadline — nothing on this path reads it, so an assertion there would pin a dead field.
- **Commit:** `feat(loomcli): compose the driver run's shuttle spec`

### Card 12: the two driver seams on loomCLI

- **Context:**
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/spec.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
- **Edits:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/wiring.go`
- **Creates:**
  - `internal/loomcli/driverlaunch.go`
  - `internal/loomcli/driverlaunch_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomcli/driverlaunch.go` declaring two interfaces and their production adapters, modelled on `runnerMasterStarter` in `internal/loomcli/cli.go`, which already adapts the same concrete runner value for the same reason.
  `driverStarter` has one method, `StartDriver(spec shuttleengine.Spec) (driverHandle, error)`, where `driverHandle` is a second small interface exposing `StrandGUID() string` and `RunDir() string`.
  `driverPaneProbe` has one method, `Strands() ([]reedengine.StrandStatus, error)`, and one more, `RemoveDriverStrand(guid string) error`.
  Write the production adapters in the same file: one over `*shuttleengine.Runner` whose `StartDriver` calls the runner's start and returns the resulting run, which already satisfies `driverHandle` given card 2's accessor; one over `*reedengine.Engine` whose `Strands` calls the engine's status and returns its strand slice, and whose `RemoveDriverStrand` calls the engine's remove with the cascade argument false.
  Note in that method's own comment that the cascade is inert for a driver strand, which is parentless and childless, so a reader does not wonder why the recursive form is not used.
  Add two fields to the `loomCLI` struct in `internal/loomcli/cli.go`, `driverStarter` and `driverPaneProbe`, each typed as its interface and each carrying a doc comment in the shape the struct's existing fields carry, stating that the seam exists because the Test Tier Purity Invariant bars a real spawn from an untagged file and both underlying engine values are concrete types.
  Populate both in `internal/loomcli/wiring.go` beside the existing runner and reed assignments, wrapping the values already constructed there — construct no second runner and no second reed engine.
  In `driverlaunch_test.go` assert that the two production adapters satisfy their interfaces, as compile-time assertions, and that the reed adapter's remove passes false for the cascade.
- **Commit:** `feat(loomcli): add the driver launch and pane probe seams`

### Card 13: the bounded pane-liveness probe

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/driverlaunch.go`
- **Edits:**
  - `internal/loomcli/driverlaunch.go`
  - `internal/loomcli/driverlaunch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add to `internal/loomcli/driverlaunch.go` a bounded probe, `awaitDriverPane(strands func() ([]reedengine.StrandStatus, error), guid string, wait func(), attempts int) (bool, error)`, plus the two constants bounding it, `driverPanePollInterval` and `driverPaneAttempts`, declared beside it with the same shape the handshake constants carry.
  Each iteration looks the strand up by GUID in the slice the seam returns and reports ready as soon as it is present and live; a strand that is absent or not live continues the loop; the seam erroring returns the error; exhausting the attempt budget reports not-ready with a nil error.
  Cap attempt **count**, not only elapsed time, per the Live-Substrate Spawn Observability invariant's retry clause, and take the wait as an injected seam so a test drives the whole poll with no wall-clock sleep — the same four-seam shape `awaitRunLock` already uses and the reason it needs no real clock.
  Keep the budget short: this probe answers "did the provider binary boot at all", which is a question settled in seconds, not the minutes the run-lock handshake allows for a full producer pass.
  Write the probe's doc comment to state both what it closes and what it does not.
  It closes the common half — binary absent, immediate exit, launch line rejected by the shell — using reed alone, with no new shuttle API and no change to the wait loop's contract, and it exists because starting a run proves the pane was created and the launch line sent into it, never that the provider booted.
  It does not close the case of a provider that boots, takes the pane, and then never reads its prompt: that leaves a live pane and a run that never moves, and this probe reports it ready.
  Name that residual in the comment and say where it degrades to — the outer run's watch budget lands in a blocked state, and an operator attaching sees a session sitting idle, which is legible in a way a dead pane is not.
  In `driverlaunch_test.go` cover the refusing case first: a strand whose pane is already dead must report not-ready within the budget.
  That case is the whole point of the probe — a test covering only the live case passes against no probe at all.
  Also cover a strand that is live on the first poll reporting ready immediately, a strand absent from the slice entirely being treated as not-ready rather than as an error, the seam erroring propagating that error, and the attempt budget being respected by counting calls rather than by measuring elapsed time.
- **Commit:** `feat(loomcli): probe the driver pane for liveness after launch`

### Card 14: the bootstrap's own seed write must preserve a recorded driver

- **Context:**
  - `internal/shedrun/seed.go`
  - `internal/shedrun/runid.go`
  - `internal/loomcli/cli.go`
- **Edits:**
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/sharedbootstrap_test.go`
  - `internal/loomcli/arm.go`
  - `internal/loomcli/start.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `loomSeedFor` in `internal/loomcli/sharedbootstrap.go` hard-codes the go driver into the seed `seedAndCommitBootstrap` writes at step 1b, and the seed writer refuses a disagreeing existing seed.
  As shipped, that combination makes this whole task unreachable: a run seeded for the llm driver is refused by `lyx loom start` at the seed-write stage, before step 5's branch is ever evaluated, with a message about a disagreeing seed rather than anything a reader would connect to driver choice.
  Fix it at the write: give `loomSeedFor` the driver to record rather than letting it choose one, and have `seedAndCommitBootstrap` read the existing seed first and pass that seed's recorded driver through when one is present, falling back to the go driver when no seed exists yet.
  This keeps the write idempotent in both directions — a first `lyx loom start` in an unseeded worktree still records the go driver exactly as today, and a start against a worktree seeded for either driver rewrites a byte-identical seed and no-ops.
  Return the effective driver from `seedAndCommitBootstrap` alongside the values it already returns, so card 15's branch consumes it directly rather than performing a second read of the file this function just wrote.
  That grows the function's return arity, so **both** of its call sites must be updated in this same card — this card's own commit must compile on its own, so neither may be deferred to card 15.
  `loomPreStep`'s call in `internal/loomcli/arm.go` discards the new value permanently: `step` spawns no driver at all, so a driver value reaching that path would be one nothing can act on.
  `startCmd`'s call in `internal/loomcli/start.go` discards it **for now**, in this card, purely to keep the build green; card 15 is what replaces that discard with the real consumer.
  Leave the rest of `start.go` untouched here — this card adds no branch and no driver behaviour to the verb body.
  Preserve the existing `Params` handling unchanged: the parent parameter is still the run's recorded startup choice, and the durable parent record stays where it is.
  Preserve the returned stage vocabulary and every existing refusal, the seed-ownership check among them.
  In `sharedbootstrap_test.go` cover: an unseeded worktree recording the go driver; a worktree already seeded for the go driver re-running as a no-op; and — the case this card exists for — **a worktree already seeded for the llm driver surviving the write and reporting that driver back**, which fails against the shipped version with a disagreeing-seed refusal.
  Add a case pinning that the driver is read from the seed rather than from any flag or config, since there is deliberately no driver flag on this command.
- **Commit:** `fix(loomcli): preserve a recorded driver when the bootstrap writes its seed`

### Card 15: the driver branch in the bootstrap

- **Context:**
  - `internal/shedrun/seed.go`
  - `internal/shedrun/paths.go`
  - `internal/shedrun/runid.go`
  - `internal/loomengine/driver.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/driverspec.go`
  - `internal/loomcli/driverprompt.go`
  - `internal/loomcli/driverreport.go`
  - `internal/loomcli/driverlaunch.go`
  - `internal/loomcli/cli.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/loomcli/start.go`
- **Creates:**
  - `internal/loomcli/start_driver_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Branch step 5 of the command `startCmd` builds in `internal/loomcli/start.go` on the seed's driver, leaving steps 1 through 4 and step 7 identical on both paths.
  Take the driver from the value card 14 added to `seedAndCommitBootstrap`'s return, which step 1 of this verb already calls — do **not** add a second `shedrun.ReadSeed` call here.
  That function has just written or found this run's seed, so re-reading the file it authored would be a second read of a value already in hand, and card 14's added return exists precisely to avoid it.
  This also keeps the Cwd Resolution Invariant satisfied for free: no path is resolved in this verb body at all, locally or otherwise.
  Per the seed contract an absent driver value already defaults to the go driver on read, so the branch is a two-value switch with the go driver as both the default and the zero-config answer.
  Feed the widened predicate two inputs of different provenance, and do not let the wording blur them: the run-lock probe is the one already in this verb's step 5 and is reused unchanged, while the strand read is **new** — a single call through the `driverPaneProbe.Strands()` seam card 12 introduces, added by this card.
  There is no strand read in today's `start.go` to reuse: the only pre-branch `reed.Status()` call in the bootstrap is the one inside `ensureStatusStrand`, which returns an error alone and never hands its strand slice back to the caller.
  Call the seam once and feed that one slice to both consumers — the widened predicate and, in the llm arm, the corpse check — rather than reading reed twice.
  The go arm keeps today's body byte-for-byte: the detached spawn, the log file, the reaper goroutine, and step 6's handshake, all unchanged and still gated on the predicate.
  The llm arm, in order: resolve the driver settings through the loom engine's driver resolver using the config and registry the receiver already carries; when the strand action is dead, remove the corpse through the probe seam before anything else; compose the report path; create the report's parent directory unconditionally with a mkdir-all immediately before composing the spec; compose the prompt and the spec; start the run through the starter seam; log the spawn at info level through the logger, as the detached spawn already does, per the Live-Substrate Spawn Observability invariant; then run the pane probe and refuse the bootstrap when it reports not-ready.
  Create the report's parent directory here rather than leaning on step 4's existing mkdir: that call's argument is the bootstrap lock's parent, and whether the predecessor task's relocation moved the bootstrap lock is a premise this task would be inheriting unverified.
  One unconditional mkdir-all on the directory this arm actually writes into is correct either way.
  Remove the corpse explicitly rather than reaching for the reed engine's relaunch-dead option: a dead driver's pane must not be relaunched with the old launch line, which points at the previous attempt's prompt file and its already-written report path, and a fresh run with a fresh report path is the only correct relaunch.
  Word the probe's refusal to name the run directory the handle reports and the strand GUID — never the driver log accessor, which names the detached Go driver's captured output and is written only by the go arm.
  Do **not** call the run-lock handshake on the llm arm, do not widen it, and do not refactor the two arms into a shared helper that reaches it.
  In `start_driver_test.go` drive the branch through the two seams with a synthetic seed value and a fake starter: assert the go driver value and an empty driver value both select the detached-spawn arm, and the llm value selects the strand launch.
- **Commit:** `feat(loomcli): branch the bootstrap on the seed's driver`

### Card 16: the llm arm's error paths and the absent handshake

- **Context:**
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/driverlaunch.go`
  - `internal/loomcli/cli.go`
  - `internal/lock/lock.go`
- **Edits:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/start_driver_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Audit every failure return inside the llm arm of the command `startCmd` builds in `internal/loomcli/start.go` and release the bootstrap lock explicitly on each, before reporting on the envelope — exactly as the go arm's own returns do, and never by deferring the release.
  The six failure sites are the driver-settings resolution, the strand read, the corpse removal, the report directory's mkdir-all, the run start, and the probe refusing.
  Each reports through the output package on the envelope, the same shape every other pre-flight failure in this verb uses.
  In `start_driver_test.go` assert the bootstrap lock is released on all six, each as its own case.
  A leaked bootstrap lock wedges every subsequent start in that worktree and is invisible until the second invocation, which is why these get their own card and their own case list rather than being left to review.
  Add the assertion this batch's scope note names: **the run-lock handshake is not reached on the llm arm.**
  Drive it by giving the test's own lock-held seam a counter and asserting it is never consulted on an llm-seeded bootstrap, so a later refactor that "unifies" the two arms fails here rather than silently reintroducing the race the handshake decision exists to avoid.
  Assert also that the corpse removal happens before the run start when the strand action is dead, and does not happen at all when the action is none — a relaunch that starts first and removes second would leave two strands under one name, which reed's add has no upsert semantics to reconcile.
- **Commit:** `fix(loomcli): release the bootstrap lock on every llm-arm failure`

### Card 17: the Driver Choice Single-Site Invariant

- **Context:**
  - `internal/shedrun/seed.go`
  - `internal/loomcli/start.go`
  - `internal/shedcli/table.go`
  - `internal/battencli/bootstrapverb.go`
  - `internal/battencli/arm.go`
  - `internal/battencli/wire.go`
  - `internal/loomcli/sharedbootstrap.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `internal/loomcli/bootstrap_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Record a new invariant in `CONSTRAINTS.md`, the **Driver Choice Single-Site Invariant**: a *recorded* seed driver value is read in exactly one place per recipe, that recipe's own bootstrap verb, and the branch on it selects a spawn and nothing else.
  No producer, no generic verb and no engine reads the recorded value, and no code path gates a refusal on it.
  State plainly in the invariant's own bullets that it governs the **read, not the vocabulary**: the driver constants are legitimately named at the seeding sites, which validate a flag before a seed exists, and those sites validate an argument rather than reading a written seed, so a naive "one consumer of the constants" rule would be false against this very task.
  Name the permitted constant consumers, which are four and were verified against the merged tree rather than assumed: the shed CLI's `seed` command, batten's own flag validation in `internal/battencli/arm.go`, batten's child-driver param reader in `internal/battencli/wire.go`, and loom's own seed writer in `internal/loomcli/sharedbootstrap.go`.
  The last two are the ones a shorter list would wrongly omit — neither reads a written seed's `Driver` field (batten's reads a `child_driver` *param*, loom's *writes* the field), so both are constant consumers rather than readers, which is exactly the distinction this invariant turns on.
  Give the rationale in one bullet: the recorded value is a startup choice, and the failure mode a second reader introduces is silent divergence between what a run was seeded as and what it is actually doing, which is unobservable from either the status file or the envelope — whereas a flag validator fails loudly at the command line, where a mistake is visible immediately.
  Record the mechanical proxy as a **tripwire, not a completeness proof**, in the wording the Completion Signal Invariant already uses for its own scan: adding a reader fails it and forces a human to confirm.
  Implement that proxy as a scan in `internal/loomcli/bootstrap_test.go` asserting that the only production reader of the seed's driver **field** outside the shedrun package is this package.
  Scan the field selector rather than the driver constants — the constants are legitimately named elsewhere, which is the distinction the invariant's own text draws, and a scan keyed on them would fail against the seeding sites this task deliberately keeps.
  Place the new section in `CONSTRAINTS.md` beside the other shed invariants rather than at the end of the file.
- **Commit:** `docs(constraints): record the Driver Choice Single-Site Invariant`

## Batch Tests

`verify: go build ./... && go test ./internal/loomcli/... ./cmd/lyx/...` runs the loom CLI's untagged suite plus the command tree's, on top of a whole-module build.
The command tree is in scope because card 15 changes the body of a registered command and because card 17's scan is a package test whose failure mode is a compile error when a scanned package moves.
The whole-module build is in scope for card 12, which adds two struct fields populated in the wiring layer — a field left unpopulated compiles here and fails as a nil-interface panic at the first llm bootstrap, so the build is the cheapest place to catch a wiring typo.

Four pieces of coverage in this batch are load-bearing beyond their size.
The widened predicate's two **mixed** truth-table rows are the finding the predicate was widened for, and a table covering only the two pure rows passes against the narrow single-argument version — which is exactly the two-drivers collision the conjunction exists to prevent.
The frozen-clock report-path case is the only test that can fail against a composer with no random component, since every other property of that path holds for a bare second-granular timestamp, and the collision it guards fires precisely under the automated relaunch batch 7 drives.
The probe's already-dead case is the whole point of the probe, and a suite covering only the live case passes against no probe at all.
The not-reached assertion on the run-lock handshake is the only thing pinning a deliberate asymmetry between the two arms; the handshake's absence on the llm path is a decision, and a refactor that unifies the arms would silently reintroduce a race that surfaces as a refused bootstrap on a perfectly healthy driver.

The six bootstrap-lock release cases are listed individually rather than sampled because the failure they guard is invisible on the invocation that causes it: the lock leaks, the command reports its real error, the operator fixes that error, and the *next* start hangs on a lock nobody is holding.
