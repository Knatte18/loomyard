# What makes runs slow, 2026-11-06

Where the wall time of a landed run goes, why darn runs got slower after 2026-11-01, and what would make runs faster.
Read from recorded data only: the reports, the archive tags, the gate run records and the session transcripts of 40 landed runs.
No agent ran and no lym code changed.
`analyze.sh` prints every table below; the scripts and tables sit beside this file.

## Answer

### The causes, ranked

**Darn.**
A darn code run took 31.8 minutes of active time before 2026-11-01 15:04 UTC and 43 to 48 minutes in the three eras after it (table "Darn code runs by era").
The growth sits in three places, and the model is none of them.

| Rank | Cause | Minutes per run, before | after 1 | after 2 | now | How sure |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Gates: tier wall time plus slot wait, in the rows' own gates and the agents' gate calls | 13.6 | 20.6 | 23.6 | 18.9 | Sure: every tier run is recorded |
| 1a | of which one test binary per package (`6c5727df`): a gate pass that runs alone takes 47 to 75 seconds more | - | about 3 to 5 | about 3 to 5 | about 3 to 5 | The step is sure; that the commit caused it is likely, not proven |
| 1b | of which other gates beside it: a tier takes 1.5 times as long with two or more tier runs beside it; the cells give the share of tier runs that had them | 18 % | 59 % | 45 % | 17 % | Sure |
| 1c | of which more passes of the Darn-Write gate since the cold fixer: 1.6 passes per run, then 2.5 and 2.2 | - | - | about 3 | about 2 | Sure of the count, not of what a pass could reuse |
| 2 | Landings queue: Finalize holds the parent lock through its whole gate | 2.3 | 3.9 | 2.1 | 0.3 | Sure of the time before the gate starts; that it is the lock is read from the code |
| 3 | Hand-over between rows, setup included: the origin check before every status push (`f5724733`), behind the remote-refs lock until `48f17897` | 0.2 | 1.1 | 4.2 | 0.9 | Sure |
| - | Model time, the second largest part of a run, did not grow; the middle figure is the mid sample, which spans after 1 and after 2 | 13.7 | 14.4 | 14.4 | 12.5 | Sure for the 32 runs read |

The between era, three hours on 2026-11-01 with 14 runs, shows cause 2 at its worst: 16 minutes of a run's Finalize passed before its gate started.

Now, since 2026-11-05 08:03 UTC, the median run takes 30.1 minutes against 32.0 before, and the mean 43.3.
Two of the 17 runs waited inside the writer's session, 126 and 31 minutes, on their parent and on another run; without those two waits the mean is 34.1.
The machine was as quiet as before in this era, so causes 1b, 2 and 3 are at rest, not gone: they return with the next busy day.
Cause 1a and 1c remain, and the writer's own part shrank by about as much, which is the saving of project `turn-savings`.

**Tapestry.**
Four runs before and four after, paired by size, so every figure is a pointer (table "Where a picked run's wall time went").

| Rank | Part | Minutes per run, before | after | Note |
| --- | --- | --- | --- | --- |
| 1 | Model time | 152.4 | 174.2 | 73 to 77 % of the run; it follows output tokens, and the after runs are larger |
| 2 | Gate calls by agents, nearly all by Webster forks | 12.5 | 28.0 | 19 seconds a call before, 62 after |
| 3 | Gates the rows run, and Parent-Align's gate | 11.8 | 16.8 | Parent-Align's gate shows as session start, see "Tapestry" |
| 4 | Rows with no session | 1.0 | 6.9 | Finalize's gate, the hand-over and a wait in the Discussion rows |
| 5 | Other tools | 12.7 | 9.6 | |

**What is not a cause.**

- **Repository size.**
  Go source grew 21.5 % from 2026-10-27 to 2026-11-06.
  The writer reads no more per run or per changed line, a Read, Edit or shell lookup costs 8 to 26 milliseconds more, a session's first request holds 1500 tokens more, and the whole-repository gate shows a step on one day and no trend with size.
- **Model speed.**
  A request takes about 1 second plus 8.2 seconds per 1000 output tokens on Opus and 5.6 on Sonnet, the same before and after, at 1.8 and at 3.4 runs in flight.
- **Context size.**
  100 000 more context tokens add 0.1 to 0.6 seconds to a request.
- **Prompt cache misses.**
  36 in 13 600 requests, about 13 seconds per run.
