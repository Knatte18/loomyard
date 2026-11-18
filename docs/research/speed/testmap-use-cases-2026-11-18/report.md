# What the per-test coverage maps are good for, 2026-11-18

Board task `testmap-use-cases` asked what the coverage maps of `internal/testcover` can do for lym and its agents.
The gate selects no test from them any more, and the redundancy list has proved a reading filter.
The run searched research and industry practice on per-test coverage, and read the map format and the earlier reports.
It measured what the hub's stored maps hold, and built one fresh map to see how fast a stored one goes wrong.
It changed no code.
Its one test run is `lym gate test --wait --map ./internal/boardengine`, once, at `dcc5e7ac`.

## Answer in brief

- What only the maps give is the lookup between a function and the tests that reach it, across packages.
  Every use worth building reads that lookup in one direction or the other.
  None of them saves test time.
- Three uses rank first, and one reader of the maps serves all three:
  1. a mutation check of the code a diff changes, which reports each changed condition or statement no test notices;
  2. a reach note for a diff: the tests that reach each changed function, and the changed functions no test reaches;
  3. failure triage, which tells a fixer whether a failing test reaches anything the run changed.
- The maps need no upkeep per landing for these.
  A three-day-old map of `internal/boardengine` was 1,410 added production lines behind.
  It still held 98.5 percent of a fresh map's pairs of function and reaching test,
  and 99.2 percent of its own pairs were still true.
  A full build on command, 12 min 50 s, and a fresh map of the packages a diff touches, seconds each, are enough.
- Not worth it, each for the reason its entry under "Not worth it" gives:
  test selection and test ordering in the gate; a suspiciousness ranking of the Tarantula and Ochiai kind;
  cutting tests by coverage beyond today's reading filter; learned test selection;
  a conflict forecast between runs; code ownership; generated "tested by" documents.

## What the stored maps hold

The hub's shared maps under Bolt were read as they stood: 212 map files, 56 MB, the largest 4.1 MB.
198 hold blocks and tests; the other 14 belong to packages with no mapped test.
Of the 198, 192 stand at `5dc7a587` of 2026-11-15 and the other six at two other commits.
[data/per-package.tsv](data/per-package.tsv) holds the per-package figures.
[data/tests.tsv](data/tests.tsv) holds the per-test ones.

| Measure | Value |
|---|---|
| Test entries | 5,394 |
| Entries with a passing measurement | 4,709: 4,474 at tier 0 and 235 at tier 1 |
| Entries the spawn scan marks, which hold no coverage | 675, 12.5 percent |
| Entries with an outcome instead of coverage | 10 |
| Blocks in the packages' own files | 89,605 |
| Blocks a test of the own package covers | 64,255, 71.7 percent |
| Covered blocks that exactly one test covers | 29,113, 45.3 percent of the covered |
| Tests per covered block | median 2, mean 6.8, 90th percentile 15, most 272 |
| Measured tests per package | median 8, mean 23.8, 90th percentile 63, most 272 |
| Functions holding a block | 11,684 |
| Functions no measured test of any package reaches | 1,170, 10.0 percent |
| Functions some test reaches | 10,615 |
| Reached by exactly one test | 2,204, 20.8 percent |
| Reached by five tests or fewer | 5,510, 51.9 percent |
| Tests per reached function | median 5, 90th percentile 97, 99th percentile 1,179 |
| Functions that 500 or more tests reach | 153 |
| Reached from the tests of one package only | 5,303, 50.0 percent |
| Reached by no test of their own package | 793, 7.5 percent; 693 of them in a package that has measured tests |
| Functions one tier 0 test reaches | median 86, 90th percentile 188, most 806 |
| Functions one tier 1 test reaches | median 703, 90th percentile 824, most 1,736 |
| Linked packages one test reaches | tier 0 median 10, most 87; tier 1 median 66, most 95 |

In the own package a function counts as reached by a test when the test covers one of its blocks.
In another package it counts when the map lists it among the test's linked functions.
The unreached functions cluster: `internal/gatecli` holds 202 of them and `internal/fabricengine` 177.

What the maps do not hold bounds every use below:

- Top-level tests only; a subtest folds into its parent.
- Tiers 0 and 1 only, and no test the spawn scan marks.
  A function only such a test runs shows as unreached.
