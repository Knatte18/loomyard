# Batch: module-rearm

```yaml
task: "Shed-generic watchdog for ly-drive and loom's CLI verbs"
batch: "module-rearm"
number: 4
cards: 8
verify: go test ./internal/loomcli/... ./internal/lifecyclecli/... ./internal/shedverbs/... && go test -tags integration ./internal/loomcli/... ./internal/lifecyclecli/...
depends-on: [1, 2, 3]
```

## Batch Scope

This batch removes the second duplication: `loomcli`'s four verb bodies and `lifecyclecli`'s two stop being hand-written and become arming over `shedverbs`, so there is exactly one body per verb in the tree.
Both modules gain an exported `Arm(cwd, verb, args)` that factors their existing `PersistentPreRunE` resolution out into one name-independent entry point, which batch 5's `lyx shed` table calls — without it, `lyx shed run --recipe lifecycle` would silently lose lifecycle's `lyxcwd.Resolve`, its `fabricengine.PrimeName` lookup, its non-prime refusal and its `args[0]` slug read, because the hooks cobra fires are the executed command's *ancestor* chain and `lifecycle` is not an ancestor of `shed run`.

The whole batch is behaviour-preserving apart from the three additive changes the overview's `no-surface-change-to-existing-verbs` Shared Decision names.
The proof is that `internal/loomcli`'s `step_test.go`, `status_test.go`, `wiring_test.go`, `stephandoff_test.go`, `start_watchdog_test.go`, `friction_test.go`, `selfreport_test.go` and `internal/lifecyclecli`'s `run_test.go`, `cli_test.go`, `paths_test.go`, `refusal_test.go`, `wire_test.go` and `lifecycle_integration_test.go` keep passing;
`internal/loomcli`'s four `//go:build smoke` suites — `smoke_test.go`, `smoke_bootstrapwiring_test.go`, `smoke_attachprobe_test.go` and `smoke_operatorstrand_test.go` — are deliberately not on that list and not in this batch's `verify:`, because the smoke tier spawns real sessions and is not run per-batch anywhere in this repo's pipeline;
the only assertion changes permitted in this batch are the ones card 27 justifies one by one.

It depends on batch 1 for `shedbuild.ShedPaths` (both `Arm` bodies fill one) and on batch 3 for `shedverbs` itself.
It also depends on batch 2, which the first draft of this plan wrongly denied: batch 2's `Env.LoomRun` rename edits `internal/lifecyclecli/run_test.go` and `internal/lifecyclecli/lifecycle_integration_test.go`, and card 27 below edits both of the same files, so the two batches must be sequenced rather than run in parallel.
`Arm` still lives in a new `arm.go` in each package — it is a new exported seam that deserves its own file rather than being buried in an existing one — but that alone was never enough to make the two batches independent.

Batch-local decision: `verbUsesLightweightWiring` stays unexported and `loomcli.Arm` is its sole caller, routing `status` and `pause` through `wireLightweight` and `run`/`step` through the full `wire`.
A verb-blind arming keyed on recipe name alone would run the full `wire()` for `lyx shed status --recipe loom` and reintroduce the broken-config hazard that lightweight path exists to avoid.

## Cards

