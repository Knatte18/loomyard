# Token usage and time by role

Generated 2026-11-04 15:15 for warp-visibility, interactive-recipe, run-verbs-shared.
Machine sleep, read from the systemd journal, is left out of every time figure.
Not counted, no transcript reader: codex.

## Most expensive parts

| part | cost | share | per run |
|---|---|---|---|
| Webster forks | $84.85 | 29.8% | $28.28 |
| Plan writer | $40.45 | 14.2% | $13.48 |
| Plan reviewer | $31.25 | 11.0% | $10.42 |
| Plan writer forks | $26.54 | 9.3% | $8.85 |
| Discussion reviewer | $18.04 | 6.3% | $6.01 |
| Plan fixer | $13.32 | 4.7% | $4.44 |
| Discussion writer | $11.71 | 4.1% | $3.90 |
| Discussion fixer | $11.32 | 4.0% | $3.77 |
| Discussion reviewer forks | $9.43 | 3.3% | $3.14 |
| Plan fixer forks | $4.89 | 1.7% | $1.63 |

## Cost per run

| run | recipe | Discussion | Plan | Webster | Code-Review | Landing | Orch | Other | Total |
|---|---|---|---|---|---|---|---|---|---|
| warp-visibility | tapestry | $19.18 | $39.44 | $22.48 | $2.91 | $0.84 | $0.00 | $0.10 | $84.96 |
| interactive-recipe | tapestry | $16.10 | $29.01 | $14.90 | $2.45 | $0.79 | $0.00 | $0.43 | $63.68 |
| run-verbs-shared | tapestry | $25.59 | $54.68 | $51.93 | $2.01 | $0.84 | $0.00 | $0.95 | $136.01 |
| average |  | $20.29 | $41.04 | $29.77 | $2.46 | $0.82 | $0.00 | $0.50 | $94.88 |

## Recipes

| recipe | runs | cost per run | span per run | landed runs | changed lines per run | cost per changed line |
|---|---|---|---|---|---|---|
| tapestry | 3 | $94.88 | 3h16m41s | 3 | 6715 | $0.014 |

A run's landed commit is the newest commit titled after the change description archived at its tag and committed within its span; its changed lines are the lines it added and deleted.

