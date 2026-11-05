# Plan and Discussion writers: exploration and lookup subagents, 2026-11-04

How the Plan-Write and Discussion-Write sessions of recently landed tapestry runs explore the code, and whether they hand lookups to subagents as their stencils suggest.
Every figure derives from the scripts and tables beside this file; the sample size stands beside each number.
No agent run was made: the work reads transcripts of finished sessions.

## Finding

The sample is 3 tapestry runs, not the 10 the measurement asked for: only 4 tapestry runs landed after 2026-11-02, and one of them, `cold-gate-fixer`, ran no Plan or Discussion writer.
The 7 writer sessions of the 3 runs follow the guidance where it matters least and break it where it costs most.

- The Plan writers follow it: 3 of 3 handed one batch of 3 or 4 `sonnet` lookup subagents to the Agent tool in a single message.
  The Discussion writers do not: 1 of 3 full sessions did.
- Exploration is 56% of the writers' turns (119 of 212) and 46% of their cost ($11.03 of $23.76).
  Only $3.61 of it (15%) is paid above 200k context, and $3.38 of that is in Plan sessions.
- Every one of the 6 chains the writers ran above 200k is in a Plan session, after that session's one lookup batch.
  3 of them, 14 turns, are single-call serial lookups.
  Batching them in the writer would save more than moving them to subagents would.
- Taking every lookup that holds a search call to subagents at the observed subagent price would save about $4.7, one fifth of writer spend, as an upper bound.
- The report recommends neither a bench nor a stronger stencil now; see Recommendation.

## Sample and method

- Runs: the tapestry runs that landed after 2026-11-02, as `go run ./tools/tokencount -since 2026-11-02` lists them: `batten-remote-order`, `decision-record-on-board`, `shed-sub-run-block`, and `cold-gate-fixer`, which has no writer session.
  The slugs are in `slugs.txt`.
- Sessions: the main sessions titled `plan` and `discussion`, 3 and 4 of them.
  `decision-record-on-board` holds two Discussion sessions, because its first one ended after 6 turns and a second started a few minutes later; both count.
- Guidance in force: every session started after the commit that added the Discussion writer's guidance (`2d319fe5`, 2026-11-02 14:02), and the text "hand a batch of independent lookups" is in all 7 transcripts.
- A turn is one assistant message id, as in the turn audit.
  An exploration turn is a turn whose every call is a lookup: the Read tool, Grep, Glob, or a Bash call the turn audit classes `read`, `search`, `git-read` or `lym-read`.
  A chain is two or more exploration turns in a row.
  A search turn is an exploration turn with at least one call other than the Read tool; the rest only read files.
- Cost: list price of the turn's tokens at `claude-opus-5-5`, the model of every writer session, from `internal/shuttleengine/claudeengine/pricing.go`.
  The Plan sessions priced this way sum to $14.48, the Plan writer figure `tokencount` gives for the same runs.
- Subagents: each Agent call of the writer, matched to its transcript through the sub-agent's meta file.
  A `general-purpose` subagent is a lookup; a `fork` is not, and is counted apart.
- Carry: the context an exploration turn adds, priced as one cache write and one cache read in each later turn of the session.
  It is the cost of keeping what a lookup returned, and is part of the session's cost, not on top of it.

## Do the writers follow the guidance

| session | role | turns | exploration turns | lookup subagents | batches | cost |
| --- | --- | --- | --- | --- | --- | --- |
| `batten-remote-order` | Plan | 46 | 22 | 4 | 1 | $5.63 |
| `decision-record-on-board` | Plan | 32 | 16 | 3 | 1 | $4.15 |
| `shed-sub-run-block` | Plan | 37 | 19 | 4 | 1 | $4.70 |
| `batten-remote-order` | Discussion | 32 | 19 | 3 | 1 | $2.79 |
| `decision-record-on-board` | Discussion | 33 | 22 | 0 | 0 | $3.33 |
| `shed-sub-run-block` | Discussion | 26 | 17 | 0 | 0 | $2.80 |
| `decision-record-on-board`, ended early | Discussion | 6 | 4 | 0 | 0 | $0.36 |

The 14 lookup subagents ran on `sonnet`, as `subagent_model` names.
Each batch went out in one message with 3 or 4 subagents.
A subagent held about 5 questions by a count of its prompt's numbered and bulleted lines (74 over 14), took 7 turns and cost $0.24; together they cost $3.41.
The writers' 36 forks, 18 each, are lens reviewers and the plan-writing fork, not lookups, and cost $22.01.

Every delegating session delegated once, early, and went on exploring in its own context afterwards.
The Plan writers' first chain, before the batch, runs 7 to 9 turns and 28 to 34 calls, so those calls average 3 to 4 a turn.
The Discussion writers that did not delegate ran chains of 20 and 15 turns, at 26k to 191k and 26k to 163k context, with 28 and 34 calls.

## Where the exploration cost falls

