# Webster fork read audit

Generated: 2026-11-04T14:15:18Z

- Arguments: `--runs warp-visibility\,interactive-recipe\,run-verbs-shared --out /home/knatte/Code/loomyard-LYMHUB/turn-savings-measure/.scratch/turn-savings-measure/baseline/forkaudit.md --work /home/knatte/Code/loomyard-LYMHUB/turn-savings-measure/.scratch/turn-savings-measure/baseline/forkaudit-work `
- Projects: `/home/knatte/.claude/projects`
- Hub: `/home/knatte/Code/loomyard-LYMHUB`
- Work: `/home/knatte/Code/loomyard-LYMHUB/turn-savings-measure-weft/.lym/scratch/turn-savings-measure/baseline/forkaudit-work`
- Records: `/home/knatte/Code/loomyard-LYMHUB/turn-savings-measure-weft`
- Project directories matched:
  - `-home-knatte-Code-loomyard-LYMHUB-warp-visibility`
  - `-home-knatte-Code-loomyard-LYMHUB-interactive-recipe`
  - `-home-knatte-Code-loomyard-LYMHUB-run-verbs-shared`
- Candidates found: 179
- Transcripts holding the job heading: 27
- Forks kept: 27
- Transcript lines dropped as unparsable: 0

## Samples

| arm | forks | runs | cards | cross-checked |
| --- | --- | --- | --- | --- |
| none | 27 | 3 | 52 | 0 |

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
| exploration calls, raw, median per card | 15.5 |
| exploration calls, raw, pooled per card | 14.27 |
| exploration calls, net, median per card | 15.5 |
| exploration calls, net, pooled per card | 14.27 |
| whole-file reads, raw, median per card | 5 |
| whole-file reads, raw, pooled per card | 6.27 |
| whole-file reads, net, median per card | 5 |
| whole-file reads, net, pooled per card | 6.27 |
| partial reads, median per card | 20 |
| partial reads, pooled per card | 23.15 |
| greps, median per card | 17 |
| greps, pooled per card | 17.63 |
| member greps, median per card | 10 |
| member greps, pooled per card | 11.29 |
| quarry lookups, median per card | 0.5 |
| quarry lookups, pooled per card | 0.52 |
| calls before the first edit, net, median per card | 8.75 (n=25) |
| calls before the first edit, net, pooled per card | 9.32 (n=25) |
| turns, median per card | 29 |
| turns, pooled per card | 31 |
| output tokens, median per card | 18381 |
| output tokens, pooled per card | 21171.6 |
| fresh input tokens, median per card | 63334 |
| fresh input tokens, pooled per card | 72234.23 |
| cache reads, median per card | 5011276 |
| cache reads, pooled per card | 6196548.9 |
| forks with 3+ parallel calls in the first work turn, share | 22% |

## Exploration quartiles

| arm | n | min | q1 | median | q3 | max |
| --- | --- | --- | --- | --- | --- | --- |
| none | 27 | 7 | 11.25 | 15.5 | 20 | 45 |

## Per-fork

