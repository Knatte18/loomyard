# internal/fabricengine: time of every test, one by one (2026-11-07)

A measurement report: one run per tier, on a quiet machine, with `-parallel=1`.
The package's tier 1 takes 132 s serial, and no test dominates it.
About 1500 leaf tests share it, with a median of 60 ms, and the slowest ten groups of tests take 28 %.

## Method

Each pass ran in the foreground, once:

1. `lym gate test --tier 0 ./internal/fabricengine -- -count=1 -run . -parallel=1`
2. `lym gate test --tier 1 ./internal/fabricengine -- -count=1 -run . -parallel=1`
3. `lym gate test --tier 2 ./internal/fabricengine -- -count=1 -run . -parallel=1`

The gate recorded `go test -v` text, not `go test -json` events, so each elapsed time is the `--- PASS: <name> (<s>s)` line of the recorded stdout.
A subtest line carries the subtest's own time, and a serial parent's line includes its serial subtests.
A parallel subtest runs after its parent's function returns, so its time is on its own line and not in the parent's.
Both facts matter for the ranking below.

### Quiet machine

The one-minute load stood at 3.2 to 3.5 with no other run in flight at the start.
It fell below 1.0 after about 5 minutes of waiting, well inside the 30-minute cap, and every pass ran with no run in flight.

| Pass | One-minute load before | Runs in flight before |
| --- | --- | --- |
| Tier 0 | 0.90 | none |
| Tier 1 | 0.83 | none |
| Tier 2 | 3.34 | none |

The load before tier 2 is the carried average of the tier 1 pass just before it.
Tier 2 had no test to run, so it timed nothing.

## Package wall time of each pass

| Pass | Gate wall | `go test` package time | Tests passed | Tests skipped |
| --- | --- | --- | --- | --- |
| Tier 0 | 0.9 s | 0.150 s | 921 | 590 |
| Tier 1 | 137.5 s | 132.028 s | 1728 (590 top-level, 1138 subtests) | 169 |
| Tier 2 | 2.1 s | none: no package of `./internal/fabricengine` holds a tier 2 test | 0 | 0 |

The gate wall of tier 1 adds about 5 s of build and launch to the package time.
Tier 0 is 921 passed lines with a combined elapsed time of 0.09 s: the slowest, `TestRunTeardownChild_AnswersBusyOnAHeldLock/BoundedWait`, took 0.05 s, `TestMergeVocabulary_GuardReasonSetMatchesConstBlock` 0.02 s and `TestHoldRecordsWriteLock` 0.02 s.
Every other tier 0 test reads 0.00 s.
Tier 0 holds no time worth cutting, so the rest of this report is tier 1.

## Where the tier 1 time goes

- The leaf tests (a test with no subtest) add up to 128.5 s of the 132.0 s, over 1505 leaves.
  The remaining 3.5 s is the serial parents' own work and the process.
- The median leaf takes 60 ms and the 90th percentile 190 ms.
  The slowest leaf, `TestPullHub_MovesEverySideOriginMovedThenReportsUpToDate`, takes 0.75 s.
- Leaves of 0.10 s or more are 529 of the 1505 and add up to 91.9 s; the 976 faster ones add up to 36.6 s.

### Ranking by test, with its subtests included

A top-level test's own line understates it when its subtests are parallel, so the ranking below takes, per top-level test, the larger of its own line and the sum of its leaves.
The 590 groups add up to 131.1 s, which agrees with the 132.0 s of the package.

- The slowest ten groups take 36.7 s, 28.0 % of the package time.
- The ten slowest top-level lines as printed (`TestPushFailureClasses` to `TestRemove_TeardownLockAgainstThePairsPush` in the second table) take 25.5 s, 19.3 %.
- The ten slowest leaves take 5.0 s, 3.8 %.