- **Turns.**
  A request costs about 1 second beside its output, so the 50 requests `turn-savings` took out of a darn writer's session save about a minute.

### What to do

Every saving is an estimate from the tables, in minutes per darn run unless it says tapestry, at today's quiet load unless it says busy.

| Rank | Change | Saving | Kind | Cost to build | Risk to quality |
| --- | --- | --- | --- | --- | --- |
| 1 | Run the tiers of one gate call side by side instead of one after the other | 3 to 5; tapestry 5 to 8 | Code, `internal/gatecli` | Medium: one call holds one slot per tier, or one slot at a higher `-p` | None; more load per call, so less under a busy machine |
| 2 | Drop tier 2 from the `darn-write` and `code-burler-fix` keys and leave it to `finalize` | 3 to 4 | Config, `gate.yaml` `gates` | None | A tmux-test failure shows first at Finalize, under the parent lock |
| 3 | Start only the test binaries that hold a test of the tier | 2 to 4 | Code: a per-tier package list in the gate | Medium | None if the list is derived, not kept by hand |
| 4 | Finalize gates outside the parent lock and takes it only to check the parent has not moved and to merge | 0 quiet; 2 to 4 busy; 16 in a burst | Code, `internal/landingshed` | Medium to large | A landing that lost the race gates twice |
| 5 | One origin check per run every few minutes instead of one per status push | 0.5 to 1; tapestry 1.5 to 2; 4 behind a slow push | Code, `internal/loomcli` | Small | A stale base is seen up to that interval later |
| 6 | Let the row's gate reuse the pass the cold fixer just made | up to 2 | Code; first settle what is rerun, see below | Small to medium | None |
| 7 | One Plan review round, as profile `light` has, where `solid`, the tapestry default, pays for two | tapestry 15 to 20 | Config, `profiles.yaml` or the task's profile | None | Plan round 2 held a gating finding in 62 % of its rounds |
| 8 | Plan and Discussion fixers on Sonnet | tapestry 5 to 8 | Config, `loom.yaml` and `tapestry.yaml` fixer models | None | Measured for Plan in [fixers-on-sonnet](../../burler/fixers-on-sonnet-2026-11-03/report.md); Discussion leaned worse |
| 9 | Cap the runs in flight near the gate slots: `max_runs` is 8 in the prime's local layer, 3 by default | 0 quiet; 3 to 5 busy | Config, `beam.yaml` | None | None; a waiting task starts later, so the hub lands no more per day |

**Quick wins:** 2, 7, 8 and 9 are config alone.
2 is the largest for darn and can be tried on one day and read back with `analyze.sh`.

**Not worth building for speed:** shorter start context, fewer turns, prompt cache tuning, and anything aimed at search or read cost.
The darn code review costs 13.8 minutes of a run now, 40 % of it, and its first round holds a gating finding in 48 % of runs; the `speed` profile already removes it, and that trade is the operator's.

### What the data cannot settle

| Question | Cheapest measurement |
| --- | --- |
| Whether one binary per package pays on the hub's other machine, where it was meant to: 8 runs there after the change show gates three times slower than here, with that machine's own sleep unknown | `lym gate report` and `gate-runs.sh` on that machine, which hold its tier runs |
| What a Webster fork's gate call spends its 62 seconds on: the recorded tiers of a `card` call add up to about 25 | Read the call's own log lines for slot wait, lints and the cold fixer's wait in one tapestry run |
| Whether the Darn-Write gate after a cold fixer reruns tiers the fixer just passed | Compare the `carried` count of `darn-write` records after a fixer, in `lym gate report` |
| How often a writer waits inside its session on its parent: the timing report records no wait for it | Count `--expect-reply` posts and the notices that answer them in every landed run's writer transcript |
| Whether a faster model or output mode per role keeps quality | A paired run of one task; an agent run, outside this report |
| Which profile the runs before 2026-11-05 used | Nothing recorded; none |

## Sample and method

- **Run-level tables** cover every landed run in the records, 279 since 2026-10-27.
  `runs.tsv` is the earlier study's extraction run again, `rows.tsv` every recipe row of every run from the shed status at its archive tag, `gate-runs.tsv` every gate tier run this machine recorded, `reviews.tsv` every review round's facts.
