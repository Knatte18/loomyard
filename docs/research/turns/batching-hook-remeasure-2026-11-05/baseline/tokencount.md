# Token usage and time by role

Generated 2026-11-05 20:17 for batten-remote-order, decision-record-on-board, shed-sub-run-block, cold-gate-fixer.
Machine sleep, read from the systemd journal, is left out of every time figure.
Not counted, no transcript reader: codex.

## Most expensive parts

| part | cost | share | per run |
|---|---|---|---|
| Webster forks | $34.14 | 19.9% | $8.53 |
| Plan reviewer | $18.82 | 11.0% | $4.70 |
| Plan writer forks | $16.47 | 9.6% | $4.12 |
| Plan writer | $14.48 | 8.4% | $3.62 |
| Plan fixer | $9.68 | 5.6% | $2.42 |
| Discussion reviewer | $9.39 | 5.5% | $2.35 |
| Discussion fixer | $9.33 | 5.4% | $2.33 |
| Discussion writer | $9.28 | 5.4% | $2.32 |
| Discussion reviewer forks | $5.39 | 3.1% | $1.35 |
| Plan reviewer forks | $5.13 | 3.0% | $1.28 |

## Cost per run

| run | recipe | Discussion | Plan | Webster | Code-Review | Landing | Orch | Other | Total |
|---|---|---|---|---|---|---|---|---|---|
| batten-remote-order | tapestry | $23.40 | $31.65 | $21.61 | $4.20 | $1.11 | $0.00 | $2.55 | $84.52 |
| decision-record-on-board | tapestry | $9.58 | $19.48 | $6.96 | $3.24 | $1.06 | $0.00 | $1.82 | $42.13 |
| shed-sub-run-block | tapestry | $9.15 | $19.01 | $7.95 | $1.62 | $0.93 | $0.00 | $0.73 | $39.39 |
| cold-gate-fixer | tapestry | $0.00 | $0.00 | $0.00 | $3.86 | $1.05 | $0.00 | $0.53 | $5.44 |
| average |  | $10.53 | $17.53 | $9.13 | $3.23 | $1.04 | $0.00 | $1.41 | $42.87 |

## Recipes

| recipe | runs | cost per run | span per run | landed runs | changed lines per run | cost per changed line |
|---|---|---|---|---|---|---|
| tapestry | 4 | $42.87 | 2h36m47s | 4 | 4192 | $0.010 |

A run's landed commit is the newest commit titled after the change description archived at its tag and committed within its span; its changed lines are the lines it added and deleted.

