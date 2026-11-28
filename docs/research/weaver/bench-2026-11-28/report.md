# Weaver against Webster, run again on one landed plan

2026-11-28.
The figures record runs on one machine and are not claiming to stay true.
The first bench is [bench-2026-11-27](../bench-2026-11-27/report.md); "11-27" in the tables is that bench.

## Result

Only `board-viewer-server` was run.
The operator parked the bench after it, so `launch-manifest-parity` and `gate-test-map-mode` were not run again.

Weaver and Webster both committed all nine cards, passed the plan-level verify on its first run and had no stop.
Weaver took 18% longer (114m29s against 97m20s) and cost 7% less ($11.39 against $12.28).
Its result drew 26 review findings against Webster's 22, with two blocking on each side.
The first bench's same-day pair of the same plan took the same time, with Weaver 18% cheaper, Webster's stop left out,
and its Weaver result drew half as many findings.

**Recommendation:** keep Webster as tapestry's default implementer, and switch Weaver on per task.

- The one saving that held across both benches is dollars, and it fell from 18% to 7%.
- Review findings point opposite ways in the two benches, so they decide nothing.
- Two of the three plans have still not had a same-day pair.
  Run them before changing the default.

## How the runs were made

As in the first bench: a worktree pair per run, the base commit `6d3a916f4`'s tree committed on top of a branch,
the landed run's archived plan and decision record copied in, the run moved straight to the implementer row,
and a pause after that row.
Weaver ran its default ladder (`sonnet[medium]`, `opus[medium]`, `opus[high]`), Webster its default models.

| Run | Implementer | Result head | Started, CET | Load average at start (1, 5, 15 min) |
|---|---|---|---|---|
| `b1` | Weaver | `0d2a26d4b` | 16:28:53 | about 40, 37, 34 |
| `b2` | Webster | `90d893bd4` | 16:29:00 | about 40, 37, 34 |

Both started on `lym` built from `af3813927`.
The 12-core machine's sampled one-minute load was between 14 and 47 while they ran, and other runs of the hub worked at the same time.

### Questions, pauses and stops

- Each run asked once how cards 1–3 could pass the test-only-code guard, and both got the same answer:
  temporary allow-list entries with a `Pattern-Departure` trailer.
  Weaver's strand waited 11 s for it in its session.
  Webster's first fork returned `waiting`, and the Master forked the batch a second time.
- A pause of the bench was asked for at about 17:03, to take effect at each run's next batch end.
  It was withdrawn at about 17:07, before either run had been paused.
- Neither run reached the stop chain.

### Limits

- One plan, one run per implementer, one review per result.
- `lym` was rebuilt at 18:19:42, from a commit that narrows the card gate.
  Webster had ended, and Weaver's last batch had ended; its plan-level verify was running.
- The two runs shared two gate slots with each other and with other runs, so wall time includes uneven waits for a slot.
- The archived decision record is the landed run's final one.
  It holds the entries added while that run ran, and names the findings its review left open.
- Each pair's history holds the landed solution.

## The first bench's changes

| Change | Shows in this run |
|---|---|
| A Weaver attempt has its own timeout, `weaver.attempt_timeout_min` | Yes. No attempt was killed. Batches 1 and 2 ran 56 and 34 minutes, past the 30 of `shuttle.run_timeout_min`, on the first rung |
| `summary.md` names deviations and departures | Yes. Each batch lists the files changed outside its targets and its departures |
| The Weaver row records gate spans | Yes. One `verify` span of 246 s |

The paths the first bench did not reach stayed unreached: a lost strand, a resume, a failed plan-level verify and the third rung.

## Wall time

| Plan | Recorded Webster | 11-27 Webster | 11-27 Weaver | Webster | Weaver |
|---|---|---|---|---|---|
| `board-viewer-server` | 63m00s | 89m28s | 91m57s | 97m20s | 114m29s |

Batch time runs from a batch's start to the next batch's start.
The last batch ends where the plan-level verify starts, or, for Weaver, where its last session ends.

| Run | Batch 1 | Batch 2 | Batch 3 | Verify and close | Row |
|---|---|---|---|---|---|
| Webster | 40m54s | 23m17s | 28m24s | 4m07s | 97m20s |
| Weaver | 56m01s | 33m49s | 20m02s | 4m31s | 114m29s |

Gate times are the summed wall time of the tier runs, in seconds, from the gate run records.
Slot wait is the time those calls queued before running, and is not part of the wall time.
"Direct" is tier runs a session started outside a keyed gate; it is not in the sum.

| Run | Card | Batch | Plan-level | Sum | Direct | Slot wait, keyed | Slot wait, direct | Fixer sessions | Fixer time |
|---|---|---|---|---|---|---|---|---|---|
| Webster | 2050 | 466 | 198 | 2714 | 3 | 1007 | 104 | 7 | 964 s |
| Weaver | 2189 | 311 | 204 | 2704 | 42 | 1306 | 626 | 9 | 1388 s |

The gates ran for the same time on both sides.
Weaver's calls queued 821 s longer for a slot, direct calls included, and its fixers ran 424 s longer.
The row difference is 1029 s.

## Cost

Counted from the session transcripts, subagents included, one count per message, at the prices in `claudeengine/pricing.go`.
All models were Sonnet.

| Run | Input | Cache write | of which 1-hour | Cache read | Output | Dollars | Of which gate fixers |
|---|---|---|---|---|---|---|---|
| Webster | 426 | 1,231,919 | 804,147 | 28,380,034 | 231,975 | $12.28 | $1.45 |
| Weaver | 402 | 1,748,866 | 0 | 23,012,516 | 241,738 | $11.39 | $1.95 |

### Where the saving comes from