- **Eras**, by a run's start in UTC: before (before `f08df88b`, 2026-11-01 15:04), between (to `6c5727df`, 18:17 the same day), after 1 (to `f5724733`, 2026-11-03 16:02), after 2 (to `48f17897`, 2026-11-05 08:03) and now.
  The first cut is the earlier study's; the other three are the commits the data shows a step at.
  The era tables take darn code runs with no machine sleep, as the earlier study did: 51, 14, 59, 44 and 17 runs.
- **Transcript tables** read 40 landed runs, the cap (`pick.tsv`, by `pick.sh`).
  - 32 darn code runs with no machine sleep and no recorded wait, in three periods: before, mid (2026-11-02 12:51 to 2026-11-04 15:15, the bulk of the after runs) and late (the now era).
    Four size bands by changed lines, and per band the runs nearest the band's middle: 4 before, 2 mid and 2 late, so the periods hold the same sizes (median 420, 407 and 416 lines).
  - 8 tapestry runs named in `pick.sh`, four before and four after, paired by size: 2695 with 2775 lines, 5068 with 5261, 7365 with 8484, 9150 with 11648.
- **The time classes.**
  `timeline.awk` cuts each row into tenths of a second and gives each to one class, so the classes of a row add up to the row; 645 rows were checked against their stamps and none differs by more than 1.5 seconds.
  A tick is model time when any request of the row is in flight, a tool when a tool call is and no request, session start from the wait for a session's first entry to its task prompt, and idle or Go work otherwise.
  A request runs from the client's request record to the message's last block.
- **Limits.**
  A run of the hub's other machine has no transcript, gate record or journal here.
  `lym report timing` reads this machine's journal, so it counts such a run's whole span as sleep: the 17 code runs of 2026-10-29 and 2026-10-30 have no active time for that reason.
  Parallel sessions overlap, so request seconds by role add up to more than model time.

### The earlier study's figures

[task-runtime-before-after](../task-runtime-before-after-2026-11-06/report.md) was checked by running its `extract.sh` again: its 277 rows come back identical, with two runs landed since.
Its darn means stand: 31.8 minutes before and 45.1 after over the runs with no sleep, from the era table here.
Two readings change:

- It put the gate's growth at about 6 minutes, from wall time alone over two thirds of the runs.
  With slot wait and every tier run the gate grew by 7.0 to 10.0 minutes, and with the Finalize queue and the hand-over the growth is accounted for.
- It found no saving from `turn-savings`.
  The writer's part of Darn-Write fell from 12.3 to 9.7 and 8.6 minutes; the rows around it grew by more.

## Darn

### Every landed run, by era

Minutes per run; a row's gate spans are the gates its Go code runs after the agent ends its turn.

| Measure | before | between | after 1 | after 2 | now |
| --- | --- | --- | --- | --- | --- |
| Runs | 51 | 14 | 59 | 44 | 17 |
| Active time, mean | 31.8 | 48.5 | 43.4 | 48.2 | 43.3 |
| Active time, median | 32.0 | 46.1 | 38.6 | 46.9 | 30.1 |
| Changed lines, median | 380 | 338 | 297 | 500 | 441 |
| Setup, before the first row | 0.1 | 0.1 | 0.5 | 1.4 | 0.3 |
| Darn-Write, less its gate spans | 12.3 | 10.4 | 9.7 | 8.6 | 15.3 |
| Darn-Write, gate spans | 4.1 | 4.6 | 7.6 | 11.4 | 8.6 |
| Code-Bouncer | 0.9 | 1.0 | 0.9 | 1.3 | 0.9 |
| Code-Burler-Review | 2.4 | 2.8 | 2.9 | 3.4 | 2.9 |
| Code-Burler-Fix, less its gate spans | 3.8 | 4.4 | 6.4 | 6.1 | 5.6 |
| Code-Burler-Fix, gate spans | 2.6 | 3.2 | 4.8 | 4.4 | 4.4 |
| Describe | 0.5 | 0.5 | 0.5 | 0.7 | 0.5 |
| Finalize | 4.7 | 20.4 | 8.7 | 7.5 | 3.7 |
| Every other row | 0.1 | 0.1 | 0.6 | 2.8 | 0.6 |
| Land, after the last row | 0.5 | 0.4 | 0.6 | 0.7 | 0.2 |
| Rows per run | 13.9 | 14.4 | 14.6 | 14.7 | 15.3 |

Darn-Write less its gate holds 9.2 minutes of in-session waiting in the now era, from two runs; without it the figure is 6.1.

### Gates

Tier runs, wall minutes and slot wait minutes per run, from the gate run records.

