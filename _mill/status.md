# Status

```yaml
phase: approved-consumer call-site migration
slug: fabric-weft-visibility-cleanup
branch: fabric-weft-visibility-cleanup
plan: _mill/plan
parent: main
task: 'fabric: close the weft-visibility leak (slice 8)'
task_description: |
  fabric: close the weft-visibility leak (slice 8)
```

## Timeline

```text
discussing  '2026-08-06T17:21:00Z'
discussion-fix-r5  '2026-08-06T18:50:04Z'
discussed  '2026-08-06T18:50:04Z'
planning  '2026-08-06T19:04:20Z'
plan-review-r1  '2026-08-06T19:14:47Z'
plan-fix-r1  '2026-08-06T19:14:47Z'
plan-review-r2  '2026-08-06T19:24:07Z'
plan-fix-r2  '2026-08-06T19:24:07Z'
plan-review-r3  '2026-08-06T19:38:04Z'
plan-fix-r3  '2026-08-06T19:38:04Z'
planned  '2026-08-06T19:39:17Z'
implementing  '2026-08-06T19:40:29Z'
approved-fabric API expand  '2026-08-07T13:18:26Z'
approved-typed Healthy reason and Clean reword  '2026-08-07T13:33:57Z'
approved-consumer call-site migration  '2026-08-07T13:58:05Z'
```

## Batches

