# What costs Code-Burler-Review its extra minutes

A measurement report: it records one analysis on 2026-11-22 and is not kept current.
It follows `darn-slowdown-measure.md`, which found the Code-Burler-Review row up 2.8 min at the median between
2026-11-16/17 and 2026-11-21/22 and did not time which input costs it.

## Result

The extra minutes are the assistant reviewer's wait, not an input the chair reads.
The assistant is a second reviewer on the Codex engine that runs beside the chair and that the chair must wait for
before it writes the review.
Its first run in the archive started 2026-11-17 16:02 UTC,
so 6 of the 34 earlier runs had one and all 25 later runs did.

Per darn run, mean over every Code-Burler-Review round, in seconds:

| | Before | After | Change | Share of a 45 min run |
| --- | --- | --- | --- | --- |
| Row | 386 | 575 | +189 | 7.0 % |
| Chair blocked on the assistant | 37 | 211 | +174 | 6.4 % |
| Chair waiting on its lens forks | 31 | 20 | -11 | -0.4 % |
| Other chair time | 311 | 321 | +10 | 0.4 % |
| Outside the chair session, after it ends | 6 | 23 | +17 | 0.6 % |

The medians are 300 s before and 438 s after, +138 s.
Mean and median differ because a few later runs spent 18 to 24 min in the row over two rounds.

The suspects the entry named do not show as a cost of their own:

- The tautological-test check is a bullet in the code-review rubric, read by the `code-tests` lens fork.
  That fork ran 41 s on the mean before and 39 s after, and was the slowest fork in 3 of 27 later runs.
- The scope lens (`code-scope`, the 0.5 more lens reviews per run) ran 29 s before and 26 s after,
  in parallel with the other forks, and was the slowest fork in none of the 27 later runs.
- The rubric grew from 12 825 to 16 828 bytes between 2026-11-17 and 2026-11-19.
  The chair's time from its start to its first fork was 89 s before and 87 s after.
- Mutation survivors cannot be separated from the size and kind of the diff;
  their share is at most about 1 min per run.
  See Mutation survivors.

## Method

Read-only, over records that already existed; no run was timed and no agent was run.

- Runs: every `darn` run in the archive tags (`refs/tags/archive/<slug>/<sha>` of `loomyard-weft`) dated
  2026-11-16 and 2026-11-17 against 2026-11-21 and 2026-11-22 that has a Code-Burler-Review row.
  That is 61 runs, 34 before (8 on 2026-11-16, 26 on 2026-11-17) and 27 after.
  Two later runs, `burler-fix-round-defects` and `harden-sandbox-fabric-verbs`,
  sat paused over 3 h in the row and are left out.
  The sample is every qualifying run, not a draw of about 15 per period, since all of them were readable.
- A row's time is its `history` entry's `at` minus `started_at` in `_lym/shed/<slug>/status.json`, summed over rounds.
- Round inputs and tokens: `_lym/reviews/code/round-N-usage.yaml`, `round-N-review.md`, `_lym/usage/session-*.json`
  and `_lym/usage/landed.json` in the same tag.
- Trace timestamps: the `trace` logs of `_bolt/.lym/log-archive/<slug>/`,
  which name when the assistant started and ended and when the review step joined.
- Mutation reports: the `gate test: mutation report:` line of the verify log of the last Darn-Write gate.
- The chair's and the forks' own timelines come from their Claude session transcripts,
  `~/.claude/projects/<worktree>/<session>.jsonl` and its `subagents/` files,
  found through the session id in the usage record.
  These sit outside the archive tags.
  They give the tool-call timestamps that split a row into the chair's reading, its fork wait,
  its wait on the assistant and its writing; each wait is a call of `lym shuttle await-forks` or `lym shuttle await <assistant end marker>`.
  Only timestamps and tool names were read.
- Diff size is `lines_added + lines_deleted` of `landed.json`, which counts the whole landing, review fixes included.
- A row's time is a chair session's time plus the time after it ends; the chair session's time matches the row's to
  within 2 s in a run with no assistant.

## Where the time goes in an assisted round

The chair reads the target and spawns its lens forks, then reviews on its own, and reaches the wait for the assistant
before the assistant has finished in every one of the 31 assisted round-1 reviews.
The row then ends one chair writing phase after the assistant ends.
The assistant's run length alone nearly fixes the row's: the correlation of row seconds with assistant seconds is 0.98,
and the row exceeds the assistant's run by 105 s at the median (74 s to 147 s at the quartiles).
In an unassisted round the row does not follow the diff size either way: the correlation with lines changed is -0.22.

One round, `convergence-report-memory`, round 1, whose row took 682 s:

| Time (UTC) | Event |
| --- | --- |
| 11:08:54 | chair starts; assistant starts nine seconds later |
| 11:09:45 to 11:09:58 | chair spawns its four lens forks |
| 11:11:35 | last lens fork ends |
| 11:12:21 | chair has done its own review and blocks on the assistant's end marker |
| 11:16:57 | assistant ends, after 475 s; the chair has been blocked 261 s |
| 11:17:45 to 11:19:58 | chair writes the lens files and the review |
| 11:20:15 | the review step joins, 15 s after the chair's run ended |

