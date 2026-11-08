# What the darn writer spends on, measured 2026-11-08

`docs/research/loom/darn-cost-2026-11-08/report.md` found the darn writer and its lookup subagents are 58% of a darn.
This report breaks that cost down by token kind and by activity, and tests the suspicion that gate waits outlast the prompt cache.
Every figure derives from the scripts and tables beside this file.
No agent run, test or gate run was made: the work reads session transcripts only.
Cost is API list price of the tokens in the transcripts.

## Sample

- 295 darn runs landed from 2026-10-25 to the start of this run; 294 have a writer transcript.
  The first writer session started 2026-10-27, the last 2026-11-08.
- 300 writer sessions (four runs have more than one), 244 lookup-subagent sessions and 98 cold gate fixer sessions.
- 17,416 assistant turns: 13,725 writer, 2,375 lookup subagent, 1,316 gate fixer.
- The writer runs `opus[medium]` in 234 runs, `sonnet[medium]` in 58 and `fable[high]` in 2.
- Two runs dominate the tail: `comment-cleanup` ($33.63, 498 turns) and `repo-size-run-time` ($25.66, on Fable).
  The writer's median is $2.47 a run, its mean $3.11, its 90th percentile $5.72.
- The sample splits at 2026-11-04, when the writer's own gate calls drop from about two a run to under one:
  172 runs before, 122 from that day on.
  Figures marked "since 11-04" describe the second group, which is closer to what the writer does today.
  The cause of the drop is not read from the transcripts.

## Finding

- **Research is the largest cost.**
  The writer's turns before its first edit are $387.63, 42.4% of the writer and $1.32 a darn, at a mean of 22.5 turns a session.
  Writing is 35.8% ($1.11 a darn).
  Since 11-04 the split is research 41.7% and writing 40.5%.
  By token kind the writer is 40.3% cache reads, 33.6% cache writes and 26.1% output.
  Research is mostly cache writes: $183.96 of its $387.63 is reading files into a growing context.
- **The suspected cause is false for plain gate runs.**
  The writer ran `lym gate` 483 times; the turn after paid a cache rewrite 5 times, $7.81, 0.9% of the writer.
  All five were `--wait` calls that ran past five minutes.
  The 84 capped calls (79 from the gate fixer, 5 from the writer) all ended at 250 to 255 seconds, and none was followed by a rewrite.
  The keep-warm guard holds, including inside the cold gate fixer.
- **The suspected cause is true for the wait on the cold gate fixer.**
  The writer idles while the verify gate and the cold fixer run, and is woken only when the fixer leaves findings.
  That happened 33 times: the writer had been idle 582 seconds on average (median 505, longest 2,056), 29 times for five minutes or more,
  and 29 of the 33 next turns rewrote the whole context, $30.12.
  Those 33 rounds cost the writer $42.31, so rewrites are 71% of what a findings round costs.
  The 51 rounds with no fixer before them cost $17.63, $0.35 a round, and one rewrote.
- **All rewrites together are 6.0% of all cost** ($60.34 of $1,003.42): $0.21 a darn.
- **The cache lifetime is five minutes.**
  Every cache write in the transcripts is a 5-minute write; there are no 1-hour writes.
  Of 197 turns after a 2 to 4 minute gap and 113 after 4 to 5 minutes, none rewrote the cache.
  Of 9 turns after 5 to 5.5 minutes, 4 did; of 52 turns after 5.5 minutes or more, 51 did.
- **Thinking is about 58% of the writer's output**, $137.72 of $238.40, 15% of the writer's cost.
  This is an estimate: the transcripts show that a turn thought, not how much.
  The estimate is the output of turns with a thinking block above the mean output of turns with the same call count and no block.
- **The writer never ran at another effort.**
  Every Opus and Sonnet session is `medium`, so effort does not show in the cost.
  The two Fable sessions ran `high`, at $20.28 on average over 29,183 changed lines, a different job from the rest.
