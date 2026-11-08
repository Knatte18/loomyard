# Writer lookups and Plan phase cost, re-measured 2026-11-08

Two open questions read the same tapestry runs: whether the writers now delegate lookups early and what that saves,
and whether the Plan phase has got cheaper since the plan writer audit of 2026-10-24.
Every figure derives from the scripts and tables beside this file; the sample size stands beside each number.
No agent run, test or gate run was made: the work reads records, transcripts and code.

## Finding

The sample is 5 tapestry runs: every one that landed after `3085f451`,
the Discussion-Write stencil change of `discussion-explore-agents-first`.
They hold 8 writer sessions, 5 Plan-Write and 3 Discussion-Write;
`slack-relay` and `creel-cross-hub-build` ran no Discussion-Write session.

- The Discussion writers now delegate:
  3 of 3 sent one batch of 4 or 5 lookup subagents at turn 6 or 7, at 60k to 65k context.
  Before the stencil change 1 of 3 full sessions did, at turn 7 and 63k context.
- The Plan writers still delegate once, early: 4 of 5 by turn 12; `slack-relay` sent its batch at turn 26.
- Delegating early cut the carry of lookup results in a Discussion session from $1.37 to $0.84,
  the writer's own and the subagents' replies together, and the writer's own results from 155k to 84k tokens.
  The lookup subagents cost $0.80 a session, and the session cost did not move: $3.05 against $3.06.
  Delegation moved cost from carry to subagents; it saved none in these Discussion sessions.
- In Plan sessions the carry is the writer's own reading after its one batch:
  213k to 320k tokens in a session against 5k to 21k of subagent replies.
  Whole-file Reads are 79% of the lookup results the five Plan writers took in themselves.
- Priced as a saving of its own, the lookup context the subagents kept out of the writers is worth at most $5.95
  over the 8 sessions, 13% of their $46.94, after the subagents' own cost.
  It is an upper bound, since the writer would not keep everything a subagent read.
- The Plan phase per run did not fall: writer plus forks is $17.05 against $17.24 in the audit's comparable runs,
  and the whole phase $34.89 against $31.38.
  Per unit of task size it did fall:
  writer plus forks costs $1.73 per 1000 changed lines against $5.11, and $0.84 per plan card against $1.21.
  The sample's tasks are 2.9 times the audit's by changed lines and 1.4 times by plan cards.
- Plan rounds rose from 2.2 to 3.2, and review and fix are now 51% of the Plan phase.

## Sample and method

- Runs: the tapestry runs whose archive tag is newer than `3085f451` (2026-11-05 06:35 +01:00),
  as `go run ./tools/tokencount -since 2026-11-05` lists them:
  `pawl-orch-landing`, `raddle-recipe`, `drift-rework`, `slack-relay`, `creel-cross-hub-build`.
  They landed from 2026-11-05 11:56 to 2026-11-06 13:25 and are in `slugs.txt`.
  `slack-push-channels` is still running and is not counted.
- Guidance in force: the 3 Discussion-Write transcripts hold the text "Start the exploration with explore subagents".
- Part 1 reruns the 2026-11-04 measurement with its scripts,
  copied here as `writers.sh`, `subagents.sh` and `run-all.sh`.
  Definitions are those of `docs/research/turns/writer-lookups-measure-2026-11-04/report.md`:
  a turn is one assistant message id, an exploration turn holds only lookups,
  a chain is two or more exploration turns in a row,
  and cost is list price of the turn's tokens at `claude-opus-5-5`.
- Carry (`carry.sh`): the context a turn adds,
  priced as one cache write in the next turn and one cache read in each turn after.
  Two parts: the writer's own lookup results, and the replies of its lookup subagents.
  The earlier sample's tables, read from the 2026-11-04 directory, give the sessions that did not delegate.
- Part 2 (`plan-table.sh`): per-run cost by Plan segment from `tools/tokencount`,
  plan cards counted in the archive tag, and changed lines of the landing commit.
  The audit's runs have no transcripts left on this machine,
  so its costs are its committed `q1-table.tsv` and only their sizes are looked up (`audit-sizes.sh`).
