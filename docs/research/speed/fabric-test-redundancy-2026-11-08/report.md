# internal/fabricengine tier 1: which tests protect nothing the others do not (2026-11-08)

A measurement report.
It deletes and moves no test.
Block coverage does not see assertions, and `fabricengine` manages worktrees and hubs, where a lost test can cost the operator data.
A later task decides the cuts.

## Result

- 590 top-level tier 1 tests ran, each once on its own under one coverage binary scoped to `internal/fabricengine`. None failed.
  The 169 other top-level tests are tier 0 and skip under tier 1.
- The 590 tests cover 6369 distinct blocks between them.
- 370 tests (63 %) are contained: every block they cover another tier 1 test also covers.
  68 of them already carry `//testtiming:keep`, so 302 are unmarked.
- Dropped one by one from the slowest, 329 of the 370 can go together without uncovering a block.
  267 of those are unmarked.
  Their standalone cost is 78 s of the 236 s all 590 cost standalone.
- Block coverage is coarse here.
  A test covers from 3 to 1870 blocks, and 498 of the 590 cover 300 or more, mostly the shared fixture path (`Add`, hub setup).
  107 contained tests have a rarest block that exactly one other test covers, so they stand on a single partner.
- Nearly every tier 1 test starts git: 583 of the 590 start at least one process, with a median of 141 per standalone run.
  A standalone run builds its fixtures afresh, so these counts and the costs below include setup a serial suite pays once.

The numbers come from `pertest.tsv`, `contained.tsv`, `greedy.tsv`, `gitprocs.tsv` and `summary.txt` beside this report.

## Method

1. `coverage-runner.sh` is the `-exec` program of one `lym gate test --tier 1` run of the package under `-cover -coverpkg=./internal/fabricengine`.
   It runs the compiled test binary once per top-level test with `-test.parallel=1`, one run in flight, and a fresh profile and `GIT_TRACE2_EVENT` directory per test.
2. `analyze-coverage.sh` reads the profiles and logs.
   A block is a profile line's file and range, covered when its count is above zero.
   A test is contained when no block it covers has a count of one across the 590 tests.
   The greedy set walks the contained tests slowest first and drops one when every block it covers still has another test left.
3. `git-census.sh` counts the trace2 files of each test, one per git process.
4. `pure-logic-candidates.sh` compares each test's rare blocks (covered by at most 5 % of the tests) with a tier 0 coverage run of the package,
   which starts no git.
5. The gate prints "ran no test ... tier's command lacks its tag" for the first step, because the runner prints no `go test -json` events.
   The runner's own output is what the analysis reads, and the gate's counters are not used.

Cost is the larger of a test's own `--- PASS` line and the sum of its leaf subtests' lines, as the per-test times report ranks.
The report's own figures agree where it lists a test: `TestCrossProduct` 7.53 s there, 8.23 s here; `TestPushFailureClasses` 4.79 s there, 4.89 s here.
Small tests cost more here than in a serial suite, because each standalone run builds its own hub template.
Read the cost column as an upper bound on a drop's saving and as a ranking.

### The committed redundancy report disagrees

`docs/benchmarks/test-redundancy.md`, written by `go run ./cmd/testtiming -redundancy`, lists no candidates for `internal/fabricengine`.
That tool measures every block of the whole module (`-coverpkg` names every package), so a test's blocks include the fixture packages' blocks
and a test is rarely contained.
This report scopes coverage to `internal/fabricengine`, as the task asked, which makes more tests look contained.
A cut that survives the module-wide check is the safer one; the package-scoped list is where to start reading.

## List 1: contained tests, slowest first

Unmarked tests only (no `//testtiming:keep`), all in the greedy set, so they can go together.
Rarest is the number of tier 1 tests covering the test's rarest block, itself included; 2 means one partner.