| Gate key | before | between | after 1 | after 2 | now |
| --- | --- | --- | --- | --- | --- |
| Runs with a gate record | 48 | 14 | 58 | 40 | 16 |
| darn-write | 4.8; 4.0; 0.2 | 3.9; 4.9; 0.5 | 4.8; 6.8; 0.8 | 7.4; 10.7; 1.0 | 6.6; 8.0; 0.5 |
| code-burler-fix | 3.7; 2.8; 0.1 | 3.9; 3.1; 0.1 | 3.8; 4.5; 0.3 | 3.7; 4.6; 0.2 | 3.8; 4.6; 0.0 |
| finalize | 3.1; 2.3; 0.1 | 3.4; 3.7; 0.1 | 3.3; 4.4; 0.1 | 3.8; 4.9; 0.2 | 3.0; 3.5; 0.0 |
| run by an agent, or with no key | 7.6; 3.3; 0.9 | 5.6; 1.9; 2.0 | 5.1; 1.4; 2.2 | 3.6; 1.1; 1.0 | 2.5; 1.3; 1.0 |
| All keys | 19.1; 12.4; 1.3 | 16.8; 13.7; 2.7 | 17.0; 17.1; 3.5 | 18.4; 21.2; 2.4 | 15.8; 17.4; 1.5 |
| Wall plus wait | 13.6 | 16.3 | 20.6 | 23.6 | 18.9 |

The gate is 43 % of a run before and 44 to 49 % after.
A gate call runs tiers 0, 1 and 2 one after the other in one slot (`internal/gatecli/run.go`), `darn-write` and `code-burler-fix` over the packages the diff reaches and `finalize` over the whole repository.

**The step on 2026-11-01.**
Mean wall seconds of a tier run, by what ran beside it, before and after `6c5727df`, which builds one test binary per package and picks the tier at run time; n in brackets.

| Gate key and tier | before, alone | before, two or more beside | after, alone | after, two or more beside |
| --- | --- | --- | --- | --- |
| darn-write tier 0 | 10.4 (32) | 18.7 (28) | 27.6 (46) | 53.1 (120) |
| darn-write tier 1 | 90.3 (23) | 170.7 (31) | 109.0 (40) | 163.0 (132) |
| darn-write tier 2 | 49.8 (25) | 70.4 (30) | 60.0 (40) | 80.0 (123) |
| darn-write, the three tiers | 150 | 260 | 197 | 296 |
| code-burler-fix, the three tiers | 109 | 183 | 168 | 271 |
| finalize tier 0 | 18.4 (42) | 36.5 (17) | 40.0 (57) | 64.7 (50) |
| finalize tier 1 | 57.8 (40) | 118.0 (15) | 85.9 (48) | 142.4 (53) |
| finalize tier 2 | 35.0 (44) | 51.5 (15) | 60.4 (51) | 80.9 (53) |
| finalize, the three tiers | 111 | 206 | 186 | 288 |

Two effects stand apart in it.
Alone, a pass takes 47, 59 and 75 seconds more after the commit.
Beside two or more other tier runs a pass takes 1.5 to 1.9 times as long, before and after: three slots at `-p=4` share 12 threads.

The whole-repository gate when it ran alone, by day, beside the size of the tree; 2026-11-01 is split at the commit.

| Day | Tier 0 s (n) | Tier 1 s (n) | Tier 2 s (n) | Three tiers s | Binaries run, tier 0 | Go lines | Test functions |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 2026-10-28 | 18.8 (13) | 63.3 (13) | 33.1 (14) | 115 | 62 | 712151 | 4961 |
| 2026-10-31 | 17.3 (7) | 48.2 (9) | 33.2 (9) | 99 | 57 | 725458 | 5013 |
| 2026-11-01 a | 19.5 (19) | 66.8 (14) | 42.1 (17) | 128 | 66 | 761714 | 5164 |
| 2026-11-01 b | 41.7 (5) | 92.2 (5) | 82.0 (5) | 216 | 161 | 761714 | 5164 |
| 2026-11-02 | 37.0 (6) | 73.4 (6) | 59.6 (5) | 170 | 155 | 772163 | 5216 |
| 2026-11-03 | 41.4 (17) | 77.6 (12) | 56.3 (14) | 175 | 156 | 800129 | 5404 |
| 2026-11-04 | 50.4 (9) | 107.4 (8) | 64.7 (9) | 223 | 166 | 807326 | 5442 |
| 2026-11-05 | 33.1 (18) | 80.4 (15) | 54.7 (16) | 168 | 152 | 845703 | 5601 |