### Card 21: add loomcli.Arm and hold a spec pointer on the receiver

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/verbs.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomengine/config.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/shedbuild/newshed.go`
  - `internal/loomrecipe/loomrecipe.go`
- **Edits:**
  - `internal/loomcli/cli.go`
- **Creates:**
  - `internal/loomcli/arm.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** add a `spec *shedverbs.Spec` field to the `loomCLI` struct in `internal/loomcli/cli.go`, initialised to a non-nil zero `&shedverbs.Spec{}` in `newLoomCLI`, so `Command()` can hand the same pointer to `shedverbs.Verbs` and the pre-run can fill it in place.
  In the new `internal/loomcli/arm.go`, declare **two** functions, because one fixed signature cannot serve both callers.
  The worker is an unexported `func (c *loomCLI) arm(cwd string, verb string, args []string) (shedverbs.Spec, error)` on the receiver: it resolves `cwd` through `lyxcwd.Resolve`, routes through `c.wireLightweight` when `verbUsesLightweightWiring(verb)` and through `c.wire` otherwise, and then returns `c.specFor(verb), nil`.
  Declare `specFor` as its own resolution-free method — `func (c *loomCLI) specFor(verb string) shedverbs.Spec` — holding the whole `Spec` fill described below and performing no resolution, no `wire`, and no I/O, with every hook closing over that same `c`.
  The split is required by the overview's `spec-fill-is-separable-from-resolution` Shared Decision: `internal/loomcli`'s untagged `cli_test.go`, `status_test.go` and `step_test.go` drive leaf commands against hand-populated receivers, and after the rearm those commands read `*c.spec`, so they need a way to fill it that does not spawn git.
  They call `c.specFor(verb)` and assign it into their own `c.spec`;
  card 22, 23 and 28's retargeting instructions below assume that seam exists.
  The exported seam is a thin wrapper `func Arm(cwd string, verb string, args []string) (shedverbs.Spec, error)` that constructs a fresh receiver via `newLoomCLI` and returns `c.arm(cwd, verb, args)`;
  it exists for `internal/shedcli`'s table, whose `entry.Arm` field is exactly this type.
  The split is required, not stylistic: `resolvePersistentPreRun` must wire **its own** `c`, because `internal/loomcli/start.go` reads thirteen receiver fields and `internal/loomcli/validate.go` reads `c.env.DecisionRecordPath`, `c.env.SupportLogPath`, `c.env.AnchorPath` and `c.env.WorktreeRoot` — all four verbs that depend on that population (`start`, `validate-discussion`, `validate-plan`, and `step`'s own bootstrap helpers) would break if the pre-run armed a throwaway receiver and assigned only `*c.spec`.
  `Arm` carries no command-name guard: the existing `cmd.Name() == "loom"` short-circuit stays behind in `resolvePersistentPreRun`, where it belongs — it exists to let the bare group listing run without a git repository, and a bare `lyx shed` listing is `shedcli`'s own equivalent guard, not this module's.
  Fill the returned `Spec` from the receiver: `StatusPath`/`LockPath`/`StatusLockPath` from `c.shedPaths`;
  `EnsureStatusLockDir: true`, because both of loom's read-only verbs call `ensureStatusLockDir` today and must keep doing so;
  `StatusLabel: "loom"`;
  `DecodeErrPrefix: "loom:"`;
  `RunBusyMessage: ""` and `StepBusyMessage: ""`, both passthrough, because loom's `run` reports the bare sentinel text as an ordinary error envelope today and `step` reports `err.Error()` verbatim alongside its kind;
  `StepBusyKind: shedverbs.KindBusy`;
  `AbsentStatus` refusing with the exact existing text `loom: no status file at <StatusPath>; run "lyx loom start" first to bootstrap this task`;
  `PauseAbsentMessage` set to the exact existing text `loom: no status file at <StatusPath>; there is nothing running to pause -- run "lyx loom start" first to bootstrap this task`.
  Leave `BuildShed` nil on the lightweight path — `status` and `pause` never call it, which is what keeps them off `wire()`.
  On the full path, fill `BuildShed` **per verb**, because loom's two producer-driving verbs build their Shed differently today and collapsing them would change behaviour:
  for `step`, a closure returning `c.buildLoomShed()`;
  for `run`, a closure returning `loomrecipe.New(c.env, c.shedPaths)` directly.
  The distinction is load-bearing: `internal/loomcli/sharedbootstrap.go`'s `buildLoomShed` already performs the whole `fabricengine.Open`/`CurrentBranch`/`OriginURL`/`ReadOrigin`/`resolveLandingParent` block and the `c.env.Landing = landingDeps(…)` assignment before calling `loomrecipe.New`, while `run` performs that same block inline in its own pre-flight and then calls `loomrecipe.New` itself.
  Giving `run` a `buildLoomShed`-backed `BuildShed` would open the fabric and read origin twice where it does so once today.
  Declare `loomVerbTexts` in this card too, as a package-level `shedverbs.VerbTexts` value in `internal/loomcli/cli.go`, lifting the four verbs' `Use`, `Short` and `Long` strings verbatim out of the still-present `runCmd`/`stepCmd`/`statusCmd`/`pauseCmd` constructors — including every `Example:` block and every embedded newline.
  It is lifted **here**, before cards 22 and 23 delete those constructors, precisely so the copy is made from live source rather than from git history.
  Rewrite `resolvePersistentPreRun` to call `c.arm(cwd, cmd.Name(), args)` — the unexported worker, on its own receiver — and assign `*c.spec = armed` on success, reporting an error exactly as it reports a `wire` error today: `output.Err(out, err.Error())` followed by `clihelp.Abort(ctx, 1)`.
  Because `arm` routes through `c.wire`/`c.wireLightweight`, the pre-run still populates `c` exactly as it does today, so `start`, `validate-discussion` and `validate-plan` keep working unchanged even though none of them is a `shedverbs` verb and none reads `c.spec`.
  It must keep reading cwd through `lyxcwd.CwdFrom(ctx)` and keep passing `lyxcwd.Resolve`'s error through bare rather than doubling text on top of it, since that error is already the self-describing "not a git repository" sentinel.
  The single-receiver property is what the two-function split buys and must be preserved: the `*loomCLI` whose fields the returned hooks close over is always the same value the caller holds — the pre-run's own `c` on the `lyx loom` path, and the wrapper's freshly-constructed one on the `lyx shed` path.
- **Commit:** `feat(loomcli): add the exported Arm resolution entry point`

### Card 22: rearm loomcli's run and step over shedverbs

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/run.go`
  - `internal/shedverbs/step.go`
  - `internal/shedverbs/verbs.go`
  - `internal/loomcli/arm.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/landingdeps.go`
  - `internal/loomcli/selfreport.go`
  - `internal/loomshed/interruptpolicy.go`
  - `internal/shedengine/shed.go`
- **Edits:**
  - `internal/loomcli/run.go`
  - `internal/loomcli/step.go`
  - `internal/loomcli/arm.go`
  - `internal/loomcli/cli.go`
  - `internal/loomcli/cli_test.go`
  - `internal/loomcli/step_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** delete `runCmd` and `stepCmd` from `internal/loomcli/run.go` and `internal/loomcli/step.go`, along with `stepEnvelope`, the five `stepKind*` constants, the `stepKinds` slice and `stepKindForBootstrapStage`'s *mapping targets* — `stepKindForBootstrapStage` itself stays, retargeted onto `shedverbs.KindUnseeded`/`shedverbs.KindOwnership`/`shedverbs.KindBootstrap`, because it maps loom's own `bootstrapStage` vocabulary and belongs here.
  Keep `shouldReflectFriction` and `reflectFriction` in `run.go`: both are loom's own and are now called from the `PostRun` hook.
  Move the body of the deleted `runCmd` into a `PreRun` hook and a `PostRun` hook filled by `Arm`, preserving the existing ordering exactly.
  `PreRun` keeps `run`'s existing pre-flight block, which is the one `run` performs today and which its own `BuildShed` therefore must not repeat (card 21).
  It runs, in this order: the status-file existence refusal naming `lyx loom start`;
  `loomengine.VerifySeedOwnership`;
  `observeEntry`, still guarded on the `selfreport` knob directly so a disabled run pays for no lock probe and no extra status decode;
  `c.reed.Up()`;
  `fabricengine.Open`/`CurrentBranch`/`OriginURL`/`ReadOrigin`/`resolveLandingParent`;
  the `c.env.Landing = landingDeps(…)` assignment;
  `friction.EnsureDir(c.frictionDir)`.
  Each of those steps' existing error handling becomes a returned error from `PreRun`, which the generic body reports on the error envelope — the same envelope they produce today.
  `OriginURL`'s existing degrade-to-empty-string behaviour stays: only Publish reads it, and only when a pull request is actually required, so an unusable origin URL must not refuse `run` itself.
  Carry the entry observation from `PreRun` to `PostRun` on the `loomCLI` receiver, which is how lifecycle's `abandonedSession` already travels, because `PreRun` returns no map.
  `PostRun` calls `detectAndFileAnomalies(…)` with `RunErr: runErr` unconditionally, then returns `map[string]any{"friction": frictionStatus}` where `frictionStatus` starts at `frictionengine.StatusSkipped` and becomes `c.reflectFriction()` only when `shouldReflectFriction(c.frictionDir, result.Outcome)` is true.
  `friction` is returned unconditionally on the success path, valued `skipped` when the reflection does not fire — gating the key instead of the call would drop it on `RunPaused` and silently break the envelope.
  Move the body of the deleted `stepCmd` into a `PreStep` hook: the `os.MkdirAll(filepath.Dir(c.shedPaths.LockPath), 0o755)`, the non-blocking run-lock probe released immediately when free, `c.seedAndCommitBootstrap`, and the bootstrap lock around `ensureStatusStrand` released *before* the producer call — each returning its existing refusal kind alongside its error.
  The `MkdirAll` is part of the probe rather than an accident of ordering, and treating a missing-parent error as "lock free" stays explicitly rejected: it would convert a filesystem fault into a false green light on the one check guarding against a second driver.
  The busy refusal `PreStep` raises when the run lock is held keeps its exact existing text naming `lyx loom pause` as the remedy — that is the early probe's own message and is not the `ErrShedBusy` told message, which stays passthrough;
  conflating the two would silently reword a shipped envelope.
  Fill the `PostStep` hook batch 3 declared with loom's `recordStepHandoff(loomengine.LoomStepHandoff(c.location), loomengine.LoomStepHandoffLock(c.location), len(res.History), res.State)` call, which is what keeps that marker at its required position — after a successful `shed.Step` and before the envelope is reported.
  Fill `InterruptPolicyFor` with `loomshed.InterruptPolicyFor`, which is the table `next_interrupt_policy` reads today.
  Preserve the comment recording that the early probe is an optimisation and `shedengine.Step`'s own acquisition is the authority.
  Retarget the in-package test call sites these deletions orphan, which is mechanical compilation repair rather than an assertion change: `internal/loomcli/cli_test.go`'s `TestVerbRefusals` table uses the method expression `(*loomCLI).runCmd` (its other two entries, `pauseCmd` and `statusCmd`, are card 23's), and `internal/loomcli/step_test.go` calls `stepEnvelope`, `stepKinds`, `stepKindBusy` and `c.stepCmd()`.
  Each of those tests hand-populates its receiver and bypasses the pre-run, so each must now also fill `c.spec` from `c.specFor(<verb>)` before executing the command — that is the tier-1 seam card 21 declares, and without it the rearmed commands read a zero `Spec`.
  Point each at its new home — `shedverbs.StepEnvelope`, `shedverbs.StepKinds`, `shedverbs.KindBusy`, and the command `shedverbs.Verbs` returns — keeping every assertion's meaning identical.
  Batch 3 exports those three identifiers precisely so these tables retarget rather than being deleted;
  `internal/loomcli/step_test.go`'s ten-key and five-kind closure assertions must keep asserting the same closed sets, and none of them may be dropped here.
  Route `--parent` through a named carrier rather than a closure over a local, because the local it is captured from today (`parentFlag` in `stepCmd`) disappears with that function: add a `parentFlag string` field to the `loomCLI` struct, have `Command()` bind the returned `step` command's flag to `&c.parentFlag` with `cmd.Flags().StringVar`, and have the `PreStep` hook read `c.parentFlag` when calling `c.seedAndCommitBootstrap(slug, c.parentFlag)`.
  A closure cannot carry it: after the move the flag variable lives in `Command()` while `PreStep` is built inside `arm`, which sees only `(cwd, verb, args)` — and a `PersistentPreRunE`'s `args` are positional only, never parsed flags.
  Keep the flag's existing help text byte-for-byte.
  State the disposition on the other path explicitly: `lyx shed step --recipe loom` registers no `--parent` flag of its own, so `c.parentFlag` is the empty string there — which is exactly the value `lyx loom step` passes when the operator omits the flag, so the two paths agree and the provenance record is simply never written from the `shed` path.
  Record that in `arm.go`'s doc comment, since it is a real behavioural difference between the two entry points rather than an oversight.