| Test | Cost (s) | Rarest | Git processes |
| --- | --- | --- | --- |
| `TestRemove_ArchiveOrder` | 1.29 | 2 | 785 |
| `TestCommitWeft_MachineLocalArtifactsNeverEnterWeftTreeAtAnyDepth` | 1.03 | 9 | 291 |
| `TestStates` | 0.97 | 18 | 444 |
| `TestWireJunctions_ScratchLink` | 0.87 | 2 | 382 |
| `TestRemove_UnlinksScratchLinkBeforeWorktreeRemoval` | 0.86 | 2 | 447 |
| `TestCleanup_ForceIsReservedAndChangesNoVerdict` | 0.70 | 4 | 194 |
| `TestUnwire_LeavesSiblingWorktreeUndirtied` | 0.67 | 6 | 293 |
| `TestCleanup_DryRunMatchesApplyVerdict` | 0.62 | 4 | 191 |
| `TestReconcile_ConvergesAllWorktreesToRepoWidePathspec` | 0.61 | 17 | 169 |
| `TestMerge_DirtyTargetHalts` | 0.59 | 3 | 327 |
| `TestAdd_RunRecords` | 0.58 | 2 | 316 |
| `TestVerbCases_ArrangeSetsItsPreconditionField` | 0.57 | 143 | 278 |
| `TestMerge_MessagePrecedence` | 0.56 | 6 | 322 |
| `TestMergeCrucible_RemoveRefusesWhenALinkedPairIsConsumingTheSource` | 0.55 | 2 | 302 |
| `TestMerge_CrashRecovery` | 0.55 | 2 | 308 |
| `TestAdd_LiveLocalWeftAgainstOrigin` | 0.54 | 2 | 389 |
| `TestCommitLock_ContendsAcrossSides` | 0.54 | 22 | 65 |
| `TestMerge_NoUpstreamSidePassesVacuously` | 0.54 | 6 | 317 |
| `TestMerge_PreMergeSyncRunsInsideTheWriteLock` | 0.54 | 4 | 226 |
| `TestRebuildIndex_EqualsIncrementallyBuiltIndex` | 0.53 | 9 | 151 |

The full ranked list is `contained.tsv`, and the greedy set `greedy.tsv`.

## List 2: tests that start git or build a hub and assert logic that needs neither

Read from `purelogic.tsv` (few rare blocks that need git) and then from the source.
None of these is certain: each still needs git for something it asserts, as the notes say.

| Test | Cost (s) | Where the pure logic sits |
| --- | --- | --- |
| `TestUnwire_LeavesSiblingWorktreeUndirtied` | 0.67 | which `info/exclude` lines survive an unwire, a function of the set of wired anchors |
| `TestList` | 0.41 | the `git worktree list --porcelain` parser: first entry is Main, prunable entries, bare repo and error shapes |
| `TestSourceDirtyPaths` | 0.43 | the parse of `git status` into dirty tracked paths, and the typed refusal built from it |
| `TestVerbCases_ArrangeSetsItsPreconditionField` | 0.57 | a check that three test-table fields are non-empty after `Arrange` |
| `TestStates` | 0.97 | a check that each fixture state changed something on disk, run per anchor |
| `TestWarpLayoutFor_FastPathMatchesResolveWorktree` | cheap | path-layout equality of two branches; a shared hub, already `//testtiming:keep` |
| `TestWireJunctions_ScratchLink` | 0.87 | link and mutation-record logic over a plain directory tree, run at two anchors |

## What each top candidate asserts that block coverage does not show

- `TestRemove_ArchiveOrder`: the archive order, enforced by an update hook on origin that refuses a tag whose target is not already on the pushed weft branch.
  A diverged origin tip gets a tag of its own, an ahead tip gets a tag and no branch push, a retryable tag refusal is retried,
  and a refused branch push never fails the removal.
  The re-added pair is a fresh pair, not an adoption of the archived tip.
  Coverage sees none of the hook outcomes.
  Keep: it guards the data an archive tag preserves.
- `TestRemove_LeasedWeftDelete`: the delete of the weft branch on origin is leased on the tip the archive read or pushed.
  An origin that moved is kept and reported, and an origin without the branch gets no delete attempt, counted by a hook.
  Keep: it guards a remote delete.
