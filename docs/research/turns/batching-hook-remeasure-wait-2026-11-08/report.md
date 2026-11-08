# The batching hook, re-measured after its transcript wait, 2026-11-08

What the batching breach hook (`internal/shuttleengine/claudeengine/batchingnudge.go`) did once it waited for its call to reach the transcript
(`7c77e08d`, landed 2026-11-07 14:14 +01:00), and what the wait costs.
It reruns the scripts of `docs/research/turns/batching-hook-remeasure-2026-11-05` on the runs landed since, against that report's own sample.
Every figure derives from the scripts and tables beside this file; the sample size stands beside each number.
No agent run was made: the work reads transcripts of finished sessions.

## Finding

Keep the hook; the earlier recommendation to remove it rested on a delivery gap that the wait closed.
It now fires on 10 of the 13 edit follows, where it fired on 2 of 156 before.
Its 34 firings in 28 runs are 1.21 a run, against 0.59.
The next message was batched after 16 of the 34, and after 8 of the 19 lookup and edit firings against 4 of the 18 breach turns it left silent.
That direction favors the hook, but 19 against 18 cannot separate it from chance, and the breach shares fell less than the mix of work moved them.
The wait adds no tokens and about 77 seconds of wall time over 122 messages of the sample, about 2.8 seconds a run.
Half of the calls above the context line waited the full two seconds and got no judgment, which is the one cost with nothing in return.

## Sample and method

- Runs: every darn run `go run ./tools/tokencount -since 2026-11-07` lists, 39 in all; no tapestry run landed in the window, so the sample has no tapestry side.
  The slugs are in `slugs.txt`, each with its recipe in `recipes.tsv`.
- Sessions: only the sessions that started at or after 2026-11-07 13:15 UTC count, the minute after the wait's commit; 28 of the 39 runs hold one, 345 sessions in all.
  Whether the `lym` binary of a session carried the wait is read from that time, not from the binary; the 10 edit firings show that it did.
- Window: the landings run from 2026-11-07 14:17 to 2026-11-08 11:27 local time, about a day.
- Comparison: the sample of the earlier report, run again by the same scripts (22 runs, 445 sessions, 20 darn and 2 tapestry).
  It reproduces that report's firings, breach turns and shares.
  There the hook already reached forks and Bash lookups but did not wait.
- Hook thresholds: `context_tokens` 150000, `lookup_run` 3, `edit_run` 2, `slice_lines` 2000, `max_nudges` 5, as in the earlier report.
- A turn is one assistant message id, as in the turn audit; a fork is a sub-agent transcript.
  Context of a turn is its input, cache write and cache read tokens.
- The two samples differ in recipe mix: the comparison holds 2 tapestry runs that are 67% of its context, and the new sample holds only darn runs.
  The darn columns are the fairer comparison.

## How often it fired

| measure | before the wait (22 runs) | after (28 runs) |
| --- | --- | --- |
| firings | 13, 0.59 a run | 34, 1.21 a run |
| runs with a firing | 5 | 10 |
| main sessions, forks | 9, 4 | 33, 1 |
| by breach: lookup chain, read slice, edit chain | 8, 3, 2 | 10, 14, 10 |
| next message batched (2 or more calls) | 5 of 13 | 16 of 34 |

By role after the wait: 28 in the darn writer, 5 in the code-review fixer, 1 in a darn writer fork.
By tool: the 10 lookup firings are 8 on Bash and 2 on Read, the 14 slice firings are all on Read, the 10 edit firings are 8 on Edit and 2 on Write.

Firings against the breach turns they target (`events.tsv` against `breaches.tsv`):

| breach | breach turns before | firings before | breach turns after | firings after |
| --- | --- | --- | --- | --- |
| lookup, a run of 3 or more by class reading, above 150k | 143 | 8 | 24 | 10 |
| edit-only turn after an edit-only turn, above 150k | 156 | 2 | 13 | 10 |

The edit row is the gap the earlier report could not explain: 10 of 13 now, so the hook reaches edit follows.
The 3 others are not explained here.
The lookup count of turns is an upper bound of what the hook sees, since the class reading takes in searches the hook's table does not class as a lookup;
10 firings against 24 turns is a lower bound of the coverage.
Slice breaches are not counted per turn by the scripts, so the 14 slice firings have no denominator.

## Did the next message batch

`nudge-effect.tsv` takes the breach turns by the hook's own rule and asks whether the next message of the session holds 2 or more calls.
A breach turn is nudged when `events.tsv` holds a firing of that breach at that turn.

| breach | nudged, next batched (before) | silent, next batched (before) | nudged, next batched (after) | silent, next batched (after) |
| --- | --- | --- | --- | --- |
| lookup | 2 of 8 | 31 of 135 | 3 of 9 | 4 of 15 |
| edit | 1 of 2 | 48 of 154 | 5 of 10 | 0 of 3 |
| both | 3 of 10 | 79 of 289 | 8 of 19 | 4 of 18 |