- Wall time: Plan-Write elapsed from `tools/tokencount`'s producer table.
  `lym report timing` reports the whole Plan phase per run and no per-producer split;
  its figures are in `timing-plan-phase.tsv`.
- A group's per-1000-line and per-card figures divide summed cost by summed size (`group-means.sh`).

## Part 1: writer lookups

### Do the writers delegate up front

| run | role | turns | lookup subagents | first batch at turn | context there | exploration turns | before / after the batch | chains | exploration cost | cost |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `pawl-orch-landing` | Discussion | 35 | 4 | 6 | 61k | 19 | 2 / 17 | 4 | $1.53 | $3.39 |
| `raddle-recipe` | Discussion | 35 | 5 | 6 | 60k | 19 | 2 / 17 | 3 | $1.29 | $3.19 |
| `drift-rework` | Discussion | 29 | 5 | 7 | 65k | 12 | 4 / 8 | 3 | $0.87 | $2.81 |
| `pawl-orch-landing` | Plan | 56 | 4 | 9 | 151k | 32 | 6 / 26 | 5 | $4.72 | $8.76 |
| `raddle-recipe` | Plan | 56 | 7 | 8 | 110k | 28 | 5 / 23 | 4 | $3.56 | $7.58 |
| `drift-rework` | Plan | 59 | 6 | 12 | 224k | 29 | 9 / 20 | 6 | $4.68 | $9.88 |
| `slack-relay` | Plan | 62 | 4 | 26 | 174k | 36 | 18 / 18 | 6 | $4.01 | $6.90 |
| `creel-cross-hub-build` | Plan | 21 | 2 | 7 | 241k | 5 | 3 / 2 | 1 | $0.64 | $4.42 |

All 8 sessions sent their lookups as one batch in one message to `general-purpose` subagents on `sonnet`.
The 37 lookup subagents averaged 7.2 turns and $0.22 and held about 5 questions in their prompts.
Seven of the 8 sessions went on exploring in their own context after the batch, for 8 to 26 further exploration turns.

Against the 2026-11-04 sample (3 runs, 7 sessions):

| | 2026-11-04 Plan | now Plan | 2026-11-04 Discussion | now Discussion |
| --- | --- | --- | --- | --- |
| sessions | 3 | 5 | 4 | 3 |
| started a lookup batch | 3 | 5 | 1 | 3 |
| turns | 115 | 254 | 97 | 99 |
| session cost | $14.48 | $37.54 | $9.28 | $9.39 |
| exploration turns | 57 (50%) | 130 (51%) | 62 (64%) | 50 (51%) |
| exploration cost | $6.15 (42%) | $17.62 (47%) | $4.88 (53%) | $3.69 (39%) |
| exploration cost above 200k context | $3.38 | $12.47 | $0.23 | $0.16 |
| turns above 200k context | 85 (74%) | 176 (69%) | 14 | 6 |
| cost of turns above 200k context | $10.99 (76%) | $30.74 (82%) | $1.79 | $0.84 |
| chains | 9 | 22 | 5 | 10 |
| turns in chains | 49 | 114 | 54 | 40 |
| carry of the writer's own lookup results | $7.63 | $16.03 | $3.62 | $2.34 |

The 2026-11-04 Discussion column holds the ended-early session of `decision-record-on-board`.
Exploration is 51% of the writers' turns and 45% of their cost, against 56% and 46%.
The Discussion writers' longest chain is 11 turns, against 20 and 15 in the two sessions that did not delegate.
The Plan sessions grew with their tasks: 56 to 62 turns in four of five, and a peak context of 353k to 522k.

### What early delegation saves in carry

Discussion, lookup results the writer kept and what they cost to carry, mean per session:

| group | sessions | own lookup results | carry of own results | carry of subagent replies | lookup subagents | exploration turns | session cost |
| --- | --- | --- | --- | --- | --- | --- | --- |
| delegated early | 4 | 84k tokens | $0.76 | $0.08 | $0.80 | 17.3 | $3.05 |
| did not delegate | 2 | 155k tokens | $1.37 | $0 | $0 | 19.5 | $3.06 |

