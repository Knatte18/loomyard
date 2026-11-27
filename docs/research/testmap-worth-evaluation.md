# What the coverage maps and their users give, 2026-11-27

Board task `testmap-worth-evaluation` asked what every user of the per-test coverage maps gives, what it costs, and whether to remove them.
This report records one reading of the hub's records on 2026-11-27 and is not rewritten when the code changes.
No test and no map build ran for it.

## Recommendation

Remove every user of the stored maps, and the maps with them.
Keep `go run ./cmd/testtiming -redundancy` for an occasional redundancy sweep: it reads no stored map.

| User | Recommendation | Main reason |
|---|---|---|
| `reach` report step, the reach turn and the `reach_report` review marker | Remove | 18 review findings and some tests, no production defect; it is the main reason for the map refresh |
| `mutation` report step and the `mutation_report` marker | Remove, the closest call | 46 review findings, all test strength except where code reading had already found the bug |
| `redundancy` report step and the `redundancy_report` marker | Remove | 22 of its 24 findings kept the test and added a keep marker |
| `lym gate mutate` | Remove | Never run outside its own trial |
| `lym gate reach`, both forms | Remove | 5 runs outside build tasks, none of which confirmed an answer; `--test --since` never run |
| `lym gate redundant` | Remove | No test cut on its word; 27 of 30 confirmed candidates read by hand were false |
| `lym gate test --map` | Remove | Feeds only the users above |
| `cmd/testtiming -redundancy` | Keep | The one redundancy tool that led to a large cut (188 test functions, 2026-11-08) |
| The red-first gate | Keep, not a map user | It reads `testreach.ChangedTests`, which needs no map |

`internal/testcover` does not go whole.
`cmd/testtiming` runs on its runner, verdict, spawn scan and keep scan.
Its map record, mutant code and source scan go, as do the map reader and index of `internal/testreach`.
By file, that is about 8,800 non-test and 8,500 test lines removed, and 2,600 and 1,800 kept.
The red-first gate also shares helpers with the redundancy step's files, which a removal has to move first.
A removal also retires `PATTERN-coverage-map-reader`, the `redundancy_*`, `reach_budget_min` and `mutation_*` keys of `gate.yaml`,
and the map parts of `PATTERN-test-speed` and `docs/overview.md`.

Where the call could go the other way:

- **Mutation step.**
  It gave the most findings, 18 of them MEDIUM and 2 BLOCKING.
  If review without them lets weak tests through, this is the step to turn back on.
  It cannot run without the map refresh, which is most of the cost.
- **`lym gate redundant`.**
  Its mutation check refutes candidates that coverage alone accepts: 542 of 972 in the last full survey.
  Keeping it keeps `--map`, the map record and the mutant code, for a sweep that was planned once and dropped before it ran.

## Users of the maps

| Reader or writer | Reads or writes | Who starts it |
|---|---|---|
| `lym gate test --map` | writes the shared maps from the prime, a worktree's own elsewhere | by hand only |
| `reach` report step | refreshes the worktree's maps, writes `reach.json` | a gate key listing `reach` |
| Reach turn (`verifytree.WithReachTurn`, also in Weaver) | `reach.json` | the verify gate of a writer or fixer |
| `redundancy` report step | one package's map | a gate key listing `redundancy` |
| `mutation` report step | refreshes the maps, writes `mutation.json` | a gate key listing `mutation` |
| Code-Review's three report markers | the three reports | the review |
| `lym gate reach`, `lym gate mutate`, `lym gate redundant` | the maps | by hand only |

`internal/testreach` and `internal/testcover` have no importer outside `internal/gatecli` and `cmd/testtiming`.
No dobby, loom row, gate key or stencil starts a map run.
Dobby did start the predecessor verb `gate testmap` from 2026-11-09 until `72b26d1af` removed that on 2026-11-15.
The one message that tells anyone to run a verb is the rebuild hint in `internal/testreach/mapread.go`.

## What the report steps gave

The window is 2026-11-19 12:17, when all three steps existed, to the archive's newest tag on 2026-11-27.
It holds 145 archive tags of 134 slugs; 106 slugs had a code review.
The steps were on for the window up to 2026-11-26 22:15.

### Code review findings

Findings that used a report as evidence, over all recipes ([citations.tsv](testmap-worth-evaluation/citations.tsv)):

| Report | Findings | BLOCKING | MEDIUM | LOW | NIT |
|---|---|---|---|---|---|
| Mutation survivor | 41 | 2 | 16 | 21 | 2 |
| Mutation uncovered block | 5 | 0 | 2 | 3 | 0 |
| Reach | 18 | 1 | 8 | 9 | 0 |
| Redundancy | 24 | 4 | 0 | 15 | 5 |
| All | 88 | 7 | 26 | 48 | 7 |