- **Commit:** `refactor(loomcli): rearm run and step over shedverbs hooks`

### Card 23: rearm loomcli's status and pause over shedverbs

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/status.go`
  - `internal/shedverbs/pause.go`
  - `internal/shedverbs/verbs.go`
  - `internal/loomcli/arm.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomshed/interruptpolicy.go`
  - `internal/loomengine/status.go`
  - `internal/shedengine/status.go`
- **Edits:**
  - `internal/loomcli/status.go`
  - `internal/loomcli/pause.go`
  - `internal/loomcli/arm.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/status_test.go`
  - `internal/loomcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** delete `statusCmd`, `pauseCmd`, `renderStatusLine`, `statusUnavailableLine` and `printStatusLinesOnChange` from `internal/loomcli/status.go` and `internal/loomcli/pause.go`, leaving each file holding only what loom still owns.
  Fill a `StatusExtras` hook in `Arm` returning loom's own five keys and no others: `pause_requested` from `st.PauseRequested`, `history_length` from `len(st.History)`, `slug` and `parent` from a `json.Unmarshal` of `st.Product` into `loomengine.Status`, and `interrupt_policy` from `loomshed.InterruptPolicyFor(st.CurrentProducer)`.
  Perform the unmarshal only when `len(st.Product) > 0`, exactly as today, and on a failure return the existing error text `loom: decode status file <StatusPath>'s product payload: <err>` — returned verbatim from the hook, never re-prefixed by the generic body, which is what keeps it from being double-prefixed.
  `interrupt_policy` is a plain map read keyed by a name this verb already has, needing no config load and no engine construction, which is what keeps `status` on the lightweight path.
  Its empty-string value when `current_producer` names no row is the caller's "no entry" signal and never a third policy value.
  If `internal/loomcli/status.go` ends up holding no declaration at all after the deletions, delete the file rather than leaving an empty one, and record that in the commit message;
  the same applies to `pause.go`.
  Give `internal/loomcli/sharedbootstrap.go`'s `ensureStatusLockDir` a disposition too: deleting `statusCmd` and `pauseCmd` removes its only two callers, so delete the function along with them rather than leaving dead code.
  Its `MkdirAll` is not lost — `shedverbs`' own `ensureStatusLockDir` performs it, gated on the told `EnsureStatusLockDir` boolean — and the crucible history its doc comment records was already carried into that function's own doc comment by batch 3, card 13.
  Confirm before deleting that no third caller has appeared since this plan was written.
  Retarget the in-package test call sites these deletions orphan, which is mechanical compilation repair rather than an assertion change: `internal/loomcli/status_test.go` calls `renderStatusLine`, `statusUnavailableLine`, `printStatusLinesOnChange` and `c.statusCmd()`, and `internal/loomcli/cli_test.go` uses the `(*loomCLI).statusCmd` and `(*loomCLI).pauseCmd` method expressions.
  Point each at `shedverbs.RenderStatusLine`, `shedverbs.UnavailableLine`, `shedverbs.PrintStatusLinesOnChange` and the command `shedverbs.Verbs` returns.
  `cli_test.go`'s `pauseCmd` and `statusCmd` table entries belong to this card;
  like card 22's, each hand-built receiver must fill `c.spec` from `c.specFor(<verb>)` before executing, per card 21's tier-1 seam.
  The first two now take a label argument (batch 3, card 14), so those call sites pass `"loom"` and keep asserting the identical rendered strings — which is exactly what card 28 requires `status_test.go` to keep pinning, so none of these tables may be deleted.
  `TestStatusCmd_EnvelopeKeySet` stays and keeps asserting loom's own nine-key envelope through the rearmed command.
  Confirm the resulting `lyx loom status` envelope is byte-identical to today's nine keys — the four generic core keys plus these five — and that `lyx loom pause`'s is still the single key `status_file`.