The first row is `batten-remote-order`'s Discussion session of the 2026-11-04 sample and the three now;
the second is the two full sessions of that sample that did not delegate.
Delegation cut the carried results by 46% and the carry by $0.53, 17% of the session;
the subagents cost more than that.
Two sessions are not a rate, and the tasks differ.

The same sessions priced on the subagents' side:
a lookup subagent read in 26k tokens on average for a Discussion writer and 37k for a Plan writer
(`lookup-context.tsv`, last context minus first).
Had the writer read the same itself, it would have carried that for the rest of the session.

| role | sessions | context the subagents read | carry avoided, upper bound | carry of their replies | subagents' cost | net, upper bound |
| --- | --- | --- | --- | --- | --- | --- |
| Discussion | 3 | 122k tokens | $1.23 | $0.07 | $0.82 | $0.34 |
| Plan | 5 | 169k tokens | $2.24 | $0.15 | $1.10 | $0.99 |

Mean per session.
Over the 8 sessions the net is at most $5.95 of $46.94, 13%.
The figure assumes the writer would have kept every token a subagent read,
and ignores the turns it would have spent reading.
The Discussion sessions realised about 43% of their upper bound ($0.53 against $1.23).

Seven of the 8 Plan sessions across both samples delegated by turn 12, so they leave almost nothing to compare against.
`slack-relay`, the one late session, carried $2.99 on a session that cost $6.90,
against a mean of $3.13 and $6.45 for the seven early ones, on tasks of different size.
What the Plan carry is made of is the writer's own reading after the batch:

| run | whole-file Reads | whole-file tokens | slice Reads | slice tokens | Bash calls | Bash tokens |
| --- | --- | --- | --- | --- | --- | --- |
| `pawl-orch-landing` | 56 | 134k | 12 | 17k | 25 | 18k |
| `raddle-recipe` | 42 | 88k | 10 | 17k | 22 | 20k |
| `drift-rework` | 65 | 166k | 8 | 0.3k | 31 | 32k |
| `slack-relay` | 47 | 93k | 0 | 0 | 39 | 39k |
| `creel-cross-hub-build` | 55 | 161k | 7 | 8k | 9 | 15k |

Whole-file Reads are 79% of the 808k result tokens.
The five sessions carried $1.64 to $4.27 for their own results, $3.21 on average, 19% of writer plus forks.
Result tokens are characters divided by 4.

### Late chains and the upper bound

The late chains are those whose first turn is above 200k context, all in Plan sessions as before.

| | 2026-11-04 | now |
| --- | --- | --- |
| chains | 6 | 12 |
| turns | 25 | 41 |
| calls | 41 | 108 |
| calls per turn | 1.6 | 2.6 |
| cost | $2.38 | $6.53 |
| single-call turns in a row: chains, turns, cost | 3, 14, $1.11 | 5, 19, $1.88 |

Two chains, one in `raddle-recipe` and one in `drift-rework`, hold 29 calls in 2 or 3 turns,
so batching is happening where it is cheap.
Moving every search turn to subagents, priced as the 2026-11-04 report did,
now saves at most about $5.4 of $46.94, 12%, against about $4.7 and 20% then.
That is the 118 search turns' $13.76, less $3.54 at the observed $0.030 a subagent turn,
less $4.80 for the 32 chains' delegating turns at their first context.
The bound fell because the delegating turns of the many late chains are dear,
and because the Plan writers already delegate their first batch.

## Part 2: Plan phase cost

Cost is API list price from `tools/tokencount`.
Writer is the Plan-Write main session; forks are its fork sessions,
which hold the plan-writing fork, the lens reviews and the lookup subagents.
Plan phase is writer, forks, review and fix.
Rounds are Plan reviewer sessions.

### Per run

The 5 runs of the sample:

| run | cards | changed lines | writer | forks | writer + forks | Plan phase | rounds | Plan-Write min | Plan phase wall min |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `pawl-orch-landing` | 19 | 8,484 | $8.76 | $9.70 | $18.46 | $32.25 | 2 | 34.5 | 69.9 |
| `raddle-recipe` | 28 | 11,648 | $7.58 | $11.51 | $19.09 | $43.01 | 5 | 37.4 | 97.1 |
| `drift-rework` | 28 | 16,913 | $9.88 | $14.08 | $23.96 | $48.06 | 4 | 41.4 | 107.1 |
| `slack-relay` | 15 | 7,416 | $6.90 | $6.28 | $13.18 | $29.79 | 3 | 23.5 | 72.5 |
| `creel-cross-hub-build` | 11 | 4,927 | $4.42 | $6.16 | $10.58 | $21.33 | 2 | 24.4 | 50.5 |
| mean | 20.2 | 9,878 | $7.51 | $9.55 | $17.05 | $34.89 | 3.2 | 32.2 | 79.4 |

The 10 tapestry runs that landed between the audit and the sample (2026-10-31 to 2026-11-03):

| run | cards | changed lines | writer | forks | writer + forks | Plan phase | rounds | Plan-Write min | Plan phase wall min |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `run-verbs-shared` | 21 | 9,150 | $16.78 | $11.64 | $28.42 | $54.68 | 3 | 28.8 | 84.1 |
| `interactive-recipe` | 13 | 3,630 | $11.26 | $5.83 | $17.09 | $29.01 | 2 | 23.4 | 48.6 |
| `warp-visibility` | 21 | 7,365 | $12.40 | $9.07 | $21.47 | $39.44 | 2 | 30.5 | 73.7 |
| `plan-split-producers` | 21 | 7,089 | $11.53 | $9.07 | $20.60 | $34.65 | 2 | 30.2 | 61.2 |
| `reed-status-buttons` | 10 | 2,695 | $6.37 | $5.36 | $11.73 | $25.65 | 2 | 19.4 | 52.4 |
| `machine-switch` | 15 | 6,734 | $7.42 | $7.23 | $14.65 | $28.04 | 3 | 23.9 | 53.6 |
| `gate-run-keeps-cache-warm` | 16 | 5,068 | $11.11 | $6.82 | $17.93 | $27.03 | 1 | 24.6 | 42.1 |
| `shed-sub-run-block` | 10 | 2,775 | $4.70 | $5.10 | $9.80 | $19.01 | 2 | 17.5 | 36.3 |
| `decision-record-on-board` | 10 | 1,969 | $4.15 | $4.53 | $8.68 | $19.48 | 2 | 15.4 | 42.5 |
| `batten-remote-order` | 15 | 5,261 | $5.63 | $6.84 | $12.47 | $31.65 | 2 | 25.5 | 88.3 |
| mean | 15.2 | 5,174 | $9.13 | $7.15 | $16.28 | $30.86 | 2.1 | 23.9 | 58.3 |

The first seven of these ran no lookup subagents (6 fork sessions each);
the last three, the 2026-11-04 sample, ran 3 or 4 (9 or 10 fork sessions).

### Beside the audit

| group | runs | cards | changed lines | writer | forks | writer + forks | Plan phase | rounds | Plan-Write min | Plan phase wall min |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| audit B, full fan, 2026-10-20 to 10-22 | 7 | | | $10.48 | $7.66 | $18.14 | $30.18 | 2.00 | 23.9 | |
| audit C, five lenses | 3 | | | $9.24 | $7.40 | $16.64 | $26.59 | 1.33 | 23.9 | |
| audit runs with a known size | 9 | 14.2 | 3,373 | $9.95 | $7.29 | $17.24 | $31.38 | 2.2 | 24.3 | 59.0 |
| 2026-10-31 to 2026-11-03 | 10 | 15.2 | 5,174 | $9.13 | $7.15 | $16.28 | $30.86 | 2.1 | 23.9 | 58.3 |
| sample | 5 | 20.2 | 9,878 | $7.51 | $9.55 | $17.05 | $34.89 | 3.2 | 32.2 | 79.4 |