- They come from 55 runs.
  Reviews explicitly dismissed about 40 more report items, 31 of them survivors.
- Over the 95 reviewed darn runs of the window, 80 of 814 findings used a report (10%), and 33 of 294 findings of MEDIUM or higher (11%).
- **No report found a production defect on its own.**
  Three findings led to a bug fix (`agents-exit-before-reply` F2, `parent-review-code-and-drift` F17, `webster-amend-stop-no-notice` F3).
  In each, code reading had argued the bug and a survivor backed it up; the first rests on an unverified premise.
- Five reach findings changed production code only so that a test could reach it, and one survivor finding removed a dead input.
- The rest added or tightened tests.
- Redundancy: 22 findings added `//testtiming:keep`, one deleted a test (`config-keys-nested` F12),
  and one deleted a test row the report had called unique (`card-gate-scope-gaps` F6).

### Reach turns

- 66 reach turns in 43 runs.
- 30 turns edited a test file (26 runs), and 23 of them added test functions: 38 functions in 19 runs.
- One cost showed in review: in `gate-dotnet-fixes`, tests added for functions the report called unreached
  were then raised as duplicates of an out-of-process test the map could not see (F16, BLOCKING).

### Report counts

Each run is counted by its largest report.

| Step | Runs with a summary line | Judged | Flagged |
|---|---|---|---|
| Mutation | 106 | - | 208 survivors in 56 runs, 2,210 uncovered changed blocks |
| Redundancy | 103 | 674 new or changed tests | 39 contained, in 25 runs |

## What the manual verbs gave

Runs by hand or from an agent session, found in transcripts and trace logs since the verbs landed.
Logs undercount, so an unlogged operator run would not show.

| Verb | Runs | Outside build and study tasks | What those gave |
|---|---|---|---|
| `gate test --map` | 10 | 1 | `seed-killed-way-forward-loops`: the "unreached" functions came from a stale map; no test added |
| `gate redundant` | 147 logged, up to 212 more in one loop | 0 | - |
| `gate reach <file>#<Func>` | 5 | 5 | Two sessions found tests the maps did not see; one filed issue #250, on direct tier-1 calls, which `e13faedc2` addresses |
| `gate reach --test --since` | 0 | 0 | - |
| `gate mutate` | 20 | 0 | The trial: 430 mutants, 374 killed, 44 survived, 12 unjudged |
| `testtiming -redundancy` | 3, none since 2026-11-09 | - | Fed the tier-1 study and the `fabricengine` cuts |

The shared maps in the prime were last written on 2026-11-16.
`lym gate reach` run from the prime answers from them.

### Redundancy sweeps

| Date | Tool | Candidates | Outcome | Source under `speed/` |
|---|---|---|---|---|
| 11-06 | testtiming | 106 in 12 packages | 5 removable tier-1 tests worth 1.4 s | `tier1-test-revision-2026-11-06` |
| 11-08 | testtiming method | 370 contained of 590 | 188 test functions removed, 64 marked; 4.3 s off the package, 4.5 s off a tier-1 pass (`cbdfc33db`) | `fabric-test-redundancy-2026-11-08`, `fabric-test-cuts-remeasure-2026-11-08.md` |
| 11-12 | maps | 2 | both false | `coverage-map-pilot-2026-11-12` |
| 11-16 | maps | 1,026 in 118 packages | 11.75 s of test time, none removed | `redundancy-candidates-survey-2026-11-16` |
| 11-17 | `gate redundant` | 972 | 430 confirmed, 3.45 s; 27 of 30 read were false | `redundancy-confirmed-survey-2026-11-16` |
| 11-17 | `gate redundant`, after a fix | 6 in `boardengine` | 5 read, 5 false | `redundant-candidates-check-2026-11-17` |

No test was removed since 2026-11-09 on the word of `gate redundant`.
The planned sweep, board entry `redundancy-sweep-existing`, was removed unrun on 2026-11-18.

## What they cost

### Gate time and slots

From the archived traces of `lym gate test` calls with a gate key, 2026-11-19 12:17 to 2026-11-26 22:15.
Six calls longer than an hour, which span hub pauses, are left out.
110 slugs made such calls, and 100 of them ran a report step.

| Part | Calls | Work, sum | Work, median | Slot wait, sum | Waits over 60 s | Test processes |
|---|---|---|---|---|---|---|
| Map refresh, for reach and mutation | 202 | 17,951 s | 57 s | 5,685 s | 37 | 103,783 |
| Mutation | 198 | 4,977 s | 10 s | 2,012 s | 10 | 14,374 |
| Redundancy | 151 | 2,138 s | 10 s | 2,548 s | 10 | 1,327 |

