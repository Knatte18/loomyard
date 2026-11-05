# Webster fork read audit

Generated: 2026-11-05T19:17:29Z

- Arguments: `--runs raddle-recipe\,pawl-orch-landing --out /home/knatte/Code/loomyard-LYMHUB/batching-hook-remeasure/.scratch/batching-hook-remeasure/new/forkaudit.md --work /home/knatte/Code/loomyard-LYMHUB/batching-hook-remeasure/.scratch/batching-hook-remeasure/new/forkaudit-work `
- Projects: `/home/knatte/.claude/projects`
- Hub: `/home/knatte/Code/loomyard-LYMHUB`
- Work: `/home/knatte/Code/loomyard-LYMHUB/batching-hook-remeasure-weft/.lym/scratch/batching-hook-remeasure/new/forkaudit-work`
- Records: `/home/knatte/Code/loomyard-LYMHUB/batching-hook-remeasure-weft`
- Project directories matched:
  - `-home-knatte-Code-loomyard-LYMHUB-raddle-recipe`
  - `-home-knatte-Code-loomyard-LYMHUB-pawl-orch-landing`
- Candidates found: 142
- Transcripts holding the job heading: 15
- Forks kept: 15
- Transcript lines dropped as unparsable: 0

## Samples

| arm | forks | runs | cards | cross-checked |
| --- | --- | --- | --- | --- |
| none | 15 | 2 | 45 | 0 |

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
| exploration calls, raw, median per card | 19.5 |
| exploration calls, raw, pooled per card | 19.13 |
| exploration calls, net, median per card | 19.5 |
| exploration calls, net, pooled per card | 19.13 |
| whole-file reads, raw, median per card | 6.67 |
| whole-file reads, raw, pooled per card | 6.89 |
| whole-file reads, net, median per card | 6.67 |
| whole-file reads, net, pooled per card | 6.89 |
| partial reads, median per card | 8 |
| partial reads, pooled per card | 8.62 |
| greps, median per card | 13 |
| greps, pooled per card | 12.11 |
| member greps, median per card | 9.5 |
| member greps, pooled per card | 9.16 |
| quarry lookups, median per card | 0.67 |
| quarry lookups, pooled per card | 0.49 |
| calls before the first edit, net, median per card | 9 |
| calls before the first edit, net, pooled per card | 10.51 |
| turns, median per card | 16.75 |
| turns, pooled per card | 15.13 |
| output tokens, median per card | 28515 |
| output tokens, pooled per card | 28817.29 |
| fresh input tokens, median per card | 96349.75 |
| fresh input tokens, pooled per card | 89550.64 |
| cache reads, median per card | 4311380.5 |
| cache reads, pooled per card | 3986663 |
| forks with 3+ parallel calls in the first work turn, share | 47% |

## Exploration quartiles

| arm | n | min | q1 | median | q3 | max |
| --- | --- | --- | --- | --- | --- | --- |
| none | 15 | 9.67 | 15.2 | 19.5 | 30.33 | 80 |

## Per-fork

