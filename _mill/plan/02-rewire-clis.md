# Batch: rewire-clis

```yaml
task: "Unify webster/burler CLI wiring into a shared module"
batch: "rewire-clis"
number: 2
cards: 2
verify: go test ./... && go test -tags integration ./internal/cliwire/... ./internal/webstercli/... ./internal/burlercli/... ./internal/standalonegeom/...
depends-on: [1]
```

## Batch Scope

This batch rewrites both `wiring.go` bodies onto `internal/cliwire`, declares each CLI's own `cliwire.Module` descriptor in its own package, retargets `internal/burlercli/run.go`'s one cross-file `resolveToldDir` call, and prunes from both `wiring_test.go` files the tests whose assertions moved into `internal/cliwire/cliwire_test.go` in batch 1.
At the end of this batch `internal/cliwire` is the only production caller of `standalonestate.Derive` and neither CLI package declares any of the nine banned helper names — which is exactly what batch 3's two enforcement tests then pin.

There are two cards, one per CLI package, and each is deliberately whole-package: deleting a helper from `wiring.go` while its test file still calls it leaves the package uncompilable, so a package's production rewrite and its test prune are one atomic change and therefore one card.

Batch-local decision beyond `## Shared Decisions`: a test that today asserts a *shared* message stays in its CLI package only as the thin composition check it already is.
Concretely, both packages keep their existing `TestWire_TargetDirRefusedInHubMode`, which asserts nothing about wording — only that `wire` returns an error naming `--target-dir` in hub mode.
That is composition proof that `wireHub` calls `RefuseTargetDirInHubMode` at all, and it does not duplicate the verbatim-message assertions batch 1 put in `internal/cliwire/cliwire_test.go`.
`TestWireHub_LeavesDurableSinkDirUntouched` likewise stays in both packages: `cliwire` owns no hub prologue for it to drive, so the assertion is per-CLI composition by construction.

## Cards

### Card 6: webstercli calls into cliwire

- **Context:**
  - `internal/cliwire/doc.go`
  - `internal/cliwire/paths.go`
  - `internal/cliwire/module.go`
  - `internal/cliwire/standalone.go`
  - `internal/cliwire/cliwire_test.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/cli_integration_test.go`
  - `internal/planparser/parse.go`
  - `internal/hubgeom/webstergeom.go`
  - `internal/standalonegeom/webstergeom.go`
  - `internal/standalonegeom/reedgeom.go`