Without the gate fixers, Weaver cost $9.44 and Webster $10.83.
Of the $1.39 between them, cache reads are $1.26, cache writes $0.14, and output is $0.02 the other way.

Cache reads per session, Weaver's batches against Webster's forks.
"Inherited" is the context a session starts with, multiplied by its calls.

| Batch | Weaver: calls | start context | cache read | Webster: fork | calls | start context | cache read |
|---|---|---|---|---|---|---|---|
| 1 | 40 | 15k | 9,891,551 | first, ended `waiting` | 19 | 102k | 3,879,685 |
| | | | | second | 32 | 106k | 7,215,659 |
| 2 | 35 | 15k | 6,669,083 | | 29 | 110k | 6,327,865 |
| 3 | 27 | 15k | 3,456,403 | | 28 | 112k | 6,581,023 |
| Master | — | | — | | 27 | 15k | 2,310,886 |
| Total | 102 | | 20,017,037 | | 135 | | 26,315,118 |

| Part of the cache reads | Weaver | Webster | Webster minus Weaver |
|---|---|---|---|
| Master session | 0 | 2,310,886 | 2,310,886 |
| Inherited context | 1,536,638 | 11,654,877 | 10,118,239 |
| Context grown in the batch session | 18,480,399 | 12,349,355 | −6,131,044 |
| Total | 20,017,037 | 26,315,118 | 6,298,081 |

- The cold start works as intended: a Webster fork carries about 100k tokens of the Master's context into every call.
- Weaver gives back 60% of that, because its batch sessions grow more than the forks do, up to 401k tokens in batch 1.
- The Master's own reads are 37% of the net difference.
- Webster's second fork of batch 1 followed from its question.
  Batches 2 and 3, which had no question, read 10.1M tokens on Weaver against 12.9M on Webster, and went one each way.

Weaver's cache writes were all 5-minute writes, and the forks' all 1-hour writes.
Twice a Weaver session wrote its whole context again after about five minutes without a call:
270,009 tokens in batch 1 after 291 s, and 194,714 tokens in batch 3 after 302 s.
Those two rewrites are $1.16, and no Webster fork had one after a wait.
Writing all of Weaver's cache for 1 hour instead would not pay here:
the higher write price outweighs the two rewrites by about $0.24.
Keeping each wait under the 5-minute cache lifetime would save them.

## Ladder and stops

| Run | Attempts | Climbed past the first rung | Stops reaching the stop chain | Questions |
|---|---|---|---|---|
| Weaver | 3 | 0 | 0 | 1 |
| Webster | — | — | 0 | 1 |

## Quality

"Files outside targets" is the scope guard's deviation count, summed over the batches.
Departures are `Pattern-Departure` trailers; for the recorded run they are what its summary reports.

| Run | Plan-level verify | Verify fixer rounds | Cards committed | Commits | Files outside targets | Departures |
|---|---|---|---|---|---|---|
| Recorded Webster | passed | 0 | 9/9 | — | 16 | 0 |
| Webster | passed | 0 | 9/9 | 11 | 17 | 4 |
| Weaver | passed | 0 | 9/9 | 9 | 22 | 3 |

Weaver's card 8 added a temporary allow-list entry that card 9 removed, with no `Pattern-Departure` trailer,
and its summary lists no departure for that batch.
Webster's card 8 added the same entry with the trailer.

"Go added" is production / test lines.

| Diff | Files | Lines added / removed | Go added | Files shared with landed |
|---|---|---|---|---|
| Landed (`7bdb031ed`) | 63 | 6048 / 87 | 2221 / 2867 | — |
| Webster | 60 | 5158 / 145 | 2004 / 2232 | 54 |
| Weaver | 64 | 5180 / 150 | 2062 / 2418 | 59 |

### Review findings

One Burler round per result: a `sonnet[medium]` reviewer with the code lens fan, over the commits above the base,
with the archived decision record and plan as the answer key.
Each round's fixer then ran in its pair; the result heads and diffs above are from before it.

| Run | Verdict | Blocking | Medium | Low | Nit | Total |
|---|---|---|---|---|---|---|
| Recorded Webster | blocking | 1 | 3 | 8 | 0 | 12 |
| 11-27 Webster | blocking | 3 | 11 | 6 | 0 | 20 |
| 11-27 Weaver | blocking | 1 | 4 | 4 | 1 | 10 |
| Webster | blocking | 2 | 8 | 12 | 0 | 22 |
| Weaver | blocking | 2 | 10 | 11 | 3 | 26 |

The recorded round was reviewed by older prompts on another day.
Weaver's count went from 10 to 26 between the two benches, on the same plan and answer key.

## Findings per run

The full reviews are in `reviews/`.

Both results have these:

- repeated literals (blocking);
- a page action that moves an unsent edit onto a new revision, so the next Save overwrites another writer;
- a failed server start that leaves no reason in the log;
- a write route that creates projects;
- a stale promote and the verb wiring without tests.

The archived decision record names four findings its landed run's review left open.
Webster's result repeats three: the watch loop's missing floor, `board.html` not pinned to LF and untested verb wiring.
Weaver's repeats one, the verb wiring.

### Weaver ([review](reviews/weaver-board-viewer-server.md))

- Blocking: the opener's exit is not logged.
- Medium, in the page: a vanished project option moves the entry under the root;
  a project dependency is prefilled as a handle the board refuses; one refused board read stops the page's updates for good.
- Medium, elsewhere: a leftover comment admits temporary allow-list entries;
  the own-origin write and the server's registration have no test.

### Webster ([review](reviews/webster-board-viewer-server.md))

- Blocking: the watch loop has no one-second floor.
- Medium: `board.html` is not pinned to LF; the token probe and the watch timer's schedule have no test.
