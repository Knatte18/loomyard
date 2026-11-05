# Glyph use audit 2: read-all-first (793c6a8ee) vs earlier Webster forks

Method: the previous audit's `extract.jq` (copied to `extract2.jq`, pack parsing changed for the per-card pack, edit and first-turn fields added), over every Webster implementer fork transcript from 2026-10-08 to 2026-10-13.
The binary a Webster ran on is read from the fork prompt text, because the task worktrees and their trace logs are gone: `raf` = the prompt holds the read-all-first FRESH-READ rule, `mid` = per-card pack without it, `base_on` = pre-`113593990` pack, `base_off` = no pack.
All forks are `claude-sonnet-5-5`.
Files: `raw.jsonl`, `raw-pack.jsonl`, `table.tsv`, `turns.tsv`, `extract2.jq`, `stats2.jq`, `stats3.jq`.

## Sample sizes

| Arm | Forks | Runs | Cards | Span |
| --- | --- | --- | --- | --- |
| base_off (no pack) | 114 | 43 | 512 | 10-07 .. 10-11 |
| base_on (old pack, before 113593990) | 41 | 15 | 180 | 10-11 .. 10-12 11:30Z |
| mid (113593990, no read-all-first) | 9 | 3 | 42 | 10-12 |
| raf (793c6a8ee or later) | 29 | 13 | 125 | 10-12 13:11Z .. 10-13 |

No pack-off forks ran after 10-11.
`mid` is shown for reference and is left out of the verdict.

## Medians per fork (per card = fork value / its cards)

| Metric | base_off | base_on | mid | raf |
| --- | --- | --- | --- | --- |
| Calls before first edit (raw) | 7 | 7 | 7 | 8 |
| Exploration calls / card | 5.2 | 5.8 | 6.0 | 5.8 |
| Whole-file reads / card | 3.5 | 2.8 | 2.6 | 2.0 |
| Partial reads / card | 8.3 | 9.3 | 6.5 | 7.8 |
| Total reads / card | 12.3 | 12.0 | 10.3 | 9.8 |
| Greps / card | 9.9 | 9.0 | 6.8 | 7.7 |
| Member greps / card | 6.5 | 5.5 | 4.0 | 5.0 |
| Quarry lookups / card | 0 | 0 | 0 | 0 |
| Turns / card | 17.4 | 18.2 | 17.3 | 14.7 |
| Output tokens / card | 13.2k | 13.6k | 12.6k | 11.6k |
| Fresh input tokens / card | 40.7k | 43.0k | 41.3k | 39.1k |
| Cache read / card | 2.78M | 3.24M | 2.85M | 2.40M |

Pooled per card (sum over forks / sum of cards): exploration calls 5.9 / 5.9 / 6.9 / 5.5; whole reads 3.7 / 3.0 / 2.7 / 2.2; total reads 13.1 / 13.5 / 11.5 / 10.6; turns 18.4 / 20.3 / 16.6 / 16.0.
Quarry: 0 forks in base_on, mid and raf; 19 of 114 base_off forks used it (the previous note's "never" holds for pack-on only).

## Pack quality (from the prompt, per card; refs listed under several cards count each time)

- raf: median 31 resolved and 13 unresolved entries per fork (7.2 and 3.5 per card); 35% of entries unresolved.
- mid: median 28 and 11 per fork; 36% unresolved.
- base_on (batch-level pack, previous audit's parse): median 29 resolved and 4 unresolved.
- The unresolved entries are almost all `plan:` handles ("names a member a card creates and does not resolve yet", about 96%), ambiguous refs are rare.
  The rise in unresolved over base_on is the per-card counting plus plan handles, not worse resolution.
- No fork had an empty pack.
- The `webster: kickstart pack` trace lines themselves are gone with the torn-down worktrees, so distinct-ref counts per batch could not be taken.

## Did forks follow read-all-first

- First work turn after the prompt read, tool calls in parallel: raf median 3 (15 of 29 forks with 3 or more, 9 of 29 serial with one call); base_on median 1 (7 of 41 with 3 or more); base_off median 1 (17 of 114).
- Turns before the first edit: median 5 in raf, 5 in base_on, 6 in base_off.
- Forks batch the card files in one `cat` loop and merge adjacent spans into a few wide `offset`/`limit` reads (for example `1/260`, `440/360`) rather than one read per span, so the call count before the first edit barely rises (8 vs 7).

## Edits outside the listed spans

Worktrees are torn down, so only file level is checkable: edits (Edit or MultiEdit) to a file that has no span in the pack.

| Arm | Edits | Edits to files with no listed span | Forks with any such edit |
| --- | --- | --- | --- |
| base_on | 3498 | 1341 (38%) | 39 of 41 |
| mid | 736 | 235 (32%) | 9 of 9 |
| raf | 2113 | 585 (28%) | 28 of 29 |

Every fork edits some unlisted file (new files, docs, tests); the share is lower under raf, but the file level cannot say whether edits inside a span file fell outside the span, so this is not a stale-read measure.
Writes (whole-file) to a file with no span: all of them in every pack arm, as expected for new files.

## Verdict

Read-all-first changed how forks read, not how much they explore.
Exploration calls per card are unchanged (5.8 raf vs 5.8 base_on, 5.2 base_off) and calls before the first edit are 8 vs 7.
What moved is the read mix and cost: about 20% fewer reads per card (9.8 vs 12.3 off, 12.0 on), whole-file reads down from 3.5 to 2.0 per card, greps about 20% lower, and turns per card 14.7 vs 17.4-18.2, with cache read 2.4M vs 2.8-3.2M per card (about 15-25% lower) and output tokens about 10% lower.
These are medians over 29 forks in 13 runs against runs of mixed task size, with no significance test, so treat the cost drop as suggestive; the exploration result (no change) is solid.
Compliance is partial: 15 of 29 forks read in parallel in the first turn, 9 went serial, and the spans were merged into wide reads.
Forks still never use quarry lookups.

## Cheap next step

Try the end-of-prompt variant from the note: one prompt whose last section is the one-turn instruction followed by the pack, then re-run this audit unchanged (`extract2.jq`, `stats2.jq`, `turns.tsv` recipe).
Target the 9 serial-first-turn forks: the success measure is the share of forks with 3 or more parallel reads in the first work turn (15 of 29 now), plus exploration calls per card.
The two-prompt variant costs a turn per fork, so try it only if the end-of-prompt variant leaves the serial share high.
Do not build the Burler side on this result yet: the exploration question is still open, only the reading mix improved.