- Another package's code at function grain; only the own package at block grain.
- No hit counts and no line ranges: a block's key is its file, function, text hash and ordinal.
- No coverage of a failing run: a failed test's entry records its outcome alone.
- One platform, the builder's.

## What a map costs, and how fast it goes stale

The measurement of 2026-11-18 built all maps in 12 min 50 s over 210 packages
([report](../coverage-map-measurement-2026-11-18/report.md)).
This run's build of `internal/boardengine` reported 4.3 s of build and 3.5 s of test runs for 139 tests.

Upkeep per landing is what made the maps cost more than they saved ([payoff report](../testmap-payoff-2026-11-15.md)).
The check this run spent its one build on is therefore staleness.
The stored `boardengine` map dates from `5dc7a587`.
Four commits touched the package since, adding 1,410 production lines and deleting 55.
The fresh map was compared with the stored one as sets of pairs of a function and a test that reaches it:

| Measure | Stored map, `5dc7a587` | Fresh map, `dcc5e7ac` |
|---|---|---|
| Test entries | 136 | 144 |
| Blocks | 2,602 | 2,974 |
| Linked functions | 253 | 256 |
| Pairs of function and reaching test | 17,511 | 17,640 |

| Pairs | Count |
|---|---|
| In both maps | 17,376 |
| Share of the fresh map's pairs the stored map holds | 98.5 percent |
| Share of the stored map's pairs still true | 99.2 percent |
| Missed by the stored map | 264 |
| of them, pairs of the 8 tests added since | 181 |
| of them, pairs of a kept test with one of 13 functions the stored map lacks | 82 |
| of them, pairs of a kept test with a function both maps know | 1 |
| Stale in the stored map | 135, all on one function |

All 136 stored tests still exist, 129 with unchanged text.
Of the 2,602 stored block keys, 2,568 are still keys of the fresh map.
The 135 stale pairs are `lock.TryAcquireWriteLock`, which 135 `boardengine` tests reached and none reaches now;
`internal/lock` did not change, so a caller stopped calling it.
The stored map itself points at 263 of the 264 misses:
they belong to a test it lacks, or to a function in a file whose hash differs from the one it records.

So a stored map is a sound advisory index for days, and its gaps sit where its own file hashes say they are.
This is one package over one interval; the bound is in Limits.

## Every use found

A build cost is given in darns.
A darn is one writer run with its review.
It costs $5.45 and 38 awake minutes on average, and $8.72 and 62 minutes at the `solid` profile
([darn cost report](../../loom/darn-cost-2026-11-08/report.md)).
The costs are estimates from the size of the change, not measurements.