The tree grew 7 % from 2026-10-28 to 2026-11-01 with the gate level, and 11 % from 2026-11-01 to 2026-11-05 with the gate level again; the one change is the step.
Since the commit every tier starts nearly every package's binary: 62 before and about 155 after in tier 0, and 3 to 21 before and about 150 after in tier 2, most of them holding no test of that tier.
[gate-fewer-binaries-trial](../gate-fewer-binaries-trial-2026-11-01/report.md) proposed the change for the other machine's scanner, at about 10 seconds a new binary, and measured no wall time here.

**Load.**
Share of tier runs by what ran beside them.

| Era | Tier runs | alone | one other | two or more |
| --- | --- | --- | --- | --- |
| before | 2381 | 59% | 24% | 18% |
| between | 311 | 23% | 21% | 55% |
| after 1 | 1364 | 18% | 23% | 59% |
| after 2 | 878 | 20% | 36% | 45% |
| now | 999 | 57% | 26% | 17% |

**The Darn-Write gate and the cold fixer.**
Before `5abbc19f` (2026-11-03) the writer ran the gate itself, 3.9 calls and 168 seconds a run in the before sample, and the row's verify gate refused its work in 40 % of runs.
Since then a cold fixer takes a failed verify: 0.8 fixer sessions a run in the late sample, 30 seconds of requests, and 1.2 gate calls of 183 seconds each, after which the row gates again.
The row's `darn-write` tier runs went from 4.8 to 7.4 and 6.6 a run.

### Finalize and the landing queue

Seconds of the Finalize row, split at its gate's first and last tier run.

| Era | Runs | Row | Before its gate starts | the same, median | Runs over a minute before the gate | Slot wait | Gate tiers | After the gate |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| before | 47 | 286 | 137 | 3 | 10 | 8 | 136 | 4 |
| between | 14 | 1224 | 957 | 961 | 12 | 9 | 222 | 7 |
| after 1 | 57 | 537 | 232 | 16 | 20 | 5 | 271 | 7 |
| after 2 | 40 | 484 | 124 | 30 | 14 | 14 | 292 | 15 |
| now | 16 | 235 | 16 | 4 | 1 | 0 | 212 | 6 |

Finalize takes the parent lock, merges the parent, runs the whole-repository gate and pushes, all under the lock (`internal/landingshed/finalize.go`).
A run that reaches Finalize while another holds the lock waits out that run's whole gate, so a third of the runs carry the mean.

### Hand-over between rows

A skipped row does nothing but hand over, so its length is what one hand-over costs.

| Era | Skipped rows | Mean s | Median s |
| --- | --- | --- | --- |
| before | 151 | 0.2 | 0.0 |
| after 1 | 180 | 2.7 | 0.0 |
| after 2 | 168 | 19.6 | 13.0 |
| now | 72 | 3.8 | 2.0 |

Each row is its own `lym` process that ends with a status commit and a detached push.
Since `f5724733` the commit first reconciles origin from the prime, a network read under the weft repository's remote-refs lock.
Until `48f17897` every push held that lock for its whole transfer, so each hand-over queued behind other runs' pushes: 19.6 seconds a row, 61.6 in the worst hour.
What is left, 2 to 4 seconds a row, is the origin read itself, 15 times a darn run and 33 or more times a tapestry run.
The wait for a session's first entry grew with it: 0.2 minutes a run before, 3.3 in the mid sample, 0.5 late.

### Inside the rows: 32 runs read

Mean (median) minutes per run.

| Class | darn before | darn mid | darn late |
| --- | --- | --- | --- |
| Runs | 16 | 8 | 8 |
| model | 13.7 (13.6) | 14.4 (11.5) | 12.5 (11.8) |
| gate run by the row | 6.9 (6.7) | 10.0 (10.6) | 8.7 (6.1) |
| gate run by an agent | 3.5 (3.6) | 3.6 (1.1) | 7.0 (6.0) |
| other tools | 0.8 (0.5) | 0.5 (0.3) | 1.1 (0.1) |
| session start | 0.9 (0.9) | 4.1 (1.9) | 1.1 (1.1) |
| idle around turns | 0.3 (0.2) | 2.5 (0.9) | 1.0 (0.6) |
| rows with no session | 3.3 (3.3) | 7.3 (5.4) | 3.9 (4.5) |
| waiting for a notice | 0.0 (0.0) | 0.0 (0.0) | 19.5 (0.0) |
| All rows | 29.4 (33.1) | 42.5 (38.8) | 55.0 (35.0) |
| All rows less waiting for a notice | 29.4 (33.1) | 42.5 (38.8) | 35.4 (26.1) |

