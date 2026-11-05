# Turn savings measured on landed runs, 2026-11-04

What project `agent-turns` and the batching breach hook (`batching-breach-nudge`, landed 2026-11-02) changed in the transcripts of runs that already landed.
Every figure derives from the scripts and tables beside this file; the sample size stands beside each number.
No agent run was made: the work reads transcripts, run records and hook state of finished sessions.

## Finding

`agent-turns` shows clearly in the transcripts.
Webster forks take 13.8 turns per card against 31, shell edits fell from a fifth of all edits to under one percent, and the darn writer batches 2.05 calls a turn against 1.08.
The batching hook shows little: it fired 4 times in 24 runs, and the lookup-chain breach it was built for fell from 17.9% to 9.8% of context tokens, a drop the hook cannot have caused most of, since it cannot see a lookup made with a Bash `grep`.
The report recommends no threshold change: 4 firings are too few to tune on.
Two coverage gaps are worth a look: the hook does not see Bash lookups, and it does not reach forks, where 57 of the sample's 117 edit-chain turns sit.

## Sample and method

- Runs: the 20 most recent darn runs and the 4 tapestry runs that landed since 2026-11-02, as `go run ./tools/tokencount -since 2026-11-02` lists them.
  Only 4 tapestry runs landed in the window, so the tapestry side is `n = 4`, not 5; one of them, `cold-gate-fixer`, ran no Webster fork.
  The slugs are in `slugs.txt`, each with its recipe in `recipes.tsv`.
- Window: the sample's sessions start 2026-11-02 18:29 and end 2026-11-04 13:44 local time.
  The hook's first firing in any transcript is 2026-11-02, after its commit at 13:51; `cold-gate-fixer` started at 18:29 and may predate the binary that carries the hook.
- Hook thresholds: unchanged inside the window.
  The template's `batching_nudge` block has not changed since the commit that added it (`eea96fe2`), the hub's `claude.yaml` has held no `batching_nudge` key since 2026-11-01 20:26, and a run's `batching-nudge.json` seeds `context_tokens` 150000, `lookup_run` 3, `edit_run` 2, `slice_lines` 2000, `max_nudges` 5.
- Baseline, Part 1: the numbers in `agent-turns`' body (`warp-visibility`, `interactive-recipe`, `run-verbs-shared`: 3 full tapestry runs, 27 forks by the fork audit), `docs/research/turns/turn-audit-2026-10-28` for the darn writer (14 sessions) and `docs/research/turns/review-turn-audit-2026-11-01` for the review roles (3 runs).
  The same scripts ran again on the baseline runs, so each baseline figure below is recomputed unless it says it is quoted.
- Baseline, Part 2: the 28-session sample behind `batching-breach-nudge` is quoted (128 turns and 34.7M context tokens of single lookups, 39 turns and 15M of edit chains, of 147M).
  Its sessions are not listed, so the figures below are also recomputed on the 3 baseline tapestry runs, 261 sessions, whose size differs.
- A turn is one assistant message id, as in the turn audit; a fork is a sub-agent transcript.
  Context of a turn is its input, cache write and cache read tokens.

## Part 1: agent-turns

### Which members show

| member | shows in the transcripts | where |
| --- | --- | --- |
| Edit tool in place of shell edits, with a guard (`f08df88b`) | yes, strongly | shell edits, refusals, calls per turn |
| `turn-batching` and the batching sentence in heddle (`a1715fa5`) | yes | calls per turn of every role |
| darn writer runs no gate | yes | darn writer's gate calls fall to 0 |
| fanned Plan reviewer pre-read in waves (`plan-reviewer-batching`, `1993acac`) | yes | plan reviewer's calls per turn |
| `lym shuttle await-forks` (`76dd5fb7`) | cannot be separated | fork wake-up turns are 3 in both samples |
| steers naming the allowed route for Python and go compile checks (`9dc4fada`) | partly | refusals fall, but `rm` refusals persist |
| card gate fix loops | not attributed to one member | 1 loop left against 5, with 7 card gates of no recorded verdict |

The mapping is by what each change touches, since one transcript set cannot isolate one member.

### Webster forks

Fork audit and token count, tapestry runs only.
Baseline: 27 forks, 52 cards, 3 runs.
Now: 11 forks, 32 cards, 3 runs.

| measure | baseline | now |
| --- | --- | --- |
| turns per card, pooled (median) | 31 (29) | 13.8 (15) |
| turns per fork, turn audit | 56.4 (n = 29 forks) | 35.7 (n = 13 forks) |
| calls per turn, turn audit | 1.45 | 2.96 |
| partial reads per card, pooled | 23.2 | 8.2 |
| greps per card, pooled | 17.6 | 8.3 |
| forks with 3+ parallel calls in the first work turn | 22% | 55% |
| cache reads per card, pooled | 6.2M | 3.1M |
| peak context, median (largest), per listed fork | 228k (476k) | 270k (453k) |
| fork cost per card, `tools/tokencount` | $1.63 ($84.85) | $1.07 ($34.14) |

