# Token usage and time by role

Generated 2026-11-05 20:17 for raddle-recipe, pawl-orch-landing.
Machine sleep, read from the systemd journal, is left out of every time figure.
Not counted, no transcript reader: codex.

## Most expensive parts

| part | cost | share | per run |
|---|---|---|---|
| Webster forks | $58.85 | 30.6% | $29.42 |
| Plan writer forks | $21.21 | 11.0% | $10.60 |
| Plan reviewer | $18.08 | 9.4% | $9.04 |
| Plan writer | $16.34 | 8.5% | $8.17 |
| Plan fixer | $10.41 | 5.4% | $5.20 |
| Discussion reviewer | $8.46 | 4.4% | $4.23 |
| Discussion writer | $6.58 | 3.4% | $3.29 |
| Discussion fixer | $6.36 | 3.3% | $3.18 |
| Other gate-fixer | $5.34 | 2.8% | $2.67 |
| Discussion writer forks | $4.59 | 2.4% | $2.30 |

## Cost per run

| run | recipe | Discussion | Plan | Webster | Code-Review | Landing | Orch | Other | Total |
|---|---|---|---|---|---|---|---|---|---|
| raddle-recipe | tapestry | $22.35 | $43.01 | $36.45 | $4.96 | $1.22 | $0.00 | $4.53 | $112.54 |
| pawl-orch-landing | tapestry | $11.72 | $32.25 | $26.74 | $5.85 | $1.15 | $0.00 | $2.30 | $80.02 |
| average |  | $17.03 | $37.63 | $31.59 | $5.41 | $1.19 | $0.00 | $3.42 | $96.28 |

## Recipes

| recipe | runs | cost per run | span per run | landed runs | changed lines per run | cost per changed line |
|---|---|---|---|---|---|---|
| tapestry | 2 | $96.28 | 4h27m59s | 2 | 10066 | $0.010 |

A run's landed commit is the newest commit titled after the change description archived at its tag and committed within its span; its changed lines are the lines it added and deleted.

