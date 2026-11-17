# Mutation-confirmed redundancy candidates of every package, and a read of 30 of them, 2026-11-16

Board task `redundancy-confirmed-survey` checks the list `lym gate redundant` prints since `e97e2474` confirmed each candidate by mutation.
It precedes the cut of `redundancy-sweep-existing`, as `redundancy-candidates-survey` preceded it for the old judge.
It changed no production code, no test and nothing under Bolt.
One machine with 12 CPUs, commit `e97e2474`, one run, with the hub's other runs live beside it.

## Answer in brief

- 430 tests stay confirmed across 94 of 121 judged packages: 422 at tier 0 and 8 at tier 1.
  The mutation check refuted another 542 candidates, so it removed 56 percent of the 972 the coverage report lists.
- The 430 take 3.45 s standalone together, and the 375 the greedy pass can delete together take 2.89 s.
  The cut saves seconds, not minutes.
- Of 30 confirmed candidates read, 27 are false confirmations: each asserts something its covering tests do not.
  A cut that trusts the list would delete about nine tests in ten that check something.
  `redundancy-sweep-existing` must read every confirmed candidate before it cuts, not a sample.
- Finding the 430 cost 9,252 s of mutation test runs, 2 h 34 min, over 18,868 mutants and 186,075 test runs.
  Seven packages cost over 500 s each, and `internal/seatengine` cost 1,372 s for one confirmed test.
- Recommendation: do not file the cut as a speed task; file it only as a test-quality cleanup that reads each candidate.

## The build

`lym gate testmap` with no package named, in this task's worktree at its own HEAD, once.
Raw output: [testmap-envelope.json](data/testmap-envelope.json), [testmap-time.txt](data/testmap-time.txt).

| Measure | Value |
|---|---|
| Wall time | 7 min 59 s (14:07:21Z to 14:15:20Z) |
| User and system CPU | 1,349.5 s and 288.3 s, 341 percent of one CPU |
| Peak resident memory | 2,065,372 kB |

## The runs

`lym gate redundant --envelope <package>` once on each of 121 packages, each reading the map the build left.
The packages are the ones the earlier survey found candidates in, plus the three the first sequential pass ran from the build's own package order.
A package that holds a candidate only under the new maps and not under the earlier survey's is therefore not in the set.

The first 14 packages ran one at a time.
The run was then interrupted at `internal/websterengine`, and the remaining packages with candidates ran three at a time,
which is the number of gate slots: 08:56:20Z to 09:54:46Z, 58 min 26 s for 102 packages.
Wall times of the parallel part include time beside two other verbs and the hub's other runs, so they are upper bounds of a lone run.
A wall time far above the package's mutation seconds is time before its test runs started, most likely a wait for a gate slot; that was not checked.
Per-package figures: [candidates-by-package.tsv](data/candidates-by-package.tsv); raw envelopes: [data/redundant/](data/redundant).

| Measure | Value |
|---|---|
| Packages judged | 121 |
| Packages with a confirmed candidate | 94 |
| Confirmed, of them removable together | 430, 375 |
| Confirmed by tier | 422 at tier 0, 8 at tier 1 |
| Standalone seconds of the confirmed, of the removable | 3.45 s, 2.89 s |
| Refuted | 542 (527 at tier 0, 15 at tier 1) |
| Not judged | 773 |
| Mutants, unbuildable, test runs | 18,868, 887, 186,075 |
| Mutation test-run seconds, summed | 9,252 s |

Reasons for the 773 not judged: unclassifiable call 492, no blocks 147, out of process 71, kills no mutant 24, its blocks hold no mutant 16,
the module's source files which no mutant changes 13, failed 10.

### Packages by confirmed candidates

The ten with the most confirmed ones; mutation seconds are the verb's own test-run time.

| Package | Confirmed | Refuted | Not judged | Mutants | Runs | Mutation s |
|---|---|---|---|---|---|---|
| `internal/shedengine` | 37 | 73 | 2 | 454 | 25,792 | 895 |
| `internal/shuttleengine/claudeengine` | 26 | 21 | 8 | 770 | 6,720 | 174 |
| `internal/battenshed` | 20 | 29 | 1 | 684 | 13,243 | 557 |
| `internal/shedverbs` | 16 | 15 | 7 | 223 | 1,879 | 51 |
| `internal/shedrecipe` | 16 | 13 | 1 | 740 | 5,524 | 113 |
| `internal/loomrecipe` | 16 | 35 | 5 | 87 | 2,236 | 56 |
| `internal/orchengine` | 15 | 58 | 1 | 627 | 27,574 | 578 |
| `internal/loomcli` | 15 | 18 | 54 | 725 | 2,137 | 534 |
| `internal/planglyph` | 14 | 11 | 16 | 569 | 2,402 | 59 |
| `internal/boardengine` | 14 | 29 | 1 | 1,284 | 30,987 | 1,123 |