The audit's solo-writer era (28 runs before 2026-10-15) paid $9.08 a run for writer plus forks.
The audit's "known size" row is the 9 of its 11 runs from 2026-10-19 on
whose landing commit and plan cards can be found; `batten-cancel-verb` and `plan-card-rules` have none.
Its wall times are the audit's `results.md` figures.

### Normalised by task size

| group | writer + forks per 1000 lines | writer + forks per card | Plan phase per 1000 lines | Plan phase per card | writer per 1000 lines | Plan-Write min per 1000 lines |
| --- | --- | --- | --- | --- | --- | --- |
| audit runs with a known size | $5.11 | $1.21 | $9.30 | $2.21 | $2.95 | 7.2 |
| 2026-10-31 to 2026-11-03 | $3.15 | $1.07 | $5.97 | $2.03 | $1.77 | 4.6 |
| sample | $1.73 | $0.84 | $3.53 | $1.73 | $0.76 | 3.3 |

A like-for-like comparison of runs in one size band, 4,900 to 8,500 changed lines:

| group | runs | mean lines | writer | writer + forks | Plan phase |
| --- | --- | --- | --- | --- | --- |
| audit | 2 | 7,065 | $18.16 | $28.28 | $50.05 |
| 2026-10-31 and 2026-11-01 | 4 | 6,564 | $10.62 | $18.66 | $32.29 |
| 2026-11-03 | 1 | 5,261 | $5.63 | $12.47 | $31.65 |
| sample | 3 | 6,942 | $6.69 | $14.07 | $27.79 |

Least-squares fits of writer plus forks on thousands of changed lines give
$6.68 + $3.13 per 1000 lines for the audit (9 runs, r 0.90),
$4.70 + $2.24 for the 10 runs between (r 0.88) and $6.40 + $1.08 for the sample (5 runs, r 0.95).
The fixed part holds near $5 to $7 a run; the part that grows with size falls by two thirds.
Per-1000-line averages fall with size in part because of that fixed part,
so the fits and the band are the fairer comparison.
Changed lines count deleted lines, and `drift-rework` deletes 5,348 of its 16,913,
so per-card figures are the cautious reading.

### Has the Plan phase got cheaper, and why

Per run, no.
Writer plus forks is flat at $17 against the audit's comparable runs ($17.05 against $17.24)
and the audit's group means ($16.64 to $18.14).
The whole phase is 11% up ($34.89 against $31.38), and Plan-Write takes 32 minutes against 24.

Per unit of size, yes for the writer and its forks:
$0.84 a card against $1.21 and $1.73 a thousand lines against $5.11, for the same cost on 2.9 times the lines.
The whole phase fell less per card, $1.73 against $2.21, because its review and fix part did not shrink.

What moved, from the tables:

- The writer's main session fell:
  $9.95 to $7.51 against the audit, and from $10.98 on the seven runs of 2026-10-31 and 2026-11-01
  to $4.83 on the three of 2026-11-03, the first with lookup subagents.
  Per 1000 lines that is $1.84, then $1.45, then $0.76.
- Forks did not fall: $9.55 against $7.29.
  The sample's forks hold about $1 of lookup subagents a run (23 subagents over the five runs).
  The rest, the plan-writing fork and the five lens forks, is about $8.5 against $7.40 for the audit's five-lens runs.
  Cost the main writer shed reappears as forks and subagents:
  writer plus forks per 1000 lines was $3.16 on the first seven runs and $3.09 on the three with lookups.
- Plan rounds rose to 3.2: `raddle-recipe` took 5 and `drift-rework` 4, the two largest.
  Review, fix and their judges are $17.84 a run, 51% of the Plan phase against 45% for the audit's runs.
  That is about $5.6 a round against the $3.70 the board note priced.