| arm | run | agent | start | cards | pack opened | pack turn | pack before edit | first work turn | first parallel | spans | strict before edit | file-level before edit | strict in first turn | calls before edit | net before edit | net exploration | net whole reads | partial reads | greps | quarry | turns | output k | cache read M | edits | edits to no-span files | dropped lines | batch | trace | cross-check | card packs |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| none | pawl-orch-landing | agent-a6b0d7e7bc0e4a444 | 11-05T08:55 | 4 | no | - | no | 2 | 4 | 0 | 0 | 0 | 0 | 23 | 23 | 51 | 22 | 19 | 26 | 2 | 35 | 72 | 7 | 52 | 52 | 0 | - | - | - | - |
| none | pawl-orch-landing | agent-a02a4ee6ec45ea151 | 11-05T09:05 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 51 | 51 | 54 | 12 | 29 | 18 | 1 | 27 | 56 | 5.8 | 71 | 71 | 0 | - | - | - | - |
| none | pawl-orch-landing | agent-a84221366474b3280 | 11-05T09:14 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 39 | 39 | 41 | 11 | 21 | 12 | 1 | 24 | 52 | 5.2 | 64 | 64 | 0 | - | - | - | - |
| none | pawl-orch-landing | agent-a185e1a2c6a52a383 | 11-05T09:20 | 3 | no | - | no | 2 | 3 | 0 | 0 | 0 | 0 | 35 | 35 | 91 | 34 | 36 | 64 | 2 | 58 | 100 | 18.1 | 70 | 70 | 0 | - | - | - | - |
| none | pawl-orch-landing | agent-a55b12b082b9c295c | 11-05T09:35 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 15 | 15 | 39 | 15 | 16 | 26 | 2 | 34 | 104 | 8.6 | 75 | 75 | 0 | - | - | - | - |
| none | pawl-orch-landing | agent-a0b6fe928b48c5e24 | 11-05T09:50 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 36 | 36 | 41 | 17 | 26 | 39 | 0 | 40 | 80 | 10.6 | 47 | 47 | 0 | - | - | - | - |
| none | pawl-orch-landing | agent-ae823dda84ccf8e5f | 11-05T10:04 | 5 | no | - | no | 2 | 5 | 0 | 0 | 0 | 0 | 29 | 29 | 76 | 28 | 26 | 49 | 4 | 64 | 135 | 20 | 86 | 86 | 0 | - | - | - | - |
| none | raddle-recipe | agent-a1497362e7a69934e | 11-05T10:41 | 4 | no | - | no | 2 | 4 | 0 | 0 | 0 | 0 | 14 | 14 | 61 | 21 | 25 | 30 | 4 | 57 | 65 | 13.1 | 54 | 54 | 0 | - | - | - | - |
| none | raddle-recipe | agent-a06b45e3e5cfdd540 | 11-05T10:53 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 23 | 23 | 48 | 11 | 24 | 39 | 1 | 42 | 57 | 11.6 | 45 | 45 | 0 | - | - | - | - |
| none | raddle-recipe | agent-ab8364109d32bf43c | 11-05T11:05 | 6 | no | - | no | 2 | 6 | 0 | 0 | 0 | 0 | 20 | 20 | 78 | 40 | 25 | 47 | 0 | 74 | 136 | 21 | 46 | 46 | 0 | - | - | - | - |
| none | raddle-recipe | agent-a40411bce4ae38514 | 11-05T11:56 | 6 | no | - | no | 2 | 6 | 0 | 0 | 0 | 0 | 39 | 39 | 58 | 20 | 36 | 36 | 0 | 64 | 127 | 16.8 | 85 | 85 | 0 | - | - | - | - |
| none | raddle-recipe | agent-a08da3d042407ae6e | 11-05T12:18 | 4 | no | - | no | 2 | 4 | 0 | 0 | 0 | 0 | 36 | 36 | 73 | 27 | 39 | 59 | 1 | 67 | 144 | 20.9 | 28 | 28 | 0 | - | - | - | - |
| none | raddle-recipe | agent-ad8d43df85ad9d528 | 11-05T12:38 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 78 | 78 | 80 | 27 | 38 | 48 | 1 | 41 | 95 | 11.7 | 81 | 81 | 0 | - | - | - | - |
| none | raddle-recipe | agent-a548880b9feed8ec4 | 11-05T12:50 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 9 | 9 | 30 | 12 | 15 | 17 | 2 | 27 | 35 | 4.4 | 47 | 47 | 0 | - | - | - | - |
| none | raddle-recipe | agent-a0714fd344033b447 | 11-05T12:55 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 26 | 26 | 40 | 13 | 13 | 35 | 1 | 27 | 39 | 4.7 | 32 | 32 | 0 | - | - | - | - |