- **Commit:** `refactor(loomcli): rearm status and pause over shedverbs`

### Card 24: assemble loomcli's subtree from the returned commands

- **Context:**
  - `internal/shedverbs/verbs.go`
  - `internal/shedverbs/spec.go`
  - `internal/loomcli/arm.go`
  - `internal/loomcli/start.go`
  - `internal/loomcli/validate.go`
  - `internal/loomcli/run.go`
  - `internal/loomcli/step.go`
  - `cmd/lyx/longlist_test.go`
  - `cmd/lyx/jsonhelp_test.go`
  - `cmd/lyx/drift_test.go`
- **Edits:**
  - `internal/loomcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** change `Command()` so its `parent.AddCommand(...)` call passes the four commands returned by `shedverbs.Verbs(loomVerbTexts, c.spec)` in place of `c.runCmd()`, `c.stepCmd()`, `c.statusCmd()` and `c.pauseCmd()`, while `c.startCmd()`, `c.validateDiscussionCmd()` and `c.validatePlanCmd()` stay exactly as they are.
  `loomVerbTexts` already exists, lifted verbatim from the live constructors by card 21 before cards 22 and 23 deleted them;
  this card only wires it into `shedverbs.Verbs`.
  Byte-identical remains the requirement, not "equivalent" — but be clear about what does and does not enforce it.
  No existing `cmd/lyx` guard machine-checks a verb's `Long`: `longlist_test.go` only asserts `root.Long` names every registered top-level module, and `jsonhelp_test.go` exercises root, `board`, `config`, `ide`, `reed` and `selfreport`, so neither fails on a reflowed `loom` or `lifecycle` `Long`.
  `drift_test.go` does fail CI on a blank `Short`, which is the one part that is machine-checked.
  The byte-for-byte copy is therefore guarded by card 21 lifting it from live source and by review, not by a test — do not reflow, re-indent, or re-wrap any of it.
  Decorate the returned `step` command with `--parent` before adding it, per card 22.
  Leave the parent `loom` command's own `Use`/`Short`/`Long`, its `RunE: clihelp.GroupRunE` and its `PersistentPreRunE` untouched apart from card 21's rewrite of the pre-run's body.
  `StartAliasCommand` is out of scope and must not be touched.
- **Commit:** `refactor(loomcli): build the four Shed verbs from shedverbs.Verbs`

### Card 25: add lifecyclecli.Arm

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/verbs.go`
  - `internal/lifecyclecli/wire.go`
  - `internal/lifecyclecli/paths.go`
  - `internal/lifecyclecli/refusal.go`
  - `internal/lifecyclerecipe/lifecyclerecipe.go`
  - `internal/shedbuild/newshed.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/worktreelist.go`
  - `internal/loomcli/arm.go`
  - `internal/lifecyclecli/run_test.go`