## All runs

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $70.13 | 40.9% | 2h36m52s | 25.0% | claude-opus-5-5: 632, claude-sonnet-5-5: 204 | 86 | 1277347 | 5187314 | 131698656 | 1838 | 26.0M |
| └ reviewer | $18.82 | 11.0% | 1h6m18s | 10.6% | claude-opus-5-5: 130, claude-sonnet-5-5: 72 | 6 | 358652 | 1778961 | 34758418 | 418 | 7.5M |
| └ writer forks | $16.47 | 9.6% | 30m24s | 4.8% | claude-opus-5-5: 109, claude-sonnet-5-5: 75 | 29 | 340589 | 927154 | 35589143 | 404 | 6.4M |
| └ writer | $14.48 | 8.4% | 33m36s | 5.4% | claude-opus-5-5: 115 | 3 | 196622 | 1071752 | 25949610 | 244 | 4.9M |
| └ fixer | $9.68 | 5.6% | 31m53s | 5.1% | claude-opus-5-5: 145 | 6 | 176843 | 717021 | 12768820 | 308 | 3.1M |
| └ reviewer forks | $5.13 | 3.0% | 5m21s | 0.9% | claude-opus-5-5: 58 | 9 | 94933 | 122612 | 13099862 | 134 | 1.9M |
| └ fixer forks | $3.83 | 2.2% | 4m56s | 0.8% | claude-opus-5-5: 75 | 24 | 78340 | 137900 | 7888033 | 198 | 1.4M |
| └ judge | $1.06 | 0.6% | 2m47s | 0.4% | claude-sonnet-5-5: 36 | 6 | 18505 | 269617 | 1011555 | 84 | 0.5M |
| └ seed | $0.66 | 0.4% | 1m59s | 0.3% | claude-sonnet-5-5: 21 | 3 | 12863 | 162297 | 633215 | 48 | 0.3M |
| **Discussion** | $42.13 | 24.6% | 1h33m15s | 14.9% | claude-opus-5-5: 442, claude-sonnet-5-5: 66 | 72 | 879820 | 2861054 | 58979546 | 1162 | 13.9M |
| └ reviewer | $9.39 | 5.5% | 28m48s | 4.6% | claude-opus-5-5: 92 | 4 | 170105 | 771188 | 10674143 | 192 | 2.9M |
| └ fixer | $9.33 | 5.4% | 30m11s | 4.8% | claude-opus-5-5: 113 | 4 | 187390 | 581838 | 13381413 | 238 | 3.0M |
| └ writer | $9.28 | 5.4% | 30m34s | 4.9% | claude-opus-5-5: 97 | 4 | 178622 | 695739 | 11152050 | 208 | 2.9M |
| └ reviewer forks | $5.39 | 3.1% | 7m53s | 1.3% | claude-opus-5-5: 54 | 16 | 146222 | 158677 | 8369870 | 140 | 1.8M |
| └ writer forks | $5.12 | 3.0% | 5m51s | 0.9% | claude-opus-5-5: 52, claude-sonnet-5-5: 24 | 21 | 111490 | 293109 | 10003578 | 188 | 1.9M |
| └ fixer forks | $2.41 | 1.4% | 3m22s | 0.5% | claude-opus-5-5: 34 | 16 | 61487 | 64024 | 4290745 | 100 | 0.8M |
| └ judge | $0.72 | 0.4% | 2m5s | 0.3% | claude-sonnet-5-5: 24 | 4 | 16055 | 171301 | 651016 | 54 | 0.4M |
| └ seed | $0.49 | 0.3% | 1m21s | 0.2% | claude-sonnet-5-5: 18 | 3 | 8449 | 125178 | 456731 | 42 | 0.2M |
| **Webster** | $36.52 | 21.3% | 2h53m20s | 27.6% | claude-sonnet-5-5: 538 | 16 | 786677 | 2802986 | 106350994 | 1178 | 18.1M |
| └ forks | $34.14 | 19.9% | 2h48m50s | 26.9% | claude-sonnet-5-5: 451 | 13 | 768056 | 2554343 | 100345875 | 946 | 17.1M |
| └ Master | $2.38 | 1.4% | 4m54s | 0.8% | claude-sonnet-5-5: 87 | 3 | 18621 | 248643 | 6005119 | 232 | 1.0M |
| **Code-Review** | $12.92 | 7.5% | 38m36s | 6.2% | claude-sonnet-5-5: 267 | 39 | 347999 | 1666576 | 26350224 | 608 | 6.5M |
| └ reviewer | $4.54 | 2.6% | 17m13s | 2.7% | claude-sonnet-5-5: 76 | 4 | 117879 | 705602 | 7959808 | 160 | 2.3M |
| └ fixer | $3.74 | 2.2% | 18m6s | 2.9% | claude-sonnet-5-5: 77 | 3 | 104625 | 450165 | 7838033 | 160 | 1.9M |
| └ reviewer forks | $2.42 | 1.4% | 5m19s | 0.8% | claude-sonnet-5-5: 40 | 12 | 86631 | 155856 | 5818029 | 104 | 1.2M |
| └ fixer forks | $1.01 | 0.6% | 1m15s | 0.2% | claude-sonnet-5-5: 25 | 12 | 21485 | 41787 | 3446551 | 74 | 0.5M |
| └ seed | $0.63 | 0.4% | 1m44s | 0.3% | claude-sonnet-5-5: 25 | 4 | 9281 | 165126 | 646094 | 56 | 0.3M |
| └ judge | $0.58 | 0.3% | 1m34s | 0.2% | claude-sonnet-5-5: 24 | 4 | 8098 | 148040 | 641709 | 54 | 0.3M |
| **Other** | $5.63 | 3.3% | 27m31s | 4.4% | claude-sonnet-5-5: 174, claude-opus-5-5: 37 | 23 | 72799 | 1041913 | 7413685 | 464 | 2.4M |
| └ gate-fixer | $3.69 | 2.2% | 23m47s | 3.8% | claude-sonnet-5-5: 174 | 19 | 50517 | 807503 | 5836484 | 386 | 1.8M |
| └ arbiter | $1.14 | 0.7% | 1m59s | 0.3% | claude-opus-5-5: 17 | 2 | 12855 | 142079 | 842060 | 34 | 0.3M |
| └ conflict | $0.80 | 0.5% | 1m46s | 0.3% | claude-opus-5-5: 20 | 2 | 9427 | 92331 | 735141 | 44 | 0.2M |
| **Landing** | $4.15 | 2.4% | 11m33s | 1.8% | claude-opus-5-5: 91, claude-sonnet-5-5: 29 | 9 | 50281 | 480544 | 6126054 | 258 | 1.5M |
| └ fixer | $3.51 | 2.0% | 9m15s | 1.5% | claude-opus-5-5: 91 | 5 | 37530 | 338576 | 5314051 | 192 | 1.1M |
| └ describe | $0.64 | 0.4% | 2m18s | 0.4% | claude-sonnet-5-5: 29 | 4 | 12751 | 141968 | 812003 | 66 | 0.3M |
| **Orch**, $0.00 per run | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ unattributed | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 2h24m47s | 23.1% | | | | | | | |

Total cost $171.48, total weight 68.3M; 4669 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
A segment's time is the union of its parts' working time, and time shares can sum past 100% where sessions overlap, a forks row overlapping its parent.
The union of working time plus idle is the span.
Every time figure leaves out machine sleep, 6h39m55s over these runs.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 4 | 3h26m54s | 32m28s | 2h54m26s | 36.2% |
| Plan-Burler-Review | 8 | 1h6m49s | 31s | 1h6m18s | 13.7% |
| Plan-Write | 4 | 58m27s | 1m45s | 56m42s | 11.8% |
| Discussion-Write | 4 | 33m55s | 1m57s | 31m58s | 6.6% |
| Plan-Burler-Fix | 8 | 35m31s | 3m38s | 31m53s | 6.6% |
| Discussion-Burler-Fix | 6 | 31m59s | 1m48s | 30m11s | 6.3% |
| Discussion-Burler-Review | 6 | 29m8s | 20s | 28m48s | 6.0% |
| Code-Burler-Fix | 4 | 31m56s | 13m50s | 18m6s | 3.8% |
| Code-Burler-Review | 4 | 17m30s | 17s | 17m13s | 3.6% |
| Parent-Align | 4 | 1h0m40s | 50m53s | 9m48s | 2.0% |
| Plan-Bouncer | 13 | 6m19s | 35s | 5m44s | 1.2% |
| Discussion-Bouncer | 14 | 15m15s | 10m48s | 4m27s | 0.9% |
| Code-Bouncer | 8 | 3m49s | 32s | 3m17s | 0.7% |
| Describe | 4 | 2m35s | 17s | 2m18s | 0.5% |
| Finalize | 4 | 25m13s | 24m0s | 1m13s | 0.3% |
| Drift-Await | 4 | 2s | 2s | 0s | 0.0% |
| Drift-Baseline | 4 | 0s | 0s | 0s | 0.0% |
| Drift-Verify | 4 | 0s | 0s | 0s | 0.0% |
| Friction-Reflect | 4 | 3s | 3s | 0s | 0.0% |
| PR-Review | 4 | 3s | 3s | 0s | 0.0% |
| Parts-Plan-Write | 4 | 1s | 1s | 0s | 0.0% |
| Parts-Write | 4 | 0s | 0s | 0s | 0.0% |
| Preflight | 4 | 59s | 59s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## Orchestrator

