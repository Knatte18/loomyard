# Test time by package, 2026-11-27

Where tier0 and tier1 time goes in every package under `internal/`, `cmd/` and `tools/` that holds tests,
at commit `2bc62d626`.
The data of every call is in [calls.tsv](calls.tsv).

## Findings

- Test time is concentrated:
  `internal/fabricengine`, `internal/battencli` and `internal/gatecli` have the three longest tier1 runs,
  in the first run by a wide margin.
- A tier1 call pays a fixed cost of about 12 s before any test of the package runs.
  That cost exceeds the test time in 72 of the 82 packages with tier1 tests, and no cut inside a package removes it.
- tier0 is not a target: a call's wall time exceeds its test time by a median of 0.47 s,
  and no package reaches 15 s of tier0 test time except `cmd/lym` in its second run.
- Three of the packages worth a cut have test rows whose length matches a push backoff waited out in real time.

## Method

- One call per package and tier: `lym gate test --wait --no-carry --tier <n> ./<pkg> -- -count=1 -v`.
  `--no-carry` forces a run where an earlier pass would be carried over, and `--wait` keeps the call in one process.
- 212 packages hold tier0 tests and 82 hold tier1 tests.
  A package without tests of a tier got no call for it.
  `internal/testkit/tiergate` only quotes a tier1 `Require` as test data, so its tier1 call ran nothing and is left out.
- The ten packages with the largest summed wall time ran a second time, both tiers.
- All 315 calls passed.
- **Test** is the package's time on go's `ok` line.
- **Wall** is the time from the call holding its gate slot to its end.
  Slot waits are excluded: 22 calls waited, the longest 300 s. `calls.tsv` holds them as `slot_wait_s`.
- **Build share** is `(wall - test) / wall`, summed over both tiers.
  At tier1 it covers `go build ./...`, `go vet` of the package, the build of the `lym` binary
  and the test binary's link.
- **Slowest** names the longest top-level tests of either tier from the `-v` output, each at the longest time of it
  or any of its subtests: the longest one, then up to two more of 1 s or longer.
  Parallel tests overlap, and a parent's time leaves out its parallel subtests,
  so these times do not add up to the package's.
- The calls ran without `--gate`, so tests marked `//lym:only` ran too,
  including some a task's gate skips, in `cmd/lym` and `internal/gatecli`.

## Machine and load

AMD Ryzen AI 7 445, 12 threads, Go 1.26.8, 16:21 to 17:33.
The machine was heavily loaded, so every time here is inflated.

| Calls | 1-minute load average at start |
|---|---|
| tier0, first run | median 17, max 33 |
| tier1, first run | min 23, median 45, max 67 |
| second run | 41 to 59 |

Of the 315 calls, 90 started with no other gate slot held, 119 with one, 28 with two, 61 with three and 17 with all four.
`calls.tsv` holds the load, the slots held before and after, and what held them, per call.

tier0 and tier1 ran at different load, so compare packages within a tier rather than across tiers.

## Packages worth a cut

Packages with 15 s or more of test time in either tier, in either run.
All are tier1 times except where noted.

| Package | First run | Second run | Kind of change |
|---|---|---|---|
| `internal/fabricengine` | 142.3 | 201.9 | restructure |
| `internal/battencli` | 75.2 | 33.4 | split the long scenario, then restructure |
| `internal/gatecli` | 55.8 | 49.1 | split the long scenarios |
| `internal/hubreconcile` | 29.6 | 28.7 | share fixtures |
| `internal/fabriccli` | 22.9 | 21.7 | drop sleeps, split scenarios |
| `internal/boardengine` | 20.1 | 18.8 | drop sleeps |
| `internal/pairteardown` | 18.4 | not rerun | drop sleeps, run in parallel |
| `internal/loomcli` | 17.7 | 20.6 | share fixtures |
| `internal/webstercli` | 17.1 | 15.5 | run in parallel |
| `cmd/lym` (tier0) | 10.5 | 15.6 | none on this evidence |

- **`internal/fabricengine`.**
  461 tier1 tests, and 347 lines that call `hubforge.CopyHub`.
  The cost is spread over many tests that drive git, so cutting single tests will not move it much.
  The change that does is the one `internal/fabriccore` and `internal/fabriclayout` start:
  decisions in packages that run no git, tested at tier0, with fewer hub tests left above them.
  `TestAnchorChange`, `TestPushFailureClasses` and `TestReconcileOrigin_Judge` are where to start.
- **`internal/battencli`.**
  `TestBattenIntegration_Rows` (54 s) runs its steps one after another on one hub copy, which sets the package's floor.
  Split it into groups with a hub each so they run in parallel.
  The cancel tests (9 to 20 s each) build and tear down real pairs;
  testing their decisions without a hub is the larger step.
  The two runs differ by more than a factor of two, so measure a change here more than once.
- **`internal/gatecli`.**
  `TestGateTest_Scopes` (54 s) and `TestGateTest_Scenario` (45 s) are long sequences on one fixture.
  Split them, and move scope computation cases that need no subprocess to tier0.
  `TestGateRedFirst_Dotnet` (53 s) and `TestGateTest_DotnetFixture` (27 s) run only under the `upkeep` key,
  so cutting them does not shorten a task's gate.
- **`internal/hubreconcile`.**
  Several `TestEnsure_*` rows take 4 to 6 s each. Share one built fixture across rows, or fold rows.
  A cut of this package's tests is in progress on another branch and is not in the measured commit.