- `TestCommitWeft_MachineLocalArtifactsNeverEnterWeftTreeAtAnyDepth`: lock files, pause flags and rendered prompts under `.lym` never reach the weft tree at the root or two nested anchors, while `_lym` state does.
  The three anchors cover the same blocks with different path arguments.
  Keep one anchor row at least; the nested anchor is the one needing `NewHub`.
- `TestStates`: each fixture state establishes what it claims at two anchors, and a structural state produces a manifest diff, so a silent no-op cannot pass.
  It tests the cross-product fixtures, not the product.
  Fold candidate: the vacuity check could fail inside `States`' own `Apply` path, leaving the per-state assertions to the cross product.
- `TestWireJunctions_ScratchLink` and `TestRemove_UnlinksScratchLinkBeforeWorktreeRemoval`: the mutation-record kinds and targets (`dir_created`, `link_created`, `link_removed`),
  their order against the warp and weft worktree removals,
  that a link is not rewritten by a write through it (an `Lstat` comparison),
  and that a real directory at the link path is never claimed or removed.
  Coverage cannot see the record contents or the order; the last check guards user content.
  The anchor loop of the first test doubles its cost for one extra path argument.
- `TestCleanup_DryRunMatchesApplyVerdict` and `TestCleanup_ForceIsReservedAndChangesNoVerdict`: both run `Add`, `Remove` and create an orphan weft branch on the same fixture.
  The first asserts a dry run agrees with `--apply` and that `--apply` deletes the orphan.
  The second asserts `force` changes no verdict and an unmanaged branch is never deleted.
  Their behaviours differ, but one hub could serve both: a fold of the two into one test saves a hub build.
- `TestUnwire_LeavesSiblingWorktreeUndirtied`: the exclude lines that remain per row (`/_lym`, bare `_lym`, `/sub/_lym`) after one of two re-anchored pairs unwires.
  A wrong strip makes every sibling report untracked dirt and trips `Remove`'s dirty gate.
  The expected table is pure; only reading the exclude path needs git.
- `TestMerge_MessagePrecedence`: git's own prepared squash message when `Message` is empty, a verbatim subject when set, and an unchanged weft HEAD.
  The message text is observable only through `git log`; coverage shows neither branch's outcome.
- `TestVerbCases_ArrangeSetsItsPreconditionField`: three copies of a hub, one per verb, to check that a fixture field is non-empty.
  The shared check skips an empty field, which is the gap this test closes.
  Fold candidate: make that check fail on an empty field for the verbs that need one, and drop the test.
- `TestList`, `TestSourceDirtyPaths`: parsers of git output, asserted against real git.
  Their tier 0 form needs the parser to take a string, which `List` and `SourceDirtyPaths` do not today.
- `TestCommitWeft_ExcludeMagicPassesThroughUntouched`, `TestCommitWeft_IndexOnlyDeletionCountsAsMatch`, `TestCommitWeft_WidenedPathspecTolerance_*`:
  the pathspec filter's behaviour is observed through what git stages.
  The first two already carry `//testtiming:keep` and the reasons hold: the oracle is git itself.

## For the later task

- Start from the 267 unmarked tests in `greedy.tsv`, but confirm each against the module-wide check of `go run ./cmd/testtiming -pkg ./internal/fabricengine -redundancy` first.
- The two folds with the clearest saving and no lost assertion are the pair of `TestCleanup_*` tests onto one hub, and `TestVerbCases_ArrangeSetsItsPreconditionField` into the check it supports.
- The anchor loops of `TestWireJunctions_ScratchLink`, `TestStates` and `TestCommitWeft_MachineLocalArtifacts...` run the same blocks again per anchor;
  a row that needs a nested anchor stays, the others are the cheapest cut by cost.
- Dropping a test whose rarest count is 2 is safe only together with its partner staying: `greedy.tsv` is built to hold that, a hand-picked set is not.
- Most of the suite's time is spread over several hundred 0.1 to 0.5 s tests, each starting a hub copy.
  The greedy set's 267 unmarked tests include 125 of 0.3 s or more standalone, 99 of 0.1 to 0.3 s and 43 under 0.1 s; cutting the first group is where most of the time is.