Orch sessions of the prime from 2026-11-02 19:29 to 2026-11-03 13:20, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| session | 0 | 0 | 0.0M | $0.00 |
| forks | 0 | 0 | 0.0M | $0.00 |
| total | 0 | 0 | 0.0M | $0.00 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| batten-remote-order | 0.0M | $0.00 | $84.52 | 0.0% |
| decision-record-on-board | 0.0M | $0.00 | $42.13 | 0.0% |
| shed-sub-run-block | 0.0M | $0.00 | $39.39 | 0.0% |
| cold-gate-fixer | 0.0M | $0.00 | $5.44 | 0.0% |
| unattributed | 0.0M | $0.00 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## batten-remote-order

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $31.65 | 37.4% | 1h25m3s | 28.8% | claude-opus-5-5: 249, claude-sonnet-5-5: 88 | 29 | 608718 | 2362837 | 59390321 | 728 | 11.9M |
| └ reviewer | $9.45 | 11.2% | 40m51s | 13.8% | claude-opus-5-5: 44, claude-sonnet-5-5: 37 | 2 | 187879 | 992302 | 17596381 | 166 | 3.9M |
| └ writer forks | $6.84 | 8.1% | 13m50s | 4.7% | claude-opus-5-5: 41, claude-sonnet-5-5: 33 | 10 | 149359 | 412015 | 13865037 | 160 | 2.6M |
| └ writer | $5.63 | 6.7% | 13m48s | 4.7% | claude-opus-5-5: 46 | 1 | 80802 | 371682 | 10762252 | 96 | 1.9M |
| └ fixer | $5.06 | 6.0% | 19m6s | 6.5% | claude-opus-5-5: 70 | 2 | 95140 | 328941 | 7581188 | 146 | 1.6M |
| └ reviewer forks | $2.14 | 2.5% | 2m35s | 0.9% | claude-opus-5-5: 19 | 3 | 44273 | 55401 | 4909771 | 44 | 0.8M |
| └ fixer forks | $1.97 | 2.3% | 2m36s | 0.9% | claude-opus-5-5: 29 | 8 | 41325 | 60809 | 4174314 | 74 | 0.7M |
| └ judge | $0.39 | 0.5% | 1m11s | 0.4% | claude-sonnet-5-5: 12 | 2 | 7562 | 98530 | 343341 | 28 | 0.2M |
| └ seed | $0.16 | 0.2% | 28s | 0.2% | claude-sonnet-5-5: 6 | 1 | 2378 | 43157 | 158037 | 14 | 0.1M |
| **Discussion** | $23.40 | 27.7% | 52m40s | 17.8% | claude-opus-5-5: 234, claude-sonnet-5-5: 42 | 33 | 493926 | 1544355 | 34214700 | 622 | 7.8M |
| └ fixer | $7.29 | 8.6% | 23m10s | 7.8% | claude-opus-5-5: 86 | 2 | 140982 | 415269 | 11973227 | 180 | 2.4M |
| └ reviewer | $5.60 | 6.6% | 16m2s | 5.4% | claude-opus-5-5: 51 | 2 | 96306 | 455705 | 6973115 | 106 | 1.7M |
| └ reviewer forks | $3.40 | 4.0% | 4m58s | 1.7% | claude-opus-5-5: 31 | 8 | 90522 | 99737 | 5434529 | 78 | 1.1M |
| └ writer | $2.79 | 3.3% | 11m32s | 3.9% | claude-opus-5-5: 32 | 1 | 63963 | 179025 | 3078098 | 72 | 0.9M |
| └ writer forks | $2.16 | 2.6% | 3m10s | 1.1% | claude-sonnet-5-5: 24, claude-opus-5-5: 17 | 9 | 49542 | 220968 | 3228031 | 94 | 0.8M |
| └ fixer forks | $1.61 | 1.9% | 2m13s | 0.8% | claude-opus-5-5: 17 | 8 | 40268 | 38334 | 3046818 | 50 | 0.6M |
| └ judge | $0.40 | 0.5% | 1m13s | 0.4% | claude-sonnet-5-5: 12 | 2 | 9715 | 94067 | 328999 | 28 | 0.2M |
| └ seed | $0.16 | 0.2% | 26s | 0.1% | claude-sonnet-5-5: 6 | 1 | 2628 | 41250 | 151883 | 14 | 0.1M |
| **Webster** | $21.61 | 25.6% | 1h47m51s | 36.5% | claude-sonnet-5-5: 258 | 6 | 461377 | 1588030 | 64440868 | 574 | 10.7M |
| └ forks | $20.65 | 24.4% | 1h45m48s | 35.8% | claude-sonnet-5-5: 223 | 5 | 454962 | 1499251 | 61759407 | 474 | 10.3M |
| └ Master | $0.96 | 1.1% | 2m9s | 0.7% | claude-sonnet-5-5: 35 | 1 | 6415 | 88779 | 2681461 | 100 | 0.4M |
| **Code-Review** | $4.20 | 5.0% | 14m29s | 4.9% | claude-sonnet-5-5: 76 | 11 | 115318 | 525010 | 8663732 | 174 | 2.1M |
| └ reviewer | $1.50 | 1.8% | 5m6s | 1.7% | claude-sonnet-5-5: 21 | 1 | 37312 | 226354 | 2828987 | 44 | 0.8M |
| └ fixer | $1.15 | 1.4% | 8m27s | 2.9% | claude-sonnet-5-5: 21 | 1 | 36497 | 150918 | 2031809 | 44 | 0.6M |
| └ reviewer forks | $0.88 | 1.0% | 1m46s | 0.6% | claude-sonnet-5-5: 12 | 3 | 30451 | 53037 | 2213807 | 30 | 0.4M |
| └ fixer forks | $0.35 | 0.4% | 24s | 0.1% | claude-sonnet-5-5: 9 | 4 | 6435 | 14617 | 1226179 | 26 | 0.2M |
| └ seed | $0.17 | 0.2% | 31s | 0.2% | claude-sonnet-5-5: 7 | 1 | 2565 | 41051 | 200754 | 16 | 0.1M |
| └ judge | $0.15 | 0.2% | 25s | 0.1% | claude-sonnet-5-5: 6 | 1 | 2058 | 39033 | 162196 | 14 | 0.1M |
| **Other** | $2.55 | 3.0% | 17m41s | 6.0% | claude-sonnet-5-5: 97, claude-opus-5-5: 8 | 11 | 36475 | 489203 | 3647871 | 230 | 1.2M |
| └ gate-fixer | $2.00 | 2.4% | 16m40s | 5.6% | claude-sonnet-5-5: 97 | 10 | 29905 | 421047 | 3256803 | 214 | 1.0M |
| └ arbiter | $0.55 | 0.7% | 1m1s | 0.3% | claude-opus-5-5: 8 | 1 | 6570 | 68156 | 391068 | 16 | 0.2M |
| **Landing** | $1.11 | 1.3% | 2m16s | 0.8% | claude-opus-5-5: 23, claude-sonnet-5-5: 6 | 2 | 12874 | 123709 | 1802107 | 62 | 0.4M |
| └ fixer | $0.95 | 1.1% | 1m47s | 0.6% | claude-opus-5-5: 23 | 1 | 9960 | 84794 | 1648721 | 48 | 0.3M |
| └ describe | $0.16 | 0.2% | 29s | 0.2% | claude-sonnet-5-5: 6 | 1 | 2914 | 38915 | 153386 | 14 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 29m50s | 10.1% | | | | | | | |

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 1 | 2h1m12s | 12m15s | 1h48m57s | 41.0% |
| Plan-Burler-Review | 2 | 41m5s | 14s | 40m51s | 15.4% |
| Plan-Write | 1 | 25m32s | 59s | 24m33s | 9.2% |
| Discussion-Burler-Fix | 2 | 24m7s | 57s | 23m10s | 8.7% |
| Plan-Burler-Fix | 2 | 19m54s | 48s | 19m6s | 7.2% |
| Discussion-Burler-Review | 2 | 16m12s | 10s | 16m2s | 6.0% |
| Discussion-Write | 1 | 12m18s | 30s | 11m48s | 4.4% |
| Code-Burler-Fix | 1 | 12m30s | 4m3s | 8m27s | 3.2% |
| Code-Burler-Review | 1 | 5m10s | 4s | 5m6s | 1.9% |
| Discussion-Bouncer | 4 | 2m54s | 14s | 2m40s | 1.0% |
| Parent-Align | 1 | 6m50s | 5m3s | 1m47s | 0.7% |
| Plan-Bouncer | 3 | 1m48s | 9s | 1m39s | 0.6% |
| Code-Bouncer | 2 | 1m2s | 6s | 56s | 0.4% |
| Describe | 1 | 32s | 3s | 29s | 0.2% |
| Drift-Await | 1 | 1s | 1s | 0s | 0.0% |
| Drift-Baseline | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Verify | 1 | 0s | 0s | 0s | 0.0% |
| Finalize | 1 | 3m31s | 3m31s | 0s | 0.0% |
| Friction-Reflect | 1 | 1s | 1s | 0s | 0.0% |
| PR-Review | 1 | 1s | 1s | 0s | 0.0% |
| Parts-Plan-Write | 1 | 1s | 1s | 0s | 0.0% |
| Parts-Write | 1 | 0s | 0s | 0s | 0.0% |
| Preflight | 1 | 40s | 40s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## decision-record-on-board

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $19.48 | 46.2% | 37m57s | 26.6% | claude-opus-5-5: 195, claude-sonnet-5-5: 60 | 28 | 344974 | 1410829 | 37503441 | 564 | 7.2M |
| └ reviewer | $5.25 | 12.5% | 15m37s | 10.9% | claude-opus-5-5: 48, claude-sonnet-5-5: 22 | 2 | 107146 | 436742 | 9884837 | 144 | 2.1M |
| └ writer forks | $4.53 | 10.8% | 7m19s | 5.1% | claude-opus-5-5: 37, claude-sonnet-5-5: 19 | 9 | 83037 | 205527 | 11480340 | 124 | 1.8M |
| └ writer | $4.15 | 9.9% | 9m27s | 6.6% | claude-opus-5-5: 32 | 1 | 56448 | 337331 | 6682323 | 68 | 1.4M |
| └ fixer | $2.42 | 5.7% | 6m42s | 4.7% | claude-opus-5-5: 36 | 2 | 42014 | 209558 | 2663943 | 78 | 0.7M |
| └ reviewer forks | $1.49 | 3.5% | 1m22s | 1.0% | claude-opus-5-5: 20 | 3 | 25635 | 29844 | 4163501 | 46 | 0.6M |
| └ fixer forks | $1.04 | 2.5% | 1m11s | 0.8% | claude-opus-5-5: 22 | 8 | 20089 | 44335 | 2088020 | 60 | 0.4M |
| └ judge | $0.36 | 0.8% | 55s | 0.6% | claude-sonnet-5-5: 12 | 2 | 6607 | 88846 | 334134 | 28 | 0.2M |
| └ seed | $0.23 | 0.5% | 36s | 0.4% | claude-sonnet-5-5: 7 | 1 | 3998 | 58646 | 206343 | 16 | 0.1M |
| **Discussion** | $9.58 | 22.7% | 20m45s | 14.5% | claude-opus-5-5: 112, claude-sonnet-5-5: 12 | 20 | 189521 | 685794 | 13095949 | 286 | 3.1M |
| └ writer | $3.69 | 8.8% | 10m8s | 7.1% | claude-opus-5-5: 39 | 2 | 59562 | 302438 | 4954829 | 82 | 1.2M |
| └ reviewer | $1.73 | 4.1% | 6m6s | 4.3% | claude-opus-5-5: 19 | 1 | 35970 | 140735 | 1514582 | 40 | 0.5M |
| └ writer forks | $1.42 | 3.4% | 1m27s | 1.0% | claude-opus-5-5: 16 | 6 | 29110 | 33570 | 3357137 | 44 | 0.5M |
| └ fixer | $1.03 | 2.5% | 3m41s | 2.6% | claude-opus-5-5: 14 | 1 | 24129 | 81826 | 708777 | 30 | 0.3M |
| └ reviewer forks | $0.91 | 2.2% | 1m6s | 0.8% | claude-opus-5-5: 13 | 4 | 23192 | 29915 | 1482381 | 34 | 0.3M |
| └ fixer forks | $0.47 | 1.1% | 38s | 0.4% | claude-opus-5-5: 11 | 4 | 11794 | 15965 | 766405 | 30 | 0.2M |
| └ judge | $0.16 | 0.4% | 25s | 0.3% | claude-sonnet-5-5: 6 | 1 | 3166 | 40256 | 159977 | 12 | 0.1M |
| └ seed | $0.16 | 0.4% | 25s | 0.3% | claude-sonnet-5-5: 6 | 1 | 2598 | 41089 | 151861 | 14 | 0.1M |
| **Webster** | $6.96 | 16.5% | 30m33s | 21.4% | claude-sonnet-5-5: 130 | 4 | 125371 | 511988 | 21512725 | 278 | 3.4M |
| └ forks | $6.29 | 14.9% | 29m29s | 20.7% | claude-sonnet-5-5: 108 | 3 | 119601 | 426853 | 20128733 | 222 | 3.1M |
| └ Master | $0.68 | 1.6% | 1m12s | 0.8% | claude-sonnet-5-5: 22 | 1 | 5770 | 85135 | 1383992 | 56 | 0.3M |
| **Code-Review** | $3.24 | 7.7% | 9m47s | 6.9% | claude-sonnet-5-5: 78 | 11 | 76651 | 398315 | 7392120 | 178 | 1.6M |
| └ fixer | $1.28 | 3.0% | 4m56s | 3.5% | claude-sonnet-5-5: 30 | 1 | 28934 | 143690 | 3144293 | 62 | 0.6M |
| └ reviewer | $0.98 | 2.3% | 3m52s | 2.7% | claude-sonnet-5-5: 20 | 1 | 25132 | 146269 | 1830127 | 42 | 0.5M |
| └ reviewer forks | $0.38 | 0.9% | 49s | 0.6% | claude-sonnet-5-5: 8 | 3 | 12005 | 21219 | 1023283 | 22 | 0.2M |
| └ fixer forks | $0.31 | 0.7% | 23s | 0.3% | claude-sonnet-5-5: 8 | 4 | 6283 | 13156 | 1086025 | 24 | 0.2M |
| └ seed | $0.15 | 0.3% | 32s | 0.4% | claude-sonnet-5-5: 6 | 1 | 2407 | 36978 | 149531 | 14 | 0.1M |
| └ judge | $0.14 | 0.3% | 26s | 0.3% | claude-sonnet-5-5: 6 | 1 | 1890 | 37003 | 158861 | 14 | 0.1M |
| **Other** | $1.82 | 4.3% | 5m44s | 4.0% | claude-sonnet-5-5: 43, claude-opus-5-5: 18 | 7 | 20636 | 327761 | 2185919 | 134 | 0.7M |
| └ gate-fixer | $0.96 | 2.3% | 4m14s | 3.0% | claude-sonnet-5-5: 43 | 5 | 11879 | 221435 | 1436677 | 96 | 0.5M |
| └ arbiter | $0.59 | 1.4% | 58s | 0.7% | claude-opus-5-5: 9 | 1 | 6285 | 73923 | 450992 | 18 | 0.2M |
| └ conflict | $0.27 | 0.6% | 33s | 0.4% | claude-opus-5-5: 9 | 1 | 2472 | 32403 | 298250 | 20 | 0.1M |
| **Landing** | $1.06 | 2.5% | 4m39s | 3.3% | claude-opus-5-5: 28, claude-sonnet-5-5: 7 | 3 | 12010 | 128293 | 1417720 | 76 | 0.4M |
| └ fixer | $0.91 | 2.2% | 4m0s | 2.8% | claude-opus-5-5: 28 | 2 | 8815 | 98109 | 1227632 | 60 | 0.3M |
| └ describe | $0.15 | 0.3% | 39s | 0.5% | claude-sonnet-5-5: 7 | 1 | 3195 | 30184 | 190088 | 16 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 36m3s | 25.3% | | | | | | | |

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 1 | 41m27s | 10m54s | 30m33s | 28.6% |
| Plan-Burler-Review | 2 | 15m44s | 7s | 15m37s | 14.7% |
| Plan-Write | 1 | 15m26s | 25s | 15m1s | 14.1% |
| Discussion-Write | 1 | 11m18s | 38s | 10m40s | 10.0% |
| Plan-Burler-Fix | 2 | 8m38s | 1m56s | 6m42s | 6.3% |
| Discussion-Burler-Review | 1 | 6m11s | 5s | 6m6s | 5.7% |
| Code-Burler-Fix | 1 | 9m45s | 4m49s | 4m56s | 4.6% |
| Code-Burler-Review | 1 | 3m56s | 4s | 3m52s | 3.6% |
| Discussion-Burler-Fix | 1 | 4m8s | 27s | 3m41s | 3.5% |
| Parent-Align | 1 | 9m4s | 5m45s | 3m19s | 3.1% |
| Plan-Bouncer | 4 | 2m43s | 13s | 2m30s | 2.3% |
| Finalize | 1 | 10m51s | 9m38s | 1m13s | 1.1% |
| Code-Bouncer | 2 | 1m7s | 8s | 59s | 0.9% |
| Discussion-Bouncer | 3 | 1m28s | 37s | 51s | 0.8% |
| Describe | 1 | 43s | 4s | 39s | 0.6% |
| Drift-Await | 1 | 1s | 1s | 0s | 0.0% |
| Drift-Baseline | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Verify | 1 | 0s | 0s | 0s | 0.0% |
| Friction-Reflect | 1 | 1s | 1s | 0s | 0.0% |
| PR-Review | 1 | 1s | 1s | 0s | 0.0% |
| Parts-Plan-Write | 1 | 0s | 0s | 0s | 0.0% |
| Parts-Write | 1 | 0s | 0s | 0s | 0.0% |
| Preflight | 1 | 9s | 9s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## shed-sub-run-block

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $19.01 | 48.3% | 33m52s | 26.1% | claude-opus-5-5: 188, claude-sonnet-5-5: 56 | 29 | 323655 | 1413648 | 34804894 | 546 | 6.9M |
| └ writer forks | $5.10 | 12.9% | 9m16s | 7.1% | claude-opus-5-5: 31, claude-sonnet-5-5: 23 | 10 | 108193 | 309612 | 10243766 | 120 | 2.0M |
| └ writer | $4.70 | 11.9% | 10m21s | 8.0% | claude-opus-5-5: 37 | 1 | 59372 | 362739 | 8505035 | 80 | 1.6M |
| └ reviewer | $4.11 | 10.4% | 9m50s | 7.6% | claude-opus-5-5: 38, claude-sonnet-5-5: 13 | 2 | 63627 | 349917 | 7277200 | 108 | 1.5M |
| └ fixer | $2.19 | 5.6% | 6m4s | 4.7% | claude-opus-5-5: 39 | 2 | 39689 | 178522 | 2523689 | 84 | 0.7M |
| └ reviewer forks | $1.49 | 3.8% | 1m23s | 1.1% | claude-opus-5-5: 19 | 3 | 25025 | 37367 | 4026590 | 44 | 0.6M |
| └ fixer forks | $0.83 | 2.1% | 1m8s | 0.9% | claude-opus-5-5: 24 | 8 | 16926 | 32756 | 1625699 | 64 | 0.3M |
| └ judge | $0.32 | 0.8% | 41s | 0.5% | claude-sonnet-5-5: 12 | 2 | 4336 | 82241 | 334080 | 28 | 0.2M |
| └ seed | $0.27 | 0.7% | 54s | 0.7% | claude-sonnet-5-5: 8 | 1 | 6487 | 60494 | 268835 | 18 | 0.1M |
| **Discussion** | $9.15 | 23.2% | 19m50s | 15.3% | claude-opus-5-5: 96, claude-sonnet-5-5: 12 | 19 | 196373 | 630905 | 11668897 | 254 | 2.9M |
| └ writer | $2.80 | 7.1% | 8m54s | 6.9% | claude-opus-5-5: 26 | 1 | 55097 | 214276 | 3119123 | 54 | 0.9M |
| └ reviewer | $2.07 | 5.2% | 6m40s | 5.1% | claude-opus-5-5: 22 | 1 | 37829 | 174748 | 2186446 | 46 | 0.6M |
| └ writer forks | $1.53 | 3.9% | 1m14s | 1.0% | claude-opus-5-5: 19 | 6 | 32838 | 38571 | 3418410 | 50 | 0.6M |
| └ reviewer forks | $1.09 | 2.8% | 1m49s | 1.4% | claude-opus-5-5: 10 | 4 | 32508 | 29025 | 1452960 | 28 | 0.3M |
| └ fixer | $1.01 | 2.6% | 3m20s | 2.6% | claude-opus-5-5: 13 | 1 | 22279 | 84743 | 699409 | 28 | 0.3M |
| └ fixer forks | $0.33 | 0.8% | 31s | 0.4% | claude-opus-5-5: 6 | 4 | 9425 | 9725 | 477522 | 20 | 0.1M |
| └ seed | $0.17 | 0.4% | 29s | 0.4% | claude-sonnet-5-5: 6 | 1 | 3223 | 42839 | 152987 | 14 | 0.1M |
| └ judge | $0.16 | 0.4% | 27s | 0.3% | claude-sonnet-5-5: 6 | 1 | 3174 | 36978 | 162040 | 14 | 0.1M |
| **Webster** | $7.95 | 20.2% | 34m57s | 27.0% | claude-sonnet-5-5: 150 | 6 | 199929 | 702968 | 20397401 | 326 | 3.9M |
| └ forks | $7.20 | 18.3% | 33m33s | 25.9% | claude-sonnet-5-5: 120 | 5 | 193493 | 628239 | 18457735 | 250 | 3.6M |
| └ Master | $0.75 | 1.9% | 1m33s | 1.2% | claude-sonnet-5-5: 30 | 1 | 6436 | 74729 | 1939666 | 76 | 0.3M |
| **Code-Review** | $1.62 | 4.1% | 3m31s | 2.7% | claude-sonnet-5-5: 39 | 6 | 39504 | 251118 | 2997386 | 90 | 0.8M |
| └ reviewer | $0.88 | 2.2% | 2m49s | 2.2% | claude-sonnet-5-5: 17 | 1 | 20851 | 148181 | 1508672 | 36 | 0.4M |
| └ reviewer forks | $0.47 | 1.2% | 49s | 0.6% | claude-sonnet-5-5: 10 | 3 | 15365 | 31243 | 1177768 | 26 | 0.2M |
| └ seed | $0.15 | 0.4% | 22s | 0.3% | claude-sonnet-5-5: 6 | 1 | 2158 | 38143 | 150938 | 14 | 0.1M |
| └ judge | $0.13 | 0.3% | 20s | 0.3% | claude-sonnet-5-5: 6 | 1 | 1130 | 33551 | 160008 | 14 | 0.1M |
| **Landing** | $0.93 | 2.4% | 2m23s | 1.8% | claude-opus-5-5: 18, claude-sonnet-5-5: 8 | 2 | 12143 | 109714 | 1281883 | 58 | 0.3M |
| └ fixer | $0.77 | 1.9% | 1m39s | 1.3% | claude-opus-5-5: 18 | 1 | 9001 | 75003 | 1058787 | 38 | 0.2M |
| └ describe | $0.16 | 0.4% | 44s | 0.6% | claude-sonnet-5-5: 8 | 1 | 3142 | 34711 | 223096 | 20 | 0.1M |
| **Other** | $0.73 | 1.8% | 2m53s | 2.2% | claude-sonnet-5-5: 34 | 4 | 8733 | 165021 | 1143004 | 76 | 0.4M |
| └ gate-fixer | $0.73 | 1.8% | 2m53s | 2.2% | claude-sonnet-5-5: 34 | 4 | 8733 | 165021 | 1143004 | 76 | 0.4M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 33m42s | 26.0% | | | | | | | |

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 1 | 44m15s | 9m18s | 34m57s | 36.4% |
| Plan-Write | 1 | 17m29s | 21s | 17m8s | 17.9% |
| Plan-Burler-Review | 2 | 10m0s | 10s | 9m50s | 10.2% |
| Discussion-Write | 1 | 10m19s | 49s | 9m30s | 9.9% |
| Discussion-Burler-Review | 1 | 6m45s | 5s | 6m40s | 7.0% |
| Plan-Burler-Fix | 2 | 6m59s | 55s | 6m4s | 6.3% |
| Discussion-Burler-Fix | 1 | 3m44s | 24s | 3m20s | 3.5% |
| Code-Burler-Review | 1 | 2m55s | 6s | 2m49s | 2.9% |
| Parent-Align | 1 | 7m7s | 5m28s | 1m39s | 1.7% |
| Plan-Bouncer | 3 | 1m48s | 13s | 1m35s | 1.7% |
| Discussion-Bouncer | 3 | 10m53s | 9m57s | 56s | 1.0% |
| Describe | 1 | 48s | 4s | 44s | 0.8% |
| Code-Bouncer | 2 | 50s | 8s | 42s | 0.7% |
| Code-Burler-Fix | 1 | 4s | 4s | 0s | 0.0% |
| Drift-Await | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Baseline | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Verify | 1 | 0s | 0s | 0s | 0.0% |
| Finalize | 1 | 5m30s | 5m30s | 0s | 0.0% |
| Friction-Reflect | 1 | 0s | 0s | 0s | 0.0% |
| PR-Review | 1 | 0s | 0s | 0s | 0.0% |
| Parts-Plan-Write | 1 | 0s | 0s | 0s | 0.0% |
| Parts-Write | 1 | 0s | 0s | 0s | 0.0% |
| Preflight | 1 | 10s | 10s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## cold-gate-fixer

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Code-Review** | $3.86 | 70.9% | 10m50s | 18.2% | claude-sonnet-5-5: 74 | 11 | 116526 | 492133 | 7296986 | 166 | 1.9M |
| └ fixer | $1.31 | 24.2% | 4m43s | 7.9% | claude-sonnet-5-5: 26 | 1 | 39194 | 155557 | 2661931 | 54 | 0.7M |
| └ reviewer | $1.17 | 21.5% | 5m26s | 9.1% | claude-sonnet-5-5: 18 | 1 | 34584 | 184798 | 1792022 | 38 | 0.6M |
| └ reviewer forks | $0.69 | 12.8% | 1m55s | 3.2% | claude-sonnet-5-5: 10 | 3 | 28810 | 50357 | 1403171 | 26 | 0.3M |
| └ fixer forks | $0.35 | 6.4% | 29s | 0.8% | claude-sonnet-5-5: 8 | 4 | 8767 | 14014 | 1134347 | 24 | 0.2M |
| └ seed | $0.17 | 3.2% | 18s | 0.5% | claude-sonnet-5-5: 6 | 1 | 2151 | 48954 | 144871 | 12 | 0.1M |
| └ judge | $0.16 | 2.9% | 23s | 0.6% | claude-sonnet-5-5: 6 | 1 | 3020 | 38453 | 160644 | 12 | 0.1M |
| **Landing** | $1.05 | 19.4% | 2m16s | 3.8% | claude-opus-5-5: 22, claude-sonnet-5-5: 8 | 2 | 13254 | 118828 | 1624344 | 62 | 0.4M |
| └ fixer | $0.87 | 16.1% | 1m49s | 3.1% | claude-opus-5-5: 22 | 1 | 9754 | 80670 | 1378911 | 46 | 0.3M |
| └ describe | $0.18 | 3.3% | 27s | 0.8% | claude-sonnet-5-5: 8 | 1 | 3500 | 38158 | 245433 | 16 | 0.1M |
| **Other** | $0.53 | 9.7% | 1m13s | 2.0% | claude-opus-5-5: 11 | 1 | 6955 | 59928 | 436891 | 24 | 0.2M |
| └ conflict | $0.53 | 9.7% | 1m13s | 2.0% | claude-opus-5-5: 11 | 1 | 6955 | 59928 | 436891 | 24 | 0.2M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 45m11s | 75.9% | | | | | | | |