Rows with no session are Finalize, whose gate holds no gate span, and the hand-over rows.
A typical darn run now, the late column less the waits: 35 minutes, of which 12.5 the model, 19 gates (8.7 by the rows, 7.0 by agents, 3.3 in Finalize), 1.1 session starts, 1.0 idle, 1.1 other tools and 0.6 hand-over.
The same by part of the recipe, late sample, mean minutes:

| Part | model | gate run by the row | gate run by an agent | other tools | session start | idle around turns | rows with no session | waiting for a notice | all |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Darn-Write | 5.2 | 3.6 | 3.9 | 1.0 | 0.2 | 0.8 | 0.0 | 19.5 | 34.2 |
| Code review rows | 7.1 | 5.1 | 3.1 | 0.1 | 0.8 | 0.1 | 0.0 | 0.0 | 16.3 |
| Describe | 0.3 | 0.0 | 0.0 | 0.0 | 0.1 | 0.0 | 0.0 | 0.0 | 0.4 |
| Finalize | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 3.3 | 0.0 | 3.3 |
| Every other row | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.6 | 0.0 | 0.6 |

The two waits are `codex-greet-finish`, whose writer followed another run for 126 minutes with monitor notices and creel posts to its parent, and `branch-name-recorded`, 31 minutes on creel mail after an interrupt.
Neither is a wait in `lym report timing`.

**The writer.**
Its own session, median per run.

| Period | Requests | Request seconds | Output tokens, thousands | Peak context, thousands | Read and search calls | Characters read, thousands | Edit calls | Characters read per changed line | Output tokens per changed line |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| before | 78 | 421 | 43.3 | 180 | 42 | 203 | 10 | 578 | 106 |
| mid | 24 | 299 | 36.4 | 155 | 22 | 166 | 25 | 419 | 111 |
| late | 33 | 232 | 25.6 | 151 | 24 | 189 | 17 | 693 | 105 |

Requests fell by more than half and request time by 2 to 3 minutes, which follows the output tokens and not the requests.
Output per changed line is level, and so is what the writer reads.

**The code review.**
Bouncer, Review and Fix with its gate take 9.7 minutes of a run before and 13.8 now; the growth is the Fix row's gate and the fixer's own gate calls, 13 seconds a call before and 56 late.
What a round found, over every landed run:

| Recipe | Review | Round | Rounds | Findings per round | Gating per round | Rounds with no finding | Rounds with no gating finding |
| --- | --- | --- | --- | --- | --- | --- | --- |
| darn | code | 1 | 206 | 4.2 | 0.9 | 10% | 52% |
| darn | code | 2 | 34 | 1.7 | 0.2 | 24% | 82% |
| tapestry | code | 1 | 15 | 4.3 | 0.7 | 7% | 40% |
| tapestry | discussion | 1 | 14 | 13.7 | 6.6 | 0% | 0% |
| tapestry | discussion | 2 | 11 | 10.5 | 5.6 | 0% | 0% |
| tapestry | discussion | 3 | 4 | 4.8 | 2.8 | 0% | 0% |
| tapestry | plan | 1 | 15 | 9.9 | 2.1 | 0% | 7% |
| tapestry | plan | 2 | 13 | 5.1 | 0.9 | 0% | 38% |
| tapestry | plan | 3 | 4 | 4.2 | 1.2 | 0% | 0% |

[review-setup-audit](../../burler/review-setup-audit-2026-10-28/report.md) holds what each round and lens is worth; this table only adds the later runs.

## Tapestry

Mean (median) minutes per run; four runs each, median 6216 and 6872 changed lines.

| Class | tapestry before | tapestry after |
| --- | --- | --- |
| model | 152.4 (138.1) | 174.2 (184.6) |
| gate run by the row | 7.5 (7.2) | 12.0 (11.3) |
| gate run by an agent | 12.5 (12.8) | 28.0 (23.0) |
| other tools | 12.7 (13.3) | 9.6 (7.1) |
| session start | 7.5 (7.5) | 8.7 (8.1) |
| idle around turns | 2.2 (1.1) | 0.8 (0.7) |
| rows with no session | 1.0 (1.0) | 6.9 (4.1) |
| waiting for a notice | 1.6 (0.0) | 0.1 (0.0) |
| All rows | 197.3 (189.7) | 240.2 (260.1) |