## All runs

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $123.13 | 43.3% | 3h16m17s | 33.3% | claude-opus-5-5: 1215, claude-sonnet-5-5: 326 | 82 | 1531511 | 6461789 | 340081894 | 3346 | 49.7M |
| └ writer | $40.45 | 14.2% | 42m3s | 7.1% | claude-opus-5-5: 387 | 3 | 247625 | 2250033 | 121210822 | 816 | 16.2M |
| └ reviewer | $31.25 | 11.0% | 1h12m47s | 12.3% | claude-sonnet-5-5: 257, claude-opus-5-5: 231 | 7 | 505749 | 2066639 | 85163479 | 1004 | 13.6M |
| └ writer forks | $26.54 | 9.3% | 41m26s | 7.0% | claude-opus-5-5: 191 | 18 | 342488 | 501824 | 85896181 | 418 | 10.9M |
| └ fixer | $13.32 | 4.7% | 32m4s | 5.4% | claude-opus-5-5: 258 | 7 | 209016 | 863597 | 24113034 | 580 | 4.5M |
| └ fixer forks | $4.89 | 1.7% | 5m46s | 1.0% | claude-opus-5-5: 91 | 28 | 103168 | 168795 | 9898264 | 238 | 1.7M |
| └ reviewer forks | $4.67 | 1.6% | 4m46s | 0.8% | claude-opus-5-5: 57 | 9 | 87235 | 116312 | 11717628 | 132 | 1.8M |
| └ judge | $1.33 | 0.5% | 3m28s | 0.6% | claude-sonnet-5-5: 43 | 7 | 26103 | 334023 | 1149695 | 100 | 0.7M |
| └ seed | $0.69 | 0.2% | 1m41s | 0.3% | claude-sonnet-5-5: 26 | 3 | 10127 | 160566 | 932791 | 58 | 0.3M |
| **Webster** | $89.31 | 31.4% | 3h9m17s | 32.1% | claude-sonnet-5-5: 1757, <synthetic>: 1 | 32 | 1130991 | 4162231 | 334991637 | 3646 | 44.4M |
| └ forks | $84.85 | 29.8% | 3h1m53s | 30.8% | claude-sonnet-5-5: 1606, <synthetic>: 1 | 29 | 1100179 | 3769334 | 322102286 | 3270 | 42.4M |
| └ Master | $4.46 | 1.6% | 8m22s | 1.4% | claude-sonnet-5-5: 151 | 3 | 30812 | 392897 | 12889351 | 376 | 1.9M |
| **Discussion** | $60.87 | 21.4% | 2h10m57s | 22.2% | claude-opus-5-5: 933, claude-sonnet-5-5: 95 | 103 | 1244528 | 3609346 | 100364005 | 2415 | 20.8M |
| └ reviewer | $18.04 | 6.3% | 46m21s | 7.9% | claude-opus-5-5: 281, claude-sonnet-5-5: 35 | 9 | 318638 | 1251557 | 30867030 | 705 | 6.2M |
| └ writer | $11.71 | 4.1% | 34m11s | 5.8% | claude-opus-5-5: 176 | 3 | 217011 | 645675 | 20693004 | 394 | 4.0M |
| └ fixer | $11.32 | 4.0% | 33m20s | 5.6% | claude-opus-5-5: 209 | 7 | 229410 | 733070 | 15297493 | 494 | 3.6M |
| └ reviewer forks | $9.43 | 3.3% | 12m27s | 2.1% | claude-opus-5-5: 118 | 28 | 253609 | 291518 | 14519784 | 292 | 3.1M |
| └ fixer forks | $4.63 | 1.6% | 6m1s | 1.0% | claude-opus-5-5: 97 | 28 | 103782 | 158685 | 8781272 | 250 | 1.6M |
| └ writer forks | $4.06 | 1.4% | 3m36s | 0.6% | claude-opus-5-5: 52 | 18 | 88411 | 109054 | 8719807 | 140 | 1.5M |
| └ judge | $1.21 | 0.4% | 3m32s | 0.6% | claude-sonnet-5-5: 42 | 7 | 27294 | 289745 | 1058730 | 98 | 0.6M |
| └ seed | $0.47 | 0.2% | 1m4s | 0.2% | claude-sonnet-5-5: 18 | 3 | 6373 | 130042 | 426885 | 42 | 0.2M |
| **Code-Review** | $7.37 | 2.6% | 15m15s | 2.6% | claude-sonnet-5-5: 218 | 33 | 169813 | 1028832 | 15505919 | 532 | 3.7M |
| └ reviewer | $2.91 | 1.0% | 6m50s | 1.2% | claude-sonnet-5-5: 64 | 3 | 58637 | 443566 | 6048038 | 148 | 1.5M |
| └ fixer | $1.63 | 0.6% | 5m14s | 0.9% | claude-sonnet-5-5: 67 | 3 | 39616 | 202309 | 3633024 | 156 | 0.8M |
| └ reviewer forks | $1.47 | 0.5% | 2m50s | 0.5% | claude-sonnet-5-5: 30 | 9 | 44633 | 117209 | 3662611 | 78 | 0.7M |
| └ seed | $0.52 | 0.2% | 1m16s | 0.2% | claude-sonnet-5-5: 20 | 3 | 8066 | 132018 | 552623 | 46 | 0.3M |
| └ fixer forks | $0.44 | 0.2% | 50s | 0.1% | claude-sonnet-5-5: 19 | 12 | 13791 | 27025 | 1163813 | 62 | 0.2M |
| └ judge | $0.41 | 0.1% | 48s | 0.1% | claude-sonnet-5-5: 18 | 3 | 5070 | 106705 | 445810 | 42 | 0.2M |
| **Landing** | $2.47 | 0.9% | 5m9s | 0.9% | claude-opus-5-5: 61, claude-sonnet-5-5: 24 | 6 | 29731 | 307800 | 3592106 | 182 | 0.9M |
| └ fixer | $1.97 | 0.7% | 3m33s | 0.6% | claude-opus-5-5: 61 | 3 | 18154 | 202238 | 2994101 | 128 | 0.6M |
| └ describe | $0.50 | 0.2% | 1m36s | 0.3% | claude-sonnet-5-5: 24 | 3 | 11577 | 105562 | 598005 | 54 | 0.2M |
| **Other** | $1.49 | 0.5% | 2m47s | 0.5% | claude-opus-5-5: 36, claude-sonnet-5-5: 16 | 5 | 14997 | 190166 | 1819057 | 110 | 0.5M |
| └ arbiter | $0.91 | 0.3% | 1m31s | 0.3% | claude-opus-5-5: 23 | 2 | 8555 | 107223 | 1007171 | 46 | 0.3M |
| └ conflict | $0.37 | 0.1% | 40s | 0.1% | claude-opus-5-5: 13 | 1 | 3301 | 44022 | 440349 | 28 | 0.1M |
| └ friction | $0.20 | 0.1% | 36s | 0.1% | claude-sonnet-5-5: 16 | 2 | 3141 | 38921 | 371537 | 36 | 0.1M |
| **Orch**, $0.00 per run | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ unattributed | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 50m21s | 8.5% | | | | | | | |