- Changes between the audit and the sample that touch the Plan writer, by commit date:
  the batching sentences (`d3b47360`, 2026-10-28), the heddle edit-round instruction (`cfad47e6`, 2026-11-01),
  subagents and forks allowed in every run (`1ceaa027`, 2026-11-01)
  and the Discussion lookup batch (`2d319fe5`, 2026-11-02).
  The data do not separate them from the change in task mix, and the audit runs cannot be re-priced.

The drop per unit of size rests on 5 runs, a size mix that differs from the audit's, and one cost model.
It says the fan, the lookups and the larger tasks together cost no more per run than the audit's fan on small tasks.
It does not pin down a cheaper phase at equal size:
in the 4,900 to 8,500-line band, writer plus forks fell 25% from the four runs of 2026-10-31 and 2026-11-01
to the sample's three.

## Board note `plan-writer-context` (R6): open ideas

| idea | what the figures show | still supported |
| --- | --- | --- |
| Fork the lenses earlier, before the code reads pile up | The plan-writing fork starts at 229k to 292k context, whenever the lookup batch went out (`raddle-recipe` delegated at 110k and forked at 229k; `slack-relay` delegated at turn 26 and forked at 251k); the five lens forks start at 280k to 349k. The lens fan is 5 forks at $0.4 to $1.3, about $3.7 to $4.2 a run, 22% to 25% of writer plus forks | Partly. The inherited context is still high, but no figure here shows what an earlier fork saves or costs in quality; it needs the bench the note asks for |
| Sub-planners per part for large bundles | Cost does not grow fast enough to need it: the largest run, `drift-rework` (28 cards, 16,913 lines), cost $23.96 for writer plus forks, 2.3 times the smallest's $10.58 for 3.4 times the lines. Peak context reached 522k and Plan-Write took 41 minutes | Not for cost. For wall time and peak context only on the two 28-card runs; the 2026-11-01 bench found 15m39s against 25m07s and a 245k peak against 408k on one run |
| Split Plan review per part | Review and fix with judges are 51% of the Plan phase; `raddle-recipe` spent $13.60 on review and $9.45 on fixes over 5 rounds, 54% of its Plan phase. Nothing here tests a per-part review | Supported as the larger pool than the writer, untested as a remedy |
| Twill against a solo Opus high or Fable writer | The sample holds no solo arm. The lens fan costs about $3.7 to $4.2 a run, less than one Plan round at about $5.6, so it pays back if it spares two thirds to three quarters of a round; rounds rose from 2.2 to 3.2 with the fan unchanged | Open; no new evidence either way |
| Excerpts from the exploration fan in place of whole-file Reads by the writer | Whole-file Reads are 79% of the lookup results the Plan writers took in (42 to 65 Reads a session, 88k to 166k tokens). Subagent replies are already short, 5k to 21k tokens. The writers' own carry is $3.21 a session, 19% of writer plus forks, a ceiling on this idea | Supported as the largest remaining pool; the note's correction that the writer must read what it judges still bounds it, and no quality figure exists |

The code digest, fresh-agent lenses and skipping the plan read-back stay rejected by the note.
`relevant.md` use and the double reading between Discussion and Plan are not measured here.

## Limits

- 5 runs and 8 writer sessions; every rate is a case count over few cases.
  The Discussion comparison sets 4 delegating sessions against 2.
- Part 1's comparison between delegating and non-delegating sessions mixes two samples and different tasks;
  no Discussion session of this sample failed to delegate.
- Pricing is list price of the tokens; every writer session of the sample ran `claude-opus-5-5`.
  Subagent and fork costs in `writer-subagents.tsv` are priced by the script from their own transcripts
  and run about 13% above `tools/tokencount` for the same sessions;
  the report uses `tools/tokencount` for run totals and the script's figures for shares and subagent costs.
- The carry uses cache write and read prices of the list; it assumes no cache expiry in a session.
- Result tokens are characters divided by 4.
- The audit's own runs were not re-priced: its costs are the committed figures,
  and its sizes come from landing commits found by title, which exist for 9 of its 11 runs.
- Changed lines include tests and deletions; plan cards are the plan's card files.
- The study measures cost, rounds and time, not plan quality.