By part of the recipe, after, mean minutes:

| Part | model | gate run by the row | gate run by an agent | other tools | session start | idle around turns | rows with no session | all |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Discussion | 38.2 | 1.2 | 0.0 | 0.4 | 1.2 | 0.2 | 2.8 | 44.0 |
| Plan | 63.2 | 1.4 | 0.0 | 6.0 | 1.7 | 0.4 | 0.3 | 73.0 |
| Webster | 60.0 | 6.5 | 26.5 | 2.9 | 0.3 | 0.1 | 0.0 | 96.2 |
| Parent-Align | 2.2 | 0.0 | 0.1 | 0.1 | 4.8 | 0.0 | 0.0 | 7.2 |
| Code review rows | 10.1 | 2.9 | 1.4 | 0.1 | 0.6 | 0.1 | 0.0 | 15.2 |
| Describe | 0.5 | 0.0 | 0.0 | 0.0 | 0.1 | 0.0 | 0.0 | 0.6 |
| Finalize | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 3.0 | 3.0 |

- Parent-Align's session start is its gate: the whole-repository gate runs first, 3.5 to 4.5 minutes with no gate span, and the fixer session opens after it.
  So the real cost of starting sessions in a tapestry run is about 4 minutes for 33 sessions, 7 to 10 seconds each.
- Webster is serial by design: one fork at a time in one tree (`internal/websterengine/doc.go`).
  Its forks made 38 gate calls a run before and 27 after, 729 seconds against 1684.
  [parallel-batches](../../webster/parallel-batches-2026-10-17/report.md) put the gain of parallel batches at 1.0 to 1.1 times, so the gate calls are the larger lever there.
- Plan is the largest part after Webster: the writer and its five lens forks, then two review and fix rounds.

**Where the model time goes.**
Request seconds per run by role, after; forks of one parent run side by side, so the rows add up to more than the 174 minutes.

| Role | Model | Sessions | Requests | Output tokens, thousands | Thinking share | Request seconds |
| --- | --- | --- | --- | --- | --- | --- |
| Webster forks | sonnet[medium] | 6.5 | 254.5 | 486.2 | 41% | 3358 |
| Plan writer's forks | opus[medium] | 6.0 | 59.2 | 126.5 | 55% | 1185 |
| Plan writer | opus[medium] | 1.0 | 48.8 | 85.4 | 23% | 708 |
| Plan reviewer on Opus | opus[medium] | 1.5 | 52.0 | 81.2 | 44% | 703 |
| Plan fixer | opus[medium] | 2.8 | 74.5 | 80.3 | 24% | 662 |
| Discussion fixer | opus[medium] | 1.8 | 44.2 | 78.0 | 39% | 660 |
| Discussion writer | opus[medium] | 1.0 | 32.0 | 67.4 | 34% | 593 |
| Discussion reviewer's forks | opus[medium] | 6.0 | 23.8 | 57.9 | 68% | 581 |
| Plan reviewer on Sonnet | sonnet[high] | 1.2 | 35.0 | 76.0 | 73% | 573 |
| Discussion reviewer | opus[medium] | 1.5 | 40.0 | 63.4 | 34% | 543 |
| Plan reviewer's forks | opus[medium] | 4.5 | 21.5 | 39.5 | 59% | 377 |
| Discussion writer's forks | opus[medium] | 6.0 | 19.5 | 34.1 | 56% | 347 |
| Plan fixer's forks | opus[medium] | 11.0 | 33.5 | 32.3 | 53% | 317 |
| Gate fixers | sonnet[medium] | 8.0 | 81.0 | 35.2 | 32% | 307 |
| Discussion fixer's forks | opus[medium] | 7.0 | 17.2 | 30.0 | 69% | 299 |
| Code fixer | sonnet[medium] | 0.8 | 18.2 | 43.5 | 46% | 288 |

A run writes about 1.6 million output tokens, a quarter to three quarters of a role's share being thinking.
The Webster forks' requests fell from 452 to 254 a run while their output rose from 333 to 486 thousand tokens with the larger runs, and their request time rose with the output.
The turn audits in [turns](../../turns/turn-savings-measure-2026-11-04/report.md) count the requests; this table says why fewer of them did not shorten the run.

