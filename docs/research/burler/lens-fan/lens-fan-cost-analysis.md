# Burler lens-fan cost analysis (webster-replan-state)

Measurement report from the `fan-usage-report` analysis (2026-10-10); the follow-up lab runs are `lens-fan-lab-phase0.md` and `lens-fan-lab-phase1.md`, and the resulting proposal is board note `fan-master-preread`.

Sources: `_lyx/reviews/{discussion,plan}/` in the webster-replan-state pair, its reviewer transcripts under `~/.claude/projects/-home-knatte-Code-loomyard-LYXHUB-webster-replan-state/` (D1 `5107a622`, D2 `084ea0eb`, D3 `f8672267`, D4 `ff2a2f55`, Plan-1 `2ad64800`) and their `subagents/`.
"Fresh" is input + cache-write + output, as in `claudeengine.transcriptTokens`; my per-fork sums, which drop the replayed spawn message, match the usage YAML exactly.

## 1. Master before the first fork

| Round | Master msgs | Fresh (cw+out) | Cache read | Context at fork | Tool results read | What it read |
|---|---|---|---|---|---|---|
| D1 | 13 | 82k (76.1k + 6.1k) | 641k | 84k | ~35k tok | prompt, 2 instructions, focus, decision-record, support-log, discussion stencil, 3 skills, **13 Bash calls over 19 code/pattern files** |
| D2 | 6 | 32k (31.2k + 1.0k) | 142k | 39k | ~19k tok | prompt, 2 instructions, focus, decision-record, support-log, discussion stencil, 3 skills; **no code** |
| D3 | 6 | 50k (48.6k + 1.1k) | 142k | 57k | ~21k tok | same as D2; **no code** |
| D4 | 6 | 33k (31.9k + 1.4k) | 143k | 40k | ~23k tok | same as D2, plus 2 pattern files; **no code** |
| P1 | 9 | 67k (65.7k + 1.5k) | 321k | 74k | ~39k tok | decision-record, plan files via 3 Bash cats; **no code** |

After forking, the master reads code itself: 12, 25, 27, 29 and 17 tool calls (D1-D4, P1), mostly greps and `sed -n` over the same files the forks are reading.
In D2-D4 the master's fork prompts say "Verify the Decisions' factual premises against the actual code in the worktree" and "by reading the real code the Decision modifies", so every fork has to do the code reading the master skipped.

## 2. Forks (sums over 5 forks; fresh = cw + out)

| Round | Turns per fork | Tool calls | Fresh | of which output | Cache read | Distinct files touched | Files read by 2+ forks | Overlap with master's pre-fork reads |
|---|---|---|---|---|---|---|---|---|
| D1 | 7/13/3/2/7 = 32 | 38 | 152k | 49k | 3.3M | 19 | 9 | 4 of 19 |
| D2 | 29/21/33/27/15 = 125 | 140 | 485k | 109k | 12.2M | 57 | 25 | 0 (master read none) |
| D3 | 29/29/37/11/8 = 114 | 132 | 435k | 116k | 12.1M | 73 | 26 | 0 |
| D4 | 20/7/42/10/2 = 81 | 96 | 462k | 145k | 9.7M | 42 | 15 | 0 |
| P1 | 55/20/63/22/13 = 173 | 183 | 697k | 191k | 27.1M | 98 | 37 | 0 |

Turn order above is generic A / generic B / correctness / error-handling / test-gaps.
Inheritance itself works: each fork's first own turn reads the full parent prefix (39-84k) from cache with only ~1-3k written.
Cost then scales with turns: about 3.5-4k fresh per fork turn (cache write of the new tool result plus thinking/output) and about 100k cache read per turn, since each turn re-reads a 75-260k context.
Correctness is the heaviest fork every round (33-63 turns); test-gaps the lightest.
In D2-D4, 32-46% of fork file references repeat a file another fork also read (105/107/66 references over 57/73/42 files): the five forks each build the same code picture in parallel.
Output (Sonnet high thinking) is 23-31% of fork fresh.

## 3. Fanned vs solo

