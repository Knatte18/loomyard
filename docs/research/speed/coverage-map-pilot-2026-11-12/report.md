# Coverage-map pilot on `internal/adoptcli`, 2026-11-12

Board task `test-redundancy-candidates` asked three questions of one package's coverage map, in order:
can a map be built, what does a full set cost, and is the map of any use for cutting tests or scoping the gate.
This report answers them for `internal/adoptcli` at commit `384d7cb7`, on one machine.
It cuts and folds no test.

## Answer in brief

- The map builds: one `lym gate testmap internal/adoptcli` call succeeded in 34.3 s wall and 47.8 s CPU,
  holding one of the hub's three gate slots.
  The verb reports 3.4 s of build and 3.0 s of run; most of the rest is bookkeeping between rounds of tests.
- A full set is estimated at 45 to 165 minutes on one slot, 65 to 230 CPU-minutes.
  The hub's upkeep pass had already built the full set today in 44 minutes, at the low end of the range.
- For cutting, the map is a reading filter, not a verdict:
  of the 30 judged tests, 2 are candidates, and both assert something their covering tests do not.
- For scoping, the map is usable:
  over two landed commits the selector never left out a test that covers the changed code.
  It cut the scope to no test for one commit and fell back to the whole package for the other.

## Choice of package

`internal/adoptcli` holds 34 mapped tests: 28 tier0 and 6 tier1.
The median package with mapped tests holds 8, and the largest, `internal/fabricengine`, 578.
So it sits above the median but far from the top.
It was chosen for three reasons:

- it has tests of both mapped tiers, so the build includes the `lym` prebuild a tier1 test needs;
- 28 commits touched it since 2026-10-20, so landed diffs exist to scope;
- it is not `internal/fabricengine`, which the task excludes.

## 1. Build

| Measure | Value |
|---|---|
| Command | `lym gate testmap internal/adoptcli`, run from the prime's code worktree |
| Result | success; 34 entries, 32 measured, 2 recorded unrun because the spawn scan marks them |
| Wall time | 34.3 s (19:10:58 to 19:11:33) |
| CPU time | 47.8 s (42.3 s user, 5.5 s system), 139 percent of one core |
| Peak memory | 579 MiB resident |
| Gate slots | 1 of 3, held for the whole call |
| Verb's own counts | `build_seconds` 3.40, `run_seconds` 2.96 |

The call's trace log splits the 34.3 s:

| Phase | Wall |
|---|---|
| Start, hub and config reads, worktree status | 1.8 s |
| `go list` of the linked packages and the package trees | 1.0 s |
| Coverage build of the test binary | 1.7 s |
| `lym` build into the prebuilt store, a warm Go build cache | 2.0 s |
| 8 rounds of 4 parallel tests | 23.4 s |
| Audit round: rebuild and re-run of the 3 oldest entries | 2.6 s |
| Last map write and exit | 2.4 s |

Each round runs its tests in 0.1 to 0.7 s,
then spends about 2.6 s before the next starts:
keying each profile's blocks and linked functions, reading the worktree status and HEAD, and writing the map.
The package links 144 in-module packages, so each profile is keyed against a large function table.
That bookkeeping, about 0.73 s of wall per test, is the build's main cost.
`run_seconds` counts only the time inside rounds, so it under-reports a package's slot time by roughly a factor of ten.

## 2. Full-set estimate

The scaling input is [packages.tsv](packages.tsv):
the 201 packages with a tier0 or tier1 test, with each one's test count, tier split and linked-package count,
5,418 tests in all.

Model, from the pilot's trace:

- once per call: about 10 s for start, the `lym` build, the audit round and the exit;
- per package: about 2.7 s for the linked-package listing and the coverage build;
- per test: the round bookkeeping.

The per-test term makes the range.
One pilot cannot tell whether the 2.6 s between rounds is a cost per test or per round,
nor how it scales with the number of linked packages, which averages 80 per test across the set against the pilot's 144.

| Assumption | Per-test wall | Tests | Packages | Total wall |
|---|---|---|---|---|
| Low: cost per test, scaled to 80 linked packages | 0.41 s | 37 min | 9 min | about 45 min |
| High: cost per round, unscaled; 43 percent of tests run one per round | 2.9 s per round, 3,200 rounds | 155 min | 9 min | about 165 min |

The 43 percent is the share of top-level tests with no top-level `t.Parallel()`,
counted by a text scan of the test files.

- CPU time, at the pilot's 1.39 CPU seconds per wall second: 65 to 230 CPU-minutes.
- Gate-slot time equals the wall, since a build holds one slot from its first package to its last:
  45 to 165 slot-minutes.
- The upkeep dobby's passes drain it at the same wall when the hub's gates are idle.
  A pass takes its slot only while another stays free and gives it back once a gate needs it,
  so on a busy hub the drain stretches by every stretch in which gates hold two or more of the three slots.
- Assumed throughout: a warm Go build cache and a machine with nothing else heavy running.

The full set already exists.
The prime's dobby began starting upkeep passes on remaining work with `36621f0e`, landed at 18:09 today,
and its first pass measured 5,381 entries between 18:10:50 and 18:55:06, 44 minutes on one slot.
That puts the low assumption close to the real cost: per-test bookkeeping that shrinks with the linked set.
After the full build, a pass re-measures only the tests a landing affects.

## 3. Use

### Redundancy

No verb listed the redundancy candidates of a whole package's map before this task.
The gate's `redundancy` report step judges only the tests a diff adds or changes,
and `cmd/testtiming -redundancy` measures afresh rather than reading the map.
This task adds `lym gate redundant <package>`.
It reads the map, leaves out the keep-marked tests,
and judges the rest with `internal/testcover`'s verdict through the new `testcover.JudgeMap`.
It runs no test and answers in under a second.
Its answer for the package is [adoptcli-redundant.json](adoptcli-redundant.json):