## All runs

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $75.27 | 39.1% | 2h31m50s | 28.3% | claude-opus-5-5: 721, claude-sonnet-5-5: 218 | 87 | 1322919 | 5215704 | 148350980 | 2058 | 28.0M |
| └ writer forks | $21.21 | 11.0% | 37m0s | 6.9% | claude-opus-5-5: 165, claude-sonnet-5-5: 77 | 23 | 375033 | 985000 | 54996941 | 508 | 8.6M |
| └ reviewer | $18.08 | 9.4% | 55m6s | 10.3% | claude-opus-5-5: 126, claude-sonnet-5-5: 90 | 7 | 377156 | 1505324 | 32570440 | 446 | 7.0M |
| └ writer | $16.34 | 8.5% | 32m57s | 6.1% | claude-opus-5-5: 112 | 2 | 201273 | 1294352 | 29211706 | 238 | 5.5M |
| └ fixer | $10.41 | 5.4% | 31m4s | 5.8% | claude-opus-5-5: 189 | 7 | 186318 | 774464 | 14041444 | 410 | 3.3M |
| └ reviewer forks | $4.19 | 2.2% | 5m3s | 0.9% | claude-opus-5-5: 48 | 12 | 88706 | 125662 | 8939476 | 120 | 1.5M |
| └ fixer forks | $3.51 | 1.8% | 4m48s | 0.9% | claude-opus-5-5: 81 | 28 | 71092 | 133490 | 7092399 | 218 | 1.2M |
| └ judge | $0.94 | 0.5% | 1m59s | 0.4% | claude-sonnet-5-5: 30 | 5 | 14760 | 247497 | 870640 | 70 | 0.5M |
| └ seed | $0.59 | 0.3% | 1m21s | 0.3% | claude-sonnet-5-5: 21 | 3 | 8581 | 149915 | 627934 | 48 | 0.3M |
| **Webster** | $63.19 | 32.8% | 3h26m44s | 38.6% | claude-sonnet-5-5: 771, claude-opus-5-5: 26 | 21 | 1329043 | 4502399 | 188519030 | 1678 | 31.1M |
| └ forks | $58.85 | 30.6% | 3h18m38s | 37.1% | claude-sonnet-5-5: 675 | 16 | 1296293 | 4037429 | 178934961 | 1382 | 29.4M |
| └ Master | $3.14 | 1.6% | 6m36s | 1.2% | claude-sonnet-5-5: 96 | 3 | 23469 | 320716 | 8133142 | 238 | 1.3M |
| └ recovery | $1.20 | 0.6% | 2m6s | 0.4% | claude-opus-5-5: 26 | 2 | 9281 | 144254 | 1450927 | 58 | 0.4M |
| **Discussion** | $34.07 | 17.7% | 1h22m8s | 15.3% | claude-opus-5-5: 377, claude-sonnet-5-5: 119 | 65 | 777508 | 2501564 | 46278937 | 1110 | 11.6M |
| └ reviewer | $8.46 | 4.4% | 28m59s | 5.4% | claude-opus-5-5: 87, claude-sonnet-5-5: 23 | 4 | 177109 | 731555 | 11906801 | 228 | 3.0M |
| └ writer | $6.58 | 3.4% | 25m24s | 4.7% | claude-opus-5-5: 70 | 2 | 150657 | 408366 | 7629244 | 150 | 2.0M |
| └ fixer | $6.36 | 3.3% | 24m47s | 4.6% | claude-opus-5-5: 78 | 4 | 148571 | 457844 | 5483032 | 164 | 1.9M |
| └ writer forks | $4.59 | 2.4% | 4m20s | 0.8% | claude-sonnet-5-5: 60, claude-opus-5-5: 42 | 21 | 99669 | 433949 | 8201874 | 228 | 1.9M |
| └ reviewer forks | $4.24 | 2.2% | 5m41s | 1.1% | claude-opus-5-5: 54 | 12 | 108411 | 113014 | 7543659 | 132 | 1.4M |
| └ fixer forks | $2.71 | 1.4% | 4m50s | 0.9% | claude-opus-5-5: 46 | 16 | 70389 | 80382 | 4516961 | 124 | 0.9M |
| └ judge | $0.78 | 0.4% | 2m13s | 0.4% | claude-sonnet-5-5: 24 | 4 | 18038 | 184798 | 675470 | 56 | 0.4M |
| └ seed | $0.34 | 0.2% | 46s | 0.1% | claude-sonnet-5-5: 12 | 2 | 4664 | 91656 | 321896 | 28 | 0.2M |
| **Code-Review** | $10.82 | 5.6% | 30m0s | 5.6% | claude-sonnet-5-5: 161 | 22 | 292317 | 1272807 | 23548912 | 368 | 5.4M |
| └ fixer | $4.31 | 2.2% | 17m48s | 3.3% | claude-sonnet-5-5: 52 | 2 | 137652 | 484471 | 8590571 | 108 | 2.2M |
| └ reviewer | $3.34 | 1.7% | 10m36s | 2.0% | claude-sonnet-5-5: 49 | 2 | 70829 | 481728 | 7115349 | 104 | 1.7M |
| └ reviewer forks | $1.51 | 0.8% | 3m30s | 0.7% | claude-sonnet-5-5: 20 | 6 | 52714 | 97863 | 3711996 | 52 | 0.8M |
| └ fixer forks | $0.98 | 0.5% | 1m5s | 0.2% | claude-sonnet-5-5: 16 | 8 | 20987 | 31373 | 3462499 | 48 | 0.5M |
| └ seed | $0.35 | 0.2% | 49s | 0.2% | claude-sonnet-5-5: 12 | 2 | 5239 | 93009 | 331180 | 28 | 0.2M |
| └ judge | $0.33 | 0.2% | 47s | 0.1% | claude-sonnet-5-5: 12 | 2 | 4896 | 84363 | 337317 | 28 | 0.2M |
| **Other** | $6.84 | 3.5% | 25m5s | 4.7% | claude-sonnet-5-5: 200, claude-opus-5-5: 13 | 20 | 122229 | 1171048 | 10716549 | 466 | 3.1M |
| └ gate-fixer | $5.34 | 2.8% | 22m4s | 4.1% | claude-sonnet-5-5: 193 | 18 | 102084 | 986141 | 9248690 | 422 | 2.7M |
| └ conflict | $1.34 | 0.7% | 2m35s | 0.5% | claude-opus-5-5: 13 | 1 | 17702 | 145661 | 1269984 | 28 | 0.4M |
| └ friction | $0.16 | 0.1% | 26s | 0.1% | claude-sonnet-5-5: 7 | 1 | 2443 | 39246 | 197875 | 16 | 0.1M |
| **Landing** | $2.38 | 1.2% | 5m11s | 1.0% | claude-opus-5-5: 48, claude-sonnet-5-5: 13 | 4 | 29269 | 272490 | 3674970 | 130 | 0.9M |
| └ fixer | $2.00 | 1.0% | 4m9s | 0.8% | claude-opus-5-5: 48 | 2 | 22090 | 179403 | 3291514 | 100 | 0.7M |
| └ describe | $0.38 | 0.2% | 1m2s | 0.2% | claude-sonnet-5-5: 13 | 2 | 7179 | 93087 | 383456 | 30 | 0.2M |
| **Orch**, $0.00 per run | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ unattributed | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 49m14s | 9.2% | | | | | | | |

