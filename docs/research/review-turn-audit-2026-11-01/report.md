# Review turn audit, 2026-11-01

Where the turns, context and money of the Discussion and Plan review segment go on three full tapestry runs, which of the turns a merge would remove, and what Sonnet fixers would have cost.
The segment is the burler reviewer and fixer, the bouncer's seed and judge, and the lens forks of the reviewer and fixer, for both Discussion and Plan.
It reuses the method of `docs/research/turn-audit-2026-10-28/report.md`.
Every figure derives from the scripts and TSV tables beside this file; `n` stands beside each figure that rests on few sessions.
A recommendation is not a decision: it changes no stencil, system prompt, profile or config.
No agent or model run beyond the darn writer itself was made: the study reads existing transcripts only.

## Finding

The review rounds' cost is the main reviewer and fixer sessions, not their forks: the main sessions cost $74.27 of the segment's $101.59, the forks $23.62, and the seed and judge sessions $3.70.
A main session spends it by re-reading a context that grows from 19k tokens to a peak of 156k to 355k over 30 to 77 turns on average; cache reads are 27% to 60% of a role's cost.
Three things stand out.

- **The Plan reviewer's serial gathers.**
  The Opus Plan reviewer (round 1) makes 89% of its turns single-call and 33 of 77 turns per session removable by a merge, 41% of its weight (n = 3).
  It already carried the batching sentence of `bf00a043`, as did every reviewer and fixer in the sample.
  The sentence did not move it: 1.05 calls per turn.
- **The fork wake-up turns.**
  A reviewer or fixer that spawns four forks wakes four times, one no-call turn per fork result, each at the full context: 79 such turns in the three roles that fork-and-wait, 8% to 9% of their weight and 13% to 19% of their time.
  A stencil sentence cannot merge them.
- **A Sonnet fixer saves about a third at equal turns.**
  The Discussion and Plan fixers with their forks cost $34.15 on Opus and $22.89 priced as Sonnet with the same tokens: $11.26 less (33%).
  The swap pays only while the Sonnet fixer's turns and context stay under about 1.5 times the Opus fixer's; this sample holds no Sonnet fixer on these two roles, so it cannot say whether they do.

## Sample and method

- Source: the Claude Code transcripts under `~/.claude/projects/` of three runs, `warp-visibility`, `interactive-recipe` and `run-verbs-shared`, main sessions and their fork sub-agents.
  The runs started 2026-10-31 14:10 to 2026-11-01 07:08.
  They are the three latest full tapestry runs; the sample is those three, not the population of landed runs.
- Sample: 143 sessions, 50 main and 93 forks, 1772 turns and 2227 tool calls, of which 20 are bouncer seed and judge sessions.
  Reviewer sessions: 9 Discussion (8 Opus, 1 Sonnet) and 7 Plan (3 Opus, 4 Sonnet).
  Fixer sessions: 7 Discussion and 7 Plan, all Opus.
- A turn is one assistant API message, as in the turn audit; the extractor is its `extract-turns.sh`, run unchanged on a copy beside this folder's `slugs.txt`.
  A fork transcript opens with its parent's spawning message, replayed; the parent already counts it, so turn 1 of every fork is dropped from every table (`run-all.sh` stops if a fork's first turn is anything but that Agent call).
- A role is the part after the last colon of the session's title, with `+fork` for a fork.
  A reviewer or fixer session that ran on Sonnet takes `@sonnet` after its role; every other session ran Opus 5.5, except the bouncer's seed and judge, which ran Sonnet 5.5.
- Round is the order of a run's sessions of one role by start time.
  `run-verbs-shared` has a Discussion reviewer session of 10 turns that started at the same second as the next one and ran no forks; it is kept as round 2.