- **Edits:**
  - `internal/lifecyclecli/cli.go`
- **Creates:**
  - `internal/lifecyclecli/arm.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** add a `spec *shedverbs.Spec` field to the `lifecycleCLI` struct, initialised non-nil where the receiver is constructed in `Command()`.
  In the new `internal/lifecyclecli/arm.go`, declare the same two-function split card 21 requires of `loomcli`, and for the same reason: an unexported worker `func (c *lifecycleCLI) arm(cwd string, verb string, args []string) (shedverbs.Spec, error)` on the receiver, plus a thin exported `func Arm(cwd string, verb string, args []string) (shedverbs.Spec, error)` wrapper that constructs a fresh `*lifecycleCLI` and returns `c.arm(...)` for `internal/shedcli`'s table.
  A package-level `Arm` alone cannot serve the pre-run: `wire` is a method on the receiver, the hooks close over `c.env`, `c.shedPaths` and `c.abandonedSession`, and the pre-run's own `c.location` and `c.slug` assignments would stop happening.
  Split it the same way card 21 splits loom's: `arm` resolves and wires, then returns `c.specFor(verb), nil`, and a resolution-free `func (c *lifecycleCLI) specFor(verb string) shedverbs.Spec` holds the whole `Spec` fill, which `internal/lifecyclecli/run_test.go`'s hand-built receivers call directly.
  The worker performs this module's whole existing resolution in its existing order: `lyxcwd.Resolve(cwd)`, then `fabricengine.PrimeName(location)`, then `refuseNonPrime(location.WorktreeName, primeName, primeNameErr)`, then the slug read from `args[0]` when `len(args) > 0`, then `wire(location, slug)`.
  Every one of those refusals stays a returned error the caller renders on the envelope, never a hard error and never a panic, exactly as `refuseNonPrime`'s own doc comment requires.
  Preserving the non-prime refusal through `Arm` is what keeps the Lifecycle Bookend Invariant intact when lifecycle is reached through `lyx shed --recipe lifecycle` rather than through `lyx lifecycle`.
  Fill the returned `Spec`: the three paths from `c.shedPaths`;
  `EnsureStatusLockDir: false`, because lifecycle's `status` is read-only and creating its per-slug directory as a side effect of reading it would be a new, unasked-for write;
  `StatusLabel: "lifecycle"`;
  `DecodeErrPrefix: "lifecyclecli:"`;
  `RunBusyMessage` set to the exact existing lock-path-naming text `lifecyclecli: another lifecycle run already holds the run lock "<LockPath>"`;
  `StepBusyMessage`/`StepBusyKind` left at their zero values, since lifecycle exposes no `step`;
  `AbsentStatus` with `Refuse: false`, the `found: false` success form;
  `PauseAbsentMessage` set to the new text `lifecyclecli: no status file at <StatusPath>; there is nothing running to pause -- run "lyx lifecycle run <slug>" first`, mirroring loom's shape and naming lifecycle's own entry verb rather than loom's;
  `BuildShed` a closure returning `lifecyclerecipe.New(c.env, c.shedPaths)`.
  Rewrite `resolvePersistentPreRun` to call `c.arm(cwd, cmd.Name(), args)` — the unexported worker, on its own receiver — and assign `*c.spec = armed`, keeping its existing `cmd.Name() == "lifecycle"` short-circuit and its existing `lyxcwd.CwdFrom(ctx)` read in place, and keeping the pass-through rendering of `lyxcwd.Resolve`'s self-describing error.
  Declare `lifecycleVerbTexts` in this card too, as a package-level `shedverbs.VerbTexts` value in `internal/lifecyclecli/cli.go`, lifting `run`'s and `status`'s `Use`/`Short`/`Long` verbatim out of the still-present `runCmd`/`statusCmd` constructors before card 26 deletes them, and leaving the `step` fields at their zero values.
  The worker performs the `c.location` and `c.slug` assignments the pre-run does today, so they keep happening on both paths rather than only on the `lyx lifecycle` one.
  The single-receiver property holds as in card 21: the `*lifecycleCLI` whose fields the returned hooks close over is always the value the caller holds — the pre-run's own `c` on the `lyx lifecycle` path, the wrapper's freshly-constructed one on the `lyx shed` path.
- **Commit:** `feat(lifecyclecli): add the exported Arm resolution entry point`

### Card 26: rearm lifecyclecli's run and status, and add pause

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/run.go`
  - `internal/shedverbs/status.go`
  - `internal/shedverbs/pause.go`
  - `internal/shedverbs/verbs.go`
  - `internal/lifecyclecli/arm.go`
  - `internal/lifecyclecli/wire.go`
  - `internal/lifecyclecli/paths.go`
  - `internal/lifecyclerecipe/names.go`
  - `internal/shedengine/status.go`
  - `internal/state/state.go`