- **A Sonnet writer is cheaper on the same size.**
  For runs of 100 to 1,500 changed lines, Opus costs $3.25 over 179 runs and Sonnet $1.83 over 35, at 46 turns each.
  Sonnet needed no re-prompt in 58 runs; Opus needed one in 64 of 234.
  The Sonnet runs were not picked by size or kind, so the reason is unmeasured.

## Cost by token kind

| part | turns | fresh input | cache writes | cache reads | output | total |
| --- | --- | --- | --- | --- | --- | --- |
| writer | 13725 | $0.12 | $307.54 | $368.23 | $238.40 | $914.28 |
| lookup subagents | 2375 | $0.00 | $27.70 | $17.81 | $19.10 | $64.61 |
| gate fixer | 1316 | $0.00 | $10.42 | $9.96 | $4.15 | $24.53 |
| all | 17416 | $0.13 | $345.66 | $395.99 | $261.64 | $1,003.42 |
| share of all | | 0.0% | 34.4% | 39.5% | 26.1% | 100% |

The writer is 91.1% of all cost, $3.11 a darn; the subagents are $0.22 and the gate fixer $0.08.
Only 25 of the 294 runs spawned a lookup subagent.

## Cost by activity

An activity is a writer turn's place in its session.
Research is a turn before the first Edit or Write.
Gate is a turn from the first edit on that ran `lym gate` or `go test`.
Post-gate is a turn after the first such turn, split into one with an edit and one without.
Re-prompt is a turn after a "Gate findings recorded at" prompt.
Write is every other turn.

| activity | turns | cost | share of the writer | cost a darn |
| --- | --- | --- | --- | --- |
| research | 6752 | $387.63 | 42.4% | $1.32 |
| write | 4283 | $327.10 | 35.8% | $1.11 |
| gate | 336 | $17.53 | 1.9% | $0.06 |
| post-gate, edit | 301 | $31.36 | 3.4% | $0.11 |
| post-gate, no edit | 1510 | $90.72 | 9.9% | $0.31 |
| re-prompt | 543 | $59.94 | 6.6% | $0.20 |
| writer | 13725 | $914.28 | 100% | $3.11 |
| lookup subagents | 2375 | $64.61 | | $0.22 |
| gate fixer | 1316 | $24.53 | | $0.08 |

Of the $90.72 of post-gate turns without an edit, $79.07 came before the writer's last gate or fix turn and $11.65 after it;
the turns after it are the commit and the change description, which the writer owes in any design.

| activity | cache writes | cache reads | output |
| --- | --- | --- | --- |
| research | $183.96 | $113.36 | $90.25 |
| write | $58.17 | $152.54 | $116.35 |
| gate | $2.36 | $13.04 | $2.13 |
| post-gate, edit | $8.38 | $11.99 | $10.99 |
| post-gate, no edit | $19.51 | $56.69 | $14.50 |
| re-prompt | $35.16 | $20.60 | $4.18 |

Since 11-04 (122 runs, writer $387.79, $3.18 a darn): research $161.70, write $157.22, gate $1.16, post-gate edit $15.71,
post-gate no edit $28.47, re-prompt $23.53.

## Gate waits

Gate calls by caller; "capped" means the call answered `done: false` at the keep-warm cap.

| caller | with `--wait` | calls | mean s | longest s | over 5 min | capped | failed | next turn rewrote | extra cost |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| writer | no | 246 | 42 | 250 | 0 | 5 | 11 | 0 | $0.00 |
| writer | yes | 237 | 55 | 526 | 5 | 0 | 4 | 5 | $7.81 |
| gate fixer | no | 225 | 172 | 255 | 0 | 79 | 118 | 0 | $0.00 |
| gate fixer | yes | 5 | 295 | 1381 | 1 | 0 | 0 | 1 | $0.14 |

The writer's own gate calls fell over the sample: 3.7 calls a run for runs started 10-27 to 10-31, 2.0 for 11-01 to 11-03, 0.1 for 11-04 to 11-05 and 0.6 since.