- **`internal/fabriccli`.**
  `TestCommand_ReconcileBoardFailureFailsTheVerb/rejected_push_fails_the_healed_config` takes 15 s against a hook that rejects the push.
  `TestCommand_HubMutationScenario`, `TestCommand_AdoptRemoteWeftScenario` and `TestCommand_MergeScenario`
  (15 to 16 s each) are step sequences; split them, and drop steps `internal/fabricengine` already covers.
- **`internal/boardengine`.**
  `TestSetOwners/Refusal_TransportFailureAtPush` (7.8 s) and
  `TestBoard_SetStatusPushed/UnreachablePushKeepsTheCommitLocal` (7.2 s) run about as long as the 1 s, 2 s and 4 s
  delays of `gitrepo.PushBackoffDelays`.
  Neither top-level test is parallel, so the two add up to most of the package's time.
  Pass a stub sleep down to the push.
- **`internal/pairteardown`.**
  `TestHandOverOwnedElsewhere/AFailedPushReturnsItsErrorAndWritesNoArchiveTagAndNoMark` takes 7.3 s of the same shape.
  That test and `TestPushRun_HandsOverOnATakeover` do not run in parallel;
  the first says why: its rows install the process-wide push notifier.
- **`internal/loomcli`.**
  No single sink: only `TestLoomFailedReedUpRefuses` (6.8 s) passes 5 s.
  The package has many hub-copy tests, so share fixtures between those that only read, and fold rows.
- **`internal/webstercli`.**
  The package time is close to the sum of its top-level tests, so they run mostly one after another.
  Its tests set the environment or the working directory in many places, which bars `t.Parallel`;
  pass those values in instead. `TestResetCmd` (6.8 s) is the longest.
- **`cmd/lym`.**
  It passes 15 s only in the second tier0 run, and its tests are whole-repository checks.

The backoff matches in `boardengine`, `fabriccli` and `pairteardown` come from durations and code reading.
Time the sleep itself before relying on them.

## Packages where build or link dominates

**tier1.** Wall minus test is nearly the same for every package: min 7.3 s, quartiles 10.6, 11.8 and 13.2 s.
So it is the cost of the call's own steps more than of the package.
A trial call on a package with no tier1 test, which only built the `lym` binary, took 3.3 s at a load near 6.

The 10 packages whose tier1 test time exceeds that cost are `cmd/lym`, `internal/battencli`, `internal/boardengine`,
`internal/fabriccli`, `internal/fabricengine`, `internal/gatecli`, `internal/hubreconcile`, `internal/loomcli`,
`internal/pairteardown` and `internal/webstercli`.
In the other 72, build dominates.
Cutting their tests would gain little.
What helps is paying the fixed steps less often, by running more packages per call, or making the steps cheaper.
This study did not time the steps one by one, which is the next measurement to take.

**tier0.** Wall minus test is the package's own compile and link: median 0.47 s, max 2.5 s.
It is a large share of most calls and a small amount of time.
The largest are `internal/webstercli` (2.5 s), `internal/weaverengine` (1.8 s), `internal/shedcli` (1.8 s),
`internal/orchcli` (1.7 s), `internal/shuttleparity` (1.6 s) and `internal/websterengine` (1.6 s).
A lighter dependency graph of the test binary is the only change that would move these.

## Spread over two runs

Seconds, first run then second run.

| Package | tier0 test | tier0 wall | tier1 test | tier1 wall |
|---|---|---|---|---|
| `internal/fabricengine` | 0.12, 0.35 | 0.9, 2.4 | 142.3, 201.9 | 156.8, 217.1 |
| `internal/battencli` | 0.62, 1.19 | 1.8, 3.1 | 75.2, 33.4 | 88.6, 48.9 |
| `internal/gatecli` | 1.21, 1.45 | 2.1, 2.8 | 55.8, 49.1 | 70.8, 60.7 |
| `internal/hubreconcile` | 0.07, 0.13 | 0.7, 1.5 | 29.6, 28.7 | 39.3, 42.0 |
| `internal/fabriccli` | 0.10, 0.25 | 0.7, 1.6 | 22.9, 21.7 | 37.7, 34.3 |
| `internal/boardengine` | 0.42, 1.50 | 1.0, 2.6 | 20.1, 18.8 | 33.3, 31.5 |
| `internal/webstercli` | 1.08, 0.92 | 3.6, 2.9 | 17.1, 15.5 | 29.5, 27.2 |
| `internal/loomcli` | 2.24, 3.56 | 3.3, 5.5 | 17.7, 20.6 | 28.5, 33.7 |
| `internal/beamcli` | 0.04, 0.11 | 0.9, 1.9 | 13.6, 6.0 | 30.2, 17.0 |
| `cmd/lym` | 10.54, 15.56 | 12.0, 17.7 | 11.5, 14.7 | 18.9, 26.7 |

## Every package

First run, in seconds, sorted by **wall**, the sum of both tiers' wall times.
Over the first run, tier0 took 197 s of wall for 75 s of test, and tier1 1578 s of wall for 591 s of test.