| Group | Tests |
|---|---|
| Entries | 34 |
| Not judged: the spawn scan cannot classify a call | 2: `TestCommand_RunsFromABeam`, `TestHubCheckDeps_StaleStampRunsTheTunedFinalizeTiers` |
| Kept by `//testtiming:keep` | 2: `TestAdoptFields`, `TestTriesStore` |
| Judged | 30 |
| Candidates, all in the removable set | 2 |

| Candidate | Tier | Map seconds | Covered by |
|---|---|---|---|
| `TestReconcileSeamWaitsOnAPushLock` | 0 | 0.00 | `TestAdoptIntegration_SecondCloneTakesOver` |
| `TestWorkerArgs` | 0 | 0.00 | `TestRunAdopt`, `TestAdoptIntegration_SecondCloneTakesOver`, `TestCheckCmd_FromABeamExitsZeroWhileOneRuns` |

Each candidate's linked-package functions are covered by its covering tests too,
so a module-wide check would not tell them apart either.
Read against their covers:

- `TestReconcileSeamWaitsOnAPushLock` asserts that the reconcile seam passes a positive `PushLockWait`
  equal to `reconcilePushLockWait`.
  The integration test runs the seam but never reads the options, and nothing contends for the push lock there,
  so a zero wait passes it.
  The candidate adds that assertion.
  It is also a tier0 test whose only cover is a tier1 test, so deleting it would trade a test that costs no measurable
  time for reliance on a slower one.
- `TestWorkerArgs` round-trips five request shapes, `--all`, `--keep-local`, `--drop-local` and `--no-orch` among them,
  through `workerArgs` and the worker command's own flag parser.
  `TestRunAdopt` asserts the spawned argument strings for `root`, `alpha` and `root --no-orch` only,
  and never parses them back; the integration tests assert outcomes, not arguments.
  A worker flag renamed on one side only passes every cover.
  The candidate adds that assertion.

Neither candidate is a cut, and both cost nothing measurable to run.
Both would warrant a `//testtiming:keep` line naming what they pin; this task adds none.
The package already carried two keep markers from an earlier prune, which may explain the low yield.
One package says nothing about the yield elsewhere;
with the full set built, `lym gate redundant` lists any package's candidates in under a second with no build.

### Scoping

For each of two landed commits that touched the package,
the coverage selector, `testcover.Selector`, ran over the commit's diff from its parent.
A scratch program built the selector's input from the pilot map:
the map's entries as measured at `384d7cb7`,
with its own, linked and environment hashes recomputed from the commit's parent.
The commit is then the selector's target.
The tests that cover the changed code came from the same map:
every test with a block in a function whose lines the diff touched, in the package or a linked package,
plus every Test function the commit changed.
Both commits are within a day of the map's commit, and only `fixloop.go` of the package changed between them,
so the map's coverage stands in for the coverage at each commit.

| Commit | Changed | Selector names | Tests covering the change |
|---|---|---|---|
| `16881b21` beam merge-parent skips held slugs | `adoptcli/fixloop.go` `hubFixDeps`; 4 functions of `beamcli`; `shedrun.RunIDs` | none | none |
| `6f8d4cff` adopt: pass pauses, wait on push locks | 7 `adoptcli` functions, 9 `fabricengine` functions, 3 Test functions, 1 test helper | all 34: miss, a test file changed outside its Test functions | 6 |

- `16881b21`: no test of the package reaches `hubFixDeps`, the production wiring the change edits,
  nor the changed `beamcli` and `shedrun` functions.
  The selector ran nothing where a package-scoped gate runs all 34 tests; both sets are empty, so it missed nothing.
- `6f8d4cff`: the covering tests are `TestAdopt`, `TestAdoptIntegration_SecondCloneTakesOver`,
  `TestCommand_HereAdoptsTheSubjectOfItsWorktree`, `TestReconcileSeamWaitsOnAPushLock`,
  `TestRunAdopt` and `TestRunWorker`.
  The commit also added one line to the test helper `world.workerDeps`, whose only callers, `TestAdopt` and
  `TestRunWorker`, are among the six.
  The helper change is a `test file changed outside its Test functions` miss, so the selector ran the whole package:
  safe, at 34 tests where 6 suffice.

The selector was safe in both, and narrowed one diff to no test at all.
Its loss was the coarse miss on a test-helper edit,
of the same kind as the non-body miss the board note `selector-non-body-miss` proposes to narrow.
Bound: the covering sets are function-granular and come from the map itself,
so they cannot show a test the map fails to record; only a re-measure at each commit could.

## Verdict

- Cutting: usable only as a filter.
  The map and `lym gate redundant` narrow 30 judged tests to 2 to read, at no build cost,
  but neither candidate survives the read, because coverage cannot see assertions.
  A cut still needs a hand read of each candidate against its covers.
- Scoping the gate: usable.
  Over two commits the selector never dropped a covering test,
  and it ran no test for one of them where the package scope runs 34; its misses fall back to the whole package.
- Cost of a full run: 45 to 165 minutes on one gate slot, 65 to 230 CPU-minutes;
  measured at 44 minutes for today's full build by the upkeep pass, which then keeps the set current per landing.

## Outside the questions

- `lym gate testmap` from a non-prime worktree refuses with a way forward naming the prime's weft directory,
  `loomyard-weft`, which holds no `go.mod`; run there, the call refuses again for that.
  The command runs from the prime's code worktree, `loomyard`.
- The pilot's premise, that the full set was not yet built, was overtaken by `36621f0e` before this run started.
  The pilot ran as asked, and section 2 uses the dobby's pass as a check on the estimate.