## Gate failures and the cold gate fixer

The first fixer session started 2026-11-02T19:23Z.
Runs whose writer started after it ran with a fixer available.

| writer started | runs | runs with a fixer session | fixer sessions | cleared the gate | left findings | prompts to the writer | writer cost in re-prompt rounds | fixer cost |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| before the first fixer session | 111 | 1 | 1 | 1 | 0 | 51 | $17.63 | $0.15 |
| from the first fixer session on | 183 | 92 | 97 | 64 | 33 | 33 | $42.31 | $24.37 |

- 84 verify-gate findings reached a writer, in 64 of 294 runs (22%): 54 runs had one, 7 had two, the rest three to seven.
- Since the fixer existed, half the runs (92 of 183) needed a fixer session.
  A session that cleared the gate averaged $0.22, 443 s and 12.8 turns; one that left findings $0.30, 481 s and 14.8 turns.
- A finding the writer fixed itself with a warm cache cost $0.35 of writer; one the writer fixed after a fixer cost $1.28 of writer, plus the fixer's $0.30.
- Writer turns on gate fixes in all: 301 post-gate edit turns, 543 re-prompt turns.

### Where the waits cost most

The writer's wait at a Gate-findings prompt:

| prompt | prompts | mean writer idle s | idle 5 min or more | next turn rewrote | extra cost | writer turns in the round | writer cost in the round |
| --- | --- | --- | --- | --- | --- | --- | --- |
| a cold fixer ended just before it | 33 | 582 | 29 | 29 | $30.12 | 195 | $42.31 |
| no fixer before it | 51 | 46 | 1 | 1 | $0.81 | 348 | $17.63 |

Median fixer run before such a prompt: 409 s.

The ten runs with the costliest rewrite after a fixer:

| run | writer | fixer wait rewrite | re-prompt round | fixer |
| --- | --- | --- | --- | --- |
| `fabric-remote-async` | $13.47 | $1.99 | $3.54 | $0.35 |
| `codex-assist-seat` | $10.11 | $1.92 | $2.38 | $0.29 |
| `codex-engine-core` | $11.52 | $1.85 | $2.82 | $0.37 |
| `agent-skills-and-effort` | $8.73 | $1.45 | $2.04 | $1.50 |
| `locks-out-of-tracked-trees` | $8.28 | $1.41 | $1.90 | $0.59 |
| `webster-subpath-anchor` | $6.94 | $1.31 | $1.77 | $0.17 |
| `test-real-time-waits` | $6.35 | $1.30 | $1.51 | $0.35 |
| `motif-board-fixes` | $6.70 | $1.28 | $1.83 | $0.38 |
| `shuttle-launch-delivery` | $6.41 | $1.26 | $1.76 | $0.83 |
| `teardown-gone-pair` | $4.75 | $1.14 | $1.32 | $0.20 |

## Model and effort

| model and effort | runs | mean writer cost | median | mean turns | cost a turn | runs with a re-prompt | mean subagents |
| --- | --- | --- | --- | --- | --- | --- | --- |
| opus medium | 234 | $3.25 | $2.69 | 46 | $0.071 | 64 | $0.16 |
| sonnet medium | 58 | $1.95 | $1.59 | 49 | $0.040 | 0 | $0.26 |
| fable high | 2 | $20.28 | $20.28 | 52 | $0.386 | 0 | $5.84 |

Both models ran in both halves of the sample: Opus averaged $3.29 up to 11-03 and $3.20 since, Sonnet $1.80 and $2.15.
Lookup subagents ran Sonnet medium (1,089 turns, $49.01), Haiku (1,137 turns, $5.02) and Opus medium (149 turns, $10.57).

## What a writer that ends at a finished change would have saved