- **Edits:**
  - `internal/lifecyclecli/run.go`
  - `internal/lifecyclecli/status.go`
  - `internal/lifecyclecli/cli.go`
  - `internal/lifecyclecli/arm.go`
  - `internal/lifecyclecli/run_test.go`
  - `internal/lifecyclecli/lifecycle_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** delete `runCmd` and `statusCmd` and fill the equivalent hooks in `Arm` instead.
  `PreRun` performs this module's existing pre-flight in its existing order: `state.ReadJSONStrict[shedengine.Status]` over the two status paths;
  on a decode error return the existing `lifecyclecli: decode status file <path>: <err>` text;
  on `found`, refuse `shedengine.StateDone` with the existing text naming the per-slug directory to delete, resume silently on `StateRunning`/`StateBlocked`/`StateFailed`/`StatePaused`, and refuse an unrecognised state with the existing `lifecyclecli: unrecognized status state %q` text;
  on `!found`, seed inline via `state.UpdateJSON` with the existing idempotent mutate closure that leaves an already-present status untouched.
  That seed-when-absent behaviour is lifecycle's alone and must not leak into the generic body — loom refuses in exactly the situation lifecycle seeds, because only `lyx loom start` may seed loom's status file.
  `PostRun` returns `map[string]any{"abandonedSession": c.abandonedSession}` only when `c.abandonedSession` is non-empty, preserving today's conditional emission, and returns a nil or empty map otherwise.
  `StatusExtras` returns lifecycle's own three keys and no others: `found: true`, `status_path` and `history` from `st.History`.
  The generic body supplies `current_producer`, `state`, `error` and `activity`, which together with these three reproduce today's **seven**-key found-envelope exactly — `internal/lifecyclecli/status.go` emits `found`, `status_path`, `current_producer`, `state`, `error`, `activity` and `history`.
  Do not pin a six-key closure assertion.
  Register the `pause` command on the lifecycle subtree for the first time, with `Args: cobra.ExactArgs(1)` like the other two, since it needs the slug before `wire` can build any path.
  Change `Command()`'s `parent.AddCommand(c.runCmd(), c.statusCmd())` to add the three commands returned by `shedverbs.Verbs(lifecycleVerbTexts, c.spec)` that this module exposes — `run`, `status` and `pause` — and not `step`, which lifecycle has no analogue for.
  `lifecycleVerbTexts` is declared by card 25, lifted verbatim from the live `runCmd`/`statusCmd` constructors before this card deletes them, for the same reason card 21 lifts loom's.
  It carries `run`'s and `status`'s existing `Use`/`Short`/`Long` byte-for-byte, plus new text for `pause` in the same shape, and extend `status`'s `Long` with an `Example:` line for `--watch` since the flag is now present.
  Leave the `step` texts at their zero values and never add the returned `step` command to the lifecycle subtree, so its blank `Short` never reaches the live tree `cmd/lyx/drift_test.go` walks — that guard fails CI on any *registered* command with a blank `Short`, and an unregistered returned command is not in the tree at all.
  Set `Args: cobra.ExactArgs(1)` on each of the three returned commands before adding them.
  Update the lifecycle parent command's own `Long` to name `pause` alongside `run` and `status`, and to state that all three run from the hub's prime worktree only.
  Preserve `internal/lifecyclecli/run.go`'s doc comment explaining why the run envelope carries neither a mutations array nor a `partial` bool — it stays true and its reasoning is unaffected by the move.
  If either file ends up holding no declaration after the deletions, delete it rather than leaving it empty.
  Retarget the in-package test call sites this deletion orphans, which is mechanical compilation repair rather than an assertion change: `internal/lifecyclecli/run_test.go` calls `c.runCmd()` at five sites and `internal/lifecyclecli/lifecycle_integration_test.go` calls it once.
  Point each at the `run` command `shedverbs.Verbs` returns, filling `c.spec` from `c.specFor("run")` on the receiver the test already builds — the resolution-free seam card 25 declares — and keeping every assertion's meaning identical.
- **Commit:** `refactor(lifecyclecli): rearm run and status over shedverbs and add pause`

### Card 27: absorb the three agreed surface changes into the existing suites

- **Context:**
  - `internal/lifecyclecli/arm.go`
  - `internal/lifecyclecli/cli.go`
  - `internal/shedverbs/run.go`
  - `internal/shedverbs/status.go`
  - `internal/shedengine/shed.go`
  - `internal/loomcli/parity_test.go`
  - `internal/loomcli/cli_test.go`
  - `internal/loomcli/step_test.go`
  - `internal/loomcli/status_test.go`
- **Edits:**
  - `internal/lifecyclecli/run_test.go`
  - `internal/lifecyclecli/cli_test.go`
  - `internal/lifecyclecli/lifecycle_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** update exactly the assertions the three agreed additive surface changes force, and no others.
  First, `lyx lifecycle run`'s success envelope now carries `history_length`, free from `len(result.History)` via the generic body: extend any envelope-key assertion in `run_test.go` and `lifecycle_integration_test.go` that enumerates the run envelope's keys, and add a case asserting the new key's value matches the run's own history length.
  This is a deliberate widening, not an accident of the move.
  Second, `lyx lifecycle` now exposes `pause`: extend `cli_test.go`'s subcommand-listing assertion, if it has one, and add a case driving `lyx lifecycle pause <slug>` against a seeded status and asserting `PauseRequested` is set and the envelope carries exactly `status_file`, plus a case asserting the new absent-file refusal text against a never-run slug.
  Third, `lyx lifecycle status` now carries `--watch` and `--interval`: add a case asserting both flags are registered, and one asserting a `--watch` over a slug whose per-slug directory exists but holds no `status.json` exits immediately with the `found: false` envelope rather than entering the tail.
  The directory must exist in that fixture — see the overview's `lifecycle-absent-file-needs-an-existing-directory` Shared Decision: against a slug whose `LifecycleDir` was never created at all, the read fails in lock acquisition before `found` is ever produced, which is today's behaviour and is not this task's to change.
  Write the same precondition into the `pause` absent-file case above, which reaches `state.UpdateJSON` and therefore behaves differently again.
  Change no other assertion in either package.
  The mechanical call-site retargeting cards 22, 23 and 26 perform in `internal/loomcli/cli_test.go`, `step_test.go`, `status_test.go`, `internal/lifecyclecli/run_test.go` and `lifecycle_integration_test.go` is explicitly carved out of that rule: it repairs compilation against moved identifiers and changes no assertion's meaning.
  If any existing assertion outside these three fails, that is a signal the extraction changed behaviour: stop and report it rather than editing the assertion to match — the overview's `no-surface-change-to-existing-verbs` Shared Decision forbids silently absorbing it.