| Segment | Mode | Rounds | Fresh per round | Cache read per round | Wall (s) | Findings B/M/L per round |
|---|---|---|---|---|---|---|
| Discussion | fan, sonnet[high] (this pair) | 4 | 301k / 619k / 599k / 638k | 5.4M-14.7M | 296 / 340 / 439 / 603 | 3/6/5, 1/9/9, 1/9/6, 1/1/9 |
| Discussion | solo opus[medium] (driver-context, test-suite-weight) | 6 | 86k-121k | 0.7M-2.0M | 167-310 | 1/6/3, 2/1/4, 2/3/1, 2/6/5, 1/3/2, 2/1/0 |
| Plan | fan (this pair, round 1) | 1 | 901k (master 204k + forks 697k) | 29.8M | ~644 | 2/12/8 |
| Plan | solo opus[medium] round 1 | 2 | 202k-227k | 5.0M-5.7M | 410-577 | 6/4/8, 4/4/1 |

The master alone (134-176k fresh per discussion round) costs more than a whole solo Opus round.
Fork findings: about 43/43/36/38/54 raw (D1-D4, P1; counted from the forks' final messages by regex, so approximate), against 14/18/14/11/21 kept as `lens:*` and 5/4/4/6/5 in `## Rejected`.
So roughly half of all raw fork findings were cross-lens duplicates merged away, and a further 10-15% were rejected by the master.
Kept findings were 82-92% lens-origin and 1-4 handler-origin per round.
The fan raised more MEDIUM/LOW items than solo but not more BLOCKING ones, and D3/D4 still circled and escalated.

## 4. Diagnosis

Primary cause: the master pre-reads too little.
In D2-D4 and P1 it forked at 39-74k context with zero code read, then told each fork to verify premises against code, so five forks did that reading independently (57-98 distinct files, 15-37 of them twice or more).
D1 is the control: the master read 19 code files (~35k tokens) before forking, and forks needed 32 turns and 152k fresh in total, a third of D2-D4.
The stencil text that allows this is `clusterRulesBlock` in `internal/burlerengine/prompt.go` (lines 314-324):
- "after exploring the target fully, spawn ALL of the fork reviewers" never says exploring includes the code the target cites; for a discussion, the target is two markdown files, so the master reads those and forks.
- "prefer your inherited context and fetch only what your lens needs" is advice with no budget, and the master's own fork prompts override it with "verify against the actual code".
- "While the forks run, YOU (the handler) do your own HOLISTIC review" moves the master's code reading to after the fork point, where no fork inherits it.
Contributing: `tool-use: true` on Discussion-Review (`contracts/recipes/loom-recipe.yaml`) renders "Drive the real substrate: build, run, test what you review" (`toolUseRules`, prompt.go:263), and the `standard` fan's code lenses (correctness, error-handling, test-gaps, `internal/burlerengine/template.yaml`) aim forks at code on a design artifact; the master even rewrote them as "applied to a design artifact: ... reading the real code".
Not the cause: cache inheritance (it hits) or the fork count by itself (D1 ran five forks cheaply).

## 5. Changes and estimated saving

1. Make the pre-fork read mandatory and the post-fork read a gap-filler.
   In `clusterRulesBlock`: "Before spawning, read every file, symbol and pattern the target cites or that you will ask a fork to verify; forks inherit it.
   Fork prompts never ask a fork to verify against code you have not read.
   Each fork makes at most N tool calls (e.g. 6), only for a lens-specific read its inherited context lacks."
   Move "your own HOLISTIC review" to before the spawn, leaving consolidation for after.
   Estimate: D1 shape (master ~80k pre-fork, forks ~150k) instead of D2-D4 (forks ~460k): about 300k fresh and 8-9M cache reads saved per round, round total ~600k to ~300k.
2. Fit the fan to the segment: use the existing `discussion` and `plan` fans (generic, goal-scope, pattern-fit, scenarios, leanness / decision-coverage, ...) instead of `standard`, and drop the second generic fork, since about half of fork findings were duplicates.
   Estimate: 4 forks at D1-level cost is ~120k fork fresh; with change 1, a round lands near 200-250k, about 2x solo Opus rather than 5-6x.
   Running forks at Sonnet medium would cut a further ~25% of fork fresh (their thinking/output share).