The design: the writer stops after it finishes the change and writes the description, runs no gate, and leaves every verify-gate failure to a cold Sonnet fixer.
The estimate takes the writer cost the transcripts show was spent on gates and their fixes, and subtracts the fixers that would do that work.
A fixer round costs the sample's mean, $0.25 ($0.22 when it clears, $0.30 when it leaves findings); a finding takes 1.5 rounds, since a third of sessions leave findings.

| basis | writer cost removed | fixer cost added | saved over the sample | a darn |
| --- | --- | --- | --- | --- |
| re-prompt rounds only | $59.94 | $31.50 (84 findings, 1.5 rounds, $0.25) | $28 | $0.10 |
| and the writer's own gate calls and the edits after them | $108.83 | $31.50 | $77 | $0.26 |
| and the turns that read the gate's output, before the last gate or fix turn | $187.90 | $31.50 | $156 | $0.53 |

The second and third rows include `go test` runs a writer makes in the course of writing, which a writer ending at a finished change still makes, so they are ceilings.
Since 11-04 the writer runs few gates: re-prompt rounds are $23.53 over 21 findings, and a cold fixer would replace them for $7.88, saving $15.65, $0.13 a darn.
Of the $59.94 the re-prompt rounds cost, $30.12 is the rewrite after a fixer wait; no fixer round the writer does not see pays it.

## Levers, in order of what they would save

1. **A Sonnet writer on the runs the Opus writer does now: about $280 to $330 over 234 Opus runs, $1.20 to $1.40 a darn.**
   The price of the same tokens at Sonnet list price is 63% of Opus (a saving of 37%, $281); the observed gap on same-size runs is $1.42 a run ($332).
   Quality is not measured here: Sonnet needed no re-prompt in its 58 runs, but nothing in the sample compares review outcomes.
2. **Lower thinking effort or a cap on the writer: up to $138 over the sample if thinking were free, $0.47 a darn, and less in practice.**
   The sample has no run at another effort, so it gives the size of the lever and no saving.
3. **A writer that ends at a finished change: $28 to $156, with $77 a likelier figure, $0.10 to $0.53 a darn.**
   Since 11-04 it is $15.65 over 122 runs.
4. **Keeping the writer's cache warm while the verify gate and the cold fixer run: about $24 of the $30.12.**
   A keep-warm turn every four minutes over a median 505 s wait is two cache reads of the context, about $0.21 against the $1.04 average rewrite.
   This applies only while the design wakes the writer after a fixer; lever 3 removes the wake and with it this saving.
5. **Research, the largest cost at $387.63, has no estimated saving.**
   The transcripts show what research costs and not what a smaller read would cost.
   The first turn of a session costs $0.064 and the 22.5 turns before the first edit $1.29, mostly cache writes of what the writer reads;
   fewer, larger read batches are the lever to try.

## Method

- `extract-turns.sh` reads the Claude Code transcripts of the writer, gate fixer and subagent sessions of the slugs in `slugs.txt` and writes
  `sessions.tsv`, `turns.tsv`, `calls.tsv` and `prompts.tsv`.
- `turn-costs.sh` prices each turn at list price (`internal/shuttleengine/claudeengine/pricing.go`), assigns its activity and marks a cache rewrite:
  the turn read under half its previous context from cache while the previous context was 20,000 tokens or more.
  Its extra cost is the rewritten tokens at the cache-write rate less the cache-read rate.
- `gate-rounds.sh` writes `fixer-sessions.tsv` and `gate-prompts.tsv`.
  A fixer left findings when a "Gate findings recorded at" prompt reached the writer within 600 s after the fixer ended.
- `per-darn.sh` writes `per-darn.tsv`, one row per run; `summary.sh` prints the tables of this report.
- Changed lines come from the optional `cost-table.tsv` of the darn cost measure and are empty for runs it lacks.
- Limits: a run reused by slug counts as one; the transcripts hold the tokens, not the thinking text; the 11-04 split dates a prompt change and not a per-run flag;
  the saving rows are estimates from the same transcripts, not a rerun.