| # | Test | Tier | Leaves | Elapsed (s) |
| --- | --- | --- | --- | --- |
| 1 | `TestCrossProduct` | 1 | 132 | 7.53 |
| 2 | `TestPushFailureClasses` | 1 | 32 | 4.79 |
| 3 | `TestAnchorChange` | 1 | 31 | 3.94 |
| 4 | `TestBolt_PullThenCommitWritten` | 1 | 16 | 3.70 |
| 5 | `TestRecordsReplay` | 1 | 30 | 3.20 |
| 6 | `TestReconcileOrigin_Judge` | 1 | 24 | 3.00 |
| 7 | `TestAdd_AdoptOnly` | 1 | 20 | 2.96 |
| 8 | `TestWorktreeSeam_Events` | 1 | 15 | 2.59 |
| 9 | `TestStoreHousekeeping` | 1 | 7 | 2.57 |
| 10 | `TestReconcileOrigin_Moves` | 1 | 17 | 2.38 |
| 11 | `TestReconcileOrigin_Ended` | 1 | 19 | 2.34 |
| 12 | `TestRegenerateHubWorkspace` | 1 | 23 | 2.28 |
| 13 | `TestReconcile_MigratesMarkerlessWefts` | 1 | 13 | 1.66 |
| 14 | `TestResetPairCode_RemoteHalf` | 1 | 10 | 1.53 |
| 15 | `TestBranchPrefixChange_PairWithNothingLeftLocally` | 1 | 7 | 1.47 |
| 16 | `TestPull_WeftDivergedWarpAdvancesCleanly` | 1 | 9 | 1.41 |
| 17 | `TestRemoveLocal` | 1 | 9 | 1.31 |
| 18 | `TestRemove_TeardownLockAgainstThePairsPush` | 1 | 6 | 1.21 |
| 19 | `TestVerbsRegenerateHubWorkspace` | 1 | 9 | 1.14 |
| 20 | `TestPairTaskBranch_ReadsThePairItself` | 1 | 11 | 1.13 |
| 21 | `TestBolt_PushRecorded_DropsSeedCommitsOnRebaseConflict` | 1 | 4 | 1.11 |
| 22 | `TestRemove_ArchiveOrder` | 1 | 5 | 1.06 |
| 23 | `TestRemove_LeasedWeftDelete` | 1 | 5 | 1.02 |
| 24 | `TestCleanupRemoteWarp_ApplyDeletesOnlyTheCandidate` | 1 | 4 | 0.97 |
| 25 | `TestReconcile_SweepsRetiredHubArtifacts` | 1 | 18 | 0.95 |
| 26 | `TestRemove_FinishesWorktreeDirGitLeftOnDisk` | 1 | 6 | 0.95 |
| 27 | `TestSlugRecordsTags` | 1 | 6 | 0.87 |
| 28 | `TestReadConfigChanges` | 1 | 11 | 0.86 |
| 29 | `TestTeardownLock_SerialisesAddDiscardAndPairPush` | 1 | 6 | 0.83 |
| 30 | `TestDiscardTaskBranch` | 1 | 5 | 0.77 |

### The slowest tests and subtests as printed

Every row is tier 1, the 60 slowest of the 1728 printed lines, as the gate printed them.
A parent's line includes its serial subtests, so a parent and its subtest both appear.
The lines of the other 1668 tests are 0.31 s or less each, and the 976 leaves under 0.10 s add up to 36.6 s.
The full list is in the gate's recorded stdout of the pass, under `.lym/gate/detach/` of the worktree that ran it.