The 15 s at the end is the same in all 31 assisted rounds to the second.
The assistant has ended by then.
Its strand is still live, and `RemoveStrand` in `internal/reedengine/strand.go` waits `reapExitTimeout`
(15 s, `internal/reedengine/lifecycle.go`) for its pane's process to exit.
A round with no assistant spends 2 s there.

Lens forks per lens, mean seconds, round 1:

| Lens | Before | After |
| --- | --- | --- |
| `code-correctness` | 68 | 55 |
| `code-affected-set` | 43 | 45 |
| `code-tests` | 41 | 39 |
| `code-scope` | 29 (16 of 34 runs) | 26 (27 of 27 runs) |

The scope lens first ran on 2026-11-17 (`6c642caf2`), mid-way through the earlier period.
The chair's fork wait fell from 31 s to 20 s.
The chair keeps working while its forks run, so a fork has to be the slowest and then outlast the chair's own review
to cost time, and none did.

## Mutation survivors

The mutation note (`6dbc7a058`, 2026-11-19) reaches both the chair and the assistant, since both read the rubric.
Round 1 of the 25 later runs, by what the last Darn-Write gate's mutation report said:

| Mutation report | Runs | Median lines changed | Mean row |
| --- | --- | --- | --- |
| No mutants run (docs, stencils, non-Go) | 7 | 333 | 327 s |
| Mutants run, none survived | 8 | 156 | 383 s |
| At least one survivor | 9 | 1 152 | 620 s |

The row rises with the report, and so does the diff.
A fit of the row on the log of lines and of files changed leaves +175 s for a survivor, with 25 runs;
the assistant's own run carries +116 s of that and the chair's own time +181 s.
A diff with no survivor is no cheaper than its size explains: a run that ran mutants and killed all of them took
longer than one that ran none, on a diff less than half the size.
So the figure holds Go content and survivors together.
The chair names a survivor in a tool call in 3 of the 9 survivor rounds, once each; the cost is not a visible detour.
With survivors in 9 of 25 later runs, the upper bound is 9/25 of 175 s, about 63 s per run, about 2 % of a 45 min run.
It overlaps the assistant row above, since a longer assistant run is what lengthens the row.
This report cannot split it further without a run that holds the diff fixed and changes the note.

## What the assistant returns

In the 31 assisted round-1 reviews, 38 of 212 kept findings carry `origin: assist`, in 19 of the 31 reviews,
19 of them BLOCKING or MEDIUM.
In round 2, 7 of 14 reviews have an assistant report and 10 findings are kept from them.
By diff size, round 1:

| Lines changed | Rounds | Mean wait | Rounds with a kept assist finding | Kept | BLOCKING or MEDIUM |
| --- | --- | --- | --- | --- | --- |
| up to 100 | 7 | 122 s | 1 | 1 | 0 |
| 101 to 500 | 9 | 169 s | 6 | 9 | 5 |
| 501 to 1 500 | 9 | 149 s | 8 | 15 | 6 |
| over 1 500 | 6 | 196 s | 4 | 13 | 8 |

The kept findings are not in the short runs.
Cutting the assistant after 300 s of its run cuts 17 of 31 runs, which hold 33 of the 38 kept findings,
to save 46 s a round;
after 420 s it cuts 4 runs and saves 16 s a round.
The loss is an upper bound, since a cut run may have kept its early findings.
The 2026-11-18 verdict (`docs/research/codex/review/assist-verdict-2026-11-18/report.md`) found 14 real findings
that no Claude seat had made, over 14 assisted rounds, and named the added wait as not recorded;
this report supplies the wait.

## Causes and fixes

Shares are of a 45 min run.

1. **Wait for the assistant**, 174 s a run, 6.4 %.
   The row follows the assistant's run, and the chair is idle while it ends.
   Candidate notes, each a separate change:
   - Skip the assistant on a diff of 100 changed lines or fewer.
     It saves 122 s of wait and 15 s of teardown in 7 of 31 rounds, about 31 s per assisted round,
     and drops 1 kept finding of 38, a LOW.
   - Let the chair write its lens files before it waits, and keep only the merge of the assistant's findings after.
     The lens files take 41 s on the mean and are written after the wait today;
     moving them saves 40 s per assisted round.
     The assistant stencil, `burler-assist-consolidation.md`, says to write nothing to the review file before the wait,
     so it needs a change that keeps the review file itself for after.
   - A time cap on the assistant is a poor trade, per the table above.
2. **Stopping the assistant's strand**, 17 s a run, 0.6 %.
   Candidate note: stop the assistant's pane when the chair reads its end marker as `done`, or force-kill a finished
   assistant at once, so the 15 s wait leaves the chair's critical path.
3. **Mutation survivors**, at most about 1 min a run, 2 %, within the figure above.
   No-survivor diffs show no cost to cap or skip: the note then carries counts only.
   Candidate note: replay two archived survivor diffs, one with the survivor list cut to its first five entries,
   and time both; that needs the timed runs this report was bound out of.
4. **Not causes**: the tautological-test bullet, the scope lens, the rubric's growth and the chair's reading time.

The entry's figure of 2.8 min is the same effect seen on a different sample: on these 59 runs the median rose 2.3 min.