Total cost $284.64, total weight 120.0M; 5581 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
A segment's time is the union of its parts' working time, and time shares can sum past 100% where sessions overlap, a forks row overlapping its parent.
The union of working time plus idle is the span.
Every time figure leaves out machine sleep, 9h17m8s over these runs.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 3 | 3h24m20s | 15m4s | 3h9m17s | 35.1% |
| Plan-Write | 3 | 1h22m43s | 39s | 1h22m4s | 15.2% |
| Plan-Burler-Review | 7 | 1h20m18s | 7m3s | 1h13m15s | 13.6% |
| Discussion-Burler-Review | 7 | 55m6s | 2m36s | 52m30s | 9.7% |
| Discussion-Burler-Fix | 7 | 39m16s | 1m49s | 37m27s | 6.9% |
| Discussion-Write | 3 | 37m40s | 1m16s | 36m24s | 6.7% |
| Plan-Burler-Fix | 7 | 37m35s | 1m45s | 35m50s | 6.6% |
| Code-Burler-Review | 3 | 7m59s | 10s | 7m49s | 1.4% |
| Discussion-Bouncer | 12 | 6m51s | 45s | 6m6s | 1.1% |
| Code-Burler-Fix | 3 | 9m7s | 3m46s | 5m21s | 1.0% |
| Plan-Bouncer | 10 | 5m48s | 39s | 5m9s | 1.0% |
| Parent-Align | 3 | 14m49s | 10m36s | 4m13s | 0.8% |
| Code-Bouncer | 6 | 2m26s | 21s | 2m5s | 0.4% |
| Describe | 3 | 1m47s | 11s | 1m36s | 0.3% |
| Friction-Reflect | 3 | 45s | 9s | 36s | 0.1% |
| Drift-Await | 3 | 1s | 1s | 0s | 0.0% |
| Drift-Baseline | 3 | 0s | 0s | 0s | 0.0% |
| Drift-Verify | 3 | 1s | 1s | 0s | 0.0% |
| Finalize | 3 | 3m21s | 3m21s | 0s | 0.0% |
| PR-Review | 3 | 0s | 0s | 0s | 0.0% |
| Preflight | 3 | 8s | 8s | 0s | 0.0% |
| Tapestry-Preflight | 1 | 1s | 1s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## Orchestrator