| Package | tier0 test | tier0 wall | tier1 test | tier1 wall | wall | build share | slowest |
|---|---|---|---|---|---|---|---|
| `internal/fabricengine` | 0.12 | 0.9 | 142.31 | 156.8 | 157.7 | 10% | `TestAnchorChange` 48.5, `TestPushFailureClasses` 34.7, `TestReconcileOrigin_Judge` 20.5 |
| `internal/battencli` | 0.62 | 1.8 | 75.17 | 88.6 | 90.4 | 16% | `TestBattenIntegration_Rows` 54.2, `TestCancelIntegration_RemoveOfABeamLeavesAPlainProject` 19.6, `TestVerbs_RunFromABeam` 18.3 |
| `internal/gatecli` | 1.21 | 2.1 | 55.81 | 70.8 | 72.9 | 22% | `TestGateTest_Scopes` 54.4, `TestGateRedFirst_Dotnet` 53.4, `TestGateTest_Scenario` 45.0 |
| `internal/hubreconcile` | 0.07 | 0.7 | 29.62 | 39.3 | 40.0 | 26% | `TestEnsure_FailedWalkNamesTheFileAndLeavesTheTreeAsItWas` 6.3, `TestEnsure_HubWideWalkRendersAndCommitsTheHeddleOverview` 6.3, `TestEnsure_MidMergePairIsLeftAloneAndRetried` 6.3 |
| `internal/fabriccli` | 0.10 | 0.7 | 22.93 | 37.7 | 38.4 | 40% | `TestCommand_HubMutationScenario` 16.3, `TestCommand_AdoptRemoteWeftScenario` 15.9, `TestCommand_MergeScenario` 15.1 |
| `internal/boardengine` | 0.42 | 1.0 | 20.13 | 33.3 | 34.3 | 40% | `TestSetOwners` 7.8, `TestBoard_SetStatusPushed` 7.2, `TestWriteAdopt` 1.9 |
| `internal/webstercli` | 1.08 | 3.6 | 17.10 | 29.5 | 33.1 | 45% | `TestResetCmd` 6.8, `TestGateParity_VerifyGate` 1.4, `TestBatchVerbs_RefuseUnderAPause` 1.4 |
| `internal/loomcli` | 2.24 | 3.3 | 17.69 | 28.5 | 31.8 | 37% | `TestLoomFailedReedUpRefuses` 6.8, `TestLoomResumeReconcilesTheHubConfigBeforeArming` 4.8, `TestHubPeersDeps_OverRealHub` 3.7 |
| `internal/beamcli` | 0.04 | 0.9 | 13.59 | 30.2 | 31.1 | 56% | `TestDobbyRuns_ReadsTheLandOutcomeTheLandingCommitAndTheEndedEntryAndPair` 7.7, `TestTearDownLeftovers_RemovesALandedAndACancelledPairAndTheirWorkspaceFolders` 7.1, `TestHubLandDeps_FileTaskReleasesTheTaskAndCodeReviewCapsReadTheHubConfig` 6.9 |
| `cmd/lym` | 10.54 | 12.0 | 11.54 | 18.9 | 30.9 | 29% | `TestTestOnlyCode_EveryProductionFunctionIsReachableFromAMain` 7.4, `TestCSharpFixtures_CompileWithDotnet` 6.7, `TestGofmt_ModuleFilesAreFormatted` 4.5 |
| `internal/pairteardown` | 0.60 | 1.5 | 18.43 | 25.9 | 27.4 | 31% | `TestHandOverOwnedElsewhere` 8.6, `TestPushRun_HandsOverOnATakeover` 6.7, `TestRemoveBenchPairs` 2.4 |
| `internal/shedcli` | 0.06 | 1.9 | 6.82 | 23.7 | 25.6 | 73% | `TestParity_Scenario` 6.8, `TestShedVerbs_StaleStampReconcilesBeforeALoomRecipeArms` 2.6 |
| `tools/machinecost` | 0.01 | 0.3 | 11.91 | 24.1 | 24.4 | 51% | `TestRunProbe_MeasuresEveryPartAndLeavesNoFolders` 7.6, `TestLinkProbeProgram_EachLinkDiffers` 7.3, `TestRunProbe_FailedRunLeavesNoFolders` 4.2 |
| `internal/boardcli` | 0.34 | 1.0 | 8.99 | 21.9 | 22.9 | 59% | `TestBoardServeScenario` 7.9, `TestCLI` 1.1 |
| `internal/shuttleengine/claudeengine` | 2.66 | 3.3 | 5.61 | 18.9 | 22.2 | 63% | `TestRunPluginHook` 5.0 |
| `internal/websterengine` | 3.32 | 4.9 | 5.16 | 16.5 | 21.4 | 60% | `TestRun_VerifyGate` 1.0 |
| `internal/runpusher` | 0.04 | 1.1 | 2.63 | 19.5 | 20.6 | 87% | `TestNewDeps_ConditionsStopOnAHandOverMarkOrARefusedPush` 2.6 |
| `internal/reedcli` | 0.36 | 1.4 | 4.33 | 18.8 | 20.2 | 77% | `TestEndPairProcesses` 3.3 |
| `internal/testkit/lymbin` | – | – | 8.85 | 20.0 | 20.0 | 56% | `TestBuild_ProducesExecutableBinary` 4.3, `TestBuildWithLDFlags_ReachTheBuild` 4.2 |
| `internal/shedrecipe` | 0.39 | 1.5 | 0.16 | 18.4 | 19.9 | 97% | – |
| `internal/preflightshed` | 0.04 | 1.1 | 5.72 | 18.8 | 19.9 | 71% | `TestPreflight_ReconcileFailureWarnsAndGoesOnToTheCheck` 5.7, `TestPreflight_StaleStampReconcilesBeforeTheCleanCheck` 5.3, `TestPreflight_Scenario` 4.3 |
| `internal/shedverbs` | 0.55 | 1.7 | 3.62 | 17.9 | 19.6 | 79% | `TestLoopIntegration_ArmingRefusalStopsAsBootstrap` 3.3 |
| `internal/creelcli` | 0.02 | 0.7 | 5.87 | 18.8 | 19.5 | 70% | `TestCreelCLI` 4.2, `TestHubDelivery_SkipsAWorktreeThatNoLongerResolves` 1.3 |
| `internal/shuttleparity` | 1.48 | 3.1 | 1.62 | 16.3 | 19.4 | 84% | `TestLoomConfigOf_StaleStampReconcilesTheHubBase` 1.6 |
| `internal/cleanupcli` | 0.02 | 0.9 | 5.23 | 18.1 | 19.0 | 72% | `TestCleanupVerb_RefusesOutsideThePrimeThenListsAndThenRemovesEverySeededLeftover` 5.2, `TestCleanupVerb_ScratchSegmentSweepsTheAnchorOfASubpathHub` 1.9 |
| `internal/lymcwd` | 5.96 | 6.4 | 0.15 | 12.1 | 18.5 | 67% | `TestEnforcement_FabricVocabulary` 2.4, `TestEnforcement_BoltDirNameDeclaredOnce` 2.3 |
| `internal/configcli` | 0.31 | 1.1 | 4.42 | 17.0 | 18.1 | 74% | `TestConfigOverRealHub` 4.2 |
| `internal/machinecli` | 0.04 | 1.2 | 4.56 | 16.7 | 17.9 | 74% | `TestName_SweepsHubs` 4.5 |
| `internal/driftcli` | 0.01 | 0.6 | 3.00 | 17.3 | 17.9 | 83% | `TestCapture_OutlivesCaller` 3.0, `TestCheck_PrintsTheFindings` 2.5, `TestDrift_RunsInTheAnchoredDirectory` 2.4 |
| `internal/proc` | 0.70 | 1.2 | 1.81 | 16.3 | 17.5 | 86% | `TestCapProcessMemory_KillsARunawayChildInTheScopeAndTheProcessLivesOn` 1.7 |
| `internal/hubforge` | – | – | 6.91 | 17.5 | 17.5 | 61% | `TestNewHub` 4.0, `TestCopyRegistered_CopyHoldsTheBuildAndIsolatesWrites` 4.0, `TestCopyHub` 3.3 |
| `internal/adoptcli` | 0.03 | 1.0 | 3.66 | 16.1 | 17.1 | 78% | `TestAdoptIntegration_SecondCloneTakesOver` 3.6, `TestCommand_RunsFromABeam` 1.2, `TestCommand_HereAdoptsTheSubjectOfItsWorktree` 1.1 |
| `internal/shuttlecli` | 0.09 | 1.0 | 1.52 | 16.0 | 17.0 | 91% | `TestCommand_AwaitForksResolvesFromTheRecordsSibling` 0.8 |
| `internal/fronttask` | 0.02 | 0.7 | 2.08 | 16.2 | 16.9 | 88% | `TestHubSettleDeps_OwnedHereReadsTheOwnerOnTheHubBoard` 1.9, `TestHubDeps_FilesAndRemovesOnTheHubBoard` 1.4 |
| `internal/loomshed` | 0.30 | 1.5 | 4.25 | 15.1 | 16.6 | 73% | `TestVerifyGate_RoundScenario` 3.3 |
| `internal/gitrepo` | 0.02 | 0.4 | 4.30 | 15.9 | 16.3 | 73% | `TestMergePrimitives` 3.0, `TestPush` 2.2, `TestDeleteRemoteBranch` 1.6 |
| `internal/burlercli` | 0.09 | 0.9 | 1.50 | 15.4 | 16.3 | 90% | `TestValidateReviewVerb_RoundChecks` 1.4 |
| `internal/preflight` | 0.03 | 1.0 | 2.90 | 15.2 | 16.2 | 82% | `TestCheckResolved_LymOwnStateIsExempt` 2.7, `TestCheckResolved_MissingOptionalJunctionIsAJunctionFault` 2.3, `TestHealthyHub` 2.3 |
| `internal/shuttleengine/codexengine` | 0.48 | 1.1 | 0.14 | 14.9 | 16.0 | 96% | `TestCheckAfterStartup` 0.1 |
| `internal/slackcli` | 0.07 | 1.2 | 1.77 | 14.6 | 15.8 | 88% | `TestWorktreeOrch` 1.7, `TestReceiverPaused` 1.3, `TestOrchProject` 1.2 |
| `internal/reedengine` | 0.60 | 1.1 | 0.04 | 14.5 | 15.6 | 96% | `TestWithOpLock_SerializesConcurrentCalls` 0.1 |
| `internal/boardengine/boardtest` | 0.39 | 1.0 | 1.84 | 14.6 | 15.6 | 86% | `TestSync` 1.8 |
| `internal/landingshed` | 0.82 | 1.5 | 3.54 | 14.0 | 15.5 | 72% | `TestFinalize_OverRealHub` 2.4, `TestMergeIn_ConflictSessionLoadsLanguageSkills` 1.1 |
| `internal/shedworktree` | – | – | 1.48 | 15.3 | 15.3 | 90% | `TestArbiterSeam_Changes` 1.4 |
| `internal/orchcli` | 1.01 | 2.7 | 1.89 | 12.5 | 15.2 | 81% | `TestWatch` 0.8 |
| `tools/tokencount` | 0.04 | 0.7 | 0.39 | 13.9 | 14.6 | 97% | `TestRunReadsArchivedShedStatus` 0.3 |
| `internal/stencilcli` | – | – | 1.46 | 14.4 | 14.4 | 90% | `TestStencilCLI_Scenario` 1.4 |
| `internal/weavercli` | 0.06 | 1.4 | 1.28 | 12.8 | 14.2 | 91% | `TestResetDepsFor_SeamsResetATaskPairThroughTheHub` 1.2 |
| `internal/planglyph` | 0.53 | 1.4 | 2.80 | 12.8 | 14.2 | 77% | `TestGlyphChain_CallerCoverageTyped` 1.8, `TestGlyphChain_RedundantPackage` 1.0 |
| `internal/retrocli` | 0.02 | 1.0 | 1.75 | 13.0 | 14.0 | 87% | `TestRetro_FilesAndReleasesTheTaskOfALandedRun` 1.7 |
| `cmd/testtiming` | 0.02 | 0.6 | 1.77 | 13.4 | 14.0 | 87% | `TestTakeGateSlot` 1.7 |
| `internal/tendercli` | 0.02 | 1.0 | 2.61 | 12.9 | 13.9 | 81% | `TestInstallNetworkGate_TellsThePairsSlugAndInstallsNothingOutsideAHub` 0.9 |
| `internal/ideengine` | 0.01 | 0.5 | 3.90 | 13.4 | 13.9 | 72% | `TestSpawnScenario` 3.0 |
| `tools/forkaudit` | – | – | 2.16 | 13.7 | 13.7 | 84% | `TestForkauditPlanSection` 1.8, `TestForkauditTraceCrossCheck` 1.5, `TestForkauditReport` 1.2 |
| `internal/shellguard` | 0.03 | 0.6 | 1.42 | 13.1 | 13.7 | 89% | `TestRows_PatternsDenyAndAllow` 0.2 |
| `internal/hibernatecli` | 0.02 | 0.8 | 2.01 | 12.7 | 13.5 | 85% | `TestHibernateVerbs_WireTheHub` 1.0 |
| `internal/driftengine` | 0.12 | 0.6 | 0.61 | 12.9 | 13.5 | 95% | `TestRunCapture` 0.6 |
| `internal/shuttleengines` | 0.06 | 0.7 | 0.06 | 12.7 | 13.4 | 99% | – |
| `internal/cardverify` | 0.01 | 0.3 | 1.29 | 13.1 | 13.4 | 90% | `TestRunner` 1.0 |
| `tools/retro` | – | – | 0.29 | 13.3 | 13.3 | 98% | `TestRetroScript_WritesTheFiguresOfEachRun` 0.3 |
| `internal/testcover` | 0.02 | 0.4 | 3.06 | 12.4 | 12.8 | 76% | `TestRunAlone` 3.0 |
| `internal/gateslot` | 0.08 | 0.3 | 0.11 | 12.4 | 12.7 | 99% | – |
| `internal/dotnetimpact` | 0.01 | 0.2 | 0.36 | 12.4 | 12.6 | 97% | `TestDeriveDiff` 0.3 |
| `internal/testkit/tmuxkit` | 0.08 | 0.8 | 0.05 | 11.5 | 12.3 | 99% | – |
| `internal/reportcli` | 0.03 | 1.2 | 0.58 | 11.1 | 12.3 | 95% | `TestRepliesVerb` 0.5 |
| `internal/hubgeom` | 0.07 | 1.2 | 1.65 | 11.1 | 12.3 | 86% | `TestGeometryOverRealHub` 1.6 |
| `internal/commentlint` | 0.01 | 0.3 | 0.12 | 11.8 | 12.1 | 99% | – |
| `internal/hardencli` | – | – | 1.59 | 12.0 | 12.0 | 87% | `TestSandboxVerbs` 1.6 |
| `internal/verifytree` | 0.03 | 0.6 | 0.52 | 11.3 | 11.9 | 95% | `TestVerify_Scenario` 0.2 |
| `internal/hubregistry` | 0.02 | 0.4 | 0.06 | 11.5 | 11.9 | 99% | – |
| `internal/desktophook` | 0.01 | 0.2 | 0.08 | 11.7 | 11.9 | 99% | – |
| `internal/dotgit` | 0.02 | 0.2 | 0.16 | 11.6 | 11.8 | 98% | `TestListWorktrees_MatchesGitPorcelain` 0.1 |
| `internal/heddleengine` | 0.11 | 0.7 | 1.79 | 11.0 | 11.7 | 84% | `TestSeam_PullTakeIn` 1.4 |
| `internal/gitexec` | 0.01 | 0.2 | 0.23 | 11.5 | 11.7 | 98% | `TestRun_RemoteBounds` 0.1 |
| `internal/verifyrun` | 0.09 | 0.7 | 0.34 | 10.7 | 11.4 | 96% | `TestRun_ExpiryKillsBackgroundedDescendant` 0.3 |
| `internal/logger` | 0.06 | 0.2 | 0.02 | 10.9 | 11.1 | 99% | – |
| `internal/idecli` | – | – | 0.52 | 10.9 | 10.9 | 95% | `TestCommand_SpawnScenario` 0.5 |
| `internal/heddlecli` | – | – | 0.93 | 10.6 | 10.6 | 91% | `TestHeddleCLI` 0.9 |
| `internal/gitkit` | 0.87 | 1.1 | 0.13 | 9.0 | 10.1 | 90% | `TestCopyRepoCallerSet_LymcwdOnly` 0.8 |
| `internal/impactset` | 0.01 | 0.2 | 0.35 | 9.1 | 9.3 | 96% | `TestDerive_OverARepository` 0.3 |
| `internal/orchroute` | 0.03 | 0.9 | 0.44 | 8.2 | 9.1 | 95% | `TestAsker` 0.4 |
| `internal/termwindow` | 0.02 | 0.3 | 1.04 | 8.3 | 8.6 | 88% | `TestAttachLine_EndsOrKeepsTheShellByAttachStatus` 1.0 |
| `internal/loomrecipe` | 4.56 | 5.7 | – | – | 5.7 | 20% | `TestDriftRows_EntryGate` 0.7 |
| `internal/orchengine` | 4.31 | 5.3 | – | – | 5.3 | 19% | `TestWatcher_StepOutcomes` 0.5 |
| `internal/weaverengine` | 3.36 | 5.2 | – | – | 5.2 | 35% | `TestRun_Reentry` 2.3, `TestRun` 1.1 |
| `internal/shedadapters` | 3.24 | 4.2 | – | – | 4.2 | 23% | `TestBouncer_SoftCapRule` 0.4 |
| `internal/testkit/envkit` | 2.16 | 3.1 | – | – | 3.1 | 30% | `TestSeamMarkers_ReflectiveTestsAreMarked` 2.1 |
| `internal/loomengine` | 1.87 | 2.8 | – | – | 2.8 | 33% | `TestDiscussionTable` 0.3 |
| `internal/configsync` | 2.20 | 2.8 | – | – | 2.8 | 21% | `TestReconcileHubWideAt_SeedOnlyFileFollowsItsTemplate` 2.1 |
| `internal/seatengine` | 0.88 | 2.3 | – | – | 2.3 | 62% | `TestEngine_Run_FailedStopReturnsTheWayForwardUnretried` 0.2 |
| `internal/shedarbiter` | 0.75 | 2.0 | – | – | 2.0 | 62% | `TestCall_SpawnPath` 0.3 |
| `internal/dobbyengine` | 1.49 | 2.0 | – | – | 2.0 | 25% | `TestRun_LockOwnerAndWakeups` 1.3 |
| `internal/shuttleengine` | 1.53 | 1.9 | – | – | 1.9 | 20% | `TestPromptEngineRule` 0.6 |
| `internal/twilldirective` | 0.53 | 1.6 | – | – | 1.6 | 67% | `TestDirective_MissingStencilNamesIt` 0.3 |
| `internal/testkit/shedcheckkit` | 0.06 | 1.5 | – | – | 1.5 | 96% | – |
| `internal/slackengine` | 0.68 | 1.3 | – | – | 1.3 | 47% | `TestRunRelay` 0.5 |
| `internal/shedbuild` | 0.08 | 1.3 | – | – | 1.3 | 94% | – |
| `internal/reportengine` | 0.10 | 1.3 | – | – | 1.3 | 92% | – |
| `internal/mergeresolve` | 0.22 | 1.2 | – | – | 1.2 | 82% | `TestResolve_SessionEditsTrackedFile_StagesTrackedBeforeConclude` 0.1 |
| `internal/testkit/shedfake` | 0.08 | 1.1 | – | – | 1.1 | 93% | – |
| `internal/burlerengine` | 0.31 | 1.1 | – | – | 1.1 | 72% | – |
| `internal/battenshed` | 0.44 | 1.1 | – | – | 1.1 | 60% | – |
| `internal/vscode` | 0.04 | 1.0 | – | – | 1.0 | 96% | – |
| `internal/quarrycli` | 0.03 | 1.0 | – | – | 1.0 | 97% | – |
| `internal/hubissues` | 0.67 | 1.0 | – | – | 1.0 | 33% | `TestListOpenIssues_PausedPageGetsItsOwnDeadline` 0.7 |
| `internal/githubclient` | 0.70 | 1.0 | – | – | 1.0 | 30% | `TestAuthRT_HeldCallGetsItsFullDeadlineAfterRelease` 0.7 |
| `internal/configreg` | 0.50 | 1.0 | – | – | 1.0 | 50% | `TestRegistration_MatchesDeclarers` 0.3 |
| `internal/cliwire` | 0.49 | 1.0 | – | – | 1.0 | 51% | `TestDeriveCallerSet_CliwireOnly` 0.4 |
| `internal/battenrecipe` | 0.04 | 1.0 | – | – | 1.0 | 96% | – |
| `internal/retroengine` | 0.02 | 0.9 | – | – | 0.9 | 98% | – |
| `internal/testkit/stencilkit` | 0.24 | 0.8 | – | – | 0.8 | 70% | – |
| `internal/sheddriver` | 0.35 | 0.8 | – | – | 0.8 | 57% | `TestDriveVerb` 0.2 |
| `internal/selfreportcli` | 0.14 | 0.8 | – | – | 0.8 | 83% | – |
| `internal/claudecli` | 0.02 | 0.8 | – | – | 0.8 | 98% | – |
| `internal/arbiterstop` | 0.04 | 0.8 | – | – | 0.8 | 95% | – |
| `internal/standalonegeom` | 0.03 | 0.7 | – | – | 0.7 | 96% | – |
| `internal/toolfloor` | 0.03 | 0.6 | – | – | 0.6 | 95% | – |
| `internal/testkit/tiergate` | 0.05 | 0.6 | – | – | 0.6 | 92% | – |
| `internal/testkit/shuttlefake` | 0.02 | 0.6 | – | – | 0.6 | 96% | – |
| `internal/shortcutcli` | 0.02 | 0.6 | – | – | 0.6 | 96% | – |
| `internal/shedtransient` | 0.01 | 0.6 | – | – | 0.6 | 98% | – |
| `internal/shedengine` | 0.19 | 0.6 | – | – | 0.6 | 68% | – |
| `internal/selfreportengine` | 0.02 | 0.6 | – | – | 0.6 | 97% | – |
| `internal/modelspec` | 0.06 | 0.6 | – | – | 0.6 | 90% | – |
| `internal/hibernateengine` | 0.06 | 0.6 | – | – | 0.6 | 91% | – |
| `internal/frictionengine` | 0.08 | 0.6 | – | – | 0.6 | 87% | – |
| `internal/failuretrace` | 0.01 | 0.6 | – | – | 0.6 | 98% | – |
| `internal/claudeimports` | 0.04 | 0.6 | – | – | 0.6 | 93% | – |
| `internal/boardserve` | 0.15 | 0.6 | – | – | 0.6 | 74% | – |
| `internal/testreach` | 0.02 | 0.5 | – | – | 0.5 | 97% | – |
| `internal/testkit` | 0.09 | 0.5 | – | – | 0.5 | 82% | – |
| `internal/testkit/logcapture` | 0.02 | 0.5 | – | – | 0.5 | 96% | – |
| `internal/tenderengine` | 0.05 | 0.5 | – | – | 0.5 | 90% | – |
| `internal/stencilstore` | 0.02 | 0.5 | – | – | 0.5 | 95% | – |
| `internal/stencil` | 0.02 | 0.5 | – | – | 0.5 | 96% | – |
| `internal/standalonestate` | 0.02 | 0.5 | – | – | 0.5 | 96% | – |
| `internal/sleepjournal` | 0.02 | 0.5 | – | – | 0.5 | 96% | – |
| `internal/shedrun` | 0.04 | 0.5 | – | – | 0.5 | 93% | – |
| `internal/samplerengine` | 0.03 | 0.5 | – | – | 0.5 | 94% | – |
| `internal/runprofile` | 0.03 | 0.5 | – | – | 0.5 | 95% | – |
| `internal/runproc` | 0.08 | 0.5 | – | – | 0.5 | 85% | – |
| `internal/runplan` | 0.02 | 0.5 | – | – | 0.5 | 96% | – |
| `internal/reviewbudget` | 0.03 | 0.5 | – | – | 0.5 | 94% | – |
| `internal/pullrequest` | 0.02 | 0.5 | – | – | 0.5 | 96% | – |
| `internal/planparser` | 0.06 | 0.5 | – | – | 0.5 | 88% | – |
| `internal/machinename` | 0.09 | 0.5 | – | – | 0.5 | 82% | – |
| `internal/hardenshed` | 0.03 | 0.5 | – | – | 0.5 | 94% | – |
| `internal/fswatch` | 0.08 | 0.5 | – | – | 0.5 | 85% | – |
| `internal/yamlengine` | 0.02 | 0.4 | – | – | 0.4 | 94% | – |
| `internal/testkit/plankit` | 0.02 | 0.4 | – | – | 0.4 | 95% | – |
| `internal/testkit/envelope` | 0.02 | 0.4 | – | – | 0.4 | 95% | – |
| `internal/summaryparser` | 0.02 | 0.4 | – | – | 0.4 | 96% | – |
| `internal/statuspane` | 0.01 | 0.4 | – | – | 0.4 | 96% | – |
| `internal/statuscommit` | 0.02 | 0.4 | – | – | 0.4 | 95% | – |
| `internal/state` | 0.03 | 0.4 | – | – | 0.4 | 93% | – |
| `internal/shell` | 0.03 | 0.4 | – | – | 0.4 | 92% | – |
| `internal/selfexec` | 0.02 | 0.4 | – | – | 0.4 | 96% | – |
| `internal/secretpath` | 0.01 | 0.4 | – | – | 0.4 | 96% | – |
| `internal/runfsm` | 0.02 | 0.4 | – | – | 0.4 | 96% | – |
| `internal/reedengine/render` | 0.02 | 0.4 | – | – | 0.4 | 96% | – |
| `internal/raddleparser` | 0.02 | 0.4 | – | – | 0.4 | 95% | – |
| `internal/pattern` | 0.07 | 0.4 | – | – | 0.4 | 84% | – |
| `internal/parentdirective` | 0.03 | 0.4 | – | – | 0.4 | 92% | – |
| `internal/output` | 0.02 | 0.4 | – | – | 0.4 | 96% | – |
| `internal/lymtemp` | 0.03 | 0.4 | – | – | 0.4 | 94% | – |
| `internal/darnengine` | 0.08 | 0.4 | – | – | 0.4 | 79% | – |
| `internal/creelengine` | 0.10 | 0.4 | – | – | 0.4 | 74% | – |
| `internal/agentname` | 0.01 | 0.4 | – | – | 0.4 | 96% | – |
| `tools/sandbox` | 0.02 | 0.3 | – | – | 0.3 | 93% | – |
| `tools/plandag` | 0.02 | 0.3 | – | – | 0.3 | 94% | – |
| `tools/mdreflow` | 0.01 | 0.3 | – | – | 0.3 | 95% | – |
| `tools/internal/devbin` | 0.01 | 0.3 | – | – | 0.3 | 96% | – |
| `tools/godocreflow` | 0.01 | 0.3 | – | – | 0.3 | 95% | – |
| `tools/deploy` | 0.02 | 0.3 | – | – | 0.3 | 92% | – |
| `internal/weftname` | 0.01 | 0.3 | – | – | 0.3 | 96% | – |
| `internal/testkit/scankit` | 0.02 | 0.3 | – | – | 0.3 | 95% | – |
| `internal/testkit/permkit` | 0.01 | 0.3 | – | – | 0.3 | 96% | – |
| `internal/testkit/locationkit` | 0.01 | 0.3 | – | – | 0.3 | 96% | – |
| `internal/testkit/llmkit` | 0.01 | 0.3 | – | – | 0.3 | 97% | – |
| `internal/shuttleengine/bashlookup` | 0.01 | 0.3 | – | – | 0.3 | 95% | – |
| `internal/segmentcolor` | 0.01 | 0.3 | – | – | 0.3 | 96% | – |
| `internal/reportconfig` | 0.02 | 0.3 | – | – | 0.3 | 94% | – |
| `internal/pawlengine` | 0.02 | 0.3 | – | – | 0.3 | 95% | – |
| `internal/partsparser` | 0.02 | 0.3 | – | – | 0.3 | 95% | – |
| `internal/parentlock` | 0.02 | 0.3 | – | – | 0.3 | 94% | – |
| `internal/lenses` | 0.07 | 0.3 | – | – | 0.3 | 76% | – |
| `internal/gatefixer` | 0.05 | 0.3 | – | – | 0.3 | 84% | – |
| `internal/fsx` | 0.01 | 0.3 | – | – | 0.3 | 96% | – |
| `internal/friction` | 0.01 | 0.3 | – | – | 0.3 | 95% | – |
| `internal/envconfig` | 0.01 | 0.3 | – | – | 0.3 | 96% | – |
| `internal/discussionparser` | 0.02 | 0.3 | – | – | 0.3 | 93% | – |
| `internal/creelconfig` | 0.01 | 0.3 | – | – | 0.3 | 97% | – |
| `internal/consoletitle` | 0.01 | 0.3 | – | – | 0.3 | 96% | – |
| `internal/configengine` | 0.05 | 0.3 | – | – | 0.3 | 84% | – |
| `internal/clihelp` | 0.02 | 0.3 | – | – | 0.3 | 94% | – |
| `internal/batcher` | 0.05 | 0.3 | – | – | 0.3 | 83% | – |
| `internal/agentconfig` | 0.04 | 0.3 | – | – | 0.3 | 88% | – |
| `tools/wordswap` | 0.02 | 0.2 | – | – | 0.2 | 91% | – |
| `tools/tapestryrename` | 0.01 | 0.2 | – | – | 0.2 | 94% | – |
| `tools/codestats` | 0.01 | 0.2 | – | – | 0.2 | 93% | – |
| `internal/testkit/fabrickit` | 0.01 | 0.2 | – | – | 0.2 | 94% | – |
| `internal/pauserecord` | 0.02 | 0.2 | – | – | 0.2 | 90% | – |
| `internal/loggerconfig` | 0.01 | 0.2 | – | – | 0.2 | 94% | – |
| `internal/lock` | 0.06 | 0.2 | – | – | 0.2 | 69% | – |
| `internal/lensreport` | 0.01 | 0.2 | – | – | 0.2 | 93% | – |
| `internal/languageconfig` | 0.01 | 0.2 | – | – | 0.2 | 94% | – |
| `internal/ideconfig` | 0.01 | 0.2 | – | – | 0.2 | 95% | – |
| `internal/helpcli` | 0.01 | 0.2 | – | – | 0.2 | 96% | – |
| `internal/haltnote` | 0.04 | 0.2 | – | – | 0.2 | 82% | – |
| `internal/gitrepo/internal/gitoracle` | 0.01 | 0.2 | – | – | 0.2 | 95% | – |
| `internal/fslink` | 0.01 | 0.2 | – | – | 0.2 | 94% | – |
| `internal/fabricvocab` | 0.01 | 0.2 | – | – | 0.2 | 95% | – |
| `internal/fabriclayout` | 0.01 | 0.2 | – | – | 0.2 | 94% | – |
| `internal/fabriccore` | 0.01 | 0.2 | – | – | 0.2 | 94% | – |
| `internal/envsource` | 0.01 | 0.2 | – | – | 0.2 | 93% | – |
| `internal/entrymerge` | 0.01 | 0.2 | – | – | 0.2 | 94% | – |
| `internal/editdirective` | 0.02 | 0.2 | – | – | 0.2 | 92% | – |
| `internal/durfmt` | 0.01 | 0.2 | – | – | 0.2 | 95% | – |
| `internal/desktoplauncher` | 0.01 | 0.2 | – | – | 0.2 | 94% | – |
| `internal/buildvcs` | 0.01 | 0.2 | – | – | 0.2 | 94% | – |
| `internal/buildinfo` | 0.01 | 0.2 | – | – | 0.2 | 94% | – |