```yaml
batches:
  - name: fabric API expand
    state: approved
    implementer_session: 871e7dd3-7358-43a8-92ac-058ce435f28f
    start_sha: 1935d82b456a1c81d62c840ba987bc0025d99aa4
    commit_sha: 14cc70cbc5d50dd65eb22ebae9bf6ab917be96f7
    verify_baseline_failures: []
  - name: typed Healthy reason and Clean reword
    state: approved
    implementer_session: eb654fde-e391-4386-9284-99ed5470c499
    start_sha: 5a7b9e1f4926f7f09d348b277074119472a349db
    commit_sha: 4354279da03c3ad57ffbb9addbcbf84de4f28e7a
    verify_baseline_failures: []
  - name: consumer call-site migration
    state: approved
    implementer_session: 15d5f3c4-b068-4ce4-903d-3cee9642f3a2
    start_sha: 992ac095eef94976bdf66ed1e181b107b25de787
    commit_sha: a435d05d3e0863a514341ac793b052514184bc4e
    verify_baseline_failures: ['--- FAIL: TestSpawnBatchCmd_ObservesPauseFlagWrittenByPauseCmd (0.02s)', '--- FAIL:
    TestPollCmd_DeadlineReturnsRunningWithoutWeftCommit (0.01s)', '--- FAIL: TestPollCmd_ReportPresentClassifiesDoneAndCommits
    (0.02s)', '--- FAIL: TestPollCmd_TerminalCleanupMatrix (0.05s)', '--- FAIL: TestPollCmd_ReportLandingDuringGatherBeatsStopEvent
    (0.02s)', '--- FAIL: TestPollCmd_DeadRecheckStatErrorPropagates (0.01s)', '---
    FAIL: TestPollCmd_NoReportTurnEndedClassifiesDeadAsking (0.01s)', '--- FAIL: TestPollCmd_TerminalPersistMergesConcurrentSpawn
    (0.02s)', '--- FAIL: TestPollCmd_ReportBatchFieldMismatchFailsLoud (0.02s)', '---
    FAIL: TestPollCmd_HalfWrittenReportGetsOneTickGrace (0.02s)', '--- FAIL: TestPollCmd_PersistentlyMalformedReportFailsAfterGrace
    (0.02s)', '--- FAIL: TestRunCmd_SuccessEnvelopeAndWeftCommit (0.00s)', '--- FAIL:
    TestRunCmd_OrchestratorErrorStillRunsBackstopWeftCommit (0.00s)', '--- FAIL: TestRunCmd_FreshFlagThreadsThrough
    (0.00s)', '--- FAIL: TestSpawnBatchCmd_ValidationRefusalCarriesFindings (0.01s)',
  '--- FAIL: TestSpawnBatchCmd_NoRunInProgress (0.02s)', '--- FAIL: TestSpawnBatchCmd_PausedEnvelope
    (0.01s)', '--- FAIL: TestSpawnBatchCmd_SuccessEnvelopeAndWeftCommit (0.02s)',
  '--- FAIL: TestSpawnBatchCmd_RecoveryRoleOverride (0.01s)', '--- FAIL: TestRunCLI_Validate_CleanPlan
    (0.01s)', '--- FAIL: TestRunCLI_Validate_FindingsEnvelope (0.01s)', "FAIL\tgithub.com/Knatte18/loomyard/internal/buildercli\t\
    0.689s", '--- FAIL: TestRun_ValidationRefusal (0.00s)', '--- FAIL: TestRun_FreshInitPersistsState
    (0.00s)', '--- FAIL: TestRun_FingerprintMismatchThenFreshArchivesAndReinits (0.00s)',
  '--- FAIL: TestRun_OutcomeMapping (0.00s)', '--- FAIL: TestRun_ClearsPauseOnDoneAndStuckButNotOnPaused
    (0.00s)', '--- FAIL: TestRun_RefusedRunLeavesPauseIntactButProceedingRunClearsIt
    (0.00s)', '--- FAIL: TestRun_ProgressRenderingPartiallyReported (0.00s)', '---
    FAIL: TestRun_ProgressRenderingStuckBatchIsNotDone (0.00s)', '--- FAIL: TestRun_SpecFieldsMapped
    (0.00s)', '--- FAIL: TestRun_ReclaimsLiveOrphanedOrchestratorAtEntry (0.00s)',
  '--- FAIL: TestRun_PersistsOrchestratorStrandBeforeWait (0.00s)', '--- FAIL: TestRun_FreshStopsSupersededRunsLiveStrands
    (0.00s)', '--- FAIL: TestSpawnBatch_RoleSelectionMatrix (0.00s)', '--- FAIL: TestSpawnBatch_PauseSentinel
    (0.00s)', '--- FAIL: TestSpawnBatch_StaleReportRefusal (0.00s)', '--- FAIL: TestSpawnBatch_RecoveryArchivesStaleReport
    (0.00s)', '--- FAIL: TestSpawnBatch_FingerprintMismatchRefused (0.00s)', '---
    FAIL: TestSpawnBatch_DeadRespawnReclaimsKeptSubstrate (0.00s)', '--- FAIL: TestSpawnBatch_RestartChainPersistsStateBeforeSpawn
    (0.00s)', '--- FAIL: TestSpawnBatch_RestartChainStopsLiveMemberStrands (0.00s)',
  '--- FAIL: TestSpawnBatch_RestartChainFromNonLowestMemberSpawnsLowest (0.00s)',
  '--- FAIL: TestSpawnBatch_InFlightGuardMatrix (0.00s)', '--- FAIL: TestSpawnBatch_ChainAnchorRecordedOnce
    (0.00s)', '--- FAIL: TestSpawnBatch_StatePersisted (0.00s)', '--- FAIL: TestSpawnBatch_SpecFieldsMapped
    (0.00s)', '--- FAIL: TestSpawnBatch_RestartChainOnChainlessBatchErrors (0.00s)',
  '--- FAIL: TestSpawnBatch_RestartChainClearsStaleReportBeforeRefusal (0.00s)', '---
    FAIL: TestRestartChain (0.00s)', '--- FAIL: TestChainEndFor (0.00s)', '--- FAIL:
    TestValidate_PlanValidFixture_ZeroFindings (0.00s)', '--- FAIL: TestValidate_PlanBrokenChain_TripsCheck4Twice
    (0.00s)', '--- FAIL: TestValidate_PlanUnapproved_TripsCheck1 (0.00s)', '--- FAIL:
    TestChainMembers (0.00s)', '--- FAIL: TestParsePlan_PlanValidFixture (0.00s)',
  '--- FAIL: TestRestartChain_ChainlessErrors (0.00s)', '--- FAIL: TestRestartChain_UnrecordedAnchorErrors
    (0.00s)', '--- FAIL: TestParsePlan_HasRenameMechanic (0.00s)', '--- FAIL: TestParsePlan_OtherFixturesParseCleanly
    (0.00s)', "FAIL\tgithub.com/Knatte18/loomyard/internal/builderengine\t0.142s",
  '--- FAIL: TestSpawnBatchCmd_ObservesPauseFlagWrittenByPauseCmd (0.01s)', '--- FAIL:
    TestPollCmd_TerminalCleanupMatrix (0.04s)', '--- FAIL: TestPollCmd_ReportLandingDuringGatherBeatsStopEvent
    (0.01s)', '--- FAIL: TestPollCmd_ReportBatchFieldMismatchFailsLoud (0.01s)', '---
    FAIL: TestPollCmd_PersistentlyMalformedReportFailsAfterGrace (0.01s)', '--- FAIL:
    TestSpawnBatchCmd_NoRunInProgress (0.01s)', '--- FAIL: TestSpawnBatchCmd_SuccessEnvelopeAndWeftCommit
    (0.01s)', "FAIL\tgithub.com/Knatte18/loomyard/internal/buildercli\t0.590s", "FAIL\t\
    github.com/Knatte18/loomyard/internal/builderengine\t0.118s"]
  - name: constructor contract (unexport)
    state: running
    implementer_session: 1eab8d47-76dd-41f6-aa9d-994249893fcf
    start_sha: d34c072cfb2ab0a58e11f36127bf5f2801fd265b
    verify_baseline_failures: []
  - name: templates describe one repo
    state: pending
    verify_baseline_failures: ['--- FAIL: TestRun_ValidationRefusal (0.00s)', '--- FAIL: TestRun_FreshInitPersistsState
    (0.00s)', '--- FAIL: TestRun_FingerprintMismatchThenFreshArchivesAndReinits (0.00s)',
  '--- FAIL: TestRun_OutcomeMapping (0.00s)', '--- FAIL: TestRun_ClearsPauseOnDoneAndStuckButNotOnPaused
    (0.00s)', '--- FAIL: TestRun_RefusedRunLeavesPauseIntactButProceedingRunClearsIt
    (0.00s)', '--- FAIL: TestRun_ProgressRenderingPartiallyReported (0.00s)', '---
    FAIL: TestRun_ProgressRenderingStuckBatchIsNotDone (0.00s)', '--- FAIL: TestRun_SpecFieldsMapped
    (0.00s)', '--- FAIL: TestRun_ReclaimsLiveOrphanedOrchestratorAtEntry (0.00s)',
  '--- FAIL: TestRun_PersistsOrchestratorStrandBeforeWait (0.00s)', '--- FAIL: TestRun_FreshStopsSupersededRunsLiveStrands
    (0.00s)', '--- FAIL: TestSpawnBatch_RoleSelectionMatrix (0.00s)', '--- FAIL: TestSpawnBatch_PauseSentinel
    (0.00s)', '--- FAIL: TestSpawnBatch_StaleReportRefusal (0.00s)', '--- FAIL: TestSpawnBatch_RecoveryArchivesStaleReport
    (0.00s)', '--- FAIL: TestSpawnBatch_FingerprintMismatchRefused (0.00s)', '---
    FAIL: TestSpawnBatch_DeadRespawnReclaimsKeptSubstrate (0.00s)', '--- FAIL: TestSpawnBatch_RestartChainPersistsStateBeforeSpawn
    (0.00s)', '--- FAIL: TestSpawnBatch_RestartChainStopsLiveMemberStrands (0.00s)',
  '--- FAIL: TestSpawnBatch_RestartChainFromNonLowestMemberSpawnsLowest (0.00s)',
  '--- FAIL: TestSpawnBatch_InFlightGuardMatrix (0.00s)', '--- FAIL: TestSpawnBatch_ChainAnchorRecordedOnce
    (0.00s)', '--- FAIL: TestSpawnBatch_StatePersisted (0.00s)', '--- FAIL: TestSpawnBatch_SpecFieldsMapped
    (0.00s)', '--- FAIL: TestSpawnBatch_RestartChainOnChainlessBatchErrors (0.00s)',
  '--- FAIL: TestSpawnBatch_RestartChainClearsStaleReportBeforeRefusal (0.00s)', '---
    FAIL: TestParsePlan_PlanValidFixture (0.00s)', '--- FAIL: TestRestartChain_ChainlessErrors
    (0.00s)', '--- FAIL: TestRestartChain_UnrecordedAnchorErrors (0.00s)', '--- FAIL:
    TestChainMembers (0.00s)', '--- FAIL: TestChainEndFor (0.00s)', '--- FAIL: TestValidate_PlanUnapproved_TripsCheck1
    (0.00s)', '--- FAIL: TestRestartChain (0.00s)', '--- FAIL: TestValidate_PlanBrokenChain_TripsCheck4Twice
    (0.00s)', '--- FAIL: TestValidate_PlanValidFixture_ZeroFindings (0.00s)', '---
    FAIL: TestParsePlan_OtherFixturesParseCleanly (0.00s)', '--- FAIL: TestParsePlan_HasRenameMechanic
    (0.00s)', "FAIL\tgithub.com/Knatte18/loomyard/internal/builderengine\t0.146s",
  "FAIL\tgithub.com/Knatte18/loomyard/internal/builderengine\t0.142s"]
  - name: comment and test vocabulary sweep
    state: pending
    verify_baseline_failures: []
  - name: vocabulary enforcement test
    state: pending
    verify_baseline_failures: []
  - name: documentation
    state: pending
    verify_baseline_failures: []
```