- **Commit:** `test(lifecyclecli): cover the three agreed additive surface changes`

### Card 28: confirm both modules' regression suites pass unchanged

- **Context:**
  - `internal/loomcli/step_test.go`
  - `internal/loomcli/status_test.go`
  - `internal/loomcli/stephandoff_test.go`
  - `internal/loomcli/wiring_test.go`
  - `internal/loomcli/friction_test.go`
  - `internal/loomcli/selfreport_test.go`
  - `internal/loomcli/parity_test.go`
  - `internal/lifecyclecli/run_test.go`
  - `internal/lifecyclecli/paths_test.go`
  - `internal/lifecyclecli/refusal_test.go`
  - `internal/lifecyclecli/wire_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** run this batch's full `verify:` command, both tiers, and confirm every listed suite passes with no assertion edited beyond card 27's three.
  The four `//go:build smoke` suites are out of this batch's gate by design and must not be added to it;
  confirm specifically that `internal/loomcli`'s `step_test.go` still asserts the ten-key envelope and the five-kind closure — now against the `shedverbs` constants — that `stephandoff_test.go` still proves `recordStepHandoff` fires after a successful step, that `status_test.go` still pins the rendered watch line's format for the `loom` label, and that `friction_test.go` and `selfreport_test.go` still prove `detectAndFileAnomalies` runs on the hard-error path and that `friction` is emitted on every success envelope.
  Those four are the properties most easily lost in this extraction;
  a green run that skipped them is not evidence.
  Confirm `internal/lifecyclecli`'s `paths_test.go` and `refusal_test.go` still pass untouched, since they are the two mechanical proxies for the Lifecycle Bookend Invariant and `Arm` must not have moved either path or weakened the refusal.
  This card changes no file.
- **Commit:** none

## Batch Tests

`verify:` runs `internal/loomcli`, `internal/lifecyclecli` and `internal/shedverbs` at the default tier, then chains a second `go test -tags integration` invocation over the two CLI packages, because `internal/lifecyclecli/lifecycle_integration_test.go`, `internal/lifecyclecli/testmain_integration_test.go` and `internal/loomcli/wiring_commitstatus_integration_test.go` all carry `//go:build integration` and a bare `go test` never compiles them.
`internal/shedverbs` is included rather than assumed-green because every hook this batch fills is one that package declares, and a green `shedverbs` run alongside the two armed modules is what proves the two halves agree.
No card in this batch edits a file under `internal/shedverbs` — the `PostStep` hook loom fills is declared in batch 3, card 10, and covered by batch 3, card 18.

The regression suites named in card 28 are the proof the extraction preserved behaviour, and the three assertion changes card 27 makes are the complete, agreed list of surface movement.
The cross-subtree parity assertions — `lyx loom step` against `lyx shed step --recipe loom`, and the rest — belong to batch 5, where `lyx shed` first exists.