- In all, 7.0 hours of step work and 2.8 hours of slot waiting: about 250 s of work and 100 s of waiting per run that ran a step.
- Work is the span from a step's first to its last log line.
  Steps run beside the tiers and each other, so these are not added wall time and the spans are approximate.
- Each step holds its own gate slots, which other runs' tests wait for.
- Added wall time has an upper bound: the `darn-write` and `code-burler-fix` calls spent 18,832 s after their last tier ended,
  about 170 s per run, lints included.
  The 2026-11-21 study measured 122 s per run before `42be7bcf7` moved the reports beside the tests.

### Memory and disk

- Each map or mutant test process is capped at `gate.redundancy_test_memory_mb`, 2,048 MB, with up to a tier's `parallel` side by side.
- 12 coverage test runs ended at that cap in the traces of 2026-11-20 to 2026-11-26.
- The one measured full build peaked at a 2.1 GB process and at most 2.4 GB over the test processes (`coverage-map-measurement-2026-11-18`).
- The shared maps take 56 MB; a worktree's own take up to 11 MB.

### Map run time

A full `lym gate test --map --all` took 12 min 50 s on 2026-11-18.
`e13faedc2` added the unresolved tests to the map run after the window, and no full build has been timed since.

### Test time

In one whole tier-1 pass on 2026-11-27, the top-level tests of the map, reach, mutation and redundancy tools in `internal/gatecli`
summed to about 150 s, with the package at 55 s wall.
The note `whole-repo-guards-off-gates` measured 194 s for them in a loaded landing gate.
`internal/testcover` added 3 s.
Turning the report steps off saves none of this: the tests run whenever the diff reaches those packages.

### Upkeep

- 11,363 non-test and 10,280 test lines belong to the maps and their users, `cmd/testtiming`'s redundancy mode included:
  2.7% and 1.7% of the module's Go.
- 38 commits since 2026-11-01 touched those files.
  Among them are repairs of the tools themselves: `007de1547`, `bd707e161`, `b904a1f80`, `cac2f1789`, `e13faedc2` and `65d516486`.
- Reach has answered wrongly where a map was stale or a test unseen, in the manual runs above and in the 2026-11-21 study.

## The effect of turning the report steps off

**It cannot be measured yet.**
The mutation step went off on 2026-11-26 22:15, and reach and redundancy on 2026-11-27 08:14, minutes before this reading.
No reviewed darn run has landed with reach and redundancy off.
One reviewed darn run started after the mutation step went off.
Defects that land were not measured: the records link no later defect to the run that introduced it.

The baseline to compare with, per reviewed darn run, with findings from the merged review rounds:

| Period, by run start | Runs | Review rounds | Findings | HIGH or BLOCKING | MEDIUM | From the test lens |
|---|---|---|---|---|---|---|
| 11-12 to 11-19, before reach and mutation | 80 | 1.29 | 8.35 | 0.70 | 2.05 | 2.25 |
| 11-19 to 11-26, all three on | 95 | 1.16 | 8.57 | 0.88 | 2.21 | 1.95 |

Adding the reports showed no clear change in these figures.
The comparison is weak: lenses, rubrics, models and tasks changed in the same weeks, and the redundancy step already ran in part of the first period.
Expect at most the 10% of findings that cited a report to be lost, and likely fewer,
since a reviewer can raise a missing test without a report.
After about 20 darn runs land with the steps off, `findings.sh` below gives the third row.

## Method

- Archive tags of the records repository dated from 2026-11-19 12:17; `seed.json` gives the recipe and the start.
- Review citations: the review, lens and fixer files of each tag, searched by wording and read where they matched.
  A reviewer who used a report without naming it is missed.
  Whether a fix touched only tests rests on fixer reports, not on every diff.
- The scripts in [testmap-worth-evaluation/](testmap-worth-evaluation/) print the figures; each says its usage at its top.
  - `cost.sh 2026-11-19T12:17 2026-11-26T22:15 > cost.tsv`, then `awk -F'\t' '$4<=3600' cost.tsv > kept.tsv` and `cost-summary.sh kept.tsv`.
  - `outcomes.sh <slug>...` for reach turns and the report summary lines.
  - `findings.sh <tag>...` for the review findings per darn run.
- Test times come from the hub's tier-1 gate log `20261127T072001Z-tier1-4017911`.
- Line counts are of the files at `a69580709`.

Earlier study of the same steps over 10 runs: [testmap-use-measure-2026-11-21](speed/testmap-use-measure-2026-11-21/report.md).