Total cost $192.55, total weight 80.2M; 5127 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
A segment's time is the union of its parts' working time, and time shares can sum past 100% where sessions overlap, a forks row overlapping its parent.
The union of working time plus idle is the span.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 3 | 3h39m26s | 12m42s | 3h26m44s | 42.5% |
| Plan-Write | 2 | 1h11m49s | 1m41s | 1h10m8s | 14.4% |
| Plan-Burler-Review | 8 | 56m36s | 1m30s | 55m6s | 11.3% |
| Plan-Burler-Fix | 7 | 33m51s | 2m47s | 31m4s | 6.4% |
| Discussion-Burler-Review | 4 | 29m34s | 35s | 28m59s | 6.0% |
| Discussion-Write | 2 | 27m2s | 1m38s | 25m24s | 5.2% |
| Discussion-Burler-Fix | 4 | 27m1s | 2m14s | 24m47s | 5.1% |
| Code-Burler-Fix | 2 | 25m32s | 7m44s | 17m48s | 3.7% |
| Code-Burler-Review | 2 | 10m49s | 13s | 10m36s | 2.2% |
| Parent-Align | 2 | 14m58s | 8m14s | 6m44s | 1.4% |
| Plan-Bouncer | 9 | 4m40s | 1m19s | 3m21s | 0.7% |
| Discussion-Bouncer | 7 | 4m31s | 1m33s | 2m58s | 0.6% |
| Code-Bouncer | 4 | 2m5s | 29s | 1m36s | 0.3% |
| Describe | 2 | 1m10s | 8s | 1m2s | 0.2% |
| Friction-Reflect | 2 | 34s | 8s | 26s | 0.1% |
| Drift-Await | 3 | 11s | 11s | 0s | 0.0% |
| Drift-Baseline | 2 | 10s | 10s | 0s | 0.0% |
| Drift-Recheck | 2 | 8s | 8s | 0s | 0.0% |
| Drift-Verify | 2 | 5s | 5s | 0s | 0.0% |
| Finalize | 2 | 3m2s | 3m2s | 0s | 0.0% |
| Motif | 2 | 35s | 35s | 0s | 0.0% |
| PR-Review | 2 | 4s | 4s | 0s | 0.0% |
| Parts-Plan-Write | 2 | 23s | 23s | 0s | 0.0% |
| Parts-Write | 2 | 13s | 13s | 0s | 0.0% |
| Preflight | 2 | 1m28s | 1m28s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## Orchestrator