Machine sleep left out of the span: 6h39m55s.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Code-Burler-Review | 1 | 5m29s | 3s | 5m26s | 37.9% |
| Code-Burler-Fix | 1 | 9m37s | 4m54s | 4m43s | 33.0% |
| Parent-Align | 1 | 37m39s | 34m37s | 3m2s | 21.2% |
| Code-Bouncer | 2 | 50s | 9s | 41s | 4.8% |
| Describe | 1 | 32s | 5s | 27s | 3.2% |
| Discussion-Bouncer | 4 | 0s | 0s | 0s | 0.0% |
| Discussion-Burler-Fix | 2 | 0s | 0s | 0s | 0.0% |
| Discussion-Burler-Review | 2 | 0s | 0s | 0s | 0.0% |
| Discussion-Write | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Await | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Baseline | 1 | 0s | 0s | 0s | 0.0% |
| Drift-Verify | 1 | 0s | 0s | 0s | 0.0% |
| Finalize | 1 | 5m21s | 5m21s | 0s | 0.0% |
| Friction-Reflect | 1 | 1s | 1s | 0s | 0.0% |
| PR-Review | 1 | 1s | 1s | 0s | 0.0% |
| Parts-Plan-Write | 1 | 0s | 0s | 0s | 0.0% |
| Parts-Write | 1 | 0s | 0s | 0s | 0.0% |
| Plan-Bouncer | 3 | 0s | 0s | 0s | 0.0% |
| Plan-Burler-Fix | 2 | 0s | 0s | 0s | 0.0% |
| Plan-Burler-Review | 2 | 0s | 0s | 0s | 0.0% |
| Plan-Write | 1 | 0s | 0s | 0s | 0.0% |
| Preflight | 1 | 0s | 0s | 0s | 0.0% |
| Webster | 1 | 0s | 0s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|
| batten-remote-order | 01-gitrepo-lease-and-retry, 02-gitrepo-branch-push-and-tracking-drop, 03-add-drops-stale-tracking, 04-add-adopt-only | 77097 | 59 | 305522 | 2.0M |
| batten-remote-order | 05-teardown-lock-add-discard-push, 06-remove-archive-order | 80576 | 35 | 375203 | 1.7M |
| batten-remote-order | 07-leased-weft-delete, 08-cleanup-teardown-lock, 09-reconcile-absent-pair | 83094 | 46 | 379028 | 2.2M |
| batten-remote-order | 10-batten-resume-readopt, 11-records-tag-reads, 12-readopt-failed-stop | 88234 | 55 | 402809 | 2.6M |
| batten-remote-order | 13-remove-follow-up, 14-tackler-remote-check | 91142 | 28 | 452912 | 1.8M |
| decision-record-on-board | 01-board-decision-record, 02-record-follows-entry, 03-design-prefix-refusal, 04-replay-carries-record, 05-board-get-record, 06-discussion-approval-row | 72473 | 57 | 288111 | 1.8M |
| decision-record-on-board | 07-discussion-approve-seam, 08-decision-add-copies, 09-decision-store-verb | 74529 | 39 | 256385 | 1.2M |
| decision-record-on-board | unattributed | 80406 | 12 | 107359 | 0.2M |
| shed-sub-run-block | 01-shedengine-block-routing, 02-shedrecipe-block-registry, 03-shedbuild-block-row-parse, 04-shedbuild-expand | 68362 | 28 | 270076 | 1.1M |
| shed-sub-run-block | 05-shedbuild-block-build, 06-loom-recipes-motif-arras | 70382 | 30 | 231056 | 0.9M |
| shed-sub-run-block | 07-arras-seedable | 72747 | 27 | 213306 | 0.8M |
| shed-sub-run-block | 08-profile-recipe, 09-checklist-block-marks | 74602 | 26 | 183376 | 0.7M |
| shed-sub-run-block | unattributed | 79020 | 9 | 91685 | 0.1M |

