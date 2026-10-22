# Token audit: where lyx spends Claude tokens (2026-10-08 to 2026-10-22)

A measurement report: it records one window of runs and is not kept current.

Window: every assistant message timestamped 2026-10-08 or later in `~/.claude/projects/-home-knatte-Code-loomyard-LYXHUB-*`, 15 days.
Total **$7,369 at API list price** (raw: 0.4M input, 512M cache write, 17.8B cache read, 106M output).
By cost type: cache reads 48%, cache writes 29%, output 24%.
Production task runs are 81% of spend, the orch 12%, bench pairs 7%.
The operator is on a subscription, so the dollars are relative weights, not a bill.

## Findings: savings levers, ranked by estimated share of total spend

1. **Fewer loom runs: small tasks to darn, small loom tasks folded together (est. 6-11%).**
   A loom run carries a fixed cost of about $22.60 whatever its size: a fit over 113 landed default loom runs gives cost = $22.6 + $0.72 per 100 changed lines (r = 0.77), so the fixed part is about half of all loom spend.
   Discussion is almost all fixed ($6.80 of $8.60 per run), and so is most of Plan ($10.50 of $18.50).
   Darn's fixed cost is $2.60 (cost = $2.6 + $0.52 per 100 lines, r = 0.78).
   The smallest loom quartile (42-816 lines, mean 461) cost $17.90 per run; the largest darn quartile (574-2,613 lines, mean 1,000) cost $7.90.
   Had the 35 loom runs under 1,000 changed lines cost what darn costs at their size, the window would have saved $483 (6.6%); for the 49 runs under 1,500 lines, $804 (10.9%).
   Batching related loom tasks saves the same $22.60 per run removed.
   Quality cost: medium, since those runs lose the Discussion and Plan review perches; darn tasks are simpler by selection, so this is an upper bound.
2. **Cap Plan-Review at one soft round and Discussion-Review at two hard rounds, profile `light`'s values (est. 6-9%).**
   Review loops (reviewer, fixer, judge, seed, advisor and their forks) cost 32% of all spend, and rounds after the first $1,054 (14.3%).
   Plan round 2 and later cost $492 (6.7%), and 85 of 128 runs reached round 2; Discussion round 3 and later cost $154 (2.1%).
   Webster's later rounds ($159) are already cut by `webster_max_bounces: 1`: runs since 2026-10-15 average 1.0-1.2 Webster rounds.
   Quality cost: real; A3 (`review-setup-audit`) found new real defects in Discussion round 3 and Plan round 2 in 3 of 3 runs that reached them.
   Only 5 `light` runs had landed, all on 2026-10-22, so the profile cannot be measured yet.
3. **Shrink the Plan writer's context (est. 2-5%).**
   The Plan writer is the costliest session type: $7.83 per session, $1,306 with its forks (17.7%).
   It averages 81 turns at a mean context of 223k tokens, holds 153k by its first fork (turn 40) and peaks at a median of 308k (max 662k), mostly from about 71 `sed -n`/`grep` code reads per session.
   Every `twill-plan` lens fork inherits that context, averaging 287k per turn; since 2026-10-20 the writer and its forks cost about $18 per run.
   A6's 8-to-5 lens cut (task `twill-plan-five-lenses`) saves about $110 (1.5%).
   Forking earlier, or handing the lead a digest of the code instead of raw reads, targets the 46% of its cost that is cache reads; cutting a quarter of the writer's and forks' cost saves about $330 (4.5%).
   Quality cost: unknown until benched.
4. **Opus roles whose output is checked downstream to Sonnet (est. 2-4.5%).**
   Sonnet 5.5 reads cache at the same $0.20/M as Opus 5.5, so a swap saves only half a role's non-cache-read cost.
   Plan and Discussion fixers about $162 (2.2%), darn writer $45, alignment $40, Webster fixer $37.
   Swapping the Plan writer ($178) or the round-1 reviewers ($144 + $115) saves more but risks the quality the rest of the run depends on.
   Quality cost: low to medium for fixers, since the judge and the next round re-check them, though A3 notes a sloppy fixer causes many late-round defects.