| Test | Elapsed (s) |
| --- | --- |
| `TestPushFailureClasses` | 4.79 |
| `TestAnchorChange` | 3.65 |
| `TestReconcileOrigin_Judge` | 3.00 |
| `TestWorktreeSeam_Events` | 2.59 |
| `TestStoreHousekeeping` | 2.57 |
| `TestReconcileOrigin_Moves` | 2.37 |
| `TestReconcileOrigin_Ended` | 2.34 |
| `TestReconcile_MigratesMarkerlessWefts` | 1.65 |
| `TestRemoveLocal` | 1.30 |
| `TestRemove_TeardownLockAgainstThePairsPush` | 1.21 |
| `TestRemove_ArchiveOrder` | 1.05 |
| `TestRemove_LeasedWeftDelete` | 1.02 |
| `TestReconcile_SweepsRetiredHubArtifacts` | 0.95 |
| `TestPushFailureClasses/ARejectionThatOutlastsTheBackoffWithOriginHoldingNothingTheBranchLacksIsNoDivergence` | 0.86 |
| `TestPushPairAnchored_PushesBothSidesRetriesAndReportsEachSide` | 0.76 |
| `TestPushFailureClasses/AParkedPushOfABranchGoneOnOriginDoesNotRecreateIt` | 0.76 |
| `TestPullHub_MovesEverySideOriginMovedThenReportsUpToDate` | 0.75 |
| `TestPushFailureClasses/AWeftPushThatMustReplayStopsOnAnAnchorChange` | 0.61 |
| `TestBoltReplayers` | 0.61 |
| `TestStoreHousekeeping/the_in-flight_probe:_an_empty_answer_packs_the_stores` | 0.59 |
| `TestStates` | 0.58 |
| `TestStoreHousekeeping/the_in-flight_probe:_live_sessions_of_other_pairs_skip_the_gc` | 0.53 |
| `TestStoreHousekeeping/Remove_on_the_default_nil_probe_packs_the_stores,_pruning_unreachable_loose_objects_older_than_a_day_and_keeping_younger_ones` | 0.53 |
| `TestReconcile_MigratesMarkerlessWefts/AddAdoptsABranchWithNoUsableMarkerAtItsContentsAnchor` | 0.52 |
| `TestCommitLock_ContendsAcrossSides` | 0.52 |
| `TestBolt_PullThenCommitWritten/a_held_push_lock_delays_the_drop_and_the_write` | 0.52 |
| `TestStoreHousekeeping/the_in-flight_probe:_a_failing_probe_skips_the_gc` | 0.51 |
| `TestPushFailureClasses/ATaskBranchRewoundOnOriginIsHeldBackUntilTheRewindIsSettled` | 0.46 |
| `TestMergeVerbs_ForeignMergeState_EverySideAndShapeRefuses` | 0.44 |
| `TestPushFailureClasses/ARejectionThatOutlastsTheBackoffWithOriginHoldingNothingTheBranchLacksIsNoDivergence/Records` | 0.42 |
| `TestPullHub_StoppedSideLeavesTheOthersPulled` | 0.42 |
| `TestWorktreeSeam_Events/The_clone_verb_prints_seam_warning_naming_the_render_verb_for_a_failing_seam_and_empty_for_a_healthy_one` | 0.38 |
| `TestAdd_FromParentPair` | 0.38 |
| `TestPushLock_BoundedWaitGivesUpAndUnboundedWaitBlocks/PushPairAnchored` | 0.37 |
| `TestCheckout_RefusesAnotherAnchor/A_target_pushed_from_a_second_clone_and_never_fetched_is_refused_with_no_local_weft_branch_created` | 0.37 |
| `TestReconcileOrigin_Ended/An_entry_done_on_origin_with_a_local_pair_is_handed_to_the_teardown_seam_once_and_reported_ended_as_landed` | 0.36 |
| `TestPullHub_StopsABoardSideAndAPairSideWithTheirDetail` | 0.36 |
| `TestRegenerateHubWorkspace/SkipsPairWhoseAnchorSubdirWasDeleted` | 0.35 |
| `TestPushLock_BoundedWaitGivesUpAndUnboundedWaitBlocks/PushAnchored` | 0.35 |
| `TestMerge_PreMergeSyncRunsInsideTheWriteLock` | 0.35 |
| `TestMerge_NoUpstreamSidePassesVacuously` | 0.35 |
| `TestMerge_MessagePrecedence` | 0.35 |
| `TestMerge_DirtyTargetHalts` | 0.35 |
| `TestRunTeardownChild_RunsTheRemoteStepsInOrderUnderTheLock` | 0.34 |
| `TestCommit_InvokesPushRecorder` | 0.34 |
| `TestMidMerge_ForeignState_EverySideAndShape` | 0.33 |
| `TestMerge_CrashRecovery` | 0.33 |
| `TestMergeCrucible_ConflictsIsEmptyNeverNil` | 0.33 |
| `TestCrossProduct/./staleWiredJunction/Remove` | 0.33 |
| `TestBoltReplayers/With_no_replayer_registered_the_ended-set_read_and_the_owner_read_fail_with_ErrReplayerMissing` | 0.33 |
| `TestWeftGuards_DetachedWeftDoesNotRefuseWarpDetachedStillDoes` | 0.32 |
| `TestBolt_PushRecorded_DropsSeedCommitsOnRebaseConflict/a_retry_that_also_fails_returns_its_error_and_leaves_the_dropped_board_clean` | 0.32 |
| `TestAdd_RunRecords` | 0.32 |
| `TestAdd_AdoptOnlyRollbackKeepsReusedRecordsWorktree` | 0.32 |
| `TestWeftGuards_AbortLeavesWeftCommitsDuringAttemptWindowIntact` | 0.31 |
| `TestReconcileOrigin_Judge/An_origin_that_moved_nothing_is_read_by_two_ls-remote_calls_and_no_fetch` | 0.31 |
| `TestPullHub_HonorsSkipGitWithEverySideUpToDate` | 0.31 |
| `TestCrossProduct/backend/dirtyWarpUntracked/Cleanup` | 0.31 |
| `TestCrossProduct/backend/bothDirty/Checkout` | 0.31 |
| `TestCrossProduct/./dirtyWarpUntracked/Cleanup` | 0.31 |