Forks take fewer turns and cost a third less a card, but their median peak context did not fall.
The forks carry about 3 cards each against about 2 in the baseline, which may explain it; the transcripts do not test that.
The turn audit counts 13 forks where the fork audit keeps 11, since it also takes in forks that name no card.
The cost figures cover different cards and runs, and the card sizes differ; a cost per card is a pointer, not a measurement.

### Darn writers

Baseline: 14 sessions of the turn audit.
Now: 20 sessions.

| measure | baseline | now |
| --- | --- | --- |
| turns per session | 54.4 | 45.5 |
| calls per turn | 1.08 | 2.05 |
| single-call turns | 86% | 59% |
| Edit-tool calls | 72 | 770 |
| shell edit calls (turn audit's `edit-script`) | 101 (58% of edits) | 12 (1.5%) |
| `awk … && mv` edit calls | not counted | 0 |
| gate calls per session | 2.71 | 0 |
| test, lint and validate turns per session | 2.29 and 2.71 | 2.25 and 1.95 |

The darn writer's check turns (`darn-writers.tsv`) are 84 over 20 sessions, 9.2% of its turns.
The baseline's gate calls are gone because the darn writer no longer runs the gate.

### Review roles

Tapestry runs: baseline 3 runs of the review turn audit, now 4 runs.

| measure | baseline | now |
| --- | --- | --- |
| round-1 Plan reviewer on Opus: calls per turn | 1.05 (n = 3 sessions) | 1.95 (n = 3) |
| round-1 Plan reviewer: turns per session | 77 | 43.3 |
| round-1 Plan reviewer: removable turns, strict, per session | 33.3 | 9.3 |
| round-1 Plan reviewer: estimated merge saving | $6.43 of $15.68 | $2.09 of $12.27 |
| Plan reviewer: share of its calls that are reads | 17% | 49% |
| Discussion reviewer: calls per turn | 1.13 | 2.25 |
| fork wake-up turns of the Plan reviewer | 3 | 3 |

`plan-reviewer-batching` shows: the fanned reviewer issues reads in parallel, calls per turn nearly double, and its removable turns fall by two thirds.
The `where.tsv` split puts the reviewer's pre-read in read calls, 124 calls over 35 turns, which is the fanned waves.
Fork wake-up turns stay at 3, one per reviewer session, so the wake-up saving of `await-forks` cannot be told from the baseline here.

### Refused turns

The baseline's count was 6 turns refused by lym's guards across the five heaviest forks.
Counted again in the same way, the baseline's five heaviest forks hold 6 refused calls, so the count reproduces.
The five heaviest forks of the sample hold 0.

| scope | baseline | now |
| --- | --- | --- |
| five heaviest Webster forks | 6 | 0 |
| every fork of the sample | 30 (17 Python, 6 raw go run, 7 `rm`) | 5 (4 other, 1 shell edit) |
| every main session | 3 | 11 (7 `rm`, 2 raw go run, 1 shell edit, 1 other) |

`rm` on a computed path is the one rule that still fires in numbers, 7 times in main sessions of review and darn roles.
`awk … && mv` edits: 1 call in the baseline, 0 now.
Shell edit calls of every session, the turn audit's `edit-script`: 339 in the baseline (272 in Webster forks) against 21 now.

### Card gate fix loops

A loop is the turns after a failing `lym gate test --gate card` call up to and including the next green card gate call of the same fork; `gate-loops.sh` reads the verdict from the tool result, and for a backgrounded call from its task notification.

| measure | baseline | now |
| --- | --- | --- |
| card gate calls: fail, green, no verdict | 5 fail, 83 green, 0 | 1 fail, 31 green, 7 no verdict |
| loops | 5 | 1 |
| turns in loops | 53 | 3 |
| context tokens read in loops | 20.4M | 1.3M |
| longest loop | 47 turns, peak 454k | 3 turns, peak 447k |

The board entry's 69-turn loop is not reproduced by this definition, which finds 47 turns for the same fork.
The 7 card gate calls of the sample with no verdict moved to the background and no task notification in the transcript names their exit code, so the loop count there is a lower bound.

## Part 2: the batching breach hook

### How often it fired

| measure | now |
| --- | --- |
| firings in 24 runs, main sessions only | 4 |
| by breach | 3 edit chains, 1 read slice, 0 lookup chains |
| by role | 2 darn writer, 2 code reviewer |
| next message batched (2 or more calls) | 2 of 4 |
| firings on a lookup that had to be sequential | 0, no lookup-chain firing |

The next message after a firing: 23 calls and 15 calls after the two edit chains that batched, 1 call after the read slice, and a message with no call after the edit chain that ended its turn.
n = 4, so these are cases, not rates.

Why so few: the hook's lookup chain matches Read, Grep and Glob tool calls only.
The sample holds 1 Grep call and 1 Glob call against 2,920 Read calls and 3,040 Bash calls, since lym sessions search with Bash `grep`, which the hook's matcher never sees.
Read chains of three are rare (`l3_turns` 0 over the sample), so the lookup-chain breach had almost nothing to fire on.

### Breach figures

Single-lookup turns that follow another above 150k context, and edit-only turns that follow an edit-only turn above 150k, as the baseline defined them.
Two readings of a lookup, as `breaches.sh` explains: by tool (what the hook sees) and by class (a Read, or a search by any tool, Bash `grep` included).

| figure | baseline, 3 tapestry runs (261 sessions) | now, all (492 sessions) | now, tapestry (245) | now, darn (247) |
| --- | --- | --- | --- | --- |
| context tokens, all turns | 838M | 699M | 372M | 327M |
| share of context in turns above 150k | 72.7% | 62.0% | 69.6% | 53.3% |
| lookup follows, tool reading | 0 turns | 4 turns, 0.95M | 2 turns, 0.37M | 2 turns, 0.58M |
| lookup follows, class reading | 582 turns, 149.7M (17.9%) | 301 turns, 68.4M (9.8%) | 202 turns, 46.5M (12.5%) | 99 turns, 21.9M (6.7%) |
| runs of 3 or more, class reading | 417 turns, 107.1M | 167 turns, 38.0M | 121 turns, 28.0M | 46 turns, 10.0M |
| edit follows | 76 turns, 21.2M (2.5%) | 117 turns, 29.4M (4.2%) | 79 turns, 21.0M (5.6%) | 38 turns, 8.4M (2.6%) |

The quoted baseline of the hook (128 turns, 34.7M of lookups and 39 turns, 15M of edits, a third of 147M) lists no sessions, so its mix may differ from every sample here; the recomputed baseline above uses this report's definitions and is the one to compare.

By role, the context share of lookup follows by class reading (`breach-summary.tsv` holds every row):

| role | baseline | now |
| --- | --- | --- |
| Webster forks | 12.3% | 14.1% (tapestry forks) |
| Plan reviewer | not split | 20.7% |
| Plan writer | not split | 12.0% |
| darn writer | not in the baseline | 9.9% |
| code reviewer, darn runs | not in the baseline | 5.0% |

Edit follows are 6.9% of tapestry fork context, 12.0% of the Plan writer's, and 4.2% of the darn writer's.
The hook skips every call from a subagent or fork, so 57 of the 79 tapestry edit-follow turns, and the forks' lookup follows, lie outside it.

### What it did

The drop of class-read lookup follows from 17.9% to 9.8% of context cannot be credited to the hook: 4 firings, and a matcher that misses the searches that make most of the figure.
The likelier causes are the heddle batching sentence and the Edit-tool route, which lift calls per turn in every role.
Edit follows rose in share, from 2.5% to 4.2% of context overall and from 2.5% to 5.6% within tapestry runs; the transcripts give no cause, and over half of them are in forks the hook does not reach.
Of the 4 firings, 2 were followed by a batched message.
The sample cannot show whether the hook changed how often a breach starts.

## Recommendation

- No `batching_nudge` threshold change; the sample has 4 firings.
- Two coverage questions, not thresholds, for the operator to take or leave: whether the lookup chain should count a Bash `grep`, `find` or `ls` call as a lookup, and whether forks should be reached, since they hold most of the edit-follow turns.
  Neither is filed as a note here, because each changes what the hook is for rather than a threshold.
- Re-run `run-all.sh` after more runs have landed with the hook: a sample with a few dozen firings can say whether the next message batches.

## Limits

- Tapestry `n = 4`, not 5, and its three Webster runs hold 11 forks; every tapestry fork figure rests on those.
- The baseline and the sample differ in recipe mix (darn writers are half the sample's main sessions), card size and model, so a role-for-role pair is the fair comparison and a total is not.
- A card gate with no recorded verdict (7 calls) leaves the gate loop count a lower bound.
- `cold-gate-fixer` may predate the hook's binary; its sessions are in every tapestry figure.
- The classification of a Bash call as a lookup, an edit or a check is the turn audit's heuristic, applied unchanged.