5. **Orch and bench overhead (est. 1.5-2% from the orch; up to 9% discretionary bench spend).**
   The orch costs $854 (11.6%): main session $476, 469 sub-agents $378.
   In the main session, operator dialogue costs $290, notice and task-notification wakes $128 (about $0.10-0.15 per wake), compacts and cycles $37.
   Its mean context is 117k over 11,027 turns; keeping it near 60k would save about half its $254 of cache reads (about $130, 1.8%).
   Benches and audits the operator asked for cost $649 (8.8%): bench pairs $489 plus orch bench sub-agents $160; the Fable 5.1 plan and discussion trial (2026-10-10 to 2026-10-12) cost $330 by itself.

Not levers, by the data:

- **Webster fork orientation.** Webster forks are the costliest part of a loom run ($10.41 per run, $1,361 in all, 18.5%), but only 8% of their cost comes before a fork's first edit.
  The cost is growth inside the fork: start context 62k, median peak 190k, mean 62 turns at 179k, 71% of it cache reads.
  A7's wider batches add high-context turns to each fork, so expect them to raise cost per card, not lower it.
  A small lever inside the forks: about 2,060 separate lint calls and about 1,500 `lyx gate test` calls each cost a turn at about 180k context, about $190 (2.6%) together.
- **Cache misses.** Mid-session cache rewrites cost $178 (2.4%), and first-message startup $208 (2.8%) across 4,716 sessions.
- **Stuck and retried rows.** 13 of 118 landed loom runs re-entered Webster once and one five times; recovery sessions cost $28.

The levers overlap: fewer loom runs (1) also removes their review rounds (2) and Plan writers (3).

## Tables

Weights are the Claude API list prices in `internal/shuttleengine/claudeengine/pricing.go`, the table `tools/tokencount` uses, in USD per million tokens:

| model | input | cache write 5m | cache write 1h | cache read | output |
|---|---|---|---|---|---|
| opus-5-5 | 4 | 5 | 8 | 0.20 | 20 |
| sonnet-5-5 | 2 | 2.5 | 4 | 0.20 | 10 |
| haiku-5-5 (prompt ≤100k / >100k) | 0.10 / 0.50 | 0.125 / 0.625 | 0.20 / 1 | 0.01 / 0.05 | 0.50 / 2.50 |
| fable-5-1 | 10 | 12.5 | 20 | 0.25 | 50 |

### Spend by bucket (raw tokens in millions)

| bucket | transcripts | input | cache write | cache read | output | cost | share |
|---|---|---|---|---|---|---|---|
| task loom/default | 5667 | 0.3 | 385.4 | 13272 | 78.45 | $5416 | 73.5% |
| bench pairs (twb, dbench) | 505 | 0.0 | 26.2 | 1098 | 7.54 | $489 | 6.6% |
| orch session | 5 | 0.0 | 21.8 | 1269 | 4.61 | $476 | 6.5% |
| orch sub-agents | 469 | 0.0 | 29.9 | 786 | 5.16 | $378 | 5.1% |
| task darn/default | 681 | 0.0 | 25.0 | 666 | 4.24 | $286 | 3.9% |
| task loom/light | 409 | 0.0 | 16.4 | 614 | 4.15 | $255 | 3.5% |
| orch-strand pair | 40 | 0.0 | 1.6 | 59 | 0.44 | $27 | 0.4% |
| task, no run record | 76 | 0.0 | 3.3 | 45 | 0.63 | $20 | 0.3% |
| prime, other | 23 | 0.0 | 1.7 | 30 | 0.48 | $15 | 0.2% |
| task darn/light | 30 | 0.0 | 0.7 | 10 | 0.09 | $6 | 0.1% |
| **total** | | 0.4 | 511.8 | 17849 | 105.78 | **$7369** | 100% |