Orch sessions of the prime from 2026-10-31 15:10 to 2026-11-01 11:27, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| session | 0 | 0 | 0.0M | $0.00 |
| forks | 0 | 0 | 0.0M | $0.00 |
| total | 0 | 0 | 0.0M | $0.00 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| warp-visibility | 0.0M | $0.00 | $84.96 | 0.0% |
| interactive-recipe | 0.0M | $0.00 | $63.68 | 0.0% |
| run-verbs-shared | 0.0M | $0.00 | $136.01 | 0.0% |
| unattributed | 0.0M | $0.00 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## warp-visibility

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $39.44 | 46.4% | 1h6m27s | 33.3% | claude-opus-5-5: 395, claude-sonnet-5-5: 75 | 25 | 495172 | 2203213 | 103474636 | 1012 | 15.6M |
| └ writer | $12.40 | 14.6% | 14m49s | 7.4% | claude-opus-5-5: 122 | 1 | 82558 | 780133 | 34254950 | 258 | 4.8M |
| └ reviewer | $10.05 | 11.8% | 23m4s | 11.6% | claude-opus-5-5: 87, claude-sonnet-5-5: 55 | 2 | 153416 | 758014 | 24817540 | 290 | 4.2M |
| └ writer forks | $9.07 | 10.7% | 15m58s | 8.0% | claude-opus-5-5: 68 | 6 | 131021 | 204150 | 27151580 | 148 | 3.6M |
| └ fixer | $4.58 | 5.4% | 11m2s | 5.5% | claude-opus-5-5: 78 | 2 | 67815 | 270923 | 9321854 | 168 | 1.6M |
| └ reviewer forks | $1.58 | 1.9% | 1m15s | 0.6% | claude-opus-5-5: 18 | 3 | 26006 | 31762 | 4489662 | 42 | 0.6M |
| └ fixer forks | $1.25 | 1.5% | 1m29s | 0.7% | claude-opus-5-5: 22 | 8 | 25431 | 32829 | 2901275 | 60 | 0.5M |
| └ judge | $0.38 | 0.4% | 52s | 0.4% | claude-sonnet-5-5: 13 | 2 | 6983 | 93053 | 367595 | 30 | 0.2M |
| └ seed | $0.13 | 0.2% | 20s | 0.2% | claude-sonnet-5-5: 7 | 1 | 1942 | 32349 | 170180 | 16 | 0.1M |
| **Webster** | $22.48 | 26.5% | 1h3m3s | 31.6% | claude-sonnet-5-5: 502 | 12 | 334399 | 1147684 | 80507566 | 1050 | 11.2M |
| └ forks | $20.98 | 24.7% | 1h0m3s | 30.1% | claude-sonnet-5-5: 448 | 11 | 326065 | 1038049 | 75627424 | 918 | 10.5M |
| └ Master | $1.50 | 1.8% | 3m12s | 1.6% | claude-sonnet-5-5: 54 | 1 | 8334 | 109635 | 4880142 | 132 | 0.7M |
| **Discussion** | $19.18 | 22.6% | 42m29s | 21.3% | claude-opus-5-5: 272, claude-sonnet-5-5: 18 | 30 | 401871 | 1100305 | 30154434 | 692 | 6.4M |
| └ reviewer | $5.32 | 6.3% | 13m44s | 6.9% | claude-opus-5-5: 75 | 2 | 95711 | 359224 | 8052025 | 172 | 1.7M |
| └ fixer | $4.09 | 4.8% | 12m13s | 6.1% | claude-opus-5-5: 78 | 2 | 82197 | 240596 | 6215447 | 182 | 1.3M |
| └ writer | $3.76 | 4.4% | 11m57s | 6.0% | claude-opus-5-5: 50 | 1 | 73537 | 211821 | 6163458 | 114 | 1.2M |
| └ reviewer forks | $2.84 | 3.3% | 3m58s | 2.0% | claude-opus-5-5: 27 | 8 | 79638 | 94666 | 3874771 | 70 | 0.9M |
| └ fixer forks | $1.37 | 1.6% | 1m59s | 1.0% | claude-opus-5-5: 25 | 8 | 32763 | 40963 | 2557257 | 66 | 0.5M |
| └ writer forks | $1.31 | 1.5% | 1m0s | 0.5% | claude-opus-5-5: 17 | 6 | 29398 | 31456 | 2842169 | 46 | 0.5M |
| └ judge | $0.33 | 0.4% | 49s | 0.4% | claude-sonnet-5-5: 12 | 2 | 6641 | 82467 | 303498 | 28 | 0.2M |
| └ seed | $0.15 | 0.2% | 19s | 0.2% | claude-sonnet-5-5: 6 | 1 | 1986 | 39112 | 145809 | 14 | 0.1M |
| **Code-Review** | $2.91 | 3.4% | 5m44s | 2.9% | claude-sonnet-5-5: 83 | 11 | 67156 | 374222 | 6517401 | 198 | 1.5M |
| └ reviewer | $1.20 | 1.4% | 2m36s | 1.3% | claude-sonnet-5-5: 25 | 1 | 22196 | 171048 | 2736950 | 54 | 0.6M |
| └ fixer | $0.73 | 0.9% | 2m21s | 1.2% | claude-sonnet-5-5: 30 | 1 | 18549 | 76499 | 1766265 | 70 | 0.4M |
| └ reviewer forks | $0.52 | 0.6% | 1m6s | 0.6% | claude-sonnet-5-5: 10 | 3 | 16057 | 39236 | 1322714 | 26 | 0.3M |
| └ fixer forks | $0.16 | 0.2% | 18s | 0.2% | claude-sonnet-5-5: 6 | 4 | 6369 | 8156 | 398225 | 20 | 0.1M |
| └ seed | $0.16 | 0.2% | 22s | 0.2% | claude-sonnet-5-5: 6 | 1 | 2375 | 42399 | 144871 | 14 | 0.1M |
| └ judge | $0.14 | 0.2% | 15s | 0.1% | claude-sonnet-5-5: 6 | 1 | 1610 | 36884 | 148376 | 14 | 0.1M |
| **Landing** | $0.84 | 1.0% | 1m45s | 0.9% | claude-opus-5-5: 21, claude-sonnet-5-5: 6 | 2 | 9837 | 109483 | 1139430 | 58 | 0.3M |
| └ fixer | $0.68 | 0.8% | 1m17s | 0.6% | claude-opus-5-5: 21 | 1 | 6596 | 69815 | 993501 | 44 | 0.2M |
| └ describe | $0.16 | 0.2% | 28s | 0.2% | claude-sonnet-5-5: 6 | 1 | 3241 | 39668 | 145929 | 14 | 0.1M |
| **Other** | $0.10 | 0.1% | 18s | 0.2% | claude-sonnet-5-5: 8 | 1 | 1568 | 20782 | 183656 | 18 | 0.1M |
| └ friction | $0.10 | 0.1% | 18s | 0.2% | claude-sonnet-5-5: 8 | 1 | 1568 | 20782 | 183656 | 18 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 19m44s | 9.9% | | | | | | | |

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 1 | 1h6m24s | 3m21s | 1h3m3s | 35.1% |
| Plan-Write | 1 | 30m32s | 14s | 30m18s | 16.9% |
| Plan-Burler-Review | 2 | 29m16s | 6m12s | 23m4s | 12.8% |
| Discussion-Burler-Review | 2 | 16m0s | 43s | 15m17s | 8.5% |
| Discussion-Burler-Fix | 2 | 14m20s | 47s | 13m33s | 7.5% |
| Discussion-Write | 1 | 13m4s | 34s | 12m30s | 7.0% |
| Plan-Burler-Fix | 2 | 12m30s | 36s | 11m54s | 6.6% |
| Code-Burler-Review | 1 | 2m46s | 4s | 2m42s | 1.5% |
| Code-Burler-Fix | 1 | 3m56s | 1m31s | 2m25s | 1.3% |
| Parent-Align | 1 | 4m35s | 3m18s | 1m17s | 0.7% |
| Plan-Bouncer | 3 | 1m22s | 10s | 1m12s | 0.7% |
| Discussion-Bouncer | 3 | 1m20s | 12s | 1m8s | 0.6% |
| Code-Bouncer | 2 | 44s | 7s | 37s | 0.3% |
| Describe | 1 | 31s | 3s | 28s | 0.3% |
| Friction-Reflect | 1 | 22s | 4s | 18s | 0.2% |
| Drift-Await | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Baseline | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Verify | 1 | 1s | 1s | 0s | 0.0% |
| Finalize | 1 | 1m44s | 1m44s | 0s | 0.0% |
| PR-Review | 1 | 0s | 0s | 0s | 0.0% |
| Preflight | 1 | 3s | 3s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## interactive-recipe

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $29.01 | 45.6% | 47m28s | 34.5% | claude-opus-5-5: 344, claude-sonnet-5-5: 76 | 25 | 377128 | 1381585 | 80584484 | 924 | 11.7M |
| └ writer | $11.26 | 17.7% | 14m41s | 10.7% | claude-opus-5-5: 131 | 1 | 89018 | 457715 | 35954026 | 276 | 4.6M |
| └ reviewer | $6.32 | 9.9% | 15m30s | 11.2% | claude-opus-5-5: 59, claude-sonnet-5-5: 57 | 2 | 108141 | 419768 | 16123671 | 242 | 2.7M |
| └ writer forks | $5.83 | 9.1% | 9m1s | 6.5% | claude-opus-5-5: 46 | 6 | 79477 | 106327 | 18520723 | 104 | 2.4M |
| └ fixer | $2.72 | 4.3% | 6m39s | 4.8% | claude-opus-5-5: 60 | 2 | 45120 | 196631 | 4166217 | 140 | 0.9M |
| └ reviewer forks | $1.30 | 2.0% | 1m17s | 0.9% | claude-opus-5-5: 21 | 3 | 25109 | 35488 | 3113342 | 48 | 0.5M |
| └ fixer forks | $1.09 | 1.7% | 1m23s | 1.0% | claude-opus-5-5: 27 | 8 | 22152 | 41639 | 2206041 | 70 | 0.4M |
| └ judge | $0.34 | 0.5% | 55s | 0.7% | claude-sonnet-5-5: 12 | 2 | 6386 | 84373 | 313406 | 28 | 0.2M |
| └ seed | $0.15 | 0.2% | 20s | 0.2% | claude-sonnet-5-5: 7 | 1 | 1725 | 39644 | 187058 | 16 | 0.1M |
| **Discussion** | $16.10 | 25.3% | 33m0s | 23.9% | claude-opus-5-5: 280, claude-sonnet-5-5: 18 | 30 | 308164 | 938152 | 28156191 | 700 | 5.5M |
| └ writer | $4.43 | 7.0% | 13m0s | 9.4% | claude-opus-5-5: 69 | 1 | 80336 | 234715 | 8266152 | 152 | 1.5M |
| └ reviewer | $4.17 | 6.6% | 9m41s | 7.0% | claude-opus-5-5: 78 | 2 | 65766 | 289131 | 7054411 | 176 | 1.4M |
| └ reviewer forks | $2.66 | 4.2% | 3m30s | 2.5% | claude-opus-5-5: 43 | 8 | 64227 | 73813 | 5029198 | 102 | 0.9M |
| └ fixer | $1.80 | 2.8% | 4m48s | 3.5% | claude-opus-5-5: 44 | 2 | 33417 | 137314 | 2214378 | 108 | 0.6M |
| └ writer forks | $1.60 | 2.5% | 1m45s | 1.3% | claude-opus-5-5: 19 | 6 | 33695 | 43431 | 3543934 | 50 | 0.6M |
| └ fixer forks | $0.95 | 1.5% | 1m20s | 1.0% | claude-opus-5-5: 27 | 8 | 21743 | 38627 | 1609309 | 70 | 0.3M |
| └ judge | $0.33 | 0.5% | 58s | 0.7% | claude-sonnet-5-5: 12 | 2 | 6888 | 79148 | 295025 | 28 | 0.2M |
| └ seed | $0.15 | 0.2% | 21s | 0.3% | claude-sonnet-5-5: 6 | 1 | 2092 | 41973 | 143784 | 14 | 0.1M |
| **Webster** | $14.90 | 23.4% | 34m3s | 24.7% | claude-sonnet-5-5: 301, <synthetic>: 1 | 7 | 228714 | 935129 | 50182156 | 630 | 7.3M |
| └ forks | $13.70 | 21.5% | 32m29s | 23.6% | claude-sonnet-5-5: 266, <synthetic>: 1 | 6 | 220451 | 775934 | 47789227 | 544 | 6.9M |
| └ Master | $1.20 | 1.9% | 1m43s | 1.2% | claude-sonnet-5-5: 35 | 1 | 8263 | 159195 | 2392929 | 86 | 0.5M |
| **Code-Review** | $2.45 | 3.8% | 5m4s | 3.7% | claude-sonnet-5-5: 69 | 11 | 56901 | 347616 | 5052928 | 174 | 1.2M |
| └ reviewer | $0.93 | 1.5% | 2m22s | 1.7% | claude-sonnet-5-5: 20 | 1 | 20022 | 150504 | 1790709 | 48 | 0.5M |
| └ fixer | $0.57 | 0.9% | 1m33s | 1.1% | claude-sonnet-5-5: 21 | 1 | 13175 | 76694 | 1235029 | 52 | 0.3M |
| └ reviewer forks | $0.50 | 0.8% | 54s | 0.7% | claude-sonnet-5-5: 10 | 3 | 14309 | 38013 | 1297389 | 26 | 0.2M |
| └ fixer forks | $0.16 | 0.3% | 18s | 0.2% | claude-sonnet-5-5: 6 | 4 | 5199 | 7957 | 441137 | 20 | 0.1M |
| └ seed | $0.15 | 0.2% | 22s | 0.3% | claude-sonnet-5-5: 6 | 1 | 2307 | 37974 | 141284 | 14 | 0.1M |
| └ judge | $0.14 | 0.2% | 17s | 0.2% | claude-sonnet-5-5: 6 | 1 | 1889 | 36474 | 147380 | 14 | 0.1M |
| **Landing** | $0.79 | 1.2% | 1m41s | 1.2% | claude-opus-5-5: 18, claude-sonnet-5-5: 9 | 2 | 9760 | 102225 | 1070910 | 58 | 0.3M |
| └ fixer | $0.61 | 1.0% | 1m6s | 0.8% | claude-opus-5-5: 18 | 1 | 5462 | 67293 | 840297 | 38 | 0.2M |
| └ describe | $0.18 | 0.3% | 35s | 0.4% | claude-sonnet-5-5: 9 | 1 | 4298 | 34932 | 230613 | 20 | 0.1M |
| **Other** | $0.43 | 0.7% | 47s | 0.6% | claude-opus-5-5: 8, claude-sonnet-5-5: 8 | 2 | 4452 | 62124 | 463079 | 34 | 0.1M |
| └ arbiter | $0.33 | 0.5% | 29s | 0.4% | claude-opus-5-5: 8 | 1 | 2879 | 43985 | 275198 | 16 | 0.1M |
| └ friction | $0.10 | 0.2% | 18s | 0.2% | claude-sonnet-5-5: 8 | 1 | 1573 | 18139 | 187881 | 18 | 0.0M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 15m43s | 11.4% | | | | | | | |