- **Edits:**
  - `internal/webstercli/wiring.go`
  - `internal/webstercli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `internal/webstercli/wiring.go` so it holds only `wire`, `wireHub`, `wireStandalone`, `setRunner`, and one new package-level descriptor value.

  Declare `var wireModule = cliwire.Module{...}` with `Name: "webster"`, `StateArtifacts: "state, locks, rendered prompts and trace logs"`, `TargetRole: "the repository it drives"`, `TargetRecourse: "Drive a target outside the state home"`, `HubTargetSubject: "the worktree is already the target"`, and a non-nil `Plan` whose `DefaultPlanDir` is `planparser.PlanDir` and whose `MissingPlanRefusal` returns today's message verbatim.
  Give the var a doc comment stating that webster owns its own descriptor because `cliwire` carries the shared implementation while the varying data lives with the caller, mirroring how `internal/shedrecipe`'s constructors live in that package while the rows that vary live outside it.

  Delete these declarations from the file, in favour of the `internal/cliwire` equivalents: `gitDirName`, `refuseNestedStandaloneGeometry`, `pathContains`, `normalizeForContainment`, `standaloneDefaultPlanDir`, `samePlanDir`, `resolveToldDir`, `resolveStandaloneTarget`, `repositoryRootOf`, `standalonePlanDirHasContent`.

  `wire` keeps its signature.
  Its body still resolves `stencilsDir` and `planDir` up front — now via `cliwire.ResolveToldDir` — and still branches on `mode == preflight.ModeHub` into `wireHub` or `wireStandalone` with today's arguments.
  Keep the whole existing doc comment on `wire`, editing only the sentences that name a helper this card deleted.

  `wireHub` keeps its signature and every config load, geometry build, engine construction and field assignment it performs today.
  Two edits only: replace the inline `--target-dir` refusal with `if err := wireModule.RefuseTargetDirInHubMode(targetDirFlag); err != nil { return err }` as the function's first statement, and replace the inline plan-dir block with a `cliwire.ResolvePlanDir(planDir, geom.PlanDir)` call whose results drive `geom.PlanDir` and the two override fields.
  The second argument is `geom.PlanDir` — the value `hubgeom.WebsterGeometry` actually built — and deliberately not `wireModule.Plan.DefaultPlanDir(anchorPath)`, even though the two are the same string today;
  comparing against the geometry's own field is what keeps the override check correct if `internal/hubgeom` ever changes how it computes `PlanDir`.
  Card 3 records the same reasoning on the field's own doc comment.
  Preserve today's exact field state: assign `c.planDirOverridden` and `c.planDirDefault` only when the resolver reports an override, and assign `geom.PlanDir` from the resolved value only when `planDir` was non-empty, so a flagless hub invocation leaves `geom.PlanDir` exactly as `hubgeom.WebsterGeometry` built it.
  Keep the existing comment explaining why hub mode records a moved plan directory at all.

  `wireStandalone` keeps its signature.
  Replace its whole prologue — the target resolve, `standalonestate.Derive`, the nested-geometry refusal, the sink redirect, the stencils resolve-and-seed, and the plan-dir resolution and content gate — with one call:
  `res, err := wireModule.ResolveStandalone(cliwire.StandaloneRequest{Cwd: cwd, StencilsDirFlag: stencilsDir, PlanDirFlag: planDir, TargetDirFlag: targetDirFlag})`.
  Note that `stencilsDir` and `planDir` are already absolute here and that re-resolving them inside the prologue is idempotent — say so in a short comment so a later reader does not hoist the resolution back out.
  Then build geometry from the result: `geom := standalonegeom.WebsterGeometry(res.Target, res.StateDir)`, `reedGeom := standalonegeom.ReedGeometry(res.Target, res.StateDir, res.Hash8)`, then `geom.StencilsDir = res.StencilsDir` and `geom.PlanDir = res.PlanDir`, and `if res.PlanDirOverridden { c.planDirOverridden = true; c.planDirDefault = res.DefaultPlanDir }`.
  Everything from the `shuttleengine.LoadConfig(stateDir, "shuttle")` call onward stays exactly as it is today, reading `res.StateDir` where it read `stateDir`.
  Move the long ordering comment off this function — it now lives on `ResolveStandalone` — and leave in its place a one-line pointer saying the prologue's ordering obligation is documented there.

  Fix the import block: add `github.com/Knatte18/loomyard/internal/cliwire` and `github.com/Knatte18/loomyard/internal/planparser`, then drop every import the rewrite orphans.
  Determine that set by walking the whole post-rewrite import block and grepping the file for each package's own identifier — an unused import is a hard compile error, so the block must be verified entry by entry rather than trusted.
  The following are named only as a non-exhaustive starting point and the list must not be treated as complete: `os`, `runtime`, `strings`, `path/filepath`, `github.com/Knatte18/loomyard/internal/standalonestate`, `github.com/Knatte18/loomyard/internal/stencilstore`, `github.com/Knatte18/loomyard/internal/buildinfo`, `github.com/Knatte18/loomyard/contracts/stencils`, and `github.com/Knatte18/loomyard/internal/logger`.
  Note that `fmt` is expected to survive in this file, since webster's `MissingPlanRefusal` closure builds its message with it.

  In `internal/webstercli/wiring_test.go`, delete the test functions whose assertions batch 1 moved into `internal/cliwire/cliwire_test.go`: `TestResolveStandaloneTarget_RefusesATargetThatIsNotAReadableDirectory`, `TestResolveStandaloneTarget_LiftsToRepositoryRoot`, `TestRefuseNestedStandaloneGeometry_SeesThroughASymlinkedStateHome`, `TestWireStandalone_RefusesStateDirNestedInTarget`, and `TestWireStandalone_RedirectsDurableSinkToStandaloneLogsDir`.
  Keep every other test in the file exactly as it is, including `TestWire_ModeHubSelectsHubMode`, `TestWire_ModeStandaloneSelectsStandaloneMode`, `TestWire_PlanDirResolution`, `TestWire_RelativeFlagDirsResolveAgainstCwd`, `TestWire_TargetDirRefusedInHubMode`, `TestWire_StandaloneRootsResolveToTarget`, `TestWire_MatcherNeverNilOpenerNilOnlyInStandalone`, `TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError`, `TestWireHub_LeavesDurableSinkDirUntouched`, `TestWireStandalone_SubdirectoryOfRepositoryWiresLikeItsRoot`, `TestWire_ReedUpSeamPerMode`, and `TestWireStandalone_PlanDirOverrideMarksRunRefusal`.
  Then delete any helper (`hubLocation`, `seedStandalonePlanDir`, `hash8For`, `seedGitRepositoryRoot`) and any import that no remaining test in the package references — determine this by grepping the package for each name, since Go does not report an unused function.

  Retarget the doc comments this card's own deletions invalidate:
  the file-header comment's claim that this file reaches "the one call site of `standalonestate.Derive` that exists anywhere in this codebase" (after this card the production call site lives in `internal/cliwire`, and this file's own remaining `Derive` calls are fixture helpers);
  `seedStandalonePlanDir`'s doc, which names `standalonePlanDirHasContent`, now `internal/cliwire`'s own `planDirHasContent`;
  and `seedGitRepositoryRoot`'s doc, which names `repositoryRootOf`, now `cliwire.RepositoryRootOf` — each only if the helper survives the deletion pass above.

  Retarget `wireStandalone`'s own doc comment in `internal/webstercli/wiring.go` too — this is production prose that neither enforcement test in batch 3 can catch, since both match on the AST.
  Three of its claims are false after this card: it opens by naming `resolveStandaloneTarget` as what settled the target, which this card deletes;
  it calls this the "only place `standalonestate.Derive` is ever called", which is now `internal/cliwire`;
  and it closes by justifying the already-absolute `stencilsDir`/`planDir` parameters "since the default-vs-override comparison below is a path equality", a comparison this card moves out of the function.
  Rewrite the comment so it describes what the function now does — call `wireModule.ResolveStandalone` and compose webster's own engines onto the result — and point at `cliwire.ResolveStandalone` for the prologue's ordering and asymmetry rules rather than restating them.

  Add one new test, `TestWireModule_DescriptorIsVerbatim`, which is what makes this task's "a reworded message fails the suite" claim true of the six descriptor fields that this task converts from inline production strings into free-floating data.
  Without it nothing anywhere asserts webster's own descriptor text: `internal/cliwire`'s tests assert their own fixtures, not this package's `wireModule`, and every test that used to touch the nested-geometry and target-resolution messages is deleted above.
  It must:
  - assert each of `wireModule`'s six string fields (`Name`, `StateArtifacts`, `TargetRole`, `TargetRecourse`, `HubTargetSubject`, and the plan default produced by `wireModule.Plan.DefaultPlanDir` for a told base) against its expected literal, so a reword of any single field fails here even for a field no reachable message exercises;
  - assert `wireModule.RefuseTargetDirInHubMode` returns the hub refusal verbatim for a non-empty flag, and `nil` for an empty one;
  - assert `wireModule.Plan.MissingPlanRefusal` returns webster's refusal verbatim for a told plan directory and recourse pair;
  - drive `wireModule.ResolveStandalone` once with `XDG_STATE_HOME` and `LOCALAPPDATA` pointed inside a `t.TempDir()` target so the state directory nests under it, and assert the resulting refusal verbatim — this is the one path that exercises `Name`, `StateArtifacts` and `TargetRole` composed into a real message rather than read as fields.
  Keep it tier 1: no `t.Parallel()` on the `t.Setenv` case, and restore the sink with `t.Cleanup` if the case reaches the redirect.
- **Commit:** `refactor(webstercli): wire standalone and hub resolution through cliwire`

### Card 7: burlercli calls into cliwire

- **Context:**
  - `internal/cliwire/doc.go`
  - `internal/cliwire/paths.go`
  - `internal/cliwire/module.go`
  - `internal/cliwire/standalone.go`
  - `internal/cliwire/cliwire_test.go`
  - `internal/burlercli/cli.go`
  - `internal/burlercli/cli_integration_test.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/standalonegeom/burlergeom.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/webstercli/wiring.go`
- **Edits:**
  - `internal/burlercli/wiring.go`
  - `internal/burlercli/wiring_test.go`
  - `internal/burlercli/run.go`
  - `internal/burlercli/cli_test.go`
  - `cmd/lyx/prerunlogging_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `internal/burlercli/wiring.go` so it holds only `wire`, `wireHub`, `wireStandalone`, and one new package-level descriptor value.

  Declare `var wireModule = cliwire.Module{...}` with `Name: "burler"`, `StateArtifacts: "instruction files, shuttle run directories and trace logs"`, `TargetRole: "the repository it reviews"`, `TargetRecourse: "Review a target outside the state home"`, `HubTargetSubject: "the anchor path is already the target"`, and `Plan` left nil.
  Give the var a doc comment stating that `Plan` is nil because burler parses no plan, and that a nil `Plan` is what makes `ResolveStandalone` skip plan-dir resolution entirely rather than each caller writing its own branch.
  Match the descriptor-ownership rationale already written on webster's equivalent var in `internal/webstercli/wiring.go`.

  Delete these declarations from the file: `gitDirName`, `refuseNestedStandaloneGeometry`, `pathContains`, `normalizeForContainment`, `resolveToldDir`, `resolveStandaloneTarget`, `repositoryRootOf`.

  `wire` keeps its signature and still resolves `stencilsDir` via `cliwire.ResolveToldDir` before branching on mode.
  Keep its existing doc comment, editing only sentences that name a deleted helper.

  `wireHub` keeps its signature and every config load, geometry build and field assignment it performs today, including the `fabricengine.StencilsDir(loc.HubPath)` default and its override.
  One edit only: replace the inline `--target-dir` refusal with `if err := wireModule.RefuseTargetDirInHubMode(targetDirFlag); err != nil { return err }` as the function's first statement.

  `wireStandalone` keeps its signature.
  Replace its prologue — the target resolve, `standalonestate.Derive`, the nested-geometry refusal, the sink redirect, and the stencils resolve-and-seed — with one call:
  `res, err := wireModule.ResolveStandalone(cliwire.StandaloneRequest{Cwd: cwd, StencilsDirFlag: stencilsDirOverride, TargetDirFlag: targetDirFlag})`, leaving `PlanDirFlag` unset.
  Then read the result: `reedGeom := standalonegeom.ReedGeometry(res.Target, res.StateDir, res.Hash8)`, the three config loads over `res.StateDir`, `burlerengine.New(runner, standalonegeom.BurlerGeometry(res.Target, res.StateDir), burlerCfg, res.StencilsDir)`, and the field assignments `c.mode = "standalone"`, `c.stateDir = res.StateDir`, `c.stencilsDir = res.StencilsDir`.
  The two lines that build `runner` — `reedEngine := reedengine.New(reedCfg, reedGeom)` and the `shuttleengine.NewDetachedRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, reedGeom.PaneCwd, shuttleCfg)` call — survive unchanged, reading the same `reedGeom` this card rebuilds from the result.
  `NewDetachedRunner` in particular stays: standalone's anchor is deliberately outside its worktree root, which `NewRunner`'s containment assertion would refuse.
  The `c.reedUp` seam assignment stays exactly as it is.
  Move the two-asymmetry paragraph and the sink-ordering paragraph off this function's doc comment — both now live on `ResolveStandalone` — and leave a one-line pointer in their place.

  In `internal/burlercli/run.go`, change the `--profile` read at the `os.ReadFile` call from `resolveToldDir(c.cwd, profilePath)` to `cliwire.ResolveToldDir(c.cwd, profilePath)`, and add the `internal/cliwire` import.
  The behaviour must stay identical: it resolves against the seam cwd `c.cwd`, never the process working directory — this is R6-17's fix and must not regress.

  Pin that with one new tier-1 case appended to `internal/burlercli/cli_test.go`, named `TestRunVerb_RelativeProfileResolvesAgainstSeamCwd`.
  Nothing pins it today — `cli_test.go` covers only the missing-`--profile` refusal and `decodeProfile` — and neither batch-3 check would catch a swapped or dropped first argument, so a silent regression to the process working directory is reachable.
  Drive the `run` verb the way `TestRunVerb_AbortedPreRunEmitsOneEnvelopeNotTwo` already does, with `c.cwd` pointed at a `t.TempDir()` holding a profile file, passing that file's name as a **relative** `--profile` value.

  The fixture must stop the flow at `decodeProfile`, and this is the case's central constraint rather than an incidental detail.
  Write the profile file with content that fails `decodeProfile`'s strict decode, so `run`'s `RunE` returns at the decode error and never reaches `c.reedUp`.
  Reaching `c.reedUp` would call `reedEngine.Up()` and boot a live tmux/reed session from an untagged tier-1 test, which the Test Tier Purity Invariant forbids;
  state that prohibition in the test's own doc comment so a later editor does not "fix" the fixture by making the profile valid.

  Assert **positively** that the output contains `profile YAML` — `decodeProfile`'s own error prefix — and additionally that it does not contain `read --profile`.
  The positive half is what makes the case falsifiable: reaching the decode error proves `os.ReadFile` succeeded, which proves the relative `--profile` resolved against `c.cwd`.
  A `does not contain` assertion alone would pass vacuously whenever the invocation ended earlier, and it can end earlier — `run`'s `RunE` checks `clihelp.ShouldAbort` first and returns `nil` on a failed pre-run, so an aborted wiring would satisfy the negative assertion without ever exercising the resolution the case exists to pin.
  Note that wiring runs in `PersistentPreRunE`, ahead of `RunE`, so the pre-run must genuinely succeed for this case to mean anything;
  if the group guard requires the seam cwd to be a git repository, seed a `.git` entry in it the way `seedGitRepositoryRoot` does.

  Redirect both `XDG_STATE_HOME` and `LOCALAPPDATA` to fresh `t.TempDir()` values before the call, and do not mark the case `t.Parallel()`.
  This is not optional hygiene: a successful pre-run reaches the real `standalonestate.Derive` and the real `stencilstore.Reconcile`, so without both redirects this untagged case seeds a stencils tree and a trace sink into the developer's own state home.
  `internal/burlercli/cli_integration_test.go` sets both and carries `//go:build integration` for exactly this reason.
  Restore the sink with `t.Cleanup(func() { logger.SetDurableSinkDir("") })`.
  Do not `os.Chdir` and do not depend on the process working directory in any way.

  Fix `internal/burlercli/wiring.go`'s import block: add `github.com/Knatte18/loomyard/internal/cliwire`, then drop every import the rewrite orphans.
  Determine that set by walking the whole post-rewrite import block and grepping the file for each package's own identifier — an unused import is a hard compile error, so the block must be verified entry by entry rather than trusted.
  The following are named only as a non-exhaustive starting point and the list must not be treated as complete: `os`, `runtime`, `strings`, `path/filepath`, `fmt`, `github.com/Knatte18/loomyard/internal/standalonestate`, `github.com/Knatte18/loomyard/internal/stencilstore`, `github.com/Knatte18/loomyard/internal/buildinfo`, `github.com/Knatte18/loomyard/contracts/stencils`, and `github.com/Knatte18/loomyard/internal/logger`.
  `fmt` in particular is expected to become orphaned here, unlike in webster's file: burler's descriptor leaves `Plan` nil, so this file builds no message of its own once the inline refusals move out.

  In `internal/burlercli/wiring_test.go`, delete the test functions whose assertions batch 1 moved into `internal/cliwire/cliwire_test.go`: `TestResolveStandaloneTarget`, `TestResolveStandaloneTarget_RefusesATargetThatIsNotAReadableDirectory`, `TestRefuseNestedStandaloneGeometry_SeesThroughASymlinkedStateHome`, `TestWireStandalone_RefusesStateDirNestedInTarget`, and `TestWireStandalone_RedirectsDurableSinkToStandaloneLogsDir`.

  `TestResolveStandaloneTarget_LiftsToRepositoryRoot` is a mixed case and must not be deleted wholesale.
  Its first three sub-tests — `CwdInsideRepositoryLiftsToRoot`, `TargetDirInsideRepositoryLiftsToRoot` and `NonRepositoryDirectoryUnchanged` — are pure-function tests whose assertions moved into `internal/cliwire/cliwire_test.go` and are deleted.
  Its fourth, `SubdirectoryWiresOntoTheRootsOwnStateDir`, is not: it drives `c.wire` and asserts `c.stateDir` against the repository root's own derived state directory, making it burler's own R4-26 end-to-end composition regression test and the exact analog of webster's `TestWireStandalone_SubdirectoryOfRepositoryWiresLikeItsRoot`, which card 6 keeps.
  Promote it to a retained top-level test named `TestWireStandalone_SubdirectoryOfRepositoryWiresLikeItsRoot`, matching webster's name, with its body unchanged and a doc comment stating what it pins and that it mirrors webster's test of the same name.
  Then delete the now-empty `TestResolveStandaloneTarget_LiftsToRepositoryRoot` wrapper.
  From `TestWire_StencilsDirFlag`, delete only the `ExplicitOverride_NeverWrittenTo` and `StandaloneDefaultSeededOnDisk` sub-tests — those two assert prologue behaviour that now lives in `internal/cliwire/cliwire_test.go` — and keep `HonouredInHubMode` and `HonouredInStandaloneMode`, which are composition checks on `c.stencilsDir`.
  Keep every other test in the file exactly as it is, including `TestWire_ModeHubSelectsHubMode`, `TestWire_ModeStandaloneSelectsStandaloneMode`, `TestWireStandalone_NeverReadsLoc`, `TestWire_StandalonePinnedValues`, `TestWire_TargetDirRefusedInHubMode`, `TestWire_RelativeStencilsDirResolvesAgainstCwd`, `TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError`, `TestWireHub_LeavesDurableSinkDirUntouched`, and `TestWire_ReedUpSeamPerMode`.
  Then delete any helper (`hubLocation`, `hash8For`, `setStandaloneStateRoot`, `hash8AndStateDir`, `readDirNames`, `seedGitRepositoryRoot`) and any import that no remaining test in the package references, determining this by grepping the package for each name.

  Retarget the doc comments this card's own deletions invalidate, the same way card 6 does for webster:
  the file-header comment's claim about being "the one call site of `standalonestate.Derive`";
  and `seedGitRepositoryRoot`'s doc, which names `repositoryRootOf`, now `cliwire.RepositoryRootOf` — only if the helper survives the deletion pass above.

  Retarget `cmd/lyx/prerunlogging_test.go`'s header comment and its failure message as well.
  Both say the guard exists because cobra runs "webstercli's and burlercli's `wireStandalone`, which now redirect the durable trace sink to `standalonegeom.LogsDir(stateDir)` the moment `standalonestate.Derive` returns" — prose this batch invalidates, since after cards 6 and 7 neither `wireStandalone` performs that redirect and `cliwire.ResolveStandalone` does.
  Name `cliwire.ResolveStandalone` as the redirect's owner and keep every other claim in that file unchanged;
  the guard itself, its assertions and its build tag are untouched.
  This retarget lives in card 7 rather than card 6 because the comment names both CLIs and is only fully stale once burler's move has landed too.

  Retarget `wireStandalone`'s own doc comment in `internal/burlercli/wiring.go` too, for the same reason card 6 gives: it is production prose no AST-matching enforcement test can catch.
  It opens by naming `resolveStandaloneTarget` as what settled the target and calls this "the only place `standalonestate.Derive` is ever called in this package", and both claims are false after this card.
  Rewrite it to describe what the function now does — call `wireModule.ResolveStandalone` and compose burler's own engine onto the result — and point at `cliwire.ResolveStandalone` for the prologue's ordering and two-asymmetry rules, which the same card already moves onto that function.

  Add one new test, `TestWireModule_DescriptorIsVerbatim`, mirroring card 6's test of the same name and existing for the same reason — nothing else asserts burler's own descriptor text once the message-bearing tests above are deleted.
  It must:
  - assert each of `wireModule`'s five string fields (`Name`, `StateArtifacts`, `TargetRole`, `TargetRecourse`, `HubTargetSubject`) against its expected literal, and assert `wireModule.Plan` is nil — the nil is a load-bearing fact, since it is what makes `ResolveStandalone` skip plan resolution for burler;
  - assert `wireModule.RefuseTargetDirInHubMode` returns burler's hub refusal verbatim for a non-empty flag, and `nil` for an empty one;
  - drive `wireModule.ResolveStandalone` once with `XDG_STATE_HOME` and `LOCALAPPDATA` pointed inside a `t.TempDir()` target so the state directory nests under it, and assert the resulting refusal verbatim.
  Keep it tier 1 under the same rules card 6's version follows.

  Do not change any existing case in `internal/burlercli/cli_test.go` — the only edit this card makes to that file is appending `TestRunVerb_RelativeProfileResolvesAgainstSeamCwd`, and any existing case there needing a change is a behaviour change and a signal to stop and report rather than to edit.
  Do not edit `internal/burlercli/cli_integration_test.go` at all, under the same rule.