Orch sessions of the prime from 2026-11-05 08:13 to 2026-11-05 14:44, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| session | 0 | 0 | 0.0M | $0.00 |
| forks | 0 | 0 | 0.0M | $0.00 |
| total | 0 | 0 | 0.0M | $0.00 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| raddle-recipe | 0.0M | $0.00 | $112.54 | 0.0% |
| pawl-orch-landing | 0.0M | $0.00 | $80.02 | 0.0% |
| unattributed | 0.0M | $0.00 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## raddle-recipe

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $43.01 | 38.2% | 1h28m21s | 28.4% | claude-opus-5-5: 471, claude-sonnet-5-5: 150 | 58 | 770188 | 3058951 | 84155126 | 1366 | 16.1M |
| └ writer forks | $11.51 | 10.2% | 20m56s | 6.7% | claude-opus-5-5: 88, claude-sonnet-5-5: 51 | 13 | 211597 | 606251 | 28885429 | 290 | 4.7M |
| └ reviewer | $11.21 | 10.0% | 34m4s | 10.9% | claude-opus-5-5: 87, claude-sonnet-5-5: 67 | 5 | 234317 | 935486 | 20679351 | 318 | 4.4M |
| └ writer | $7.58 | 6.7% | 15m51s | 5.1% | claude-opus-5-5: 56 | 1 | 96803 | 595485 | 13332073 | 118 | 2.6M |
| └ fixer | $7.26 | 6.5% | 19m59s | 6.4% | claude-opus-5-5: 147 | 5 | 122999 | 530271 | 10730740 | 322 | 2.4M |
| └ reviewer forks | $2.39 | 2.1% | 3m11s | 1.0% | claude-opus-5-5: 33 | 9 | 51244 | 79358 | 4824798 | 84 | 0.8M |
| └ fixer forks | $2.19 | 1.9% | 2m48s | 0.9% | claude-opus-5-5: 60 | 20 | 40673 | 84215 | 4780306 | 160 | 0.8M |
| └ judge | $0.55 | 0.5% | 1m8s | 0.4% | claude-sonnet-5-5: 18 | 3 | 8446 | 142720 | 527989 | 42 | 0.3M |
| └ seed | $0.33 | 0.3% | 44s | 0.2% | claude-sonnet-5-5: 14 | 2 | 4109 | 85165 | 394440 | 32 | 0.2M |
| **Webster** | $36.45 | 32.4% | 1h57m21s | 37.7% | claude-sonnet-5-5: 460, claude-opus-5-5: 26 | 13 | 720837 | 2525634 | 110814751 | 1026 | 17.8M |
| └ forks | $33.30 | 29.6% | 1h51m33s | 35.8% | claude-sonnet-5-5: 400 | 9 | 696652 | 2176087 | 104436040 | 818 | 16.6M |
| └ Master | $1.96 | 1.7% | 4m6s | 1.3% | claude-sonnet-5-5: 60 | 2 | 14904 | 205293 | 4927784 | 150 | 0.8M |
| └ recovery | $1.20 | 1.1% | 2m6s | 0.7% | claude-opus-5-5: 26 | 2 | 9281 | 144254 | 1450927 | 58 | 0.4M |
| **Discussion** | $22.35 | 19.9% | 53m53s | 17.3% | claude-opus-5-5: 254, claude-sonnet-5-5: 81 | 42 | 507530 | 1659870 | 31517260 | 748 | 7.8M |
| └ reviewer | $6.43 | 5.7% | 21m57s | 7.1% | claude-opus-5-5: 61, claude-sonnet-5-5: 23 | 3 | 138623 | 574997 | 9494879 | 174 | 2.4M |
| └ fixer | $4.70 | 4.2% | 17m18s | 5.6% | claude-opus-5-5: 59 | 3 | 106958 | 343021 | 4217176 | 124 | 1.4M |
| └ writer | $3.19 | 2.8% | 12m32s | 4.0% | claude-opus-5-5: 35 | 1 | 76141 | 193967 | 3468115 | 76 | 1.0M |
| └ reviewer forks | $3.09 | 2.7% | 4m3s | 1.3% | claude-opus-5-5: 41 | 8 | 75758 | 78988 | 5888467 | 98 | 1.1M |
| └ writer forks | $2.39 | 2.1% | 2m0s | 0.6% | claude-sonnet-5-5: 34, claude-opus-5-5: 24 | 11 | 47969 | 237306 | 4482517 | 128 | 1.0M |
| └ fixer forks | $1.81 | 1.6% | 3m4s | 1.0% | claude-opus-5-5: 34 | 12 | 45462 | 47492 | 3299534 | 92 | 0.6M |
| └ judge | $0.59 | 0.5% | 1m42s | 0.5% | claude-sonnet-5-5: 18 | 3 | 14310 | 138951 | 506235 | 42 | 0.3M |
| └ seed | $0.17 | 0.1% | 23s | 0.1% | claude-sonnet-5-5: 6 | 1 | 2309 | 45148 | 160337 | 14 | 0.1M |
| **Code-Review** | $4.96 | 4.4% | 13m44s | 4.4% | claude-sonnet-5-5: 77 | 11 | 145918 | 603706 | 9978886 | 178 | 2.5M |
| └ fixer | $1.84 | 1.6% | 7m31s | 2.4% | claude-sonnet-5-5: 25 | 1 | 65294 | 200557 | 3423966 | 52 | 0.9M |
| └ reviewer | $1.53 | 1.4% | 5m26s | 1.7% | claude-sonnet-5-5: 21 | 1 | 37653 | 239157 | 2801659 | 46 | 0.8M |
| └ reviewer forks | $0.76 | 0.7% | 1m41s | 0.5% | claude-sonnet-5-5: 10 | 3 | 26311 | 54451 | 1813863 | 26 | 0.4M |
| └ fixer forks | $0.48 | 0.4% | 38s | 0.2% | claude-sonnet-5-5: 9 | 4 | 11670 | 19228 | 1600158 | 26 | 0.2M |
| └ seed | $0.18 | 0.2% | 24s | 0.1% | claude-sonnet-5-5: 6 | 1 | 2487 | 47990 | 170132 | 14 | 0.1M |
| └ judge | $0.16 | 0.1% | 24s | 0.1% | claude-sonnet-5-5: 6 | 1 | 2503 | 42323 | 169108 | 14 | 0.1M |
| **Other** | $4.53 | 4.0% | 16m19s | 5.2% | claude-sonnet-5-5: 112, claude-opus-5-5: 13 | 12 | 83800 | 730093 | 6639937 | 274 | 2.0M |
| └ gate-fixer | $3.03 | 2.7% | 13m18s | 4.3% | claude-sonnet-5-5: 105 | 10 | 63655 | 545186 | 5172078 | 230 | 1.5M |
| └ conflict | $1.34 | 1.2% | 2m35s | 0.8% | claude-opus-5-5: 13 | 1 | 17702 | 145661 | 1269984 | 28 | 0.4M |
| └ friction | $0.16 | 0.1% | 26s | 0.1% | claude-sonnet-5-5: 7 | 1 | 2443 | 39246 | 197875 | 16 | 0.1M |
| **Landing** | $1.22 | 1.1% | 2m23s | 0.8% | claude-opus-5-5: 24, claude-sonnet-5-5: 7 | 2 | 14062 | 142174 | 1946678 | 66 | 0.4M |
| └ fixer | $1.02 | 0.9% | 1m54s | 0.6% | claude-opus-5-5: 24 | 1 | 10408 | 93648 | 1724250 | 50 | 0.3M |
| └ describe | $0.20 | 0.2% | 30s | 0.2% | claude-sonnet-5-5: 7 | 1 | 3654 | 48526 | 222428 | 16 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 28m2s | 9.0% | | | | | | | |

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 2 | 2h6m30s | 9m9s | 1h57m21s | 41.4% |
| Plan-Write | 1 | 37m21s | 30s | 36m51s | 13.0% |
| Plan-Burler-Review | 6 | 35m18s | 1m14s | 34m4s | 12.0% |
| Discussion-Burler-Review | 3 | 22m19s | 22s | 21m57s | 7.8% |
| Plan-Burler-Fix | 5 | 22m8s | 2m9s | 19m59s | 7.1% |
| Discussion-Burler-Fix | 3 | 18m50s | 1m32s | 17m18s | 6.1% |
| Discussion-Write | 1 | 12m58s | 26s | 12m32s | 4.4% |
| Code-Burler-Fix | 1 | 10m45s | 3m14s | 7m31s | 2.7% |
| Code-Burler-Review | 1 | 5m32s | 6s | 5m26s | 1.9% |
| Parent-Align | 1 | 8m58s | 4m29s | 4m29s | 1.6% |
| Discussion-Bouncer | 4 | 2m31s | 26s | 2m5s | 0.7% |
| Plan-Bouncer | 6 | 2m55s | 1m3s | 1m52s | 0.7% |
| Code-Bouncer | 2 | 1m0s | 12s | 48s | 0.3% |
| Describe | 1 | 34s | 4s | 30s | 0.2% |
| Friction-Reflect | 1 | 32s | 6s | 26s | 0.2% |
| Drift-Await | 2 | 7s | 7s | 0s | 0.0% |
| Drift-Baseline | 1 | 2s | 2s | 0s | 0.0% |
| Drift-Recheck | 1 | 5s | 5s | 0s | 0.0% |
| Drift-Verify | 1 | 2s | 2s | 0s | 0.0% |
| Finalize | 1 | 2m27s | 2m27s | 0s | 0.0% |
| Motif | 1 | 3s | 3s | 0s | 0.0% |
| PR-Review | 1 | 2s | 2s | 0s | 0.0% |
| Parts-Plan-Write | 1 | 2s | 2s | 0s | 0.0% |
| Parts-Write | 1 | 2s | 2s | 0s | 0.0% |
| Preflight | 1 | 7s | 7s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## pawl-orch-landing

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $32.25 | 40.3% | 1h3m29s | 28.2% | claude-opus-5-5: 250, claude-sonnet-5-5: 68 | 29 | 552731 | 2156753 | 64195854 | 692 | 11.9M |
| └ writer forks | $9.70 | 12.1% | 16m4s | 7.2% | claude-opus-5-5: 77, claude-sonnet-5-5: 26 | 10 | 163436 | 378749 | 26111512 | 218 | 3.9M |
| └ writer | $8.76 | 10.9% | 17m6s | 7.6% | claude-opus-5-5: 56 | 1 | 104470 | 698867 | 15879633 | 120 | 3.0M |
| └ reviewer | $6.88 | 8.6% | 21m2s | 9.4% | claude-opus-5-5: 39, claude-sonnet-5-5: 23 | 2 | 142839 | 569838 | 11891089 | 128 | 2.6M |
| └ fixer | $3.15 | 3.9% | 11m5s | 4.9% | claude-opus-5-5: 42 | 2 | 63319 | 244193 | 3310704 | 88 | 1.0M |
| └ reviewer forks | $1.80 | 2.3% | 1m52s | 0.8% | claude-opus-5-5: 15 | 3 | 37462 | 46304 | 4114678 | 36 | 0.7M |
| └ fixer forks | $1.32 | 1.6% | 2m0s | 0.9% | claude-opus-5-5: 21 | 8 | 30419 | 49275 | 2312093 | 58 | 0.4M |
| └ judge | $0.39 | 0.5% | 51s | 0.4% | claude-sonnet-5-5: 12 | 2 | 6314 | 104777 | 342651 | 28 | 0.2M |
| └ seed | $0.25 | 0.3% | 38s | 0.3% | claude-sonnet-5-5: 7 | 1 | 4472 | 64750 | 233494 | 16 | 0.1M |
| **Webster** | $26.74 | 33.4% | 1h29m23s | 39.8% | claude-sonnet-5-5: 311 | 8 | 608206 | 1976765 | 77704279 | 652 | 13.3M |
| └ forks | $25.55 | 31.9% | 1h27m4s | 38.7% | claude-sonnet-5-5: 275 | 7 | 599641 | 1861342 | 74498921 | 564 | 12.8M |
| └ Master | $1.19 | 1.5% | 2m29s | 1.1% | claude-sonnet-5-5: 36 | 1 | 8565 | 115423 | 3205358 | 88 | 0.5M |
| **Discussion** | $11.72 | 14.6% | 28m15s | 12.6% | claude-opus-5-5: 123, claude-sonnet-5-5: 38 | 23 | 269978 | 841694 | 14761677 | 362 | 3.9M |
| └ writer | $3.39 | 4.2% | 12m51s | 5.7% | claude-opus-5-5: 35 | 1 | 74516 | 214399 | 4161129 | 74 | 1.1M |
| └ writer forks | $2.21 | 2.8% | 2m19s | 1.0% | claude-sonnet-5-5: 26, claude-opus-5-5: 18 | 10 | 51700 | 196643 | 3719357 | 100 | 0.9M |
| └ reviewer | $2.04 | 2.5% | 7m1s | 3.1% | claude-opus-5-5: 26 | 1 | 38486 | 156558 | 2411922 | 54 | 0.6M |
| └ fixer | $1.66 | 2.1% | 7m29s | 3.3% | claude-opus-5-5: 19 | 1 | 41613 | 114823 | 1265856 | 40 | 0.5M |
| └ reviewer forks | $1.15 | 1.4% | 1m38s | 0.7% | claude-opus-5-5: 13 | 4 | 32653 | 34026 | 1655192 | 34 | 0.4M |
| └ fixer forks | $0.91 | 1.1% | 1m46s | 0.8% | claude-opus-5-5: 12 | 4 | 24927 | 32890 | 1217427 | 32 | 0.3M |
| └ judge | $0.19 | 0.2% | 30s | 0.2% | claude-sonnet-5-5: 6 | 1 | 3728 | 45847 | 169235 | 14 | 0.1M |
| └ seed | $0.17 | 0.2% | 23s | 0.2% | claude-sonnet-5-5: 6 | 1 | 2355 | 46508 | 161559 | 14 | 0.1M |
| **Code-Review** | $5.85 | 7.3% | 16m16s | 7.2% | claude-sonnet-5-5: 84 | 11 | 146399 | 669101 | 13570026 | 190 | 2.9M |
| └ fixer | $2.47 | 3.1% | 10m17s | 4.6% | claude-sonnet-5-5: 27 | 1 | 72358 | 283914 | 5166605 | 56 | 1.2M |
| └ reviewer | $1.80 | 2.3% | 5m10s | 2.3% | claude-sonnet-5-5: 28 | 1 | 33176 | 242571 | 4313690 | 58 | 0.9M |
| └ reviewer forks | $0.75 | 0.9% | 1m48s | 0.8% | claude-sonnet-5-5: 10 | 3 | 26403 | 43412 | 1898133 | 26 | 0.4M |
| └ fixer forks | $0.50 | 0.6% | 26s | 0.2% | claude-sonnet-5-5: 7 | 4 | 9317 | 12145 | 1862341 | 22 | 0.2M |
| └ seed | $0.17 | 0.2% | 25s | 0.2% | claude-sonnet-5-5: 6 | 1 | 2752 | 45019 | 161048 | 14 | 0.1M |
| └ judge | $0.16 | 0.2% | 23s | 0.2% | claude-sonnet-5-5: 6 | 1 | 2393 | 42040 | 168209 | 14 | 0.1M |
| **Other** | $2.30 | 2.9% | 8m46s | 3.9% | claude-sonnet-5-5: 88 | 8 | 38429 | 440955 | 4076612 | 192 | 1.2M |
| └ gate-fixer | $2.30 | 2.9% | 8m46s | 3.9% | claude-sonnet-5-5: 88 | 8 | 38429 | 440955 | 4076612 | 192 | 1.2M |
| **Landing** | $1.15 | 1.4% | 2m48s | 1.2% | claude-opus-5-5: 24, claude-sonnet-5-5: 6 | 2 | 15207 | 130316 | 1728292 | 64 | 0.4M |
| └ fixer | $0.98 | 1.2% | 2m16s | 1.0% | claude-opus-5-5: 24 | 1 | 11682 | 85755 | 1567264 | 50 | 0.3M |
| └ describe | $0.18 | 0.2% | 32s | 0.2% | claude-sonnet-5-5: 6 | 1 | 3525 | 44561 | 161028 | 14 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 21m12s | 9.4% | | | | | | | |

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 1 | 1h32m56s | 3m33s | 1h29m23s | 43.9% |
| Plan-Write | 1 | 34m28s | 1m11s | 33m17s | 16.3% |
| Plan-Burler-Review | 2 | 21m18s | 16s | 21m2s | 10.3% |
| Discussion-Write | 1 | 14m4s | 1m13s | 12m51s | 6.3% |
| Plan-Burler-Fix | 2 | 11m43s | 38s | 11m5s | 5.4% |
| Code-Burler-Fix | 1 | 14m47s | 4m30s | 10m17s | 5.1% |
| Discussion-Burler-Fix | 1 | 8m11s | 42s | 7m29s | 3.7% |
| Discussion-Burler-Review | 1 | 7m15s | 14s | 7m1s | 3.4% |
| Code-Burler-Review | 1 | 5m17s | 7s | 5m10s | 2.5% |
| Parent-Align | 1 | 6m0s | 3m44s | 2m16s | 1.1% |
| Plan-Bouncer | 3 | 1m45s | 16s | 1m29s | 0.7% |
| Discussion-Bouncer | 3 | 2m0s | 1m7s | 53s | 0.4% |
| Code-Bouncer | 2 | 1m5s | 17s | 48s | 0.4% |
| Describe | 1 | 36s | 4s | 32s | 0.3% |
| Drift-Await | 1 | 4s | 4s | 0s | 0.0% |
| Drift-Baseline | 1 | 8s | 8s | 0s | 0.0% |
| Drift-Recheck | 1 | 3s | 3s | 0s | 0.0% |
| Drift-Verify | 1 | 3s | 3s | 0s | 0.0% |
| Finalize | 1 | 35s | 35s | 0s | 0.0% |
| Friction-Reflect | 1 | 2s | 2s | 0s | 0.0% |
| Motif | 1 | 32s | 32s | 0s | 0.0% |
| PR-Review | 1 | 2s | 2s | 0s | 0.0% |
| Parts-Plan-Write | 1 | 21s | 21s | 0s | 0.0% |
| Parts-Write | 1 | 11s | 11s | 0s | 0.0% |
| Preflight | 1 | 1m21s | 1m21s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|
| raddle-recipe | 01-shuttle-usage-split, 02-codex-usage-reader, 03-shuttle-session-end-hook, 04-usage-record-writer | 97381 | 56 | 337836 | 1.9M |
| raddle-recipe | 05-usage-wiring, 06-seed-and-landed-size | 100539 | 41 | 384876 | 1.8M |
| raddle-recipe | 07-report-config, 08-report-cost, 09-raddle-paths-and-keys, 10-raddle-proposal-spec, 11-discussion-part-splice, 12-raddleparser | 103943 | 73 | 464324 | 3.2M |
| raddle-recipe | 13-board-promote-raddle, 14-raddle-store-slug-rule, 15-raddle-store-replay, 16-board-raddle-writes, 17-board-raddle-verb, 18-record-write-stored | 83929 | 63 | 383766 | 2.7M |
| raddle-recipe | 19-raddle-checks-and-state, 20-raddle-verbs, 21-raddle-write-producer, 22-raddle-file-producer | 86554 | 66 | 470694 | 3.3M |
| raddle-recipe | 23-raddle-recipe | 89243 | 40 | 421354 | 2.0M |
| raddle-recipe | 24-batten-raddle-start-guard, 25-pawl-raddle-handoff | 92725 | 26 | 217433 | 0.8M |
| raddle-recipe | 26-report-raddle, 27-raddle-docs | 95013 | 26 | 222259 | 0.8M |
| raddle-recipe | unattributed | 99502 | 9 | 115302 | 0.1M |
| pawl-orch-landing | 01-shed-retired-rows, 02-landing-record, 03-pr-merge-state, 04-describe-function | 93471 | 34 | 290304 | 1.3M |
| pawl-orch-landing | 05-pawl-rows | 96298 | 26 | 298320 | 1.1M |
| pawl-orch-landing | 06-remove-reopen | 99088 | 23 | 298451 | 1.0M |
| pawl-orch-landing | 07-ready-beam-starts-no-member, 08-retired-move-pawl, 09-await-survives-loop-exit | 101124 | 57 | 493123 | 2.8M |
| pawl-orch-landing | 10-beam-await-entry, 11-finalize-beam-record | 103659 | 33 | 366258 | 1.7M |
| pawl-orch-landing | 12-beam-describe-verb, 13-beam-pr-open-update | 106141 | 39 | 350994 | 1.8M |
| pawl-orch-landing | 14-beam-pr-status-close, 15-beam-ready-refusals, 16-member-landing-notice, 17-beam-start-in-landing, 18-orch-role-landing | 108860 | 63 | 467045 | 3.1M |