## Models, context and cache

Request seconds fitted on output tokens and context, over 14 762 requests.

| Model and period | Requests | Fixed s | Per 1000 output tokens s | Per 100000 context tokens s | R squared | Mean output tokens | Mean context tokens |
| --- | --- | --- | --- | --- | --- | --- | --- |
| opus[medium], before | 3896 | 0.95 | 8.21 | 0.04 | 0.96 | 975 | 170544 |
| opus[medium], after | 2319 | 1.00 | 8.10 | 0.21 | 0.97 | 1587 | 161943 |
| opus[high], all | 337 | 0.61 | 7.61 | 0.68 | 0.97 | 455 | 57429 |
| sonnet[medium], before | 4095 | 1.60 | 5.68 | 0.03 | 0.47 | 681 | 130092 |
| sonnet[medium], after | 3621 | 1.28 | 5.56 | 0.60 | 0.91 | 1164 | 125654 |
| sonnet[high], all | 494 | 0.63 | 7.11 | 0.06 | 0.98 | 1510 | 177834 |

- **Speed is a constant of the model.**
  Output tokens explain 96 to 97 % of an Opus request's time.
  The rate did not move between the periods, so neither load nor a rate limit slowed the API.
- **Sonnet writes a token in 69 % of Opus's time**, so a role moved from Opus to Sonnet at the same output saves 31 % of its request time, and no more.
- **Effort is output.**
  A lens fork spends 55 to 73 % of its output on thinking; a fork inherits its parent's model and effort, and no config key sets either for it.
- **Late turns at large context cost money, not time.**
  [turn-savings-measure](../../turns/turn-savings-measure-2026-11-04/report.md) prices them.
- **Prompt cache.**
  A session idle over 5 minutes missed the cache in 19 of 71 requests, writing 200 000 tokens again and taking 2.5 seconds longer; 17 more misses came under a minute apart, 27 seconds each.
  [startup-context](../../tokens/startup-context-2026-11-05/report.md) covers what a session loads at start.

Tool calls, median milliseconds per call, every picked run:

| Tool | Calls before | Median before | Calls after | Median after |
| --- | --- | --- | --- | --- |
| Read | 1250 | 40 | 3833 | 48 |
| Edit | 1392 | 54 | 3085 | 64 |
| Write | 697 | 60 | 946 | 71 |
| Bash, a read or search | 3609 | 72 | 2287 | 98 |
| Bash, git | 279 | 100 | 376 | 109 |

Reads, edits and lookups add up to 12 to 42 seconds of a darn run and 2 to 3 minutes of a tapestry run.

## The two machines

Medians of the row stamps, which hold the other machine's wall clock, its own sleep included.

| Machine and era | Runs | All rows, minutes | Darn-Write less its gate, seconds | Darn-Write gate spans, seconds | Finalize, seconds |
| --- | --- | --- | --- | --- | --- |
| this machine, before | 51 | 32.3 | 608 | 221 | 183 |
| other machine, before | 17 | 27.8 | 357 | 456 | 95 |
| this machine, after 1 | 59 | 37.7 | 480 | 296 | 356 |
| other machine, after 1 | 8 | 95.2 | 957 | 904 | 1938 |

The other machine's Darn-Write gate took twice as long as here before and three times after.
[fabric-test-speed-survey](../fabric-test-speed-survey-2026-11-03.md) measured `internal/fabricengine` 7.6 times slower there and could not say why.
Eight runs on one day settle nothing about the change of 2026-11-01 on that machine.

## Reproduce

- `../task-runtime-before-after-2026-11-06/extract.sh <this directory>` writes `runs.tsv` and `gate-by-slug.tsv`.
- `rows.sh > rows.tsv`, `gate-runs.sh > gate-runs.tsv`, `reviews.sh > reviews.tsv` and `repo-size.sh > repo-size.tsv` read the archive tags, the gate run records and the parent branch.
- `pick.sh > pick.tsv` picks the 40 runs, `turns.sh` reads their transcripts into `sessions.tsv`, `turns.tsv`, `tools.tsv` and `prompts.tsv` by `compact.jq` and `session.jq`, and `timeline.sh > timeline.tsv` splits the rows by `timeline.awk`.
- `analyze.sh` prints every table from those files, by `run-tables.awk` and `turn-tables.awk` over `lib.awk`.
- The transcripts and the gate run records live on this machine only, so the last three steps give these figures only here.
