# Batching effect audit, 2026-11-01

Whether the batching sentences of `bf00a043` (2026-10-28) changed how the Plan and Discussion writers spend their turns, context and money.
The turn audit (`docs/research/turns/turn-audit-2026-10-28/report.md`) recommended the sentences after finding that the Plan writer spent 33 of its 101 turns on serial single reads and searches.
Every figure derives from the scripts and TSV tables beside this file; `n` stands beside each mean.
No agent or model run was made: the analysis reads existing transcripts and the records repo only.

## Finding

The transcripts show no batching effect, and the sample cannot show a small one.
The writers after the change do not issue more calls per turn, do not spend a smaller share of their turns on serial reads and searches, and run at a higher context.
Two landed runs make the "after" group, so the means are a pointer and not a measurement.
The Plan-writer ideas of notes `plan-writer-context` and `loom-beam-darns` are worth what they were worth before the sentences: the serial-read share the audit found is still there.

## Sample and method

- Source: the Claude Code transcripts under `~/.claude/projects/`, main writer sessions and their fork sub-agents, extracted by the turn audit's `extract-turns.sh` and `session-metrics.sh` unchanged.
  `run-all.sh` runs a copy of the extractor beside this folder's `slugs.txt`.
- Runs: every loom run with a Plan-Write or Discussion-Write transcript on this machine.
  Eight runs have one, and seven of them landed (an `archive/*` tag in the records repo).
  The eighth, `interactive-recipe`, is still running; its two writer sessions are finished and are shown in the tables but left out of every group mean.
  The Plan and Discussion writers of a run are two sessions, so 7 landed runs give 14 landed writer sessions.
  Writer transcripts exist only from 2026-10-27 on; earlier runs have none on this machine.
- Group: a session is **after** when the Plan or Discussion stencil's batching sentence ("Issue every search and read you can name in one message") appears in its transcript, otherwise **before**.
  The stencil is read when the session starts, so this is the evidence that the sentence reached the agent.
  It does not confirm the second change of the commit, the sentence in shuttle's appended system prompt, which a transcript does not hold; that sentence reaches a session only if the `lym` binary that spawned it was built after the commit.
  Before: 5 runs (`dobby-and-adopt-hardening`, `creel-hook-post`, `landing-settles-in-place`, `review-setup-tuning`, `validator-checks`), all started 2026-10-27 or the morning of 2026-10-28.
  After: 2 landed runs (`creel-agents`, `run-verbs-shared`) and the unlanded `interactive-recipe`, started 2026-10-28 15:23 or later.
- A turn is one assistant API message, as in the turn audit.
  Calls per turn count turns with no call.
  A **serial turn** is a turn of the turn audit's mergeable run: a single-call read, search, git-read, lym-read, quarry or skill turn after the first of a run of them.
  `serial_strict` leaves out a call that needs a path out of the previous turn's result; `serial_class` is the upper bound.
  `serial_share` is `serial_strict` over turns.
- Context of a turn is its input + cache write + cache read tokens; the table gives the session mean and peak.
- Cost is the Claude API list price of `claude-opus-5-5`, the one model every writer session used, from the price row in `internal/shuttleengine/claudeengine/pricing.go`, with the one-hour cache write priced apart from the five-minute one.
  `cost_usd` is the main writer session; `total_cost_usd` adds its fork sub-agents (the twill lens forks, and for the Plan writer the whole-plan fork).
- Task size comes from what the writer was handed: for the Plan writer the cards of `_lym/plan/` and their bytes, for the Discussion writer the decision record's bytes.
  `cost_per_size_kb` divides the main session's cost by that size.

