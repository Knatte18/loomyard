MILL_REVIEW_BEGIN
# Review: fabric: audit and migrate all remaining direct git mutations onto Fabric — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic), self-assessed
reviewed_file: plan/
date: 2026-08-01
```

## Findings

### [BLOCKING] Card 5 omits Bisector injection for a second bisect-reaching test
**Location:** batch 02-webster-bisect-migrate, Card 5
**Issue:** Card 5 sets `fx.Deps.Bisector = gitrepo.New(fx.Worktree)` only for `TestIntegrationStage_FailingForkTriggersBisectAndEscalates`, but `TestIntegrationStage_FailedSuite_DoneOutcomeFailsLoud` (integration_test.go, ~line 424) also drives a FAILED integration report through `Run()` to the same `BisectAndEscalate` call inside `runIntegrationStage`. Its fixture comes from `newRunFixture`, which builds `Layout{WorktreeRoot: worktree, Cwd: worktree}` with `Hub` empty — after Card 4's migration, `deps.Bisector` being nil there makes `runIntegrationStage` call `fabricengine.New(deps.Layout.WorktreeRoot, deps.Layout.WeftWorktree())`, which fails (`ErrMissingPath`, weft path doesn't exist under the empty Hub) before ever calling `BisectAndEscalate`. The test's own assertions (`err.Error()` containing "a done outcome requires a passing integration suite", and `st.Batches[-1]` existing after the loud return) both then fail — this test breaks, and batch 2's own `verify:` (`-run 'TestIntegrationStage|...'`) would catch it.
**Fix:** Add `fx.Deps.Bisector = gitrepo.New(fx.Worktree)` to `TestIntegrationStage_FailedSuite_DoneOutcomeFailsLoud` as well (and add the same `gitrepo` import usage there), or move the injection into `newRunFixture` itself so every fixture consumer gets it uniformly — mirroring how Card 9 fixes this class of issue in builder by adding `Resetter` unconditionally inside `newSpawnFixture` rather than per-test.

### [BLOCKING] Card 2 Context omits the files defining the methods under test
**Location:** batch 01-fabric-warp-methods, Card 2
**Issue:** Card 2's Requirements call `fabricengine.New(...)` and the four `*Fabric` methods (`CurrentBranch`, `CheckoutDetached`, `RestoreBranch`, `ResetHard`) whose exact signatures live in `internal/fabricengine/fabric.go` and `internal/fabricengine/warpforward.go` (both created/edited by Card 1). Neither file is listed in Card 2's `Context:` (only checkout_rollback_test.go, reconcile_stale_registration_test.go, testmain_test.go, gitrepo.go, lyxtest). Per the Context completeness rule, an implementer scoped to Card 2's stated Context cannot confirm these signatures without cold-start exploration.
**Fix:** Add `internal/fabricengine/fabric.go` and `internal/fabricengine/warpforward.go` to Card 2's `Context:` list.

### [BLOCKING] Card 4 Context omits integration.go, which defines WarpBisector
**Location:** batch 02-webster-bisect-migrate, Card 4
**Issue:** Card 4's Requirements say to add `Bisector WarpBisector` to `RunDeps` in `runlevel.go`, but `WarpBisector` is defined in `internal/websterengine/integration.go` (Card 3's edit), which is not listed in Card 4's `Context:` (only hubgeometry.go, webstercli/weft.go, fabric.go). Same-package types still originate from a specific file the rule requires naming.
**Fix:** Add `internal/websterengine/integration.go` to Card 4's `Context:` list.

### [NIT] Card 9's test enumeration is incomplete but harmless
**Location:** batch 03-builder-resethard-migrate, Card 9
**Issue:** Card 9 names five `RestartChain: true` SpawnBatch tests as beneficiaries of the fixture-level `Resetter` addition, but two more (`TestSpawnBatch_RestartChainOnChainlessBatchErrors`, `TestSpawnBatch_RestartChainClearsStaleReportBeforeRefusal`) also use `RestartChain: true` and are omitted from the list — the second of these actually reaches the real reset call. This does not break anything because the fix is applied unconditionally inside `newSpawnFixture`, covering every consumer regardless of enumeration, but the doc list is inaccurate/incomplete.
**Fix:** Update the enumeration to name all seven `RestartChain: true` tests, or drop the enumeration in favor of "every test built on `newSpawnFixture`."

## Verdict

REQUEST_CHANGES
Card 5 leaves one bisect-reaching test broken post-migration; two cards have Context-completeness gaps for cross-card-referenced files.
MILL_REVIEW_END