## The sample

30 confirmed candidates from those ten packages, which hold 189 of the 430.
Each package gave a count near its share of the ten, and within a package the picks are evenly spaced over its confirmed tests sorted by name.
The list: [sample.tsv](data/sample.tsv).
Five subagents (Sonnet, one pass each, read-only) read the candidate and its covering tests, and judged whether the candidate asserts anything no covering test asserts, each verdict with a file and line.
Verdicts: [sample-verdicts.tsv](data/sample-verdicts.tsv).
No verdict was re-checked.

27 of 30 are false confirmations, 90 percent; a 95 percent interval on 3 genuine duplicates in 30 puts the genuine share at about 3 to 26 percent.
The three genuine duplicates are `TestStep_LockReleasedBetweenCalls`, `TestResolveSeedRecipe` and `TestBindHandles_PartialMatchOnOneCardMismatchesAndRewritesNeither`.
"False" here means the candidate checks something its covering tests do not, not that the check is worth its cost;
the readers called `TestRun_UnconditionalRecall` the weakest case.

### What the mutator missed

| Kind of check | Tests |
|---|---|
| Absence of a side effect, or a refusal: nothing is written, called or injected | 6 |
| State persisted or recorded, read back from a file, history or record | 6 |
| An input shape or combination the covering tests never feed: a stale file, a partial set, two filters, several items in one call, terminal mode | 7 |
| State kept across two calls | 2 |
| The value of a field production code only passes through, a produced type, or a closure no covering test calls | 4 |
| Exact error text | 1 |
| Another package's real result, from a hub fixture | 1 |

The mutator negates a condition, drops a statement or changes a constant, and a mutant only counts as killed when a test fails on it.
The checks above leave no condition or constant the covering tests depend on, so a candidate can cover the same blocks and still be the only test of them.

## Read

- **What the cut could save.**
  430 confirmed tests, 2.89 s of removable standalone time.
  Against the earlier survey's estimate of about 1 percent off a full run of tiers 0 and 1, the confirmation changes the count, not the order of magnitude.
- **How much `redundancy-sweep-existing` must read.**
  With 90 percent false in the sample, a cut by the list alone deletes tests that check something.
  The sweep must read every one of the 430 against its covering tests; the sample does not bound which ones are safe.
  The kinds above name what to look for first: absent side effects, persisted state and inputs the covering tests do not feed.
- **Where the verb is slow.**
  The mutation seconds per package above are the cost; `internal/boardengine` 1,123 s, `internal/shedengine` 895 s, `internal/shuttleengine` 684 s,
  `internal/orchengine` 578 s, `internal/battenshed` 557 s, `internal/loomcli` 534 s and `internal/hubreconcile` 391 s.
  `internal/loomcli` took 534 s here against 8.6 min in the landing's measurement, so it did not get faster.
  `internal/seatengine` is the worst return: 1,372 s over 3,169 runs, 0.43 s a run, for 1 confirmed test.
  The envelope records no per-mutant time, so a mutant that held a test to its bound shows only in a package's seconds per run:
  `internal/seatengine` 0.43, `internal/loomcli` 0.25 and `internal/hubreconcile` 0.24 are the highest.
  Wall times past the mutation seconds, as in `internal/orchcli` (1,249 s against 15 s), are not verb cost.

## Limits

- One run, with other runs on the machine; the parallel part's wall times are inflated.
- The package set is the earlier survey's, at commit `894abe59`, not rebuilt from the new maps.
- The sample covers the ten packages with the most confirmed tests, 44 percent of the confirmed, and is not random over the rest.
- The 30 verdicts are one subagent's reading each.
- Mutant test runs left untracked files in the source tree, under `internal/burlercli`, `internal/burlerengine`, `internal/fabriccli`, `internal/shedverbs` and `internal/shuttleengine/codexengine`.
  They were removed before the commit; the verb does not clean them up.