Machine sleep left out of the span: 9h17m8s.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 1 | 40m7s | 6m4s | 34m3s | 27.9% |
| Plan-Write | 1 | 23m23s | 9s | 23m14s | 19.0% |
| Plan-Burler-Review | 2 | 15m49s | 16s | 15m33s | 12.7% |
| Discussion-Write | 1 | 14m44s | 27s | 14m17s | 11.7% |
| Discussion-Burler-Review | 2 | 12m22s | 36s | 11m46s | 9.6% |
| Plan-Burler-Fix | 2 | 7m52s | 26s | 7m26s | 6.1% |
| Discussion-Burler-Fix | 2 | 6m13s | 35s | 5m38s | 4.6% |
| Code-Burler-Review | 1 | 2m53s | 3s | 2m50s | 2.3% |
| Discussion-Bouncer | 4 | 2m3s | 14s | 1m49s | 1.5% |
| Code-Burler-Fix | 1 | 3m17s | 1m42s | 1m35s | 1.3% |
| Plan-Bouncer | 3 | 1m34s | 18s | 1m16s | 1.0% |
| Parent-Align | 1 | 5m2s | 3m56s | 1m6s | 0.9% |
| Code-Bouncer | 2 | 47s | 8s | 39s | 0.5% |
| Describe | 1 | 39s | 4s | 35s | 0.5% |
| Friction-Reflect | 1 | 22s | 4s | 18s | 0.2% |
| Drift-Await | 1 | 1s | 1s | 0s | 0.0% |
| Drift-Baseline | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Verify | 1 | 0s | 0s | 0s | 0.0% |
| Finalize | 1 | 36s | 36s | 0s | 0.0% |
| PR-Review | 1 | 0s | 0s | 0s | 0.0% |
| Preflight | 1 | 2s | 2s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## run-verbs-shared

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $54.68 | 40.2% | 1h22m22s | 32.6% | claude-opus-5-5: 476, claude-sonnet-5-5: 175 | 32 | 659211 | 2876991 | 156022774 | 1410 | 22.5M |
| └ writer | $16.78 | 12.3% | 12m33s | 5.0% | claude-opus-5-5: 134 | 1 | 76049 | 1012185 | 51001846 | 282 | 6.7M |
| └ reviewer | $14.88 | 10.9% | 34m13s | 13.5% | claude-sonnet-5-5: 145, claude-opus-5-5: 85 | 3 | 244192 | 888857 | 44222268 | 472 | 6.8M |
| └ writer forks | $11.64 | 8.6% | 16m27s | 6.5% | claude-opus-5-5: 77 | 6 | 131990 | 191347 | 40223878 | 166 | 4.9M |
| └ fixer | $6.03 | 4.4% | 14m23s | 5.7% | claude-opus-5-5: 120 | 3 | 96081 | 396043 | 10624963 | 272 | 2.0M |
| └ fixer forks | $2.54 | 1.9% | 2m54s | 1.1% | claude-opus-5-5: 42 | 12 | 55585 | 94327 | 4790948 | 108 | 0.9M |
| └ reviewer forks | $1.79 | 1.3% | 2m14s | 0.9% | claude-opus-5-5: 18 | 3 | 36120 | 49062 | 4114624 | 42 | 0.7M |
| └ judge | $0.61 | 0.5% | 1m41s | 0.7% | claude-sonnet-5-5: 18 | 3 | 12734 | 156597 | 468694 | 42 | 0.3M |
| └ seed | $0.40 | 0.3% | 1m1s | 0.4% | claude-sonnet-5-5: 12 | 1 | 6460 | 88573 | 575553 | 26 | 0.2M |
| **Webster** | $51.93 | 38.2% | 1h32m11s | 36.5% | claude-sonnet-5-5: 954 | 13 | 567878 | 2079418 | 204301915 | 1966 | 25.9M |
| └ forks | $50.17 | 36.9% | 1h29m21s | 35.3% | claude-sonnet-5-5: 892 | 12 | 553663 | 1955351 | 198685635 | 1808 | 25.1M |
| └ Master | $1.76 | 1.3% | 3m28s | 1.4% | claude-sonnet-5-5: 62 | 1 | 14215 | 124067 | 5616280 | 158 | 0.8M |
| **Discussion** | $25.59 | 18.8% | 55m29s | 21.9% | claude-opus-5-5: 381, claude-sonnet-5-5: 59 | 43 | 534493 | 1570889 | 42053380 | 1023 | 8.8M |
| └ reviewer | $8.55 | 6.3% | 22m56s | 9.1% | claude-opus-5-5: 128, claude-sonnet-5-5: 35 | 5 | 157161 | 603202 | 15760594 | 357 | 3.1M |
| └ fixer | $5.43 | 4.0% | 16m18s | 6.5% | claude-opus-5-5: 87 | 3 | 113796 | 355160 | 6867668 | 204 | 1.7M |
| └ reviewer forks | $3.93 | 2.9% | 4m59s | 2.0% | claude-opus-5-5: 48 | 12 | 109744 | 123039 | 5615815 | 120 | 1.3M |
| └ writer | $3.51 | 2.6% | 9m13s | 3.6% | claude-opus-5-5: 57 | 1 | 63138 | 199139 | 6263394 | 128 | 1.2M |
| └ fixer forks | $2.30 | 1.7% | 2m42s | 1.1% | claude-opus-5-5: 45 | 12 | 49276 | 79095 | 4614706 | 114 | 0.8M |
| └ writer forks | $1.14 | 0.8% | 51s | 0.3% | claude-opus-5-5: 16 | 6 | 25318 | 34167 | 2333704 | 44 | 0.4M |
| └ judge | $0.55 | 0.4% | 1m44s | 0.7% | claude-sonnet-5-5: 18 | 3 | 13765 | 128130 | 460207 | 42 | 0.3M |
| └ seed | $0.17 | 0.1% | 23s | 0.2% | claude-sonnet-5-5: 6 | 1 | 2295 | 48957 | 137292 | 14 | 0.1M |
| **Code-Review** | $2.01 | 1.5% | 4m26s | 1.8% | claude-sonnet-5-5: 66 | 11 | 45756 | 306994 | 3935590 | 160 | 1.0M |
| └ reviewer | $0.77 | 0.6% | 1m52s | 0.7% | claude-sonnet-5-5: 19 | 1 | 16419 | 122014 | 1520379 | 46 | 0.4M |
| └ reviewer forks | $0.45 | 0.3% | 50s | 0.3% | claude-sonnet-5-5: 10 | 3 | 14267 | 39960 | 1042508 | 26 | 0.2M |
| └ fixer | $0.33 | 0.2% | 1m21s | 0.5% | claude-sonnet-5-5: 16 | 1 | 7892 | 49116 | 631730 | 34 | 0.2M |
| └ seed | $0.22 | 0.2% | 32s | 0.2% | claude-sonnet-5-5: 8 | 1 | 3384 | 51645 | 266468 | 18 | 0.1M |
| └ judge | $0.13 | 0.1% | 16s | 0.1% | claude-sonnet-5-5: 6 | 1 | 1571 | 33347 | 150054 | 14 | 0.1M |
| └ fixer forks | $0.11 | 0.1% | 13s | 0.1% | claude-sonnet-5-5: 7 | 4 | 2223 | 10912 | 324451 | 22 | 0.1M |
| **Other** | $0.95 | 0.7% | 1m42s | 0.7% | claude-opus-5-5: 28 | 2 | 8977 | 107260 | 1172322 | 58 | 0.3M |
| └ arbiter | $0.58 | 0.4% | 1m2s | 0.4% | claude-opus-5-5: 15 | 1 | 5676 | 63238 | 731973 | 30 | 0.2M |
| └ conflict | $0.37 | 0.3% | 40s | 0.3% | claude-opus-5-5: 13 | 1 | 3301 | 44022 | 440349 | 28 | 0.1M |
| **Landing** | $0.84 | 0.6% | 1m43s | 0.7% | claude-opus-5-5: 22, claude-sonnet-5-5: 9 | 2 | 10134 | 96092 | 1381766 | 66 | 0.3M |
| └ fixer | $0.68 | 0.5% | 1m10s | 0.5% | claude-opus-5-5: 22 | 1 | 6096 | 65130 | 1160303 | 46 | 0.2M |
| └ describe | $0.16 | 0.1% | 33s | 0.2% | claude-sonnet-5-5: 9 | 1 | 4038 | 30962 | 221463 | 20 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 14m53s | 5.9% | | | | | | | |

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 1 | 1h37m49s | 5m38s | 1h32m11s | 38.8% |
| Plan-Burler-Review | 3 | 35m13s | 35s | 34m38s | 14.6% |
| Plan-Write | 1 | 28m48s | 16s | 28m32s | 12.0% |
| Discussion-Burler-Review | 3 | 26m44s | 1m17s | 25m27s | 10.7% |
| Discussion-Burler-Fix | 3 | 18m43s | 27s | 18m16s | 7.7% |
| Plan-Burler-Fix | 3 | 17m13s | 43s | 16m30s | 6.9% |
| Discussion-Write | 1 | 9m52s | 14s | 9m38s | 4.0% |
| Discussion-Bouncer | 5 | 3m28s | 19s | 3m9s | 1.3% |
| Plan-Bouncer | 4 | 2m52s | 11s | 2m41s | 1.1% |
| Code-Burler-Review | 1 | 2m20s | 3s | 2m17s | 1.0% |
| Parent-Align | 1 | 5m12s | 3m22s | 1m50s | 0.8% |
| Code-Burler-Fix | 1 | 1m54s | 33s | 1m21s | 0.6% |
| Code-Bouncer | 2 | 55s | 7s | 48s | 0.3% |
| Describe | 1 | 37s | 4s | 33s | 0.2% |
| Drift-Await | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Baseline | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Verify | 1 | 0s | 0s | 0s | 0.0% |
| Finalize | 1 | 1m1s | 1m1s | 0s | 0.0% |
| Friction-Reflect | 1 | 1s | 1s | 0s | 0.0% |
| PR-Review | 1 | 0s | 0s | 0s | 0.0% |
| Preflight | 1 | 3s | 3s | 0s | 0.0% |
| Tapestry-Preflight | 1 | 1s | 1s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|
| warp-visibility | 01-pattern-entries, 02-bolt-dir-name | 88787 | 32 | 142200 | 0.5M |
| warp-visibility | 03-fabric-bolt-dir-sweep | 91439 | 20 | 124174 | 0.3M |
| warp-visibility | 04-fabric-test-bolt-dir-sweep | 93414 | 30 | 148003 | 0.5M |
| warp-visibility | 05-module-bolt-dir-sweep | 95367 | 20 | 123024 | 0.3M |
| warp-visibility | 06-module-test-bolt-dir-sweep | 97258 | 39 | 161370 | 0.7M |
| warp-visibility | 07-module-bolt-dir-uses | 99213 | 49 | 182973 | 0.9M |
| warp-visibility | 08-stencil-skill-bolt-dir-sweep, 09-fabric-exclude-list | 101345 | 48 | 200941 | 1.0M |
| warp-visibility | 10-fabric-worktree-seam, 11-entrymerge-package, 12-board-replay-entrymerge | 103558 | 52 | 276161 | 1.6M |
| warp-visibility | 13-bolt-replayer-registry | 105837 | 66 | 289717 | 2.1M |
| warp-visibility | 14-heddle-store, 15-heddle-render, 16-heddle-worktree-seam, 17-heddle-bolt-replayer | 108477 | 43 | 248162 | 1.3M |
| warp-visibility | 18-shuttle-prompt-seam, 19-heddle-cli, 20-root-prerun-installs | 110787 | 49 | 228172 | 1.2M |
| interactive-recipe | 01-sampler-config, 02-sampler-record, 03-sampler-stencil, 04-sampler-session-row | 73601 | 54 | 273338 | 1.5M |
| interactive-recipe | 05-sampler-registry-entry, 06-sampler-recipe | 75967 | 61 | 232578 | 1.4M |
| interactive-recipe | 07-sampler-vocabulary | 78217 | 28 | 166002 | 0.6M |
| interactive-recipe | 08-rows-keyed-wiring, 09-sampler-wiring, 10-sampler-done-verb, 11-decision-sampler-caller | 80200 | 89 | 330620 | 2.7M |
| interactive-recipe | 12-operator-wait-rows | 82289 | 23 | 144602 | 0.4M |
| interactive-recipe | unattributed | 87261 | 12 | 102793 | 0.1M |
| run-verbs-shared | 01-discussion-fence-rule, 02-drop-beam-alias, 03-loom-contract-check, 04-retired-key-way-forward | 81719 | 51 | 178113 | 1.0M |
| run-verbs-shared | 05-shed-keys-in-shed-only | 84400 | 71 | 244698 | 1.7M |
| run-verbs-shared | 06-tapestry-config-module | 87136 | 164 | 476158 | 6.4M |
| run-verbs-shared | 07-recipe-key-accessors, 08-one-loom-builder | 89870 | 63 | 201248 | 1.3M |
| run-verbs-shared | 09-drop-tapestry-preflight | 92505 | 115 | 289795 | 2.8M |
| run-verbs-shared | 10-drift-verify-before-review, 11-record-write-row | 95368 | 74 | 273747 | 2.0M |
| run-verbs-shared | 12-darn-review-answer-key | 98316 | 23 | 153128 | 0.4M |
| run-verbs-shared | 13-one-discussion-validator, 14-shared-writer-wrapper | 100639 | 102 | 342564 | 3.2M |
| run-verbs-shared | 15-darn-write-scope-guards | 103093 | 61 | 254309 | 1.7M |
| run-verbs-shared | 16-decision-add-by-rows, 17-one-describe | 106196 | 75 | 265779 | 2.0M |
| run-verbs-shared | 18-recipe-keyed-wiring, 19-loom-verbs-loom-only, 20-peers-per-recipe | 110676 | 83 | 305990 | 2.5M |
| run-verbs-shared | unattributed | 119288 | 10 | 129848 | 0.1M |