| arm | run | agent | start | cards | pack opened | pack turn | pack before edit | first work turn | first parallel | spans | strict before edit | file-level before edit | strict in first turn | calls before edit | net before edit | net exploration | net whole reads | partial reads | greps | quarry | turns | output k | cache read M | edits | edits to no-span files | dropped lines | batch | trace | cross-check | card packs |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| none | interactive-recipe | agent-a388696df514a7fff | 10-31T20:47 | 4 | no | - | no | 2 | 4 | 0 | 0 | 0 | 0 | 15 | 15 | 39 | 13 | 30 | 40 | 3 | 55 | 64 | 9.4 | 31 | 31 | 0 | - | - | - | - |
| none | interactive-recipe | agent-aac0f9e495c544261 | 11-01T06:14 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 13 | 13 | 24 | 8 | 47 | 27 | 2 | 62 | 45 | 10.3 | 67 | 67 | 0 | - | - | - | - |
| none | interactive-recipe | agent-a7b09120581dca07e | 11-01T06:21 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 18 | 18 | 20 | 8 | 20 | 11 | 1 | 29 | 24 | 3.8 | 63 | 63 | 0 | - | - | - | - |
| none | interactive-recipe | agent-a7854f381b3c1e238 | 11-01T06:25 | 4 | no | - | no | 2 | 4 | 0 | 0 | 0 | 0 | 27 | 27 | 53 | 20 | 48 | 51 | 2 | 90 | 73 | 20.6 | 61 | 61 | 0 | - | - | - | - |
| none | interactive-recipe | agent-a9fb44737ff63d2f4 | 11-01T06:35 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 12 | 12 | 16 | 4 | 12 | 13 | 1 | 24 | 14 | 3 | 26 | 26 | 0 | - | - | - | - |
| none | run-verbs-shared | agent-ac4d9ca8ccf7839e8 | 10-31T16:33 | 4 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 8 | 8 | 45 | 9 | 27 | 25 | 3 | 52 | 31 | 7.2 | 9 | 9 | 0 | - | - | - | - |
| none | run-verbs-shared | agent-a3f6cde0f2e7c1b1f | 10-31T16:39 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 20 | 20 | 22 | 15 | 55 | 32 | 1 | 72 | 36 | 13.1 | 10 | 10 | 0 | - | - | - | - |
| none | run-verbs-shared | agent-a1d64a59b270648d6 | 10-31T16:45 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 21 | 21 | 31 | 37 | 143 | 90 | 0 | 165 | 112 | 53.2 | 22 | 22 | 0 | - | - | - | - |
| none | run-verbs-shared | agent-ae969e51519f6cf31 | 10-31T17:02 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 10 | 10 | 16 | 18 | 35 | 49 | 0 | 64 | 37 | 10 | 17 | 17 | 0 | - | - | - | - |
| none | run-verbs-shared | agent-a547c616b0dacb3bb | 10-31T17:08 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 41 | 41 | 43 | 14 | 143 | 54 | 0 | 116 | 54 | 22.8 | 3 | 3 | 0 | - | - | - | - |
| none | run-verbs-shared | agent-ab6f65f6d03ea8e28 | 10-31T17:17 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 8 | 8 | 29 | 16 | 51 | 55 | 2 | 75 | 41 | 15.5 | 14 | 14 | 0 | - | - | - | - |
| none | run-verbs-shared | agent-a436f985ebed46236 | 10-31T17:25 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 15 | 15 | 18 | 3 | 8 | 9 | 1 | 24 | 12 | 3.2 | 21 | 21 | 0 | - | - | - | - |
| none | run-verbs-shared | agent-a581498e851089b62 | 10-31T17:27 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 24 | 24 | 31 | 43 | 72 | 48 | 1 | 103 | 67 | 25.4 | 29 | 29 | 0 | - | - | - | - |
| none | run-verbs-shared | agent-a93a508821c471fc7 | 10-31T17:38 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 27 | 27 | 29 | 10 | 28 | 38 | 1 | 62 | 50 | 12.6 | 45 | 45 | 0 | - | - | - | - |
| none | run-verbs-shared | agent-a6b61825cecb443d9 | 10-31T17:45 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 11 | 11 | 23 | 11 | 42 | 40 | 2 | 76 | 49 | 16.1 | 38 | 38 | 0 | - | - | - | - |
| none | run-verbs-shared | agent-ae66305d1e5ccbaa4 | 10-31T17:52 | 3 | no | - | no | 2 | 3 | 0 | 0 | 0 | 0 | 20 | 20 | 38 | 15 | 50 | 51 | 1 | 84 | 66 | 19.2 | 50 | 50 | 0 | - | - | - | - |
| none | warp-visibility | agent-ae23d74935228bdc2 | 11-01T09:07 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 19 | 19 | 19 | 4 | 33 | 14 | 1 | 33 | 15 | 3.9 | 3 | 3 | 0 | - | - | - | - |
| none | warp-visibility | agent-af60f66b3f1b06e83 | 11-01T09:11 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | - | - | 17 | 2 | 22 | 8 | 0 | 21 | 8 | 2.3 | 0 | 0 | 0 | - | - | - | - |
| none | warp-visibility | agent-ade20caa7c4c99aa7 | 11-01T09:15 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | - | - | 27 | 1 | 43 | 21 | 0 | 31 | 17 | 3.9 | 0 | 0 | 0 | - | - | - | - |
| none | warp-visibility | agent-a73c6cf38bb7c1c8f | 11-01T09:20 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 18 | 18 | 17 | 1 | 18 | 8 | 0 | 21 | 8 | 2.4 | 0 | 0 | 0 | - | - | - | - |
| none | warp-visibility | agent-a9bc95a7cdda0bef8 | 11-01T09:22 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 5 | 5 | 7 | 1 | 39 | 39 | 0 | 40 | 19 | 5.4 | 26 | 26 | 0 | - | - | - | - |
| none | warp-visibility | agent-adaa23f850f784cd3 | 11-01T09:25 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 47 | 47 | 45 | 1 | 70 | 29 | 0 | 50 | 17 | 7.3 | 0 | 0 | 0 | - | - | - | - |
| none | warp-visibility | agent-a883d68c703e2581b | 11-01T09:29 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 11 | 11 | 32 | 11 | 34 | 34 | 1 | 49 | 22 | 7.9 | 10 | 10 | 0 | - | - | - | - |
| none | warp-visibility | agent-a515e24bab2491d27 | 11-01T09:34 | 3 | no | - | no | 2 | 3 | 0 | 0 | 0 | 0 | 19 | 19 | 25 | 7 | 27 | 33 | 1 | 53 | 57 | 11.5 | 31 | 31 | 0 | - | - | - | - |
| none | warp-visibility | agent-ab57e4993bd579494 | 11-01T09:44 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 12 | 12 | 16 | 17 | 42 | 29 | 0 | 67 | 65 | 15.1 | 17 | 17 | 0 | - | - | - | - |
| none | warp-visibility | agent-ad8ce503bf2dfd142 | 11-01T09:53 | 4 | no | - | no | 2 | 3 | 0 | 0 | 0 | 0 | 35 | 35 | 33 | 29 | 22 | 23 | 0 | 44 | 58 | 8.1 | 8 | 8 | 0 | - | - | - | - |
| none | warp-visibility | agent-aad0c269651fede22 | 11-01T10:03 | 3 | no | - | no | 2 | 3 | 0 | 0 | 0 | 0 | 10 | 10 | 27 | 8 | 43 | 46 | 3 | 50 | 41 | 8.9 | 6 | 6 | 0 | - | - | - | - |
