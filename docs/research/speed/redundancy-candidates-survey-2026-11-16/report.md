# Redundancy candidates of every package: one full coverage-map build, timed, 2026-11-16

Board task `redundancy-candidates-survey` is the first step of `redundancy-sweep-existing`.
It built the coverage maps of every package once, timed, and ran `lym gate redundant` once on each mapped package.
It changed no code and no test.
One machine with 12 CPUs, commit `894abe59`, one run, with the hub's other runs live beside it.

## Answer in brief

- The full build took 7 min 2 s of wall and 25.1 CPU-minutes, with a 1.9 GB peak, and failed no package.
  That is 3 minutes over the smart-build estimate of about 4 minutes at four tests side by side, and a sixth of the 44 minutes of 2026-11-12.
- 118 of 212 packages hold candidates: 1,026 tests, of 5,397 map entries.
- Their standalone seconds sum to 11.75 s: 6.43 s over 1,001 tier-0 candidates and 5.32 s over 25 tier-1 candidates.
  Half of the tier-1 seconds are in `internal/hubreconcile` and `internal/orchcli`.
- The candidates are many and nearly free.
  Cutting every one would take at most about 10 percent off a full run of tiers 0 and 1,
  and the reads below find most of them worth keeping.
  The estimate is about 1 percent off a full run and about 1 percent off a typical darn verify gate.
- Recommendation: do not file the cut as a speed task.

## The build

`lym gate testmap` with no package named, in this task's worktree at its own HEAD, once.
The maps went to this worktree's `.lym/testcover/`; nothing was written to the hub's shared maps.
Raw output: [testmap-envelope.json](data/testmap-envelope.json), [testmap-time.txt](data/testmap-time.txt).

| Measure | Value |
|---|---|
| Wall time | 7 min 2.4 s (09:54:26Z to 10:01:29Z) |
| User and system CPU | 1,247.7 s and 261.2 s, 25.1 CPU-minutes, 357 percent of one CPU |
| Peak resident memory | 1,930,404 kB |
| Packages built | 212 |
| Packages with a time record | 197 |
| Packages failed, timed out or skipped | none |
| Audit and self-check mismatches | 0 and 0 |
| Plan before the build | 212 of 212 packages missed with `no coverage map` |
| Build seconds, summed over packages | 286.2 |
| Run seconds, summed over packages | 206.9 |

The 15 built packages without a time record hold no test.

### Scope

| Measure | Value |
|---|---|
| Map entries | 5,397 |
| Tier-0 entries and their standalone seconds | 4,475, 31.0 s |
| Tier-1 entries and their standalone seconds | 922, 77.5 s |
| Entries marked as spawning by the spawn scan | none |
| Entries with a failed outcome | 10 |
| `//testtiming:keep` markers in test files | 763 across 387 files |
| Tests the redundancy report keeps, by those markers or by a reason it names | 740 |

The envelope's own test count, summed over packages, is 4,714; it does not say why it is 683 below the entry count.
The 10 failed entries are `internal/proc` (7), `internal/reedengine` (2) and `internal/githubclient` (1),
and their names are Windows and tmux-floor cases, so they most likely fail on this platform by design.
The build did not name them as failed packages, and the envelope does not record why.
`lym gate redundant` does not judge them.

Tests it did not judge, over all packages: 520 with an unclassifiable call, 167 covering no block,
89 out of process, and the 10 failed.

### Time per package

Build and run seconds are the envelope's per-package figures.
The ten slowest by their sum:

| Package | Build s | Run s | Tests |
|---|---|---|---|
| `internal/fabricengine` | 11.3 | 19.1 | 275 |
| `internal/loomcli` | 13.9 | 13.5 | 267 |
| `internal/fabriccli` | 2.8 | 17.2 | 34 |
| `cmd/lym` | 5.5 | 10.5 | 86 |
| `internal/battencli` | 3.4 | 8.0 | 143 |
| `internal/shedrecipe` | 4.1 | 6.4 | 99 |
| `internal/websterengine` | 4.4 | 4.5 | 175 |
| `internal/shedcli` | 4.6 | 4.1 | 17 |
| `internal/burlerengine` | 5.1 | 3.3 | 50 |
| `internal/hubreconcile` | 2.4 | 5.2 | 24 |

All 197 are in the envelope's `packages` array.

### Against the estimates

| | Wall | CPU |
|---|---|---|
| This build | 7.0 min | 25.1 min |
| Smart-build estimate | about 4 min at four tests side by side | about 16 min |
| Build of 2026-11-12 | 44 min | not recorded |

The build kept to the estimate's order of magnitude and missed it by 75 percent of the wall and 57 percent of the CPU.
The envelope's per-package build and run seconds sum to 8.2 minutes, which exceeds the 7.0-minute wall,
so packages overlap and the wall is not a simple sum.
The sum covers 493 s of the 1,509 CPU-seconds.
The rest is work the envelope does not time: process start-up per test, profile parse, and `go tool covdata`.
The estimate's own parts were 365 s of start-up, 304 s of parse and 210 s of builds, 965 s in all,
so this build spent about 540 CPU-seconds more than the estimate on those parts.
Which part ran over was not measured.
The hub's other runs shared the machine, and their load is not subtracted.