### Task spend by phase ($5,984 attributed to runs)

| phase | cost | share | writer | review loop |
|---|---|---|---|---|
| Plan | $2379 | 39.8% | $1306 (55%) | $1073 (45%) |
| Webster | $1881 | 31.4% | $1465 (78%) | $415 (22%) |
| Discussion | $1131 | 18.9% | $415 (37%) | $715 (63%) |
| Darn | $265 | 4.4% | $184 (70%) | $80 (30%) |
| Alignment | $99 | 1.6% | | |
| Merge | $96 | 1.6% | | $96 |
| Driver | $51 | 0.9% | | |
| Describe, Arbiter, Friction, rework, other | $82 | 1.4% | | |

### Mean cost per landed loom run, by part

118 runs landed 2026-10-09 to 2026-10-22: mean $43.38, median $36.20, p25 $25.50, p75 $55.60; orch share not included.

| part | per run | share |
|---|---|---|
| Webster forks (sonnet) | $10.41 | 24.0% |
| Plan writer (opus) | $8.08 | 18.6% |
| Plan reviewer | $5.10 | 11.7% |
| Discussion reviewer | $2.90 | 6.7% |
| Discussion writer | $2.52 | 5.8% |
| Plan writer forks (twill-plan lenses) | $2.44 | 5.6% |
| Plan fixer | $1.79 | 4.1% |
| Discussion fixer | $1.50 | 3.5% |
| Webster reviewer | $1.30 | 3.0% |
| judges + seeds (all perches) | $1.28 | 3.0% |
| Webster fixer | $0.92 | 2.1% |
| Discussion writer forks | $0.82 | 1.9% |
| Describe, driver, arbiter, recovery, rework | $0.77 | 1.8% |
| Merge (all parts) | $0.73 | 1.7% |
| Alignment | $0.67 | 1.5% |
| Webster Master | $0.58 | 1.3% |
| advisors | $0.53 | 1.2% |

Darn, 62 runs, mean $4.71 (median $3.70): writer (opus) $2.98, reviewer $0.46 plus forks $0.15, fixer $0.38 plus forks $0.08, alignment $0.32, judge and seed $0.21, describe $0.10.

The loom mean by period was $43.40 since 2026-10-09, $41.60 since 2026-10-15, and $68.30 for the 12 runs landed since 2026-10-18.
Those 12 are larger (median about 3,300 lines); over them Plan rose to $32 per run, and Plan writer forks rose from $0.80 to $7.70 per run once the full `twill-plan` fan landed on 2026-10-20.

### Recipe comparison

| recipe/profile | landed runs | mean | median | lines/run | $/100 lines | fixed + per 100 lines |
|---|---|---|---|---|---|---|
| loom/default | 113 | $43.0 | $36.2 | 2834 | 1.52 | $22.6 + $0.72 |
| loom/light | 5 | $51.1 | $42.7 | 2313 | 2.21 | too few |
| darn/default | 60 | $4.8 | $3.7 | 418 | 1.14 | $2.6 + $0.52 |
| darn/light | 2 | $2.8 | | 84 | | too few |

| recipe | quartile | lines | mean lines | mean cost |
|---|---|---|---|---|
| loom | Q1 | 42-816 | 461 | $17.9 |
| loom | Q2 | 829-1778 | 1248 | $32.3 |
| loom | Q3 | 1799-3720 | 2730 | $47.0 |
| loom | Q4 | 3732-13154 | 6757 | $73.9 |
| darn | Q1 | 16-109 | 66 | $2.7 |
| darn | Q2 | 110-249 | 175 | $3.1 |
| darn | Q3 | 259-561 | 432 | $5.4 |
| darn | Q4 | 574-2613 | 1000 | $7.9 |

Fixed versus scaling cost per loom phase: Discussion $6.8 + $0.06 per 100 lines, Plan $10.5 + $0.28, Webster $4.0 + $0.35.

### Review cost by round

A round is one reviewer session; its fixer, judge and forks count with it.