| writer | sessions | cost | exploration | exploration above 200k | search turns | search turns above 200k | carry |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Plan | 3 | $14.48 | $6.15 | $3.38 | $4.08 | $2.14 | $7.63 |
| Discussion | 4 | $9.28 | $4.88 | $0.23 | $4.11 | $0.17 | $3.62 |
| both | 7 | $23.76 | $11.03 | $3.61 | $8.19 | $2.31 | $11.25 |

The Plan sessions pass 200k early and stay there: 85 of their 115 turns are above it, the largest context is 380k, and the sessions' turns above 200k cost $11.00 of $14.48.
Most of that is not exploration.
The Discussion sessions peak at 249k and have 14 of 97 turns above 200k.
Their exploration is cheap per turn and expensive in carry: one session took in 170k tokens of lookup results and kept them for every turn after.

By context of the turn, all 119 exploration turns:

| context | turns | calls | turns the turn audit marks as dependent on the previous result | cost |
| --- | --- | --- | --- | --- |
| under 100k | 38 | 99 | 10 | $2.42 |
| 100k to 200k | 43 | 80 | 3 | $4.99 |
| above 200k | 38 | 88 | 2 | $3.61 |

The dependency mark is the turn audit's heuristic: a path in the call that first appeared in the previous turn's result.
It marks 2 of the 38 turns above 200k, so most of them name nothing the previous turn found; read with that caution, the calls could have gone out a turn earlier.

## What could have moved to subagents up front

The late chains, the ones the entry's example is about, are the six chains whose first turn is above 200k.

| chain | session | turns | calls | context | cost |
| --- | --- | --- | --- | --- | --- |
| 1 | `batten-remote-order` Plan | 5 | 5 | 200k to 213k | $0.35 |
| 2 | `batten-remote-order` Plan | 6 | 6 | 232k to 259k | $0.49 |
| 3 | `decision-record-on-board` Plan | 5 | 10 | 226k to 252k | $0.55 |
| 4 | `shed-sub-run-block` Plan | 4 | 12 | 216k to 260k | $0.52 |
| 5 | `shed-sub-run-block` Plan | 2 | 5 | 270k to 281k | $0.21 |
| 6 | `shed-sub-run-block` Plan | 3 | 3 | 336k to 340k | $0.27 |

Together: 25 turns, 41 calls, $2.38.
Chains 1, 2 and 6 are single-call turns in a row, greps and `sed -n` slices of Go files already named; chains 3 to 5 mix batched reads of 2 to 6 files a turn with greps.

Two ways to cheapen these, priced on the sample:

- One delegating turn per chain at the chain's first context ($0.55 over the six) plus one lookup subagent per chain at the observed $0.24 costs about $2.01, saving about $0.37.
- One batched turn per chain at its first context costs $0.55, saving about $1.83; it holds only where the later calls do not need an earlier result.

Short late chains gain little from a subagent, since the subagent's own cost nearly equals the chain's; batching gains more.
`batching-hook-reach` (T2) extends the batching nudge to Bash lookups, which is the route to the second figure.

Across all search turns the upper bound is larger.
The 80 search turns cost $8.19; the lookup subagents averaged $0.034 a turn, so the same turns in subagents would cost $2.75, and the 14 chains' delegating turns at their first context about $0.73.
That leaves about $4.7, 20% of writer spend, before counting the smaller carry a short answer leaves behind, and before the lookups the writer has to read whole.
The estimate is an upper bound, since a writer keeps the code it needs to see, as the stencil says.

## Recommendation

- No bench of the guidance.
  The Plan writers already follow it, so the "with" arm exists in every landed run.
  A one-input bench gets one session per arm, and the three Plan sessions here vary from $4.15 to $5.63, a spread larger than the $0.4 to $1.8 the late chains offer.
- No stronger guidance for the Plan stencil.
  What remains in Plan sessions is short serial chains after the batch, a batching question that `batching-hook-reach` addresses by letting the hook count Bash lookups.
  Re-measure after it lands, with the scripts here and the batching hook's own re-measure note.
- One candidate for later: the Discussion stencil.
  2 of its 3 full sessions did not delegate and ran 15 and 20 serial turns, costing $1.74 and $1.13 in search turns.
  That is $2.87 over two sessions, and subagents would save a part of it, so it is a small candidate; decide it on a sample of 10 or more tapestry runs.
- Re-run `run-all.sh` once more tapestry runs have landed: add the slugs to `slugs.txt` and the tables rebuild.

## Limits

- 3 runs and 7 sessions, not 10 runs; every rate here is a case count, not a rate.
- Pricing is list price of the tokens; the model of every writer session was `claude-opus-5-5` and none wrote a one-hour cache entry, so the price table is exact for them.
- The classification of a Bash call as a lookup is the turn audit's heuristic, applied unchanged.
- The question count of a subagent's prompt counts numbered and bulleted lines; two prompts of `shed-sub-run-block` hold none by that count and ask in prose.
- The counterfactual prices assume the writer keeps the code it needs to see, and treat a subagent as costing what the observed ones cost; a subagent given a larger lookup would cost more.
- The sample has no writer without the guidance, so a difference the guidance makes is not measured here, only whether it is followed.