### Tests rerun alone after failing beside others

The envelope does not report them, so this run cannot say how many there were.
The 10 failed entries and `audit_mismatches: 0` are all the record shows.
No sign of tests sharing a fixed path appeared.

## What the maps offer

Raw judgments: one JSON file per package in [data/redundant/](data/redundant), and two tables made from them:
[candidates.tsv](data/candidates.tsv) lists every candidate, slowest package first, with the columns
package, test, tier, seconds, removable, covering tests;
[candidates-by-package.tsv](data/candidates-by-package.tsv) lists the 118 packages with their totals.
Seconds are standalone times to 10 ms, so a test shown as 0 ran in under 5 ms.

### Totals

| Tier | Candidates | Seconds | Removable | Removable seconds |
|---|---|---|---|---|
| 0 | 1,001 | 6.43 | 906 | 6.23 |
| 1 | 25 | 5.32 | 21 | 4.73 |
| Both | 1,026 | 11.75 | 927 | 10.96 |

### The slowest packages by candidate seconds

| Package | Tests | Candidates | Seconds | Tier 0 s | Tier 1 s | Removable s |
|---|---|---|---|---|---|---|
| `internal/hubreconcile` | 25 | 10 | 2.62 | 0.00 | 2.62 | 2.04 |
| `internal/loomrecipe` | 80 | 51 | 1.71 | 1.71 | 0.00 | 1.70 |
| `internal/orchcli` | 53 | 9 | 1.08 | 0.04 | 1.04 | 1.08 |
| `internal/gitkit` | 11 | 4 | 0.95 | 0.94 | 0.01 | 0.95 |
| `internal/loomcli` | 307 | 46 | 0.76 | 0.48 | 0.28 | 0.75 |
| `internal/hubissues` | 9 | 2 | 0.66 | 0.66 | 0.00 | 0.66 |
| `internal/fabriccli` | 37 | 5 | 0.58 | 0.00 | 0.58 | 0.58 |
| `internal/orchengine` | 127 | 73 | 0.52 | 0.52 | 0.00 | 0.49 |
| `internal/preflight` | 7 | 1 | 0.38 | 0.00 | 0.38 | 0.38 |
| `internal/preflightshed` | 9 | 2 | 0.36 | 0.00 | 0.36 | 0.36 |
| `internal/shuttleengine/claudeengine` | 145 | 47 | 0.31 | 0.31 | 0.00 | 0.31 |
| `internal/treadleengine` | 30 | 10 | 0.23 | 0.23 | 0.00 | 0.20 |
| `cmd/lym` | 97 | 5 | 0.21 | 0.21 | 0.00 | 0.21 |

The other 105 packages hold 0.12 s or less each.
Only 17 candidates take 0.2 s or more, 7.24 s together, in nine packages:
`internal/hubreconcile` (7), `internal/orchcli` (2), `internal/loomcli` (2),
and one each in `internal/gitkit`, `internal/hubissues`, `internal/fabriccli`, `internal/preflight`,
`internal/preflightshed` and `cmd/lym`.
`internal/fabricengine` holds 12 candidates of 0.03 s together; it was listed, not read.
`internal/shedengine` holds the most candidates, 111, at 0.05 s together.

### Packages with no candidates

94 packages, 84 of them with tests and 10 with none.
The table, with each package's test count, is [packages-without-candidates.tsv](data/packages-without-candidates.tsv).
It includes `internal/creelengine`, `internal/darnengine`, `internal/friction`, `internal/gitexec`,
`internal/verifyrun` and `internal/verifytree`.

### A read of the largest candidates

The five packages with the most candidate seconds, and one test from each of them read in the source.
`internal/fabricengine` is left out as asked.
Equal coverage is not equal checks, so each candidate was asked what it asserts that its covering tests do not.
This read is of names, covering lists and about a dozen test bodies; it is not a read of every candidate.