A nudged breach turn was followed by a batched message in 42% of the cases after the wait and 30% before; a silent one in 22% and 27%.
The silent edit cell holds 3 turns, so the after column is 8 of 19 against 4 of 18, and neither the 20-point gap nor the sign of it can be told from chance at these counts.
Pooled over both samples the figures are 11 of 29 nudged and 83 of 307 silent, 38% against 27%.
A batched next message need not repeat the batch the nudge asked for.
The 14 slice firings are 7 of 13 batched in main sessions and 0 of 1 in a fork; they have no silent comparison.

## Breach figures

Darn runs only, in the context share of all turns (`breach-summary.tsv`):

| figure | before, darn (226 sessions) | after, darn (345) |
| --- | --- | --- |
| context tokens, all turns | 220M | 254M |
| share of context in turns above 150k | 41.6% | 30.8% |
| lookup follows, class reading | 56 turns, 10.2M (4.7%) | 45 turns, 8.4M (3.3%) |
| runs of 3 or more, class reading | 34 turns, 6.0M (2.7%) | 24 turns, 4.2M (1.7%) |
| edit follows | 24 turns, 5.0M (2.3%) | 13 turns, 2.5M (1.0%) |

Every figure fell, edit follows the most.
Which part of that is the hook cannot be read here: the share above 150k fell by a quarter with it, and the two samples are different work.
In the darn writer, where 28 of the 34 firings sit, the breach shares are 5.8% for lookup follows and 2.0% for edit follows after the wait.

The 13 edit follows form 8 chains, all in main sessions and all in the middle of a round; none follows a failed gate or validator, as before.
Forks hold no edit follow in the new sample, where 105 of the 156 sat before, because the sample holds no Webster run.

## What the wait costs

The wait is wall-clock time and adds no tokens: the session is idle and the 5 minute cache of the prefix does not expire in two seconds.
`latency.sh` reads it from the transcripts: the time from a Read, Grep, Glob, Edit or Write call's line to its result's line.
A result line follows the call's after-tool-call hooks, so the latency holds the hook's time.
The bands are by the context of the call's message, at the hook's line of 150000.

| sample | band | calls | messages | median ms | p90 ms | mean ms | calls at 2 s or more | added seconds |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| before | below | 3586 | 1065 | 56 | 108 | 95 | 5 | 63 |
| before | above | 2500 | 581 | 55 | 93 | 61 | 0 | 3 |
| after | below | 2708 | 941 | 66 | 145 | 105 | 1 | 51 |
| after | above | 544 | 122 | 1791 | 2868 | 1446 | 268 | 77 |
| after | above, one call a message | 42 | 42 | 145 | 180 | 178 | 1 | 5 |
| after | above, 2 or more calls | 502 | 80 | 2079 | 2892 | 1552 | 267 | 73 |

Added seconds is the sum over calls of the latency over the below-band median, divided by the message's call count, so the parallel calls of one message count once.
Per call, a call above the line adds about 1.7 seconds at the median; per message, about 0.6 seconds on average and about 0.9 seconds for a message of several calls.
Below the line the latency moved by 10 ms at the median, which is what the one tail read costs plus noise.
By tool above the line, the mean is 1716 ms for Edit, 1035 for Write and 598 for Read.

Half of the calls above the line, 268 of 544, ran to the bound of 80 ticks of 25 ms and gave up, and 267 of them sit in a message of several calls.
The call of a message that holds several is judged only once its message has reached the transcript, which for the early calls of a message still being written takes longer than the bound.
The same band shows 0 calls at 2 seconds in the earlier sample, so the time is the wait's.
Those calls pay the full bound and are never judged; the nudge of such a message comes from the last call that reaches the transcript in time.

## Recommendation

- Keep the batching hook.
  Its delivery works, it costs no tokens and about 3 seconds of a run, and the earlier case for removing it rested on a gap that no longer exists.
- Do not read the nudge as proven.
  The next message batched after 42% of the nudged breach turns and 22% of the silent ones; the counts are 19 and 18.
  Re-measure when the window holds about 100 edit follows and the tapestry recipe, which this window lacks; the scripts take a new `slugs.txt` unchanged.
- A shorter wait for a call of a multi-call message is the one cost to look at.
  Half of the calls above the line wait the full two seconds for no judgment; the change is to the bound, not to the hook's rules, and is not filed here.

## Limits

- The window is about a day of landings, the sample is darn runs only, and 11 of the 39 runs have no session after the cutoff.
- Both samples differ in recipe mix and in the work done; a total across them is a pointer, and the darn column the fairer pair.
- The nudge comparison has 19 and 18 turns; its sign is not a finding.
- The latency reading takes the line of the call's message as the hook's start, and attributes the difference between the bands to the wait because the earlier sample, which had no wait, shows none.
  A slow tool call at 2 seconds or more is counted as the wait.
- The lookup count against firings is an upper bound of what the hook could see; the class reading counts searches the hook's table does not.
- Slice firings have no breach-turn count, so their coverage is not read.
- A batched next message is any message of 2 or more calls.
- The classification of a Bash call as a lookup, an edit or a check is the turn audit's heuristic, applied unchanged.