| File | Content |
| --- | --- |
| `turns.tsv`, `calls.tsv` | per turn and per call rows of the writer sessions (turn audit's `extract-turns.sh`) |
| `sessions.tsv` | per session counts and serial runs (turn audit's `session-metrics.sh`) |
| `prices.tsv` | list-price cost per session, with the group (`list-price.sh`) |
| `sizes.tsv` | task size per run (`task-size.sh`) |
| `writers.tsv` | one row per writer main session, all joined (`writers-table.sh`) |
| `groups.tsv` | group means over the landed runs (`group-means.sh`) |

## Plan writer

| Run | Group | Cards | Plan KB | Turns | Calls per turn | Single-call | Serial turns | Serial share | Mean context | Peak context | Cost | Cost with forks |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `creel-hook-post` | before | 18 | 81 | 89 | 1.62 | 69% | 39 | 44% | 203k | 336k | $7.47 | $15.72 |
| `dobby-and-adopt-hardening` | before | 15 | 74 | 101 | 1.40 | 79% | 33 | 33% | 210k | 327k | $7.95 | $15.94 |
| `landing-settles-in-place` | before | 4 | 27 | 79 | 1.15 | 73% | 30 | 38% | 189k | 287k | $5.09 | $8.55 |
| `review-setup-tuning` | before | 5 | 22 | 66 | 1.12 | 76% | 31 | 47% | 152k | 235k | $3.87 | $6.44 |
| `validator-checks` | before | 9 | 61 | 89 | 1.18 | 78% | 38 | 43% | 204k | 318k | $7.37 | $13.38 |
| `creel-agents` | after | 16 | 66 | 101 | 1.23 | 76% | 32 | 32% | 280k | 424k | $10.61 | $18.53 |
| `run-verbs-shared` | after | 20 | 106 | 134 | 1.04 | 90% | 74 | 55% | 388k | 576k | $16.78 | $29.13 |
| `interactive-recipe` (running) | after | 12 | 68 | 131 | 1.35 | 76% | 68 | 52% | 278k | 466k | $11.26 | $17.71 |

Group means over the landed runs:

| Group | n | Turns | Calls per turn | Single-call | Serial turns | Serial share | Mean context | Peak context | Cost | Cost with forks | Plan KB |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| before | 5 | 84.8 | 1.29 | 75% | 34.2 | 41% | 192k | 301k | $6.35 | $12.01 | 53 |
| after | 2 | 117.5 | 1.14 | 83% | 53.0 | 44% | 334k | 500k | $13.70 | $23.83 | 86 |

- Calls per turn did not rise: 1.29 before against 1.14 after, and the single-call share rose from 75% to 83%.
  Batching would have lowered the single-call share and raised calls per turn.
- The serial share is 41% before and 44% after; the 33 of 101 turns the turn audit found in `dobby-and-adopt-hardening` is a 33% share, and the other four before runs sit at 38% to 47%.
  Only `creel-agents` (32%) is within or below the before range's low end; `run-verbs-shared` (55%) and the unlanded `interactive-recipe` (52%) are above its top.
- Context and cost are higher after: mean context 334k against 192k, cost $13.70 against $6.35.
  The tasks are larger too (86 KB of plan against 53 KB), so this part of the gap is not an effect of the sentences.
  Per KB of plan the cost is $0.16 after against $0.14 before.

## Discussion writer

| Run | Group | Record KB | Turns | Calls per turn | Single-call | Serial turns | Serial share | Mean context | Peak context | Cost | Cost with forks |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `creel-hook-post` | before | 31 | 53 | 1.21 | 68% | 10 | 19% | 128k | 217k | $3.80 | $5.75 |
| `dobby-and-adopt-hardening` | before | 36 | 51 | 1.06 | 76% | 23 | 45% | 90k | 155k | $2.43 | $4.30 |
| `landing-settles-in-place` | before | 17 | 48 | 1.17 | 69% | 20 | 42% | 124k | 194k | $2.99 | $4.90 |
| `review-setup-tuning` | before | 16 | 47 | 1.19 | 66% | 16 | 34% | 103k | 168k | $2.77 | $4.33 |
| `validator-checks` | before | 26 | 57 | 1.16 | 72% | 25 | 44% | 114k | 204k | $3.70 | $5.72 |
| `creel-agents` | after | 28 | 43 | 1.21 | 63% | 11 | 26% | 117k | 200k | $3.14 | $4.58 |
| `run-verbs-shared` | after | 47 | 57 | 1.14 | 75% | 21 | 37% | 113k | 196k | $3.51 | $4.94 |
| `interactive-recipe` (running) | after | 25 | 69 | 1.14 | 78% | 34 | 49% | 123k | 232k | $4.43 | $6.42 |

| Group | n | Turns | Calls per turn | Single-call | Serial turns | Serial share | Mean context | Peak context | Cost | Cost with forks | Record KB |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| before | 5 | 51.2 | 1.16 | 70% | 18.8 | 37% | 112k | 188k | $3.14 | $5.00 | 25 |
| after | 2 | 50.0 | 1.17 | 69% | 16.0 | 32% | 115k | 198k | $3.33 | $4.76 | 38 |

- Calls per turn (1.16 and 1.17) and the single-call share (70% and 69%) are the same in both groups.
- The serial share is 5 points lower after (32% against 37%), but the spread inside the before group (19% to 45%) is larger than that difference, and `creel-hook-post` alone at 19% is below both landed after runs (26% and 37%).
- Context and cost are level, at a larger task: the decision record is 38 KB after against 25 KB before, so the writer did not grow more expensive per KB (about $0.09 against $0.14).
  The Discussion writer is the one place the cost per KB fell, and with n = 2 it is not distinguishable from task variation.

## How far the groups compare

- **Two landed runs after.** A mean of two sessions has no spread worth quoting; the table shows the per-run rows instead.
  Counting the unlanded `interactive-recipe` makes three, and the three Plan writers disagree with each other (serial share 32%, 52%, 55%).
- **Task size differs.** The after runs are larger: the Plan writer's cards are 18 against 10 on average and 86 KB against 53 KB.
  `run-verbs-shared` (20 cards, 106 KB) is larger than every before run, so it has no counterpart.
  The closest size-matched Plan comparison is `creel-agents` (16 cards, 66 KB) against `creel-hook-post`, `dobby-and-adopt-hardening` and `validator-checks` (9 to 18 cards, 61 to 81 KB):
  its calls per turn (1.23 against 1.18 to 1.62) and serial share (32% against 33% to 44%) are not distinguishable from them, and its mean context (280k against 203k to 210k) is higher.
- **Other changes in the window.** Between the two groups the Plan and Discussion stencils also gained the gate-check sentences of `26f0d9dc` (14:52 on 2026-10-28, before the batching commit but after every before session), and `161a4a7f` changed how creel mail reaches an agent.
  Longer prompts and checks the gate now runs can raise context and turns without bearing on batching.
  The difference in cost and context between the groups cannot be assigned to the sentences or to these.
- **Model and effort.** Every writer session ran `claude-opus-5-5`; no other model is in the sample.
- **Forks.** The fork sub-agents are summed per writer in `fork_turns` and `fork_cost_usd` of `writers.tsv`.
  Their turn mix was not compared.
- **Single readings.** Each session is one sample of a stochastic writer.
  The before runs alone range from 66 to 101 Plan turns (task sizes aside), so a shift smaller than that range is not detectable at this n.

## What this decides

- The sentences did not remove the serial reads: the Plan writer still spends about 4 turns in 10 on single-call reads and searches, and `run-verbs-shared` spent 55%.
  The idea behind notes `plan-writer-context` (sub-planners per part for large bundles) and `loom-beam-darns` (splitting a large task so each writer's context stays small) keeps its full value: the largest task here produced the largest context, 576k at peak.
- To measure a smaller effect, collect more landed runs: add each new run to `slugs.txt` and `runs.tsv` and rerun `run-all.sh`.
  Task size has to be compared run by run until both groups cover the same size range.