| # | Use | Source | What it needs from lym | Build cost | Expected gain |
|---|---|---|---|---|---|
| 1 | Mutation check of the code a diff changes | [Petrović et al.](https://arxiv.org/abs/2102.11378), [Petrović et al., ICSE 2021](https://arxiv.org/abs/2103.07189), [Meta ACH](https://arxiv.org/abs/2501.12862) | A gate report step: mutants in the diff's new blocks, each run against the tests covering its block, survivors listed for the review | A trial darn, then 3 to 4 darns; per diff seconds to minutes of test runs under a budget | Changed behaviour no test checks is found before it lands; the only use that sees assertions |
| 2 | Reach note for a diff: the tests reaching each changed function, and changed functions nothing reaches | [coverage.py contexts](https://coverage.readthedocs.io/en/6.5.0/contexts.html), [IntelliJ](https://www.jetbrains.com/help/idea/code-coverage.html), [Teamscale](https://docs.teamscale.com/reference/test-gap-analysis), [Eder et al.](https://wwwbroy.in.tum.de/publ/papers/AST13Hauptmann.pdf), [Ivanković et al.](https://research.google/pubs/code-coverage-at-google/), [TestPrune](https://arxiv.org/abs/2510.18270) | A reader of the maps, a finder of the functions a diff changes, a note in the code review rubric, a read verb for writers | 2 darns; a full map build on command now and then | The review's coverage findings rest on facts; a writer sees the tests its change will hit, in other packages too |
| 3 | Failure triage: does the failing test reach what the run changed | [DeFlaker](https://cs.cornell.edu/~legunsen/pubs/BellETAL18DeFlaker.pdf), [Chianti](https://research.ibm.com/publications/chianti-a-tool-for-change-impact-analysis-of-java-programs), [AutoFL](https://arxiv.org/abs/2308.05487), [AutoCodeRover](https://arxiv.org/abs/2404.05427), [Ziftci and Reardon](https://research.google/pubs/who-broke-the-build-automatically-identifying-changes-that-induce-test-failures-in-continuous-integration-at-google-scale/) | Lines in the gate's failure report and the fixer's brief, from the reader of use 2 | 1 darn on top of use 2 | A fixer starts at the changed function the test reaches, or learns the failure is not the run's; small in money, see below |
| 4 | Sweep for pseudo-tested functions: covered, yet no test fails when the body is emptied | [Niedermayr et al.](https://arxiv.org/abs/1611.07163), [Vera-Pérez et al.](https://arxiv.org/abs/1807.05030), [Descartes](https://arxiv.org/abs/1811.03045) | One body-emptying operator in the mutant planner and a verb over a package's map | 2 darns after use 1; about 90 minutes of test runs for the module, at the survey's 0.49 s a mutant | A list of functions that tests run and never check, for test writing or for the code cleanup project |
| 5 | Behaviour-drift signal: tests whose covered blocks change in code a diff did not touch | Own idea, after DeFlaker's differential coverage | Maps of the touched packages at the base and at HEAD, and a comparison | 2 to 3 darns; two map builds per run | Unknown; a refactor that changes a path is flagged without a test failing |
| 6 | List of functions no test reaches | [Teamscale](https://docs.teamscale.com/reference/test-gap-analysis), [Productive Coverage](https://research.google/pubs/productive-coverage-improving-the-actionability-of-code-coverage/) | Nothing new once the reader of use 2 exists | None beyond use 2 | A by-product; one plain coverage profile gives the same list without the maps |
| 7 | Audit of test breadth and tier | Own analysis of the stored maps | A query over the maps | None beyond use 2 | A by-product: a list of tier 0 tests that reach hundreds of functions |
| 8 | Suspiciousness ranking (Tarantula, Ochiai, DStar) | [Pearson et al.](https://homes.cs.washington.edu/~rjust/publ/fault_localization_effectiveness_icse_2017.pdf), [Parnin and Orso](https://web.eecs.umich.edu/~weimerw/2018-481/readings/debugging-helping.pdf) | Coverage of failing runs, which the maps do not record, and a ranker | 3 darns | Little beyond use 3 |
| 9 | Cutting redundant tests | [Rothermel et al.](https://bibtex.github.io/ICSM-1998-RothermelHOH.html), [Inozemtseva and Holmes](https://cs.ubc.ca/~rtholmes/papers/icse_2014_inozemtseva.pdf) | Exists: `lym gate redundant` and the gate's redundancy report | None | 2.89 s of removable test time; a reading filter only |
| 10 | Test selection in the gate | [Ekstazi](https://github.com/gliga/ekstazi), [Datadog](https://docs.datadoghq.com/tests/test_impact_analysis/how_it_works/), [Azure Pipelines](https://learn.microsoft.com/en-us/vsts/build-release/test/test-impact-analysis), [pytest-testmon](https://pypi.org/project/pytest-testmon) | The selector and upkeep `72b26d1a` removed | Retired | 0 to 4.2 s a gate against 7.7 to 31 minutes of upkeep |
| 11 | Test ordering, covering tests first | [Rothermel et al., 2001](https://digitalcommons.unl.edu/csearticles/9) | An ordered `-run` list per package | 2 darns | None: a tier 0 or tier 1 package runs in seconds and the gate reports every failure |
| 12 | Learned test selection | [Machalica et al.](https://arxiv.org/abs/1810.05286) | A history of failures per change and a model | Large | None at lym's volume |
| 13 | Coverage-guided test writing | [CoverUp](https://arxiv.org/abs/2403.16218), [TestGen-LLM](https://arxiv.org/abs/2402.09171) | The unreached functions of use 2, handed to the writer | None beyond use 2 | Folded into use 2; a separate generator adds nothing a writer does not already do |
| 14 | Conflict forecast between runs in flight | [Brun et al.](https://cs.ubc.ca/~rtholmes/papers/fse_2011_brun.pdf) for the aim; the coverage variant is an own idea | The reader, joined with `lym loom peers` | 2 darns | Low: the landing gate tests the merged tree anyway |
| 15 | Change-risk score from the number of reaching tests | Own idea | A count in the reach note | None beyond use 2 | No use alone; the count shows in use 2's note |
| 16 | Code ownership from who tests what | The task's own list | An owner per module | None | Not applicable |
| 17 | Generated "tested by" documents | [TCTracer](https://link.springer.com/article/10.1007/s10664-021-10079-1) | A generator and its upkeep | 1 to 2 darns | None beyond the read verb of use 2 |

## Ranking

1. Mutation check of the code a diff changes (use 1).
2. Reach note for a diff (use 2).
3. Failure triage (use 3).
4. Sweep for pseudo-tested functions (use 4), once use 1 has shown the mutants are worth reading.
5. Behaviour-drift signal (use 5), as a trial only.
6. The by-products of the reader (uses 6, 7, 13 and 15), which cost nothing more and justify no build alone.
7. Not worth it: uses 8 to 12, 14, 16 and 17.

The order is by expected gain.
The build order differs.
Use 2's reader comes first because uses 1 and 3 read through it, then use 3, then the trial of use 1.

## The top three

### 1. Mutation check of the code a diff changes

After a gate's tiers pass, a report step mutates only the blocks the diff added or changed.
For each mutant it runs the tests whose map entries cover the mutant's block, and stops at the first that fails.
A mutant no test kills goes to the code review with its file, line and operator.
There it is a candidate for the rubric's finding "a behavior the diff adds with no coverage".
`internal/testcover` already plans mutants per block, builds them into one test binary and judges the kills.

Outside evidence:

- Google mutates only the lines a change touches and shows the surviving mutants in code review.
  Petrović and others report 2 million mutants surfaced over 760,000 changes.
  They also report that developers at first called 85 percent of the mutants unproductive.
  The system therefore caps mutants per line and per review and filters them by context.
- The same authors found that developers who see mutants write more tests.
  They also found mutants coupled with real faults:
  on the change that introduced a past high-priority fault, mutation testing would have reported a live mutant.
- Meta's ACH keeps a generated test only when it kills a mutant no existing test kills.
  Engineers accepted 73 percent of its tests.

Evidence in lym:

- The redundancy survey found 27 of 30 confirmed candidates false
  ([survey](../redundancy-confirmed-survey-2026-11-16/report.md)).
  Equal coverage and equal kills do not show equal assertions.
  That use reads mutation as evidence of sameness.
  This use reads it the other way, where the evidence is direct: a mutant that survives is a change no test noticed.
- The survey ran 18,868 mutants in 186,075 test runs and 9,252 s: 9.9 runs and 0.49 s a mutant, 0.05 s a run.
- A first-parent commit since 2026-11-01 changes 111 production Go lines at the median and 782 at the 90th percentile.
  `boardengine` holds one block per 3.6 production lines, so that is about 30 and about 220 blocks.
  With a cap on mutants per diff, a step costs one mutant build and seconds to a few minutes of test runs.
  This is an estimate from those ratios; no such step was run.

Why this beats doing without the maps:

- Without per-test coverage each mutant runs every test of its package, 23.8 on average and up to 272.
  With the map it runs the tests covering its block, 2 at the median and 6.8 on average, fewer when one fails early.
- The map shows which changed blocks no test covers at all.
  Those are reported as uncovered and no mutant is spent on them.
- The map of a changed package has to be fresh, since new code has new blocks.
  That is the build measured above, seconds per package, in the run's own worktree; it is not the stored set.

Bounds and risks:

- The step builds one package's test binary, so a mutant only a test of another package kills shows as a survivor.
  The reach note of use 2 names those tests, so the review can discount it.
- Unproductive and equivalent mutants cost review tokens.
  Google's figure says a cap and a filter are part of the first build, not a later refinement.
- No lym figure says how often a landed diff holds an unchecked change.
  The first step is therefore a trial.
  It replays the step by hand over about twenty landed diffs, counts the survivors and reads a sample.

### 2. Reach note for a diff

For each function a diff changes, the note names the tests that reach it.
Tests of the own package come first, and among them the most specific.
The note also lists the changed functions no test reaches.
The code review reads it as it reads the redundancy report today.
A read verb gives a writer the same answer for a function before it edits.

Outside evidence:

- coverage.py records which test ran each line and shows it per line in its report.
  IntelliJ offers "Show Tests Covering Line".
- Teamscale's test gap analysis reports methods changed since a baseline and executed by no test.
  The study behind it, Eder and others, found about half of the changes of the system it studied untested.
- Google shows coverage per changelist in code review, where the authors say it becomes actionable.
- TestPrune maps coverage to the functions an issue points at.
  It cuts the regression tests an agent runs about a thousandfold.
  Its authors report 8.0 to 12.9 percent more issues resolved, in relative terms, by three agents.
- A caution from TCTracer: counting all executed code as tested code is inaccurate, so the note has to rank and cap.
  In lym 153 functions are reached by 500 tests or more.

Evidence in lym:

- The rubric already asks the reviewer for "a behavior the diff adds with no coverage".
  It also asks that a redundancy finding name the covering test.
  Today the reviewer answers both by searching and reading.
- Code-Review's reviewers, judge and fixers cost $2.03 of a darn's $5.45.
- By the staleness check above, the stored map answers for unchanged code.
  A fresh map of the diff's packages answers for the rest.

Why this beats doing without the maps:

- A search by name finds the tests that call a function directly.
  It cannot find tests that reach it through callers.
  793 reached functions, 7.5 percent, are reached by no test of their own package,
  and half of all reached functions are reached from more than one test package.
- A search cannot show that nothing reaches a function; the maps list 1,170 such functions.
- A plain coverage profile shows the unreached functions too, but names no test.
  Naming the tests is the part the review's two findings need.

Bounds:

- The note inherits the maps' gaps.
  A function only a spawn-marked or tier 2 test runs reads as unreached, and the note has to say so.
- The finder of changed functions went with the selector in `72b26d1a`.
  A smaller one, comparing function texts, has to be written.
- A lookup across packages reads every map that links the changed package, up to 56 MB of JSON.
  The reader therefore needs an index.

### 3. Failure triage

When a gate fails, the failure report gains a line for each failing test the maps hold.
The line names the functions changed by the run that the test reaches, or says it reaches none.
The fixer's brief carries the same lines.
For a parent branch that went red, the same lookup runs over the landings since the last green.
It names those whose changed functions the test reaches.

Outside evidence:

- DeFlaker marks a newly failing test flaky when it executed no changed code, with no rerun.
  Its authors report 95.5 percent recall of confirmed flaky tests and a 1.5 percent false alarm rate.
- Chianti reports, for each test a change affects, the changes that affect it.
  On its subject system those were 3.88 percent of the atomic changes on average.
- AutoFL gives a model the methods a failing test covers as its search space.
  It ranks the fault first in 149 of 353 Defects4J cases.
  AutoCodeRover's authors report that adding coverage-based localisation raised the share of issues it resolved.

Evidence in lym:

- Eight landings since 2026-11-01 list a failure that passed on a rerun.
  Four of those failures are in tests the stored maps hold.
  None of the four tests reaches a file its landing changed.
  Each reaches 213 or 226 functions, in 35 or 46 files:

  | Landing | Failing test | Production Go files the landing changed | Of them reached by the test |
  |---|---|---|---|
  | `f26165a6` | `dobbyengine` `TestLandNotice_ReachesAnOrchStrandListedAfterTheLanding` | 3 | 0 |
  | `e97e2474` | the same test | 11 | 0 |
  | `9ce7233b` | `slackengine` `TestRunRelay` | 4 | 0 |
  | `4eab123d` | the same test | 10 | 0 |

  DeFlaker's rule would have called all four flaky before any rerun.
  The other four failures are a tier 1 test added after the maps were built and a tier 2 test, twice each.
- The gate fixer ran in 92 of 291 darns and cost $0.27 a run, so the money at stake is small.
  The landing gate already reruns a failed test once.
- What the lookup adds is the case a rerun does not settle:
  a failure that repeats and is not the run's own, which today costs a round trip to the parent to test the base.

Why this beats doing without the maps:

- Without them the same answer needs the failing test run alone under coverage at a commit where it passes.
  That is a coverage build of its package and one run, 4.3 s and under a second for `boardengine`, so it is affordable.
  The stored map has the answer already, for every mapped test.
  For a red parent branch it answers across landings with no build.
- This is the weakest of the three on that count.
  An on-demand measurement would serve the single failing test nearly as well.
  It ranks third because it costs one darn once use 2's reader exists.

Bounds: four failures at file grain, with maps of 2026-11-15 read against landings of 2026-11-15 to 2026-11-18.
A failing subtest is looked up by its parent, and a tier 2 or spawn-marked test not at all.

## Not worth it

- **Suspiciousness ranking (use 8).**
  The formulas need the coverage of failing runs beside passing ones, and the maps record none for a failed test.
  In a run the fault is almost always in the run's own diff, so the intersection of use 3 is the ranking.
  Pearson and others found the formulas indistinguishable on real faults,
  and Parnin and Orso found the assumptions behind a ranked list do not hold for programmers at work.
- **Cutting redundant tests (use 9).**
  Not worth more work than the reading filter that exists.
  The survey puts the removable time at 2.89 s and 27 of 30 confirmations false.
  The literature agrees: Rothermel and others found minimised suites lose fault detection,
  and Inozemtseva and Holmes found coverage a weak predictor of a suite's effectiveness.
- **Test selection in the gate (use 10).**
  Stays retired.
  It would pay only on tier 2 and above, where a test costs seconds to minutes and the maps hold nothing.
  Go can measure a spawned binary since 1.20, through `go build -cover` and `GOCOVERDIR`.
  The [Go documentation](https://go.dev/doc/build-cover) describes it, so maps of those tiers are possible.
  Building them is a project of its own and this report does not recommend it.
- **Test ordering (use 11).**
  Not worth it: nothing waits on the first failure of a package that runs in seconds.
- **Learned test selection (use 12).**
  Not worth it: it needs a failure history far larger than about 30 landings a day yield.
- **Conflict forecast between runs (use 14).**
  Not worth it now: `lym loom peers` already shows overlapping paths, and the landing gate tests the merged tree.
- **Code ownership (use 16).**
  Not applicable: lym has one operator and its agents own no module.
- **Generated "tested by" documents (use 17).**
  Not worth it: a generated list goes stale with every landing, and the read verb of use 2 answers on demand.

Uses 6, 7, 13 and 15 are worth having only as output of use 2's reader.
A list of unreached functions needs no per-test map, so it is no reason to keep the maps.

## Method

- **Web.**
  Searches on 2026-11-18 over the fields the task names, and over coverage in agent tooling.
  Each figure quoted from a paper comes from its abstract or a summary of it, and was not re-derived.
- **Stored maps.**
  `jq` over each `map.json` under the hub's shared map directory.
  A test counts when its entry holds no outcome and no spawn mark.
- **Staleness.**
  Each of the two `boardengine` maps became a set of pairs of a function and a test reaching it.
  A function is named by file and name in the own package, and by package, file and name elsewhere.
  The two sets were compared.
- **Flaky landings.**
  The `## Flaky test failures` sections of the first-parent commits since 2026-11-01 name the failures.
  The files each test reaches, by the stored map, were set against the production Go files the landing's diff changed.
- **Diff sizes.**
  Added plus deleted lines of non-test Go files per first-parent commit since 2026-11-01; 307 of the 398 change any.

## Limits

- The staleness check is one package over one three-day interval.
  A package with a changed embedded file or a changed widely used type may fare worse.
  The misses of [the miss-causes report](../testmap-miss-causes-2026-11-15.md) were of those kinds.
- The stored maps are three days old and their counts are of that tree.
- The triage check is four failures of two tests, at file grain.
- No use was built or trialled; every gain is argued from outside results and lym's own cost figures.
- Build costs are estimates in darns.
- The papers' figures are their authors' and come from Java, Python and Kotlin code bases, not Go.
- The run read abstracts and summaries, not every paper in full.