- **Commit:** `refactor(burlercli): wire standalone and hub resolution through cliwire`

## Batch Tests

`verify:` is the same two-command pair every batch runs, and the same `verify-full-suite` justification applies: this batch is the one that makes `internal/cliwire` a real cross-cutting dependency, `cmd/lyx/prerunlogging_test.go` source-guards the ordering precondition the relocated redirect rests on — not the redirect itself — and the hub's own `pipeline.done_gate` is the same repo-wide command.

The tagged half carries the load for this batch specifically.
`internal/webstercli/cli_integration_test.go` and `internal/burlercli/cli_integration_test.go` exercise the real `standalonestate.Derive` and the real standalone stencil seed end-to-end through each CLI's public entry point, which is precisely the path these two cards re-route.
They are the only end-to-end coverage of the moved prologue and must pass untouched;
`internal/burlercli/cli_test.go` and `internal/webstercli/verbs_test.go` likewise.
Needing to edit any of them means behaviour changed.

The surviving `wiring_test.go` cases in both packages are the composition suite: the mode truth table, flag threading into the prologue and assignment onto each struct's own fields, the per-mode pinned facts (`NeverMatches`, the nil fabric opener, `wireStandalone` never reading `loc`, the `reedUp` seam's presence in standalone and absence in hub, the detached runner reaching a public entry point without a told-path error), webster's `--plan-dir` override marking `run`'s refusal, and the hub sink-untouched guard.
The shared prologue's own behaviour is covered by `internal/cliwire/cliwire_test.go` from batch 1.