| Package | Read | Way it would go |
|---|---|---|
| `internal/hubreconcile` | The tier-1 candidates, 2.6 s, each build a stale hub and call `Ensure`: `TestEnsure_PairDirectoryDeletedIsSkippedAsRemoved` asserts a deleted pair leaves the stamp present, which the covering test for concurrent calls never reads. Their blocks coincide because they share one code path and differ in the hub's state. | Keep. Folding a hub setup into a row of a table saves the setup time but is a rewrite per test. |
| `internal/loomrecipe` | The 51 candidates are 49 removable, each 0.1 s or less. `TestZeroBudget_BouncerSettlesAtOnce` is a table of four routings asserting a Bouncer's route. The routing tests listed as covering it assert other budgets. | Keep, or fold as rows into the routing table test. |
| `internal/orchcli` | `TestStart_NeverAttaches` asserts a start with `TMUX` set exits 0 and reports a relaunch, a check the covering `TestStart_EnsuresTheRunsStrandBeforeTheOrch` does not make. The tier-1 `TestUnderAdoptWatch_CoversThePrimeWhileARootAdoptIsUnfinished` asserts the prime's cover where its covering test asserts a beam's. | Keep. A fold into the covering test as an extra assertion is possible for `TestStart_NeverAttaches`. |
| `internal/gitkit` | `TestCopyRepoCallerSet_LymcwdOnly` is a source scan, 0.94 s, that asserts which packages call `CopyRepo`. `TestCopiedRepoScenario` covers its blocks because the scan helpers it uses run there too, and asserts nothing about callers. | Keep. A guard test's blocks are not its check. |
| `internal/loomcli` | `TestDriverChoiceSingleSiteInvariant_OnlyLoomcliReadsTheSeedDriverField` is a scan, 0.38 s, with the same reading as above. The rest are 0.01 s tests whose covering tests assert other results. | Keep the scans. The rest could fold, and save under 0.1 s. |

The read finds the same pattern five times.
A candidate is a test whose blocks others cover, and in these packages each one pins a different result through the same code.
The scans assert an invariant over the source tree, and any test that runs the helpers covers their blocks.
The candidates that look cuttable are the 0.00 to 0.01 s tests, where cutting saves nothing worth the review.

## The payoff

Suite time comes from the hub's gate run records of 2026-11-15 and 2026-11-16, on a machine shared with other runs.
A whole-suite run is `scope_form: all`; a typical darn verify gate is the `computed` scope of the `darn-write` key.

| Tier | Whole-suite run, median and minimum | Typical darn verify gate, median | Candidate seconds | Share of the whole-suite median |
|---|---|---|---|---|
| 0 | 43.7 s, 25.9 s | 26.2 s | 6.43 | 14.7 percent |
| 1 | 76.4 s, 62.6 s | 73.2 s | 5.32 | 7.0 percent |
| 2 | 49.2 s, 46.5 s | 50.3 s | none | none |

Candidate seconds are standalone seconds summed over tests, not wall time.
Tier 0 runs its tests in parallel, so its saving in wall time is below its candidate seconds,
and it is bounded by them, since removing a test cannot add wall.
Tier 1 runs one test at a time inside its package, and packages run side by side, so its wall saving is also below its candidate seconds.

The share the read found cuttable is low.
Of the five packages, which hold 7.1 s of the 11.75 s, the read finds nothing to cut and only a few 0.01 s tests to fold.
Applied to the whole list as an estimate: 10 percent cuttable, with a spread from 0 to 30 percent,
since a package outside the five could hold tests that really repeat another.

| | Candidate seconds cut | Off a whole run of tiers 0 and 1 (120 s) | Off a typical darn verify gate, tiers 0 and 1 (99 s) |
|---|---|---|---|
| Low, 0 percent | 0 s | 0 s, 0 percent | 0 s, 0 percent |
| Estimate, 10 percent | 1.2 s | at most 1.2 s, 1 percent | at most 1.2 s, 1 percent |
| High, 30 percent | 3.5 s | at most 3.5 s, 3 percent | at most 3.5 s, 4 percent |
| Everything cut | 11.75 s | at most 11.75 s, 10 percent | at most 11.75 s, 12 percent |

A darn verify gate runs only the packages a diff touches, so it can save only the candidates of those packages;
its column is a ceiling, not an estimate.
Either way the saving is small beside the spread of the gate's own wall time:
a tier-0 whole-suite run ranged from 25.9 s to 92.1 s in the records read.

## Recommendation

The cut is not worth tasks for the time it saves.
It is worth one small task for the reasons that are not time: a test that really repeats another is a maintenance cost.

- Do not file a cut task per package.
  The candidates are 19 percent of the tests and 11 percent of the standalone seconds,
  and the candidates read each assert something their covers do not.
- If the operator wants the suite smaller anyway, take the 17 candidates of 0.2 s or more first, 7.24 s in nine packages,
  with `internal/hubreconcile` first.
  Most of those seconds are tier-1 tests.
- Shape: one darn for that group of nine packages, which rebuilds each map at its own HEAD with `lym gate testmap <package>` first.
  A darn per package would spend more on review than it removes.
- Leave `internal/loomrecipe`, `internal/orchengine` and the other many-candidate packages: they hold the count, not the seconds,
  and a count of 0.00 to 0.01 s tests costs nothing to run.
- At 7 minutes the full build is cheap enough to run on command before a sweep.

## Limits

- One build and one judgment per package, on a shared machine; the wall and CPU carry the load of the hub's other runs.
- Seconds are rounded to 10 ms, so a 0.00 s test may take up to 5 ms, and the tier-0 sum carries that rounding over 1,001 candidates.
- The read covers five packages and about a dozen test bodies, and the 10 percent estimate rests on it.
- The envelope does not report side-by-side reruns or the reason a failed entry failed.
- `lym gate redundant internal/adoptcli` was run once more by hand before the loop, to see its output form; it reads the map and runs no test.
