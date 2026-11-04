# Webster fork read audit

Generated: 2026-11-04T14:15:14Z

- Arguments: `--runs batten-remote-order\,decision-record-on-board\,shed-sub-run-block\,cold-gate-fixer --out /home/knatte/Code/loomyard-LYMHUB/turn-savings-measure/.scratch/turn-savings-measure/new/forkaudit.md --work /home/knatte/Code/loomyard-LYMHUB/turn-savings-measure/.scratch/turn-savings-measure/new/forkaudit-work `
- Projects: `/home/knatte/.claude/projects`
- Hub: `/home/knatte/Code/loomyard-LYMHUB`
- Work: `/home/knatte/Code/loomyard-LYMHUB/turn-savings-measure-weft/.lym/scratch/turn-savings-measure/new/forkaudit-work`
- Records: `/home/knatte/Code/loomyard-LYMHUB/turn-savings-measure-weft`
- Project directories matched:
  - `-home-knatte-Code-loomyard-LYMHUB-batten-remote-order`
  - `-home-knatte-Code-loomyard-LYMHUB-decision-record-on-board`
  - `-home-knatte-Code-loomyard-LYMHUB-shed-sub-run-block`
  - `-home-knatte-Code-loomyard-LYMHUB-cold-gate-fixer`
- Candidates found: 152
- Transcripts holding the job heading: 11
- Forks kept: 11
- Transcript lines dropped as unparsable: 0

## Samples

| arm | forks | runs | cards | cross-checked |
| --- | --- | --- | --- | --- |
| none | 11 | 3 | 32 | 0 |

Arms with no forks: card, file, raf, inline, unknown.

## Pack use

| metric | none |
| --- | --- |
| pack file opened | n/a |
| pack read in assistant turn, median | n/a |
| pack read before the first edit | n/a |
| distinct spans per fork, median | n/a |
| distinct spans per card, median | n/a |
| spans reached before the first edit, strict share, median | n/a |
| spans reached before the first edit, strict share, pooled | n/a |
| span files touched before the first edit, file-level share, median | n/a |
| span files touched before the first edit, file-level share, pooled | n/a |
| spans reached in the first work turn, strict share, pooled | n/a |
| spans reached in the first work turn, file-level share, pooled | n/a |
| forks reaching half their spans (strict) before the first edit | n/a |

## Arm comparison

| metric | none |
| --- | --- |
| exploration calls, raw, median per card | 18 |
| exploration calls, raw, pooled per card | 16.06 |
| exploration calls, net, median per card | 18 |
| exploration calls, net, pooled per card | 16.06 |
| whole-file reads, raw, median per card | 5.67 |
| whole-file reads, raw, pooled per card | 5.91 |
| whole-file reads, net, median per card | 5.67 |
| whole-file reads, net, pooled per card | 5.91 |
| partial reads, median per card | 8.5 |
| partial reads, pooled per card | 8.19 |
| greps, median per card | 9 |
| greps, pooled per card | 8.34 |
| member greps, median per card | 5 |
| member greps, pooled per card | 6 |
| quarry lookups, median per card | 0.33 |
| quarry lookups, pooled per card | 0.38 |
| calls before the first edit, net, median per card | 10.67 |
| calls before the first edit, net, pooled per card | 11.44 |
| turns, median per card | 15 |
| turns, pooled per card | 13.78 |
| output tokens, median per card | 24393 |
| output tokens, pooled per card | 23812.44 |
| fresh input tokens, median per card | 80907.5 |
| fresh input tokens, pooled per card | 78644.56 |
| cache reads, median per card | 3299329.75 |
| cache reads, pooled per card | 3102808.69 |
| forks with 3+ parallel calls in the first work turn, share | 55% |

## Exploration quartiles

| arm | n | min | q1 | median | q3 | max |
| --- | --- | --- | --- | --- | --- | --- |
| none | 11 | 8.67 | 13.33 | 18 | 22.5 | 75 |

## Per-fork

| arm | run | agent | start | cards | pack opened | pack turn | pack before edit | first work turn | first parallel | spans | strict before edit | file-level before edit | strict in first turn | calls before edit | net before edit | net exploration | net whole reads | partial reads | greps | quarry | turns | output k | cache read M | edits | edits to no-span files | dropped lines | batch | trace | cross-check | card packs |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| none | batten-remote-order | agent-ad0bcb92cf8ad6c29 | 11-03T09:53 | 4 | no | - | no | 2 | 4 | 0 | 0 | 0 | 0 | 16 | 16 | 36 | 19 | 12 | 24 | 2 | 60 | 88 | 13.2 | 70 | 70 | 0 | - | - | - | - |
| none | batten-remote-order | agent-a2b5bec2de69f2f56 | 11-03T10:19 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 30 | 30 | 45 | 20 | 23 | 33 | 1 | 36 | 85 | 9.2 | 52 | 52 | 0 | - | - | - | - |
| none | batten-remote-order | agent-a620465628d6a503c | 11-03T10:35 | 3 | no | - | no | 2 | 3 | 0 | 0 | 0 | 0 | 49 | 49 | 54 | 17 | 27 | 21 | 1 | 47 | 89 | 13.9 | 65 | 65 | 0 | - | - | - | - |
| none | batten-remote-order | agent-a56eda91227d880e7 | 11-03T11:10 | 3 | no | - | no | 2 | 3 | 0 | 0 | 0 | 0 | 32 | 32 | 59 | 18 | 30 | 52 | 1 | 56 | 113 | 16.1 | 27 | 27 | 0 | - | - | - | - |
| none | batten-remote-order | agent-a563283c1d0574679 | 11-03T11:34 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 44 | 44 | 47 | 22 | 17 | 18 | 1 | 29 | 81 | 9.8 | 71 | 71 | 0 | - | - | - | - |
| none | decision-record-on-board | agent-a5f389e66723d2324 | 11-03T08:25 | 6 | no | - | no | 2 | 6 | 0 | 0 | 0 | 0 | 24 | 24 | 52 | 17 | 23 | 32 | 4 | 58 | 66 | 12.2 | 61 | 61 | 0 | - | - | - | - |
| none | decision-record-on-board | agent-ad539af83a47531ff | 11-03T08:41 | 3 | no | - | no | 2 | 3 | 0 | 0 | 0 | 0 | 31 | 31 | 40 | 7 | 28 | 33 | 1 | 40 | 49 | 7 | 53 | 53 | 0 | - | - | - | - |
| none | shed-sub-run-block | agent-a8af52b2352667b10 | 11-03T08:29 | 4 | no | - | no | 2 | 4 | 0 | 0 | 0 | 0 | 23 | 23 | 38 | 24 | 8 | 8 | 0 | 29 | 63 | 5 | 63 | 63 | 0 | - | - | - | - |
| none | shed-sub-run-block | agent-ac5e3542af19b37ab | 11-03T08:41 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 17 | 17 | 39 | 29 | 5 | 18 | 0 | 31 | 49 | 4.9 | 53 | 53 | 0 | - | - | - | - |
| none | shed-sub-run-block | agent-ac72ed2ecd848e522 | 11-03T08:49 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 73 | 73 | 75 | 5 | 81 | 8 | 0 | 28 | 50 | 4.2 | 91 | 91 | 0 | - | - | - | - |
| none | shed-sub-run-block | agent-a12bf991a2787fc97 | 11-03T08:57 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 27 | 27 | 29 | 11 | 8 | 20 | 1 | 27 | 30 | 3.8 | 38 | 38 | 0 | - | - | - | - |