## What the slowest fifteen spend their time on

Read from the code of each test and of the fixtures it calls.
The git-process counts are counted from the test and `gitkit` helpers where the code is in the test files, and a guess where the count sits in production code the review did not open.

A fixture costs one of two ways in this package.
A hub copy (`hubforge.CopyHub`, `CopyRegistered`) pays a template build once per binary, then per call relocates every text file of the template and runs 2 git processes.
A scratch hub (`hubforge.NewHub`, `CloneSecond`) runs the whole clone-and-wire on every call, which is dozens of git processes (a guess).
`AddPair` adds a pair with its worktrees and records commit, and then a foreground push of both sides; it is probably 20 to 40 git processes (a guess).
Test-side helpers add to that: `gitkit.CommitFile` is 3 git processes and the `originCommit` family is 5.

1. **`TestCrossProduct`**, 7.53 s over 132 leaves, about 57 ms each.
   Each leaf copies a hub, so there is no scratch build.
   The leaf then walks and hashes the whole hub twice, with `CaptureManifest` before and after the verb, then runs one verb (Add, Remove, Prune, Cleanup, Checkout, Pull, Reconcile or Unwire) in one of 10 states.
   The time is the copy, the two manifest walks and the verb's git processes, in that order.
   The state setup adds 2 to about 14 git processes, and the Pull arrangement about 9.
2. **`TestPushFailureClasses`**, 4.79 s, 32 leaves, serial because it installs a process-global notifier.
   Each leaf copies a hub with a board replay installed, and the pair fixtures also call `AddPair`.
   Each leaf then drives a push, which retries a declined push through the backoff, but the backoff sleep is injected as a counter, so no fixed wait runs.
   Each declined push launches git with a `pre-receive` shell hook, and a failed fetch or ls-remote per class adds more launches.
   The `ARejectionThatOutlasts...` subtest alone is 0.86 s: 4 leaves, each a copy, a commit and a push that the hook declines on every backoff step (git process per step).
3. **`TestAnchorChange`**, 3.94 s over 31 leaves, 3.65 s of it serial in the parent.
   Every leaf copies the anchor hub and calls `AddPair` once or twice (`mover`, `stayer`).
   The serial leaves with a failure hook, five of them, each begin a change and apply or fail it, and then re-run it.
   `newIntegrateHub` adds a `CloneSecond`, a re-anchor, a push from the second clone and a fetch, so the three integration leaves are the most expensive: about 0.3 s each.
   Move, restore and verification add `requireMovedToSub`, which runs `git show`, `ls-files` and `status`.
4. **`TestBolt_PullThenCommitWritten`**, 3.70 s over 16 leaves.
   It is the only test among the fifteen that builds from scratch: each leaf calls `hubforge.NewHub`, 16 clone-and-wire builds one after another.
   After that it adds an `otherMachine` clone with a commit and a push, and then the Bolt fetch, rebase and replay.
   One leaf holds the push lock and sleeps a fixed 300 ms (`a_held_push_lock_delays_the_drop_and_the_write`, 0.52 s).
5. **`TestRecordsReplay`**, 3.20 s over 30 leaves, about 107 ms each.
   Each leaf copies a hub, and four add a pair.
   Almost all the time is test-side git: a push of the prime weft, an `originCommit` (5 processes), one to three local `CommitFile` calls (3 each) and one or two fetches, 15 to 30 processes before the replay under test, which cherry-picks or rebases per commit.
   There are no sleeps.
6. **`TestReconcileOrigin_Judge`**, 3.00 s over 24 leaves, serial because it sets a process-global hook.
   Each leaf copies a template hub with `newOriginHub` or `newOriginTaskHub`, then runs `ReconcileOrigin` (2 ls-remote, up to 2 fetches, and a verdict, move or replay per branch) one to several times.
   The first leaf makes three `otherMachine` clones with a commit and a push each, about 15 processes.
   Another leaf polls the lock files of both repositories from the call hook, which adds a try-acquire of two locks per remote call.
7. **`TestAdd_AdoptOnly`**, 2.96 s over 20 leaves, about 148 ms each.
   Each leaf copies a hub and runs one `AddPair`; the `removedPair` leaves also run a `Remove`, which is a full local teardown.
   The adopt-only `Add` under test comes next, and then drops of 2 to 6 processes.
   `pushCommitToOrigin` (about 6 processes) and a bare clone (about 5) account for the rest.
