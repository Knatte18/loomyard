# The batching hook and the edit-round rule, re-measured, 2026-11-05

What the batching breach hook (`internal/shuttleengine/claudeengine/batchingnudge.go`) did once it counted Bash lookups and reached forks (`119c1c05`, landed 2026-11-04 16:15), and whether the heddle entry `lym-plan-edits-before-gate` ("make all of those edits in one message") shows in edit-chain turns.
Every figure derives from the scripts and tables beside this file; the sample size stands beside each number.
No agent run was made: the work reads transcripts of finished sessions.

## Finding

The hook still shows little effect, and the report recommends removing it.
It fired 13 times in 22 runs, 9 in main sessions and 4 in forks, and the next message batched after 5 of the 13.
Its firings are about 2.5% of the breach turns it was built for: 10 lookup and edit firings against 399 breach turns (243 class-read lookup follows and 156 edit follows).
The breach shares did not fall where the hook now reaches: edit follows rose from 4.2% to 6.9% of context, and in Webster forks from 6.9% to 11.2%.
The edit chains do not split by cause: none of the 71 chains with an edit follow comes after a failed gate or validator, and all 156 edit follows sit in the middle of a round of edits.
A replay of the hook on one recorded edit follow fires, so the live gap between 156 edit follows and 2 edit firings has a cause this report did not find.

## Sample and method

- Runs: the 20 most recent darn runs and the 2 tapestry runs that landed after `batching-hook-reach`, as `go run ./tools/tokencount -since 2026-11-04` lists them, most recent first.
  Only 2 tapestry runs landed in the window (`raddle-recipe`, `pawl-orch-landing`), so the tapestry side is `n = 2`, not 5.
  The slugs are in `slugs.txt`, each with its recipe in `recipes.tsv`.
- Window: the 22 runs landed from 2026-11-05 06:35 to 18:37 local time, about a day of landings, not the week the task names; the cap of 20 darn and 5 tapestry runs bounded the sample first.
  Every one of the 445 sessions started between 2026-11-05 07:02 and 17:33 UTC, after the hook commit.
- Four of the darn runs (`codex-creel-trial`, `codex-credit-line`, `codex-greet-finish`, `codex-trial-darn-e2e`) hold one Claude transcript each; their Codex sessions have no transcript reader and are not counted.
- Hook thresholds: a run's `batching-nudge.json` seeds `context_tokens` 150000, `lookup_run` 3, `edit_run` 2, `slice_lines` 2000, `max_nudges` 5, unchanged since `eea96fe2`.
- Baseline: the sample of `docs/research/turns/turn-savings-measure-2026-11-04` (24 runs, 20 darn and 4 tapestry, 492 sessions), run again by the same scripts as `baseline-slugs.txt`.
  There the hook saw main sessions only and judged only Read, Grep and Glob as lookups, so the comparison is before and after `119c1c05`.
  Its figures reproduce that report's.
- A turn is one assistant message id, as in the turn audit; a fork is a sub-agent transcript.
  Context of a turn is its input, cache write and cache read tokens.

## How often it fired

| measure | baseline | now |
| --- | --- | --- |
| firings | 4 in 24 runs, 0.17 a run | 13 in 22 runs, 0.59 a run |
| main sessions, forks | 4, 0 | 9, 4 |
| by breach: lookup chain, read slice, edit chain | 0, 1, 3 | 8, 3, 2 |
| next message batched (2 or more calls) | 2 of 4 | 5 of 13 |

By session kind and breach, with the next message batched:

| breach | main | fork | batched next |
| --- | --- | --- | --- |
| lookup chain | 7 | 1 | 2 of 8 |
| read slice | 1 | 2 | 2 of 3 |
| edit chain | 1 | 1 | 1 of 2 |
| all | 9 | 4 | 3 of 9 main, 2 of 4 fork |

By role: 5 in the darn writer, 4 in Webster forks, 2 in the Plan writer, 2 in the Discussion reviewer.
A batched next message is a message of 2 or more calls; it need not repeat the batch the nudge asked for.
n = 13, so these are cases and a rough rate, not a measurement.

Bash lookups now count: all 8 lookup firings are on a Bash call, so the first blind spot is closed.
The fork blind spot is closed too, since 4 firings sit in forks.
Even so, the firings are few against the breaches (`events.tsv` against `breaches.tsv`):

| breach | breach turns | firings |
| --- | --- | --- |
| lookup, a run of 3 or more by class reading, above 150k | 143 | 8 |
| edit-only turn after an edit-only turn, above 150k | 156 | 2 |

The lookup count of turns is an upper bound of what the hook sees, since the class reading takes in `awk` and other searches the hook's table does not class as a lookup.
The edit count is the hook's own rule, and 2 of 156 is the gap this report cannot explain.
One recorded edit follow was replayed: turn 27 of the darn writer of `arbiter-settles-escalations`, with a settings file of the thresholds above, through `lym shuttle hook-batching claude`.
It returned the edit-chain nudge, so the hook's judgment holds on that turn and the live run did not reach it.
Candidates the replay cannot separate are a transcript that did not yet hold the current message when the hook ran, and a hook that did not run for the Edit and Write matcher; neither was tested.

## Breach figures

Lookup follows by class reading and edit follows, as the baseline defined them, in the context share of all turns:

| figure | baseline, all (492 sessions) | now, all (445) | baseline, tapestry (245) | now, tapestry (219) | baseline, darn (247) | now, darn (226) |
| --- | --- | --- | --- | --- | --- | --- |
| context tokens, all turns | 699M | 673M | 372M | 453M | 327M | 220M |
| share of context in turns above 150k | 62.0% | 63.7% | 69.6% | 74.4% | 53.3% | 41.6% |
| lookup follows, class reading | 301 turns, 68.4M (9.8%) | 243 turns, 57.4M (8.5%) | 202 turns, 46.5M (12.5%) | 187 turns, 47.2M (10.4%) | 99 turns, 21.9M (6.7%) | 56 turns, 10.2M (4.7%) |
| runs of 3 or more, class reading | 167 turns, 38.0M | 143 turns, 33.8M | 121 turns, 28.0M | 109 turns, 27.8M | 46 turns, 10.0M | 34 turns, 6.0M |
| edit follows | 117 turns, 29.4M (4.2%) | 156 turns, 46.4M (6.9%) | 79 turns, 21.0M (5.6%) | 132 turns, 41.3M (9.1%) | 38 turns, 8.4M (2.6%) | 24 turns, 5.0M (2.3%) |

Forks, where the hook now reaches (`breach-summary.tsv`):

| figure | baseline, tapestry forks (152 sessions) | now, tapestry forks (142) |
| --- | --- | --- |
| lookup follows, class reading | 128 turns, 30.2M (14.1%) | 144 turns, 37.8M (12.6%) |
| edit follows | 57 turns, 14.9M (6.9%) | 105 turns, 33.7M (11.2%) |

Lookup follows fell a little in every column, and edit follows rose in tapestry and held in darn.
The two samples differ in recipe mix, in size (`raddle-recipe` alone holds 138 transcripts) and in the work done, so a share change of a point or two is inside what the sample can say.
What the figures can say is that no breach share fell by a margin that 13 firings could explain, and that edit follows in forks, the newly reached part, grew.

## Edit chains: after a failed gate, or in the middle of a round

`edit-chains.sh` groups the edit-follow turns into chains, runs of turns that each hold only Edit and Write calls, and names what the session did before a chain began by the last `lym gate test`, validator or lint result.
A chain after a failed gate or validator is the fix of a failure; any other chain is mid-round.

| origin | baseline chains, follow turns | now chains, follow turns |
| --- | --- | --- |
| after a failed gate | 0, 0 | 0, 0 |
| after a failed validator or lint | 0, 0 | 0, 0 |
| mid-round | 62, 117 | 71, 156 |

The sample holds 11 failed gate results (9 in gate fixers, 2 in Webster forks) and 6 failed validator results (1 in a darn writer, 4 in describe sessions, 1 in a Plan reviewer); the baseline holds 22 and 10.
None of those 17 is followed by an edit chain above 150k, so the rule "make all of them in one message" holds where a gate fails, and the chains are all in the middle of a round.

Where the 156 mid-round edit follows sit, with the chains that hold them:

| session | chains | follow turns |
| --- | --- | --- |
| Webster forks | 34 | 64 |
| Plan writer forks | 9 | 41 |
| darn writer | 11 | 19 |
| Plan writer | 3 | 14 |
| Discussion writer | 3 | 5 |
| every review and fix role together | 11 | 13 |

Forks hold 105 of the 156 follows.
The rule's step 3, all edits in one message, is not met in the middle of a round in any role that edits; the transcripts do not say why.

## Recommendation

- Remove the batching hook: the `batching_nudge` block, `batchingnudge.go`, the `hook-batching` verb and the matcher entry.
  It fires on about 2.5% of the breaches it targets, a nudge was followed by a batched message in 5 of 13 cases, and no breach share fell where it reaches.
  It costs a `lym` process start on every Read, Grep, Glob, Bash, Edit and Write call of every session, which this report did not measure.
- The operator may take one open question first, since its answer would change the verdict: why the hook fired on 2 of 156 edit follows when its judgment fires on a replay of a recorded one.
  Finding that is a debugging task on the live hook, and it only matters if the hook is worth keeping, which these figures do not show.
- The edit-round rule needs no change for failed gates and validators: no chain follows one.
  The mid-round chains are the open part, and a hook that fires on them is what was tried here; a stencil or heddle change that targets Webster forks and Plan writer forks, where 105 of the 156 follows sit, is the other lever.
  Neither is filed as a note here, since each changes what the rule is for.

## Limits

- The window is about a day of landings, not a week, and the tapestry side is `n = 2`; every tapestry and fork figure rests on `raddle-recipe` and `pawl-orch-landing`.
- The baseline and the sample differ in recipe mix, in size and in the work done; a total across them is a pointer, and a role-for-role pair the fairer comparison.
- A call whose verdict the transcript does not hold is pending (5 gate calls in the sample, 25 in the baseline) and counts as mid-round, so the after-failure rows are lower bounds.
  The failed-result counts come from the tool result and the task notification, as `events.sh` reads them.
- The lookup count against firings is an upper bound of what the hook could see; the class reading counts searches by `awk` and similar that its table does not.
- A batched next message is any message of 2 or more calls.
- The replay covers one recorded turn and the thresholds' defaults; it shows the judgment, not the live delivery.
- Whether the lym binary of each run carried the hook of `119c1c05` is read from the commit time, not from the binary; every session started about 15 hours or more after it.
- The classification of a Bash call as a lookup, an edit or a check is the turn audit's heuristic, applied unchanged.