| perch | round 1 | round 2 | round 3 | round 4 | round 5+ | rounds 2+ |
|---|---|---|---|---|---|---|
| Discussion | $306 (128 runs, $2.39 each) | $239 (122) | $109 (60) | $32 (13) | $13 (7) | $393 |
| Plan | $564 (128, $4.41 each) | $313 (85) | $118 (33) | $46 (14) | $15 (8) | $492 |
| Webster | $245 (130, $1.88 each) | $64 (40) | $46 (27) | $26 (13) | $23 (14) | $159 |
| Merge | $50 (61) | $2 (2) | $1 (1) | | | $3 |
| Darn | $69 (55) | $5 (7) | $1 | $1 | | $7 |

### Session shapes (main sessions)

| part | sessions | turns/session | $/session | mean context per turn | cache write/session | output/session |
|---|---|---|---|---|---|---|
| Plan writer | 130 | 81 | 7.83 | 223k | 375k | 72.8k |
| Darn writer | 66 | 58 | 2.79 | 124k | 136k | 35.0k |
| Discussion writer | 128 | 39 | 2.49 | 94k | 142k | 36.8k |
| Plan reviewer | 268 | 48 | 2.48 | 123k | 167k | 37.1k |
| Discussion reviewer | 330 | 27 | 1.13 | 71k | 92k | 19.4k |
| Plan fixer | 252 | 25 | 1.03 | 77k | 79k | 13.6k |
| Webster reviewer | 224 | 28 | 0.93 | 91k | 113k | 14.1k |
| Webster fixer | 164 | 28 | 0.91 | 73k | 71k | 11.6k |
| Discussion fixer | 302 | 18 | 0.68 | 52k | 57k | 11.1k |
| Webster Master | 150 | 21 | 0.51 | 56k | 62k (1h TTL) | 4.0k |
| Alignment | 198 | 12 | 0.50 | 48k | 58k | 5.4k |
| judge / seed / describe | about 1,100 | 6-8 | 0.12-0.15 | 29-32k | 28-37k | 2-3k |

| forks of | forks | turns/fork | $/fork | mean context per turn |
|---|---|---|---|---|
| Webster Master (batch forks) | 445 | 62 | 3.06 | 179k |
| Plan writer (lens forks) | 431 | 7 | 0.67 | 287k |
| Plan reviewer (lens fan) | 63 | 12 | 0.71 | 176k |
| Discussion writer (lens forks) | 535 | 3 | 0.18 | 117k |
| Discussion reviewer (lens fan) | 84 | 7 | 0.36 | 116k |

### Opus spend and Sonnet-swap saving (half of non-cache-read cost)

| role | opus cost | non-cache-read | swap saves |
|---|---|---|---|
| Plan writer | $801 | $356 | ~$178 |
| Plan reviewer | $512 | $288 | ~$144 |
| Orch session | $486 | $228 | ~$114 |
| Discussion reviewer | $334 | $229 | ~$115 |
| bench pairs | $372 | $187 | ~$93 |
| Plan fixer | $260 | $168 | ~$84 |
| Discussion writer | $245 | $160 | ~$80 |
| Discussion fixer | $211 | $156 | ~$78 |
| Darn writer | $184 | $91 | ~$45 |
| Alignment | $99 | $79 | ~$40 |
| Webster fixer | $126 | $74 | ~$37 |
| all Opus | $4442 | $2393 | ~$1196 |

### Orch main session by wake type

| wake | wakes | turns | cost | share | $/wake |
|---|---|---|---|---|---|
| operator dialogue | 2398 | 7026 | $290.5 | 63.4% | 0.12 |
| task-notification | 677 | 1627 | $65.3 | 14.3% | 0.10 |
| lyx notice (shuttle, creel, batten, loom) | 423 | 1481 | $62.7 | 13.7% | 0.15 |
| compact / cycle / distill | 393 | 534 | $36.8 | 8.0% | 0.09 |
| slash and local commands | 139 | 31 | $2.9 | 0.6% | 0.02 |