- Cost is the Claude API list price of each message's model, from the table in `internal/shuttleengine/claudeengine/pricing.go`, with the one-hour cache write priced apart from the five-minute one.
  It is the same table `tools/tokencount` prices with; `tools/tokencount` itself was not run, since `go run` is refused in this session.
  The orch's four figures that prompted this study, read from `tools/tokencount`, come out of this table to the dollar: Plan reviewer $31.25, Discussion reviewer $18.38, Discussion and Plan fixers $24.64, all review and fixer forks $23.62.
  The Plan reviewer's time is 80 minutes here, summing each session from first to last message, against the orch's 1h13m; that figure was not reproduced.
- Weight, call classes, mergeable runs and their strict and class-only counts are the turn audit's definitions.
  Only the activity groups of section 3 are new.
- Time is the seconds from a turn's first message to the next turn's, so it covers the turn's generation and the run time of its tools; a session's last turn has none.

| File | Content |
| --- | --- |
| `session-costs.tsv` | per session: model, wall seconds, turns, context, list-price cost, cost priced as Sonnet (`session-costs.sh`) |
| `turns.tsv`, `calls.tsv` | per turn and per call rows of the segment (the turn audit's `extract-turns.sh`) |
| `sessions.tsv`, `role-summary.tsv` | per session and per role counts, mergeable runs (the turn audit's `session-metrics.sh`, `role-summary.sh`) |
| `role-table.tsv` | per role: turns, context, cost, cache-read share, wall time (`role-table.sh`) |
| `rounds.tsv` | per main session, with its forks' cost (`rounds-table.sh`) |
| `where.tsv` | per role and activity group: calls, share of weight, share of time (`where-turns-go.sh`) |
| `merge-kinds.tsv`, `kinds-total.tsv` | the calls of the removable turns (the turn audit's `merge-kinds.sh`, `kinds-total.sh`) |
| `merge-saving.tsv` | removable weight times cost, per role (`merge-saving.sh`) |
| `fixer-cost.tsv` | fixer cost as run and priced as Sonnet (`fixer-cost.sh`) |

## 1. Turns, context and cost per role

Mean context is the mean over the role's turns of input + cache write + cache read tokens; first context is the mean of each session's first turn.
Cache read is the share of the role's cost.
Wall is the mean minutes per session from first to last message; the forks of one parent run side by side.

| Role | Sessions | Turns per session | First context | Mean context | Peak context | Cost | Per session | Cache read | Wall, min |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `plan-burler-review` | 3 | 77.0 | 19k | 169k | 311k | $15.68 | $5.23 | 49% | 11.4 |
| `plan-burler-review@sonnet` | 4 | 64.2 | 19k | 186k | 355k | $15.57 | $3.89 | 60% | 11.4 |
| `plan-burler-review+fork` | 9 | 6.3 | 204k | 207k | 256k | $4.67 | $0.52 | 50% | 1.6 |
| `discussion-burler-review` | 8 | 36.2 | 19k | 99k | 221k | $16.91 | $2.11 | 33% | 6.2 |
| `discussion-burler-review@sonnet` | 1 | 35.0 | 19k | 104k | 158k | $1.47 | $1.47 | 48% | 5.4 |
| `discussion-burler-review+fork` | 28 | 4.2 | 121k | 125k | 186k | $9.43 | $0.34 | 31% | 1.5 |
| `plan-burler-fix` | 7 | 36.9 | 19k | 96k | 212k | $13.32 | $1.90 | 36% | 5.2 |
| `plan-burler-fix+fork` | 28 | 3.2 | 104k | 110k | 191k | $4.89 | $0.17 | 41% | 0.6 |
| `discussion-burler-fix` | 7 | 29.9 | 19k | 76k | 156k | $11.32 | $1.62 | 27% | 5.5 |
| `discussion-burler-fix+fork` | 28 | 3.5 | 87k | 92k | 134k | $4.63 | $0.17 | 38% | 0.6 |
| `plan-bouncer-seed` | 3 | 8.7 | 19k | 42k | 88k | $0.69 | $0.23 | 27% | 0.6 |
| `plan-bouncer-judge` | 7 | 6.1 | 19k | 34k | 56k | $1.33 | $0.19 | 17% | 0.5 |
| `discussion-bouncer-seed` | 3 | 6.0 | 19k | 30k | 46k | $0.47 | $0.16 | 18% | 0.4 |
| `discussion-bouncer-judge` | 7 | 6.0 | 19k | 32k | 48k | $1.21 | $0.17 | 18% | 0.5 |

- The segment costs $101.59: reviewers with their forks $63.73 (63%), fixers with their forks $34.15 (34%), seed and judge sessions $3.70 (4%).
- The Plan reviewer is the most expensive role at $31.25 over 7 main sessions, about $4.46 each, and the longest: about 11 minutes a session.
  Its context is the figure the orch saw: 169k (Opus) and 186k (Sonnet) a message, from a 19k start.
  It grows by about 4k to 5k tokens per turn, from the reads, to a peak of 311k and 355k.
- A fork starts at its parent's context at the spawn (204k for a Plan reviewer fork, 121k for a Discussion reviewer fork) and lives 3 to 6 turns, so a fork costs $0.17 to $0.52.
  Forks are 23% of the segment because there are many of them (93), not because one is dear.
- The Plan reviewer runs on Opus in round 1, with three forks, and on Sonnet in every later round, with none.
  Per session the Sonnet rounds cost $3.89 against $5.23 on Opus, but the Opus round also pays $1.30 to $1.79 of fork cost; the Sonnet rounds have the larger context (186k against 169k).
  The Discussion reviewer runs Opus in all but one session.
- The seed and judge sessions are 6 to 9 turns of 30k to 42k context at $0.16 to $0.23; there is nothing to cut in them.

Per main session, the reviewer and fixer rounds (cost with forks):

| Run | Round | Model | Turns | Calls per turn | Mean context | Peak context | Serial turns | Forks | Cost | With forks | Wall, min |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `interactive-recipe` | 1 | Opus | 59 | 1.03 | 127k | 197k | 20 | 3 | $3.39 | $4.69 | 8.0 |
| `interactive-recipe` | 2 | Sonnet | 57 | 1.44 | 158k | 238k | 13 | 0 | $2.93 | $2.93 | 7.7 |
| `run-verbs-shared` | 1 | Opus | 85 | 1.02 | 176k | 283k | 43 | 3 | $5.69 | $7.48 | 11.6 |
| `run-verbs-shared` | 2 | Sonnet | 87 | 1.46 | 228k | 355k | 23 | 0 | $5.91 | $5.91 | 15.7 |
| `run-verbs-shared` | 3 | Sonnet | 58 | 1.59 | 176k | 277k | 8 | 0 | $3.29 | $3.29 | 7.8 |
| `warp-visibility` | 1 | Opus | 87 | 1.09 | 192k | 311k | 37 | 3 | $6.60 | $8.18 | 14.7 |
| `warp-visibility` | 2 | Sonnet | 55 | 1.22 | 160k | 242k | 12 | 0 | $3.45 | $3.45 | 14.5 |

The table is the Plan reviewer.
`rounds.tsv` holds the same columns for the Discussion reviewer and both fixers; the Discussion reviewer's Opus rounds run 28 to 49 turns at 84k to 124k mean context and $1.77 to $3.31, bar the 10-turn session, and every fixer round 17 to 61 turns at $0.68 to $3.90.
Serial turns are the turn audit's strict count (section 4).

## 2. Where the turns go

Each cell is the group's share of the role's weight, then of its time.
A call's group follows its class: `read` is the Read tool and shell `sed -n` and `cat`; `search` is `grep`, `rg`, `find`, `ls`, `awk`; `lookup` is git and `lym` reads, `lym quarry`, skills and compound shell reads; `write` is the Edit and Write tools and shell writes; `check` is `lym gate`, tests and `lym loom validate-*`; `fork` is the Agent tool.
`fork-wait` is a no-call turn after a turn that spawned forks, before the next write or check call; `text` is any other no-call turn.
A turn with several calls splits its weight and time evenly among them.
`other` (ToolSearch, WebFetch) is under 1% everywhere and left out.

| Role | Sessions | Turns | read | search | lookup | write | check | fork | fork-wait | text |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `plan-burler-review` | 3 | 231 | 13% / 14% | 44% / 36% | 15% / 9% | 12% / 22% | 10% / 14% | 2% / 1% | – | 4% / 4% |
| `plan-burler-review@sonnet` | 4 | 257 | 16% / 18% | 53% / 38% | 10% / 8% | 8% / 10% | 10% / 12% | – | – | 3% / 14% |
| `discussion-burler-review` | 8 | 290 | 15% / 16% | 33% / 23% | 13% / 5% | 20% / 33% | – | 4% / 3% | 9% / 19% | 5% / 1% |
| `discussion-burler-review@sonnet` | 1 | 35 | 22% / 22% | 53% / 53% | 15% / 13% | 6% / 10% | – | – | – | 5% / 1% |
| `plan-burler-fix` | 7 | 258 | 10% / 8% | 23% / 16% | 13% / 8% | 29% / 39% | 8% / 10% | 4% / 5% | 8% / 13% | 5% / 1% |
| `discussion-burler-fix` | 7 | 209 | 13% / 9% | 24% / 20% | 7% / 4% | 32% / 42% | 3% / 3% | 4% / 4% | 9% / 16% | 7% / 2% |
| `plan-burler-review+fork` | 9 | 57 | 7% / 7% | 67% / 85% | 6% / 7% | – | – | – | – | 21% / 0% |
| `discussion-burler-review+fork` | 28 | 118 | 6% / 16% | 53% / 73% | 9% / 11% | – | – | – | – | 32% / 0% |
| `plan-burler-fix+fork` | 28 | 91 | 3% / 4% | 58% / 87% | 5% / 9% | – | – | – | – | 34% / 0% |
| `discussion-burler-fix+fork` | 28 | 97 | 8% / 20% | 54% / 77% | 3% / 3% | – | – | – | – | 34% / 0% |

- **Reviewers gather.** `read`, `search` and `lookup` together are 61% to 90% of a reviewer's weight, and `grep` and `sed` shell calls dominate them (section 3).
  The Opus Plan reviewer's `write` is 12% of its weight and 22% of its time: its 13 write turns are the review file, a long generation.
- **Fixers write.** `write` is 29% (Plan) and 32% (Discussion) of the fixers' weight, 39% and 42% of their time, and 45% and 54% of their calls: the Discussion fixer makes 205 edit calls in 37 turns, 5.5 per turn, with 23 re-edits of a file per session (n = 7).
  The fixers batch their edits already; the turns they could save are the gathers (section 3).
- **The Plan roles wait on checks.** `check` is 10% of the Plan reviewer's weight and 12% to 14% of its time (validate runs), 8% and 10% of the Plan fixer's.
- **Fork wake-ups.** `fork-wait` is 27 turns of the Discussion reviewer (8 sessions, 7 of which fork), 28 of the Discussion fixer and 24 of the Plan fixer: one per fork, since each fork's result arrives as its own message to the parent.
  They are 9%, 9% and 8% of the three roles' weight and 19%, 16% and 13% of their time.
  The Plan reviewer works on after its spawn, with reads, searches and validate calls, so its wake-ups are mixed into `text`: 8 of its 11 no-call turns over 3 sessions come after the spawn.
- **Forks search.** A fork's weight is 53% to 67% `search` and 21% to 34% its closing `text` turn, which reports back.
  Fork time is 73% to 87% `search`.

## 3. Mergeable turns

A removable turn is the turn audit's: in a run of two or more consecutive single-call read, search, git-read, lym-read, quarry or skill turns, any turn but the first.
Strict leaves out a turn whose call names a path found only in the previous result; class-only is the upper bound.
The saving is the role's cost times the removable turns' share of its weight, an estimate.

| Role | Sessions | Calls per turn | Single-call | Removable per session, strict | Share of weight, strict | Share, class-only | Saving, strict | Saving, class-only |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `plan-burler-review` | 3 | 1.05 | 89% | 33.3 | 41% | 44% | $6.43 | $6.90 |
| `plan-burler-review@sonnet` | 4 | 1.43 | 70% | 14.0 | 22% | 26% | $3.43 | $4.05 |
| `plan-burler-review+fork` | 9 | 0.84 | 84% | 2.2 | 32% | 53% | $1.49 | $2.48 |
| `discussion-burler-review` | 8 | 1.13 | 70% | 12.9 | 30% | 36% | $5.07 | $6.09 |
| `discussion-burler-review@sonnet` | 1 | 1.37 | 66% | 6.0 | 18% | 28% | $0.26 | $0.41 |
| `discussion-burler-review+fork` | 28 | 0.91 | 69% | 1.4 | 23% | 27% | $2.17 | $2.55 |
| `plan-burler-fix` | 7 | 1.57 | 60% | 4.9 | 14% | 16% | $1.86 | $2.13 |
| `plan-burler-fix+fork` | 28 | 0.90 | 59% | 0.7 | 21% | 24% | $1.03 | $1.17 |
| `discussion-burler-fix` | 7 | 1.82 | 56% | 5.9 | 18% | 22% | $2.04 | $2.49 |
| `discussion-burler-fix+fork` | 28 | 0.74 | 68% | 1.0 | 23% | 33% | $1.06 | $1.53 |
| seed and judge, both segments | 20 | 0.85 to 1.26 | 33% to 73% | 0 to 1.7 | 0% to 24% | 0% to 31% | $0.17 | $0.21 |

- Over the segment (n = 143 sessions), 450 turns are removable (strict), 25% of the 1772 turns: 345 in main sessions (6.9 per session, 24% of main-session weight, n = 50) and 105 in forks (1.1 per session, 24% of fork weight, n = 93).
  The estimated saving is $25.01 of the segment's $101.59 (strict) and $30.01 (class-only).
- The two Plan reviewers differ: the Opus round has 33.3 removable turns per session (43% of 77 turns), the Sonnet rounds 14.0 (22% of 64).
  The Sonnet rounds batch more (1.43 calls per turn against 1.05) but they are later rounds with no forks and a different prompt, so the difference is not a model effect shown by this sample.
- Fixers batch their edits (1.57 and 1.82 calls per turn); their removable turns are the gathers before the first edit, 4.9 and 5.9 per session.
- The calls of the 450 removable turns: `grep` 229 (51%), `sed` 108 (24%), `cat` 23, `lym board` 19, `ls` 16, `awk` 12, the Read tool 11.
  The reviewers search through the Bash tool as the agents of the turn audit did.
- The fork wake-up turns of section 2 are not in these counts: they are no-call turns, and they cannot be merged by an instruction, since the results arrive one by one.

## 4. The batching sentences were in place

Every reviewer and fixer main session of the sample (16 reviewers, 14 fixers) holds the burler stencil's batching line in its transcript: the explore step's "Make the reads your focus needs, or the whole target when no focus is set, in one message", and the fixer's "Read the files the findings name together, in one message".
Both came with `bf00a043` (2026-10-28), as did the sentence in the system prompt shuttle appends, which also appears in each transcript.
The turn audit recommended these lines on the reviewers' 17.7 and 15.7 removable turns per session (n = 3 and n = 3).
Three runs after the lines, the Opus Plan reviewer has 33.3 and the Discussion reviewer 12.9 (n = 3 and n = 8).

The lines did not take on the Opus Plan reviewer.
The sample cannot say whether they helped the other roles: every session in it started after the commit, so there is no before-group.

## 5. What Sonnet fixers would have cost

`fixer-cost.tsv` prices every message of every Discussion and Plan fixer session, forks included, at the `claude-sonnet-5-5` row, with the turns, context and output exactly as run.

| Role | Sessions | Forks | As run, Opus | Priced as Sonnet | Saving | Saving share |
| --- | --- | --- | --- | --- | --- | --- |
| `discussion-burler-fix` | 7 | 28 | $15.94 | $10.38 | $5.56 | 35% |
| `plan-burler-fix` | 7 | 28 | $18.21 | $12.51 | $5.70 | 31% |
| both | 14 | 56 | $34.15 | $22.89 | $11.26 | 33% |

Per run, the saving is $0.99 to $2.72 for the Discussion fixer and $1.27 to $2.74 for the Plan fixer; `fixer-cost.tsv` holds the rows.

- The saving matches note `fixers-on-sonnet`: Opus and Sonnet read cache at the same price, so a swap saves the non-cache-read part at about half.
  The fixers' cache-read share is 27% and 36% in the main sessions, so their non-cache-read part is 73% and 64% and half of it 37% and 32%; the forks' share is 38% and 41%.
- The fixer forks ran on the fixer's model; a Sonnet fixer's forks would be Sonnet too, and the table prices them so.
- The figure is a price swap at equal tokens, not a forecast.
  A Sonnet fixer that takes more turns, or leaves findings an Opus fixer would have closed and so costs another round, is dearer: the swap breaks even when the Sonnet fixer's cost is 1.54 times (Discussion) or 1.46 times (Plan) the figure above.
- This sample holds no Sonnet fixer of these two roles and does not measure whether a Sonnet fixer carries out the findings.
  The finding is the note's open question, unchanged: the findings must be concrete enough for Sonnet, and that needs the bench.
  The only Sonnet reviewer rounds here (section 1) say nothing about fixers.
- For scale, the Opus reviewers priced as Sonnet at equal tokens: Discussion $26.35 to $17.39 and Plan $20.35 to $15.19, forks included, over the Opus sessions only.

## 6. Recommendations

These are recommendations, not decisions.
The transcripts show where turns go, not what a stencil line or a model change would do; no run measured that and none could here.
No change proposed here hands an agent pre-read or digested source: the agents keep reading the code themselves.

- **Plan reviewer, round 1 (largest single cut).**
  The estimated saving of merging its serial gathers is $6.43 of $15.68 over the three runs, and the lines already in its stencil did not move it.
  A further line repeats them.
  What differs for the Sonnet rounds is the model and the absent forks; a test of whether the Opus reviewer batches under a different instruction, or whether round 1 can run on the model that batched, needs a bench the study did not run.
- **Fork wake-ups.**
  79 no-call turns at the full context are 8% to 9% of three roles' weight.
  Fewer forks per round, or forks whose results the parent gathers in one turn, would remove them; the data does not say what either costs in review quality.
- **Sonnet fixers.**
  $11.26 over the three runs is the price swap at equal tokens (section 5).
  It is worth the bench `fixers-on-sonnet` already asks for, and the bench should record the fixer's turns and context, since a 1.5 times larger Sonnet fixer erases the saving.
- **Discussion reviewer and fixers.**
  Their removable turns are 12.9 and 5.9 and 4.9 per session; the estimated saving is $5.07, $2.04 and $1.86.
  They are small beside the Plan reviewer's and are left to the same lines.
- **Seed and judge sessions.**
  No recommendation: 20 sessions, $3.70.

## Limits

- Three runs, started within a day of each other; the roles that ran once or twice per run have `n` of 1 to 9, and the Sonnet Discussion reviewer is one session.
- Rounds are the order of a run's sessions of a role, not the review's round count; the model and the forks change with the round, so a round-1 against round-2 comparison mixes the two.
- The dependency test of the strict count is a text match; its error runs both ways (the turn audit's method note).
- The saving is a share of weight applied to a list-price cost: close, not equal.
  A merged turn is a little heavier in output and results, and that is not subtracted.
- `fork-wait` reads the order of turns, not the harness: it assumes a no-call turn after a spawn waits on a fork, and the closing report of a session counts the same way.
- Time includes tool run time and generation together; the forks of one parent overlap, so fork wall times do not add up to elapsed time.
- List price is not the bill: a plan or discount is not modeled.