8. **`TestWorktreeSeam_Events`**, 2.59 s over 15 leaves.
   Its parent builds six template copies and `NewHub` plus `CloneSecond` before the leaves: a scratch hub and a second clone, which is two of the heaviest fixtures, run once.
   The leaves are `AddPair` calls (`p1`, `p2`) and `fabriccli.CloneAndWire` calls that clone both bare repositories and wire a hub (0.38 s for the clone-verb leaf).
   Each wire fires the made event and `excludeLinesPresent` runs `git rev-parse --git-common-dir` per event.
9. **`TestStoreHousekeeping`**, 2.57 s over 7 leaves, serial because it lowers a process-global gc threshold.
   Four leaves call `NewHub`, so they are scratch builds, and each also runs `AddPair`, which together with the build is most of their 0.5 to 0.6 s.
   Then `fillSampledLooseObjects` writes 4000 files in a loop with a `MkdirAll` and a `WriteFile` each, and runs `git add` and `git commit` on them, a heavy file and index load, and `Remove` runs a foreground `git gc --auto` over the packed store.
   The other three leaves copy a hub and add a pair.
10. **`TestReconcileOrigin_Moves`**, 2.38 s over 17 leaves, serial.
    About 24 template copies of the origin, moves and ended templates.
    Per leaf, one to six `ReconcileOrigin` calls (guess 20 to 50 processes each), plus two to five `originCommitFile` runs of 5 processes.
    Locks are held by the test, so a busy outcome returns at once.
11. **`TestReconcileOrigin_Ended`**, 2.34 s over 19 leaves, serial.
    Each leaf copies a hub (11 `endedHub`, which adds an `originBoard` with 5 processes), and four add pairs.
    Teardown is a fake, so a removal costs nothing.
    Per leaf: one `ReconcileOrigin`, 5 to 15 test-side processes and zero to four `AddPair` calls.
12. **`TestRegenerateHubWorkspace`**, 2.28 s over 23 leaves, about 99 ms each.
    Each leaf copies a hub and the leaves add 29 pairs between them.
    A regenerate is file work, so the time is `AddPair` first, then the copy.
    One leaf waits for a held write lock until the test releases it, and one uses a 50 ms lock timeout.
13. **`TestReconcile_MigratesMarkerlessWefts`**, 1.66 s over 13 leaves, serial.
    Eleven leaves copy a hub that already holds one pair.
    The other two copy a hub, add a pair, remove it and clone a bare repository, then re-anchor and add again.
    Each leaf then reconciles once or twice over two wefts (guess 20 to 40 processes a pass), with push replaced by a recorder, so there is no push cost.
    `seedBoardMarkers` adds about 8 processes in seven leaves.
14. **`TestResetPairCode_RemoteHalf`**, 1.53 s over 10 leaves.
    Each leaf copies a hub, runs one `AddPair`, two `CommitFile` calls, a push and a `remote get-url`, about 8 test-side processes after the `AddPair`.
    `ResetPairCode` then runs an ls-remote or fetch, a push with a lease and a `reset --hard` (a guess of 5 to 10 processes).
    One leaf plants a `pre-receive` hook that exits 1.
15. **`TestBranchPrefixChange_PairWithNothingLeftLocally`**, 1.47 s over 7 leaves.
    Each leaf copies a hub, adds a pair, pushes twice and removes the pair locally with a full teardown, so every leaf pays an `AddPair` and a `Remove`.
    The action under test, a Remove with remote, a Discard or a refusal, and a few branch-exists checks come after that.

The common cost is the pair and the git processes around it, not a wait.
Fixed waits are rare: one `time.Sleep` of 300 ms in `TestBolt_PullThenCommitWritten`, and a 50 ms lock timeout in `TestRegenerateHubWorkspace`.
Backoff delays are injected, so none of the push tests waits them out.
The scratch builds (`hubforge.NewHub`) sit in `TestBolt_PullThenCommitWritten` (16 leaves) and `TestStoreHousekeeping` (4 leaves), and no other test of the fifteen builds a hub from scratch.

## Reading the numbers

- With `-parallel=1` the package takes 132 s; the 42 to 44 s of the earlier reports is the same work run in parallel, a factor of about 3.
- The serial time is flat: the slowest test group is 5.7 % of the package, and the 976 leaves under 0.10 s are 28 % of it.
  A cut of one slow test moves the package by seconds at most, and a cut of the per-leaf floor (the hub copy and `AddPair`) moves it by a multiple.
- A leaf's floor is a hub copy, whose cost is relocating every text file of the template and 2 git processes, and then most leaves add an `AddPair` of 20 to 40 processes (a guess).