The orch's sub-agents: 273 forks that inherit its context $82, benches and grading $160, audits and research $60, fixes and reviews $18, other about $56.

### Costliest landed runs

| run | recipe | cost | Discussion | Plan | Webster | other | lines |
|---|---|---|---|---|---|---|---|
| webster-replan-state | loom | $141.9 | 32.4 | 58.5 | 48.5 | 2.6 | 5041 |
| machine-handover | loom | $117.7 | 20.5 | 53.4 | 38.0 | 5.9 | 7286 |
| test-suite-weight | loom | $107.3 | 12.9 | 29.3 | 60.8 | 4.3 | 7727 |
| creel-notices | loom | $101.5 | 17.4 | 46.9 | 32.8 | 4.4 | 9495 |
| run-process-ownership | loom | $99.3 | 20.5 | 46.7 | 28.4 | 3.8 | 6844 |
| driver-context | loom | $89.3 | 14.2 | 32.9 | 39.8 | 2.4 | 7781 |
| batten-start-speed | loom | $87.1 | 13.3 | 44.1 | 25.5 | 4.2 | 6026 |
| merge-review | loom | $85.2 | 8.9 | 26.6 | 47.7 | 2.0 | 8925 |

## Method

- **Extraction.** jq pulls one row per assistant message with usage from 8,359 transcripts, sub-agent transcripts included; only messages from 2026-10-08 on count.
  Usage is deduplicated per `message.id`, taking each field's maximum, and the 1,407 message ids found in both a parent and a fork transcript count once, under the parent.
- **Weighting.** Each message is priced at its model's list price, with 5-minute and 1-hour cache writes priced apart.
- **Role.** The strand name in the transcript's `custom-title` line, minus any `-N` and `-chair` suffix; a sub-agent counts under its parent's role as that role's forks.
- **Run.** A session maps to the weft archive tag whose window (seed `started_at` to the last history entry) holds its start; 99.7% of task-pair spend mapped.
- **Phase.** Strand names carry the phase since 2026-10-15; for older names, the producer whose history interval holds the session's start.
- **Recipe, outcome and profile.** Recipe from `seed.json`; landed means Merge-In done or Finalize recorded; profile from the board entry's `profile` field in `_board` git history (only 22 slugs ever had one, every other run counts as default).
- **Size.** Changed lines are added plus deleted lines of the `main` squash commit whose subject matches the first line of the run's landing summary; 252 runs matched.
- **Rounds.** Counted at each reviewer session, in order, per run and perch.
- **Orientation.** A Webster fork's turns before its first `Edit`/`Write`.

The per-run cost table sits beside this file as `runcost.tsv`, tab-separated with no header row.
Its columns: start time, slug, archive tag, recipe, profile, landed (1/0), cost, Discussion, Plan, Webster, Merge, Darn and other cost (USD), lines added, lines deleted, files changed, review rounds for Discussion, Plan, Webster, Merge and Darn, and main sessions.

## Gaps

- Quality is not measured here; the quality costs come from A3, A6 and `docs/research/model-downgrade-results.md`. There is no darn-versus-loom follow-up-fix rate, and the convergence report does not split by recipe.
- Darn tasks were picked because they were small, so the darn-routing saving is an upper bound.
- Too little profile data: 5 `light` loom runs, 2 `light` darn runs, no `budget` runs; a profile set only through run settings may be missed.
- Changed lines include tests, docs and fixtures; runs that matched no commit are left out of the per-line figures.
- The orch ($854) is not charged to runs; spread evenly it adds about $3-4 per run, and it includes the operator's own dialogue.
- $18 of task spend could not be placed in a phase.
- Reviewer lens-fan forks appear only from about 2026-10-16.
- Configuration changed during the window (Webster rounds, twill fans, the Fable trial), so the window totals mix setups.
- Webster batches are not costed per card; `tools/tokencount -calibrate` is the check for A7.
