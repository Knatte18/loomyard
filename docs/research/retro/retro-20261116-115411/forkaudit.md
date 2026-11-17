# Webster fork read audit

Generated: 2026-11-17T06:47:58Z

- Arguments: `--runs adopt-worker-record\,gate-scope-by-coverage-map\,decision-record-replace\,testmap-upkeep-design\,creel-notice-hygiene\,plan-writer-context\,adopt-redesign\,test-only-functions-in-prod\,weaver\,codex-same-as-claude --out docs/research/retro/retro-20261116-115411/forkaudit.md --work .scratch/forkaudit `
- Projects: `/home/knatte/.claude/projects`
- Hub: `/home/knatte/Code/loomyard-LYMHUB`
- Work: `/home/knatte/Code/loomyard-LYMHUB/retro-20261116-115411-weft/.lym/scratch/forkaudit`
- Records: `/home/knatte/Code/loomyard-LYMHUB/retro-20261116-115411-weft`
- Project directories matched:
  - `-home-knatte-Code-loomyard-LYMHUB-adopt-worker-record`
  - `-home-knatte-Code-loomyard-LYMHUB-gate-scope-by-coverage-map`
  - `-home-knatte-Code-loomyard-LYMHUB-decision-record-replace`
  - `-home-knatte-Code-loomyard-LYMHUB-testmap-upkeep-design`
  - `-home-knatte-Code-loomyard-LYMHUB-creel-notice-hygiene`
  - `-home-knatte-Code-loomyard-LYMHUB-plan-writer-context`
  - `-home-knatte-Code-loomyard-LYMHUB-adopt-redesign`
  - `-home-knatte-Code-loomyard-LYMHUB-test-only-functions-in-prod`
  - `-home-knatte-Code-loomyard-LYMHUB-weaver`
  - `-home-knatte-Code-loomyard-LYMHUB-codex-same-as-claude`
- Candidates found: 261
- Transcripts holding the job heading: 4
- Forks kept: 4
- Transcript lines dropped as unparsable: 0

## Samples

| arm | forks | runs | cards | cross-checked |
| --- | --- | --- | --- | --- |
| none | 4 | 1 | 10 | 0 |

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
| exploration calls, raw, median per card | 12.63 |
| exploration calls, raw, pooled per card | 13 |
| exploration calls, net, median per card | 12.63 |
| exploration calls, net, pooled per card | 13 |
| whole-file reads, raw, median per card | 5.38 |
| whole-file reads, raw, pooled per card | 5.3 |
| whole-file reads, net, median per card | 5.38 |
| whole-file reads, net, pooled per card | 5.3 |
| partial reads, median per card | 5.38 |
| partial reads, pooled per card | 5.6 |
| greps, median per card | 8.67 |
| greps, pooled per card | 9.1 |
| member greps, median per card | 6.92 |
| member greps, pooled per card | 7.2 |
| quarry lookups, median per card | 0 |
| quarry lookups, pooled per card | 0.1 |
| calls before the first edit, net, median per card | 10.92 |
| calls before the first edit, net, pooled per card | 10.5 |
| turns, median per card | 13.25 |
| turns, pooled per card | 14.1 |
| output tokens, median per card | 25324.38 |
| output tokens, pooled per card | 35197.2 |
| fresh input tokens, median per card | 97300.63 |
| fresh input tokens, pooled per card | 97137.1 |
| cache reads, median per card | 3018782.25 |
| cache reads, pooled per card | 3235747.1 |
| forks with 3+ parallel calls in the first work turn, share | 50% |

## Exploration quartiles

| arm | n | min | q1 | median | q3 | max |
| --- | --- | --- | --- | --- | --- | --- |
| none | 4 | 9.67 | 11.25 | 14 | 14 | 28 |

## Per-fork

| arm | run | agent | start | cards | pack opened | pack turn | pack before edit | first work turn | first parallel | spans | strict before edit | file-level before edit | strict in first turn | calls before edit | net before edit | net exploration | net whole reads | partial reads | greps | quarry | turns | output k | cache read M | edits | edits to no-span files | dropped lines | batch | trace | cross-check | card packs |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| none | adopt-worker-record | agent-ad228120fb6f9dd58 | 11-09T20:48 | 3 | no | - | no | 2 | 3 | 0 | 0 | 0 | 0 | 28 | 28 | 29 | 5 | 21 | 22 | 1 | 25 | 30 | 3.5 | 22 | 22 | 0 | - | - | - | - |
| none | adopt-worker-record | agent-a4729460ddd3ef029 | 11-11T06:03 | 2 | no | - | no | 2 | 2 | 0 | 0 | 0 | 0 | 25 | 25 | 28 | 11 | 12 | 7 | 0 | 26 | 30 | 4.5 | 26 | 26 | 0 | - | - | - | - |
| none | adopt-worker-record | agent-ae3508cfc61667e9f | 11-11T06:11 | 1 | no | - | no | 2 | 1 | 0 | 0 | 0 | 0 | 27 | 27 | 28 | 16 | 4 | 22 | 0 | 36 | 150 | 9.1 | 70 | 70 | 0 | - | - | - | - |
| none | adopt-worker-record | agent-a37cd551d50efa61e | 11-11T06:33 | 4 | no | - | no | 2 | 4 | 0 | 0 | 0 | 0 | 25 | 25 | 45 | 21 | 19 | 40 | 0 | 54 | 142 | 15.2 | 69 | 69 | 0 | - | - | - | - |
