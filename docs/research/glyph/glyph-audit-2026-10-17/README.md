# Webster per-card kick-start pack audit, 2026-10-17

The fourth glyph use audit, a follow-up to `../glyph-audit-2026-10-15/`, and the first on the per-card pack layout (arm `card`: a fork fetches each card's pack with `lyx loom card-pack`), the default since `aa925bdcc`.
`report.md` is the raw output of `tools/forkaudit/forkaudit.sh`; this file holds the audit agent's summary and verdict, written by the orch from the agent's returned text.

## Reproduce

```
mkdir -p .scratch/fork-audit-4
tools/forkaudit/forkaudit.sh --since '2026-10-15 14:10' --pack-check --plan --out .scratch/fork-audit-4/report.md --work .scratch/fork-audit-4/work
```

The intermediates in `--work` are not kept; the transcripts under `~/.claude/projects/` they come from are machine-local.

## Sample

- Card arm: 90 forks, 26 runs, 199 cards, started 2026-10-15 12:20Z to 2026-10-17 12:06Z.
- Five forks fell in arm `none` (their prompts match no marker row) and are left out.
- `pf` and `raf` are from the 2026-10-15 report: 24 forks, 10 runs, 93 cards and 31 forks, 14 runs, 134 cards.

## Comparison

Medians per card unless noted.

| Metric | card | pf | raf |
| --- | --- | --- | --- |
| Net calls before first edit | 4 | 8.5 | 8 |
| First work turn with 3+ parallel calls | 41% | 29% | 52% |
| Turns | 24 | 19.5 | 15 |
| Output tokens | 19.7k | 16.0k | 12.6k |
| Cache read | 3.39M | 3.14M | 2.50M |
| Net exploration calls | 7 | 4.8 | 5.8 |
| Spans reached before first edit | 52% | 24% | 17% |
| Forks reaching at least half their spans before first edit | 46 of 87 | 6 of 24 | 3 of 31 |

The card arm's parallel figure counts card-file reads, not span reads, and its "spans reached" counts spans read after the `card-pack` call listing them and before the next edit, so both rows compare loosely.

## Pack correctness

First `card-pack` call per fork, resolved at batch start: 1680 spans, 1620 correct, 41 correct as declaration only, 0 stale, wrong or missing file, 19 with no tree to check against.
Spans from later `card-pack` calls are not graded, so drift during a batch is still unmeasured.

## Missed files

873 of 4281 edits (20%) went to files with no span, against 18% (`pf`) and 27% (`raf`).
Three forks had no spans at all, among them a creel-core fork whose 81 edits all went to no-span files.
Only the first `card-pack` call is graded, so files a later card's pack covered count as missed: 20% is an upper bound (open finding from `aa925bdcc`).

## Verdict

Card forks fetch their pack and read about half its spans before their first edit, against a quarter or less in the older arms, and they start editing after half as many calls.
The pack is correct at batch start.
About a fifth of edits still go to files with no span, likely overstated by the later-card grading gap.
Cost per card is higher than both older arms, but those were measured on different tasks, so this sample cannot separate layout from workload.
Too small or not measured: the cost difference, the parallelism difference (different metric), the effect of the two open tool findings, mid-batch drift, anything per run type; 90 forks from 26 runs, several of them large, with no significance test.
