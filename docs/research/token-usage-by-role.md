# Token usage and time by role

Generated 2026-10-12 20:08 for the 6 most recent full loom runs: cleanup-1012, intake-bugs-1012, operator-followups, convergence-plan, run-robustness, lyx-halt.

## All runs

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| webster+sub | 17 | 49321 | 2624773 | 198425972 | 2262 | 23.4M | 21.0% | 2h22m46s | 14.8% | $46.74 | claude-sonnet-5-5: 1114 |
| plan | 6 | 616460 | 1924249 | 177194643 | 1574 | 23.2M | 20.8% | 1h41m57s | 10.6% | $57.40 | claude-opus-5-5: 766 |
| burler-review-chair | 43 | 768853 | 4353954 | 108502972 | 2584 | 20.1M | 18.1% | 2h28m27s | 15.4% | $53.68 | claude-opus-5-5: 922, claude-sonnet-5-5: 370 |
| burler-fix-chair | 35 | 276432 | 1597495 | 22945060 | 1102 | 5.7M | 5.1% | 45m36s | 4.7% | $16.26 | claude-opus-5-5: 366, claude-sonnet-5-5: 185 |
| discussion | 6 | 250465 | 946433 | 29675917 | 692 | 5.4M | 4.8% | 46m11s | 4.8% | $15.68 | claude-opus-5-5: 316 |
| plan+sub | 24 | 18431 | 586320 | 45716321 | 370 | 5.4M | 4.8% | 34m16s | 3.5% | $12.44 | claude-opus-5-5: 161 |
| bouncer-judge | 43 | 90763 | 1397919 | 6360377 | 516 | 2.8M | 2.5% | 15m4s | 1.6% | $5.68 | claude-sonnet-5-5: 258 |
| discussion+sub | 36 | 23324 | 430161 | 20691029 | 432 | 2.7M | 2.4% | 16m14s | 1.7% | $6.76 | claude-opus-5-5: 180 |
| alignment | 11 | 82804 | 674716 | 8087079 | 358 | 2.1M | 1.9% | 15m20s | 1.6% | $6.65 | claude-opus-5-5: 179 |
| driver | 6 | 54842 | 730001 | 7867780 | 442 | 2.0M | 1.8% | 5m45s | 0.6% | $0.20 | claude-haiku-5-5: 181 |
| bouncer-seed | 27 | 49265 | 725088 | 4142765 | 364 | 1.6M | 1.4% | 9m57s | 1.0% | $3.13 | claude-sonnet-5-5: 182 |
| webster | 7 | 23536 | 387686 | 6281573 | 280 | 1.2M | 1.1% | 6m35s | 0.7% | $3.04 | claude-sonnet-5-5: 123 |
| arbiter | 8 | 28284 | 287039 | 2480360 | 160 | 0.7M | 0.7% | 5m40s | 0.6% | $2.50 | claude-opus-5-5: 80 |
| describe | 9 | 29808 | 284115 | 1611419 | 134 | 0.7M | 0.6% | 4m24s | 0.5% | $1.33 | claude-sonnet-5-5: 67 |
| conflict | 4 | 29235 | 162325 | 3103010 | 164 | 0.7M | 0.6% | 7m8s | 0.7% | $2.02 | claude-opus-5-5: 82 |
| rework | 1 | 6478 | 64828 | 606227 | 26 | 0.2M | 0.2% | 1m8s | 0.1% | $0.58 | claude-opus-5-5: 13 |
| friction | 3 | 3887 | 58472 | 502569 | 46 | 0.1M | 0.1% | 59s | 0.1% | $0.29 | claude-sonnet-5-5: 23 |
| idle | | | | | | | | 6h38m12s | 41.2% | | |
| orch | | | | | | 13.6M | 12.2% | | | $33.97 | |

Total weight 111.6M; 6257 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
Time shares can sum past 100% where sessions overlap, a `+sub` row overlapping its parent.
The union of working time plus idle is the span.

### Producers

| producer | entries | elapsed | idle | share |
|---|---|---|---|---|
| Webster | 7 | 2h42m50s | 13m57s | 16.9% |
| Merge-Bouncer | 18 | 2h3m34s | 1h56m58s | 12.8% |
| Describe | 9 | 2h0m12s | 1h55m47s | 12.5% |
| Plan-Write | 6 | 1h50m49s | 1m53s | 11.5% |
| Discussion-Write | 6 | 1h31m13s | 35m15s | 9.4% |
| Plan-Burler-Review | 11 | 1h11m6s | 2m34s | 7.4% |
| Discussion-Burler-Review | 13 | 56m29s | 3m6s | 5.9% |
| Merge-In | 9 | 38m8s | 31m0s | 4.0% |
| Discussion-Burler-Fix | 13 | 24m4s | 3m12s | 2.5% |
| Webster-Burler-Review | 13 | 20m35s | 3m25s | 2.1% |
| Webster-Burler-Fix | 11 | 19m48s | 14m42s | 2.1% |
| Discussion-Bouncer | 24 | 19m14s | 6m26s | 2.0% |
| Merge-Burler-Fix | 8 | 16m58s | 9m57s | 1.8% |
| Plan-Burler-Fix | 11 | 15m18s | 2m41s | 1.6% |
| Merge-Burler-Review | 8 | 13m17s | 1m54s | 1.4% |
| Publish | 8 | 12m39s | 12m39s | 1.3% |
| PR-Align | 7 | 11m7s | 1m40s | 1.2% |
| Plan-Bouncer | 19 | 10m33s | 3m52s | 1.1% |
| Webster-Bouncer | 18 | 9m11s | 3m34s | 1.0% |
| PR-Gate | 11 | 6m17s | 4m10s | 0.7% |
| Finalize | 6 | 5m22s | 5m22s | 0.6% |
| Friction-Reflect | 6 | 1m31s | 32s | 0.2% |
| Preflight | 6 | 1m27s | 1m3s | 0.2% |
| Loom-Preflight | 6 | 1m20s | 1m19s | 0.1% |
| PR-Rework | 1 | 1m15s | 7s | 0.1% |
| Batchifier | 7 | 1m7s | 1m6s | 0.1% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.

## Orchestrator

Orch sessions of the prime from 2026-10-12 12:19 to 2026-10-12 19:18, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| orch | 1 | 515 | 54.6M | $20.56 |
| orch+sub | 35 | 475 | 39.1M | $13.41 |
| total | 36 | 990 | 93.7M | $33.97 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| cleanup-1012 | 9.0M | $3.47 | $14.35 | 19.5% |
| intake-bugs-1012 | 7.4M | $3.05 | $24.26 | 11.2% |
| operator-followups | 9.6M | $3.85 | $25.41 | 13.2% |
| convergence-plan | 11.7M | $5.08 | $52.04 | 8.9% |
| run-robustness | 14.1M | $5.31 | $44.46 | 10.7% |
| lyx-halt | 41.9M | $13.21 | $73.85 | 15.2% |
| unattributed | 0.0M | $0.00 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## cleanup-1012

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| burler-review-chair | 5 | 68638 | 385312 | 6276541 | 212 | 1.5M | 21.6% | 12m42s | 7.3% | $4.19 | claude-opus-5-5: 79, claude-sonnet-5-5: 27 |
| plan | 1 | 26778 | 128509 | 3677016 | 92 | 0.7M | 9.9% | 4m33s | 2.6% | $1.91 | claude-opus-5-5: 42 |
| discussion | 1 | 29045 | 110458 | 3271897 | 98 | 0.6M | 9.1% | 5m53s | 3.4% | $1.79 | claude-opus-5-5: 44 |
| burler-fix-chair | 4 | 22305 | 137273 | 1573163 | 96 | 0.4M | 6.6% | 4m1s | 2.3% | $1.36 | claude-opus-5-5: 38, claude-sonnet-5-5: 10 |
| discussion+sub | 6 | 4188 | 53798 | 2477983 | 70 | 0.3M | 5.0% | 2m15s | 1.3% | $0.85 | claude-opus-5-5: 29 |
| webster+sub | 1 | 2107 | 53104 | 2476830 | 58 | 0.3M | 4.8% | 2m50s | 1.6% | $0.65 | claude-sonnet-5-5: 28 |
| bouncer-judge | 5 | 8871 | 154353 | 734222 | 60 | 0.3M | 4.6% | 1m37s | 0.9% | $0.62 | claude-sonnet-5-5: 30 |
| plan+sub | 4 | 6060 | 39801 | 2228137 | 46 | 0.3M | 4.5% | 2m25s | 1.4% | $0.77 | claude-opus-5-5: 19 |
| alignment | 2 | 12988 | 91862 | 1209323 | 62 | 0.3M | 4.5% | 2m31s | 1.4% | $0.96 | claude-opus-5-5: 31 |
| bouncer-seed | 4 | 7452 | 112734 | 641196 | 56 | 0.2M | 3.6% | 1m26s | 0.8% | $0.48 | claude-sonnet-5-5: 28 |
| driver | 1 | 3213 | 85688 | 308248 | 30 | 0.2M | 2.3% | 16s | 0.1% | $0.02 | claude-haiku-5-5: 12 |
| webster | 1 | 2384 | 47055 | 481688 | 26 | 0.1M | 1.8% | 32s | 0.3% | $0.31 | claude-sonnet-5-5: 12 |
| arbiter | 1 | 4379 | 36558 | 299413 | 20 | 0.1M | 1.5% | 50s | 0.5% | $0.33 | claude-opus-5-5: 10 |
| describe | 1 | 2418 | 24758 | 127512 | 12 | 0.1M | 0.8% | 21s | 0.2% | $0.11 | claude-sonnet-5-5: 6 |
| idle | | | | | | | | 2h15m53s | 77.8% | | |
| orch | | | | | | 1.3M | 19.4% | | | $3.47 | |

### Producers

| producer | entries | elapsed | idle | share |
|---|---|---|---|---|
| Describe | 1 | 1h54m21s | 1h54m0s | 65.5% |
| Discussion-Write | 1 | 9m35s | 1m37s | 5.5% |
| Merge-In | 1 | 8m15s | 8m15s | 4.7% |
| Discussion-Burler-Review | 2 | 7m0s | 29s | 4.0% |
| Webster | 1 | 5m49s | 2m28s | 3.3% |
| Plan-Write | 1 | 5m35s | 18s | 3.2% |
| Plan-Burler-Review | 1 | 4m49s | 14s | 2.8% |
| Discussion-Burler-Fix | 2 | 3m12s | 27s | 1.8% |
| Discussion-Bouncer | 4 | 3m0s | 47s | 1.7% |
| Finalize | 1 | 2m3s | 2m3s | 1.2% |
| Publish | 1 | 1m38s | 1m38s | 0.9% |
| PR-Align | 1 | 1m13s | 15s | 0.7% |
| Merge-Burler-Review | 1 | 1m9s | 16s | 0.7% |
| Plan-Burler-Fix | 1 | 1m3s | 16s | 0.6% |
| Webster-Bouncer | 2 | 1m3s | 31s | 0.6% |
| Webster-Burler-Review | 1 | 57s | 14s | 0.5% |
| Merge-Bouncer | 2 | 56s | 13s | 0.5% |
| Plan-Bouncer | 2 | 56s | 21s | 0.5% |
| Merge-Burler-Fix | 1 | 49s | 20s | 0.5% |
| Webster-Burler-Fix | 1 | 27s | 27s | 0.3% |
| PR-Gate | 1 | 13s | 13s | 0.1% |
| Batchifier | 1 | 12s | 12s | 0.1% |
| Preflight | 1 | 12s | 8s | 0.1% |
| Loom-Preflight | 1 | 11s | 11s | 0.1% |
| Friction-Reflect | 1 | 3s | 3s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.

## intake-bugs-1012

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| burler-review-chair | 6 | 95967 | 588004 | 13038866 | 350 | 2.5M | 24.0% | 17m29s | 9.5% | $6.95 | claude-opus-5-5: 140, claude-sonnet-5-5: 35 |
| plan | 1 | 70976 | 228573 | 13728164 | 206 | 2.0M | 19.2% | 12m11s | 6.6% | $5.31 | claude-opus-5-5: 99 |
| webster+sub | 1 | 3027 | 144921 | 8286226 | 128 | 1.0M | 9.7% | 6m15s | 3.4% | $2.05 | claude-sonnet-5-5: 63 |
| discussion | 1 | 39737 | 145397 | 5832271 | 142 | 1.0M | 9.2% | 6m48s | 3.7% | $2.69 | claude-opus-5-5: 65 |
| plan+sub | 4 | 1686 | 81779 | 4576760 | 54 | 0.6M | 5.4% | 4m28s | 2.4% | $1.36 | claude-opus-5-5: 23 |
| burler-fix-chair | 4 | 22706 | 162342 | 1876119 | 104 | 0.5M | 4.8% | 3m44s | 2.0% | $1.64 | claude-opus-5-5: 52 |
| discussion+sub | 6 | 3935 | 72054 | 3715454 | 74 | 0.5M | 4.6% | 2m42s | 1.5% | $1.18 | claude-opus-5-5: 31 |
| alignment | 2 | 17672 | 138318 | 1851170 | 78 | 0.4M | 4.2% | 3m6s | 1.7% | $1.42 | claude-opus-5-5: 39 |
| bouncer-judge | 6 | 11508 | 180929 | 896463 | 72 | 0.4M | 3.6% | 1m59s | 1.1% | $0.75 | claude-sonnet-5-5: 36 |
| bouncer-seed | 4 | 7178 | 113829 | 579570 | 52 | 0.2M | 2.2% | 1m18s | 0.7% | $0.47 | claude-sonnet-5-5: 26 |
| webster | 1 | 2515 | 45334 | 472810 | 26 | 0.1M | 1.1% | 30s | 0.3% | $0.30 | claude-sonnet-5-5: 12 |
| driver | 1 | 1571 | 43925 | 161598 | 16 | 0.1M | 0.8% | 4s | 0.0% | $0.01 | claude-haiku-5-5: 7 |
| describe | 1 | 3151 | 29041 | 168146 | 14 | 0.1M | 0.7% | 30s | 0.3% | $0.14 | claude-sonnet-5-5: 7 |
| idle | | | | | | | | 2h7m35s | 69.5% | | |
| orch | | | | | | 1.1M | 10.6% | | | $3.05 | |

### Producers

| producer | entries | elapsed | idle | share |
|---|---|---|---|---|
| Merge-Bouncer | 2 | 1h54m23s | 1h53m47s | 62.3% |
| Plan-Write | 1 | 13m51s | 18s | 7.5% |
| Discussion-Write | 1 | 10m23s | 1m16s | 5.7% |
| Plan-Burler-Review | 2 | 9m18s | 28s | 5.1% |
| Webster | 1 | 8m52s | 2m8s | 4.8% |
| Discussion-Burler-Review | 2 | 6m46s | 29s | 3.7% |
| Discussion-Burler-Fix | 2 | 2m46s | 31s | 1.5% |
| Discussion-Bouncer | 3 | 1m56s | 48s | 1.1% |
| Merge-In | 1 | 1m51s | 1m51s | 1.0% |
| PR-Align | 1 | 1m51s | 15s | 1.0% |
| Plan-Burler-Fix | 2 | 1m44s | 15s | 0.9% |
| Plan-Bouncer | 3 | 1m35s | 35s | 0.9% |
| Publish | 1 | 1m29s | 1m29s | 0.8% |
| Webster-Burler-Review | 1 | 1m27s | 15s | 0.8% |
| Merge-Burler-Review | 1 | 1m24s | 13s | 0.8% |
| Webster-Bouncer | 2 | 1m2s | 29s | 0.6% |
| Describe | 1 | 47s | 17s | 0.4% |
| Webster-Burler-Fix | 1 | 27s | 27s | 0.2% |
| Merge-Burler-Fix | 1 | 23s | 23s | 0.2% |
| Preflight | 1 | 22s | 18s | 0.2% |
| Finalize | 1 | 20s | 20s | 0.2% |
| PR-Gate | 1 | 13s | 13s | 0.1% |
| Batchifier | 1 | 12s | 12s | 0.1% |
| Loom-Preflight | 1 | 11s | 11s | 0.1% |
| Friction-Reflect | 1 | 8s | 8s | 0.1% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.

## operator-followups

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| burler-review-chair | 7 | 95980 | 619478 | 14673318 | 370 | 2.7M | 23.0% | 17m32s | 23.0% | $7.07 | claude-opus-5-5: 126, claude-sonnet-5-5: 59 |
| plan | 1 | 69059 | 218639 | 12301529 | 188 | 1.8M | 15.6% | 11m36s | 15.2% | $4.94 | claude-opus-5-5: 90 |
| webster+sub | 1 | 2832 | 159082 | 9880668 | 134 | 1.2M | 10.1% | 8m12s | 10.7% | $2.40 | claude-sonnet-5-5: 66 |
| discussion | 1 | 44172 | 164976 | 6400362 | 136 | 1.1M | 9.0% | 7m28s | 9.8% | $2.99 | claude-opus-5-5: 62 |
| plan+sub | 4 | 2116 | 91960 | 5015640 | 62 | 0.6M | 5.3% | 5m37s | 7.3% | $1.51 | claude-opus-5-5: 27 |
| discussion+sub | 6 | 3751 | 72230 | 3819289 | 68 | 0.5M | 4.1% | 2m28s | 3.2% | $1.20 | claude-opus-5-5: 28 |
| driver | 1 | 14540 | 148998 | 2103910 | 114 | 0.5M | 4.0% | 1m39s | 2.2% | $0.05 | claude-haiku-5-5: 47 |
| burler-fix-chair | 5 | 20708 | 165933 | 1563107 | 102 | 0.5M | 3.9% | 3m28s | 4.5% | $1.37 | claude-opus-5-5: 32, claude-sonnet-5-5: 19 |
| bouncer-judge | 7 | 13610 | 213182 | 1035867 | 84 | 0.4M | 3.7% | 2m25s | 3.2% | $0.88 | claude-sonnet-5-5: 42 |
| alignment | 2 | 13416 | 118055 | 1019850 | 50 | 0.3M | 2.7% | 2m42s | 3.5% | $1.06 | claude-opus-5-5: 25 |
| bouncer-seed | 4 | 7104 | 106957 | 595057 | 52 | 0.2M | 1.9% | 1m20s | 1.8% | $0.46 | claude-sonnet-5-5: 26 |
| arbiter | 2 | 5491 | 67326 | 568066 | 38 | 0.2M | 1.4% | 1m6s | 1.4% | $0.56 | claude-opus-5-5: 19 |
| describe | 2 | 5479 | 68904 | 350652 | 30 | 0.1M | 1.3% | 51s | 1.1% | $0.30 | claude-sonnet-5-5: 15 |
| webster | 1 | 2304 | 50731 | 396400 | 22 | 0.1M | 1.0% | 36s | 0.8% | $0.31 | claude-sonnet-5-5: 10 |
| conflict | 1 | 2821 | 25403 | 328823 | 24 | 0.1M | 0.7% | 41s | 0.9% | $0.25 | claude-opus-5-5: 12 |
| friction | 1 | 888 | 17700 | 122427 | 12 | 0.0M | 0.3% | 14s | 0.3% | $0.08 | claude-sonnet-5-5: 6 |
| idle | | | | | | | | 14m36s | 19.1% | | |
| orch | | | | | | 1.4M | 12.0% | | | $3.85 | |

### Producers

| producer | entries | elapsed | idle | share |
|---|---|---|---|---|
| Plan-Write | 1 | 13m23s | 19s | 17.5% |
| Discussion-Write | 1 | 10m19s | 46s | 13.5% |
| Webster | 1 | 9m38s | 51s | 12.6% |
| Plan-Burler-Review | 1 | 7m43s | 16s | 10.1% |
| Discussion-Burler-Review | 2 | 6m39s | 22s | 8.7% |
| Webster-Burler-Review | 4 | 5m15s | 1m3s | 6.9% |
| Discussion-Bouncer | 4 | 2m49s | 52s | 3.7% |
| Webster-Burler-Fix | 3 | 2m20s | 1m21s | 3.1% |
| Discussion-Burler-Fix | 2 | 2m19s | 30s | 3.0% |
| Merge-In | 2 | 2m15s | 1m34s | 2.9% |
| Webster-Bouncer | 4 | 1m50s | 35s | 2.4% |
| Publish | 1 | 1m38s | 1m38s | 2.1% |
| PR-Align | 1 | 1m31s | 16s | 2.0% |
| PR-Gate | 2 | 1m29s | 58s | 1.9% |
| Describe | 2 | 1m21s | 29s | 1.8% |
| Merge-Burler-Review | 1 | 1m10s | 14s | 1.5% |
| Plan-Bouncer | 2 | 1m8s | 29s | 1.5% |
| Plan-Burler-Fix | 1 | 58s | 18s | 1.3% |
| Merge-Bouncer | 2 | 56s | 21s | 1.2% |
| Merge-Burler-Fix | 1 | 30s | 30s | 0.7% |
| Finalize | 1 | 21s | 21s | 0.5% |
| Friction-Reflect | 1 | 21s | 7s | 0.5% |
| Preflight | 1 | 14s | 10s | 0.3% |
| Loom-Preflight | 1 | 12s | 12s | 0.3% |
| Batchifier | 1 | 4s | 4s | 0.1% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.

## convergence-plan

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| webster+sub | 4 | 11505 | 649736 | 45461895 | 500 | 5.4M | 23.4% | 38m59s | 23.5% | $10.83 | claude-sonnet-5-5: 246 |
| plan | 1 | 152485 | 408486 | 38009707 | 328 | 5.1M | 21.9% | 25m0s | 15.1% | $12.70 | claude-opus-5-5: 162 |
| burler-review-chair | 8 | 155533 | 839773 | 19113959 | 486 | 3.7M | 16.1% | 35m42s | 21.5% | $10.35 | claude-opus-5-5: 184, claude-sonnet-5-5: 59 |
| plan+sub | 4 | 4845 | 176551 | 11962734 | 82 | 1.4M | 6.2% | 10m49s | 6.5% | $3.37 | claude-opus-5-5: 37 |
| burler-fix-chair | 6 | 69270 | 354043 | 6009250 | 242 | 1.4M | 6.0% | 11m48s | 7.1% | $4.24 | claude-opus-5-5: 103, claude-sonnet-5-5: 18 |
| discussion | 1 | 40791 | 148237 | 4513821 | 108 | 0.8M | 3.6% | 9m9s | 5.5% | $2.46 | claude-opus-5-5: 48 |
| alignment | 3 | 22104 | 174809 | 2380536 | 102 | 0.6M | 2.4% | 4m10s | 2.5% | $1.79 | claude-opus-5-5: 51 |
| bouncer-judge | 8 | 17675 | 283830 | 1169596 | 96 | 0.6M | 2.4% | 2m51s | 1.7% | $1.12 | claude-sonnet-5-5: 48 |
| discussion+sub | 6 | 3144 | 70600 | 3458240 | 70 | 0.4M | 1.9% | 2m55s | 1.8% | $1.11 | claude-opus-5-5: 29 |
| bouncer-seed | 6 | 11742 | 157676 | 995824 | 86 | 0.4M | 1.5% | 2m25s | 1.5% | $0.71 | claude-sonnet-5-5: 43 |
| driver | 1 | 8901 | 126669 | 1304279 | 82 | 0.3M | 1.4% | 56s | 0.6% | $0.03 | claude-haiku-5-5: 33 |
| webster | 2 | 5836 | 99203 | 1388818 | 70 | 0.3M | 1.3% | 1m31s | 0.9% | $0.73 | claude-sonnet-5-5: 31 |
| conflict | 1 | 14067 | 53373 | 1383682 | 64 | 0.3M | 1.2% | 3m47s | 2.3% | $0.83 | claude-opus-5-5: 32 |
| arbiter | 2 | 9280 | 85330 | 825871 | 46 | 0.2M | 1.0% | 1m57s | 1.2% | $0.78 | claude-opus-5-5: 23 |
| rework | 1 | 6478 | 64828 | 606227 | 26 | 0.2M | 0.8% | 1m8s | 0.7% | $0.58 | claude-opus-5-5: 13 |
| describe | 2 | 6952 | 66429 | 416566 | 32 | 0.2M | 0.7% | 1m3s | 0.6% | $0.32 | claude-sonnet-5-5: 16 |
| friction | 1 | 1569 | 18164 | 172787 | 16 | 0.0M | 0.2% | 21s | 0.2% | $0.10 | claude-sonnet-5-5: 8 |
| idle | | | | | | | | 23m51s | 14.4% | | |
| orch | | | | | | 1.8M | 7.9% | | | $5.08 | |

### Producers

| producer | entries | elapsed | idle | share |
|---|---|---|---|---|
| Webster | 2 | 44m46s | 4m21s | 27.0% |
| Plan-Write | 1 | 25m56s | 18s | 15.6% |
| Discussion-Burler-Review | 3 | 21m19s | 44s | 12.9% |
| Plan-Burler-Review | 2 | 11m56s | 24s | 7.2% |
| Discussion-Write | 1 | 11m52s | 41s | 7.2% |
| Discussion-Burler-Fix | 3 | 8m34s | 48s | 5.2% |
| Merge-In | 2 | 6m14s | 2m27s | 3.8% |
| Discussion-Bouncer | 6 | 6m13s | 1m56s | 3.7% |
| Plan-Burler-Fix | 2 | 3m36s | 24s | 2.2% |
| Publish | 2 | 3m14s | 3m14s | 1.9% |
| PR-Align | 2 | 3m11s | 26s | 1.9% |
| Merge-Burler-Fix | 1 | 2m43s | 1m53s | 1.6% |
| Webster-Bouncer | 4 | 2m15s | 1m5s | 1.4% |
| Merge-Burler-Review | 1 | 2m12s | 15s | 1.3% |
| Webster-Burler-Review | 2 | 2m3s | 25s | 1.2% |
| Plan-Bouncer | 4 | 1m56s | 43s | 1.2% |
| PR-Gate | 3 | 1m34s | 1m0s | 0.9% |
| Describe | 2 | 1m32s | 29s | 0.9% |
| Merge-Bouncer | 3 | 1m16s | 26s | 0.8% |
| PR-Rework | 1 | 1m15s | 7s | 0.8% |
| Webster-Burler-Fix | 2 | 50s | 50s | 0.5% |
| Friction-Reflect | 1 | 26s | 5s | 0.3% |
| Finalize | 1 | 19s | 19s | 0.2% |
| Batchifier | 2 | 15s | 15s | 0.2% |
| Loom-Preflight | 1 | 11s | 11s | 0.1% |
| Preflight | 1 | 11s | 7s | 0.1% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.

## run-robustness

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| burler-review-chair | 10 | 178311 | 1011080 | 26789595 | 614 | 4.8M | 22.9% | 32m40s | 20.4% | $12.23 | claude-opus-5-5: 178, claude-sonnet-5-5: 129 |
| plan | 1 | 107281 | 327502 | 27843596 | 282 | 3.7M | 17.7% | 17m27s | 10.9% | $9.35 | claude-opus-5-5: 137 |
| webster+sub | 4 | 9120 | 444111 | 30700143 | 406 | 3.7M | 17.4% | 25m41s | 16.0% | $7.34 | claude-sonnet-5-5: 199 |
| burler-fix-chair | 10 | 77330 | 446270 | 6750689 | 332 | 1.6M | 7.7% | 12m32s | 7.8% | $4.20 | claude-sonnet-5-5: 90, claude-opus-5-5: 76 |
| plan+sub | 4 | 2493 | 148776 | 8687355 | 74 | 1.1M | 5.1% | 8m46s | 5.5% | $2.53 | claude-opus-5-5: 33 |
| discussion | 1 | 46551 | 152333 | 4606390 | 96 | 0.9M | 4.2% | 7m34s | 4.7% | $2.61 | claude-opus-5-5: 47 |
| driver | 1 | 20222 | 265423 | 3322436 | 154 | 0.8M | 3.6% | 2m17s | 1.4% | $0.08 | claude-haiku-5-5: 63 |
| bouncer-judge | 10 | 22474 | 327918 | 1476846 | 120 | 0.7M | 3.2% | 3m33s | 2.2% | $1.34 | claude-sonnet-5-5: 60 |
| discussion+sub | 6 | 4355 | 75106 | 3846009 | 78 | 0.5M | 2.4% | 2m52s | 1.8% | $1.23 | claude-opus-5-5: 33 |
| bouncer-seed | 5 | 8748 | 123578 | 734429 | 66 | 0.3M | 1.3% | 1m47s | 1.1% | $0.54 | claude-sonnet-5-5: 33 |
| webster | 1 | 5261 | 72115 | 1529984 | 60 | 0.3M | 1.3% | 1m35s | 1.0% | $0.65 | claude-sonnet-5-5: 26 |
| arbiter | 3 | 9134 | 97825 | 787010 | 56 | 0.2M | 1.2% | 1m46s | 1.1% | $0.83 | claude-opus-5-5: 28 |
| alignment | 1 | 8805 | 71704 | 830506 | 32 | 0.2M | 1.0% | 1m30s | 0.9% | $0.70 | claude-opus-5-5: 16 |
| conflict | 1 | 5376 | 36115 | 706413 | 42 | 0.1M | 0.7% | 1m28s | 0.9% | $0.43 | claude-opus-5-5: 21 |
| describe | 2 | 6524 | 60851 | 315065 | 28 | 0.1M | 0.7% | 1m1s | 0.6% | $0.28 | claude-sonnet-5-5: 14 |
| friction | 1 | 1430 | 22608 | 207355 | 18 | 0.1M | 0.3% | 24s | 0.2% | $0.11 | claude-sonnet-5-5: 9 |
| idle | | | | | | | | 47m13s | 29.5% | | |
| orch | | | | | | 2.0M | 9.6% | | | $5.31 | |

### Producers

| producer | entries | elapsed | idle | share |
|---|---|---|---|---|
| Webster | 1 | 29m22s | 2m16s | 18.3% |
| Plan-Write | 1 | 19m21s | 17s | 12.1% |
| Plan-Burler-Review | 2 | 16m50s | 26s | 10.5% |
| Merge-In | 2 | 16m26s | 14m58s | 10.3% |
| Webster-Burler-Fix | 3 | 12m1s | 9m27s | 7.5% |
| Merge-Burler-Fix | 3 | 8m50s | 5m26s | 5.5% |
| Discussion-Write | 1 | 8m26s | 31s | 5.3% |
| Webster-Burler-Review | 4 | 8m6s | 1m12s | 5.1% |
| Discussion-Burler-Review | 2 | 6m26s | 26s | 4.0% |
| Merge-Bouncer | 7 | 5m1s | 1m54s | 3.1% |
| Merge-Burler-Review | 3 | 4m51s | 48s | 3.0% |
| Plan-Burler-Fix | 2 | 4m10s | 35s | 2.6% |
| Discussion-Burler-Fix | 2 | 3m24s | 26s | 2.1% |
| Discussion-Bouncer | 4 | 3m13s | 1m13s | 2.0% |
| Publish | 2 | 3m2s | 3m2s | 1.9% |
| Plan-Bouncer | 4 | 2m40s | 58s | 1.7% |
| Webster-Bouncer | 4 | 1m51s | 35s | 1.2% |
| PR-Align | 1 | 1m44s | 14s | 1.1% |
| PR-Gate | 2 | 1m34s | 1m2s | 1.0% |
| Describe | 2 | 1m28s | 27s | 0.9% |
| Friction-Reflect | 1 | 30s | 6s | 0.3% |
| Finalize | 1 | 19s | 19s | 0.2% |
| Loom-Preflight | 1 | 18s | 18s | 0.2% |
| Batchifier | 1 | 12s | 12s | 0.1% |
| Preflight | 1 | 11s | 6s | 0.1% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.

## lyx-halt

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| webster+sub | 6 | 20730 | 1173819 | 101620210 | 1036 | 11.7M | 30.7% | 1h0m48s | 29.7% | $23.47 | claude-sonnet-5-5: 512 |
| plan | 1 | 189881 | 612540 | 81634631 | 478 | 9.9M | 25.9% | 31m10s | 15.2% | $23.19 | claude-opus-5-5: 236 |
| burler-review-chair | 7 | 174424 | 910307 | 28610693 | 552 | 4.9M | 12.8% | 32m22s | 15.8% | $12.89 | claude-opus-5-5: 215, claude-sonnet-5-5: 61 |
| plan+sub | 4 | 1231 | 47453 | 13245695 | 52 | 1.4M | 3.6% | 2m11s | 1.1% | $2.91 | claude-opus-5-5: 22 |
| burler-fix-chair | 6 | 64113 | 331634 | 5172732 | 226 | 1.3M | 3.3% | 10m2s | 4.9% | $3.46 | claude-opus-5-5: 65, claude-sonnet-5-5: 48 |
| discussion | 1 | 50169 | 225032 | 5051176 | 112 | 1.0M | 2.7% | 9m19s | 4.6% | $3.14 | claude-opus-5-5: 50 |
| bouncer-judge | 7 | 16625 | 237707 | 1047383 | 84 | 0.5M | 1.3% | 2m39s | 1.3% | $0.97 | claude-sonnet-5-5: 42 |
| discussion+sub | 6 | 3951 | 86373 | 3374054 | 72 | 0.5M | 1.2% | 3m2s | 1.5% | $1.19 | claude-opus-5-5: 30 |
| webster | 1 | 5236 | 73248 | 2011873 | 76 | 0.3M | 0.8% | 1m50s | 0.9% | $0.75 | claude-sonnet-5-5: 32 |
| bouncer-seed | 4 | 7041 | 110314 | 596689 | 52 | 0.2M | 0.6% | 1m39s | 0.8% | $0.47 | claude-sonnet-5-5: 26 |
| alignment | 1 | 7819 | 79968 | 795694 | 34 | 0.2M | 0.6% | 1m22s | 0.7% | $0.72 | claude-opus-5-5: 17 |
| driver | 1 | 6395 | 59298 | 667309 | 46 | 0.2M | 0.5% | 34s | 0.3% | $0.02 | claude-haiku-5-5: 19 |
| conflict | 1 | 6971 | 47434 | 684092 | 34 | 0.2M | 0.4% | 1m13s | 0.6% | $0.51 | claude-opus-5-5: 17 |
| describe | 1 | 5284 | 34132 | 233478 | 18 | 0.1M | 0.2% | 38s | 0.3% | $0.18 | claude-sonnet-5-5: 9 |
| idle | | | | | | | | 49m3s | 24.0% | | |
| orch | | | | | | 5.9M | 15.4% | | | $13.21 | |

### Producers

| producer | entries | elapsed | idle | share |
|---|---|---|---|---|
| Webster | 1 | 1h4m23s | 1m53s | 31.5% |
| Discussion-Write | 1 | 40m38s | 30m25s | 19.9% |
| Plan-Write | 1 | 32m43s | 23s | 16.0% |
| Plan-Burler-Review | 3 | 20m30s | 46s | 10.0% |
| Discussion-Burler-Review | 2 | 8m19s | 37s | 4.1% |
| Discussion-Burler-Fix | 2 | 3m49s | 30s | 1.9% |
| Plan-Burler-Fix | 3 | 3m47s | 54s | 1.8% |
| Merge-Burler-Fix | 1 | 3m43s | 1m25s | 1.8% |
| Webster-Burler-Fix | 1 | 3m43s | 2m11s | 1.8% |
| Merge-In | 1 | 3m7s | 1m54s | 1.5% |
| Webster-Burler-Review | 1 | 2m47s | 16s | 1.4% |
| Merge-Burler-Review | 1 | 2m31s | 7s | 1.2% |
| Plan-Bouncer | 4 | 2m18s | 46s | 1.1% |
| Discussion-Bouncer | 3 | 2m3s | 51s | 1.0% |
| Finalize | 1 | 2m0s | 2m0s | 1.0% |
| Publish | 1 | 1m38s | 1m38s | 0.8% |
| PR-Align | 1 | 1m37s | 15s | 0.8% |
| PR-Gate | 2 | 1m14s | 44s | 0.6% |
| Webster-Bouncer | 2 | 1m10s | 19s | 0.6% |
| Merge-Bouncer | 2 | 1m2s | 18s | 0.5% |
| Describe | 1 | 43s | 5s | 0.4% |
| Loom-Preflight | 1 | 17s | 16s | 0.1% |
| Preflight | 1 | 17s | 14s | 0.1% |
| Batchifier | 1 | 12s | 12s | 0.1% |
| Friction-Reflect | 1 | 3s | 3s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|
| cleanup-1012 | 01-tokencount-no-cutoff, 02-reader-paren-dollar, 03-vocab-root-replaceall | 53326 | 28 | 105642 | 0.3M |
| intake-bugs-1012 | 01-external-test-unit-dir, 02-verify-package-missing, 03-gate-test-package-dirs, 04-loop-status-stop, 05-cli-runnable-group-rule | 50406 | 63 | 194539 | 1.0M |
| operator-followups | 01-overview-pause-marker, 02-hold-deferral-held-advisor, 03-paste-stdin-buffer, 04-shortcutcli-package, 05-mount-shortcuts, 06-loom-status-eighth | 56083 | 66 | 214387 | 1.2M |
| convergence-plan | 01-halt-note-leaf, 02-shed-record-parsers, 03-loom-record-readers, 04-alignment-record-readers, 05-review-round-and-decision-readers | 54463 | 56 | 165851 | 0.8M |
| convergence-plan | 06-board-and-archive-readers, 07-batten-change-classifier, 08-gitrepo-history-reads, 09-gitrepo-landing-reads, 10-report-run-selection | 58162 | 64 | 231734 | 1.2M |
| convergence-plan | 11-report-wakes-and-stops, 12-report-repairs-and-rounds, 13-report-landings, 14-report-weeks-and-markdown, 15-report-cli | 60927 | 114 | 403606 | 3.3M |
| convergence-plan | 16-single-seed-parser | 50458 | 12 | 69431 | 0.1M |
| run-robustness | 01-board-encoding-pin, 02-state-tolerant-read, 03-state-keep-unknown, 04-state-rename, 05-shedrun-seed-tolerant | 58853 | 74 | 230729 | 1.3M |
| run-robustness | 06-batten-cleanup-tolerant, 07-pattern-tolerant-records, 08-gate-busy-quit-key, 09-proc-tree-cpu, 10-gateslot-watchdog-line | 68773 | 45 | 149806 | 0.6M |
| run-robustness | 11-gate-test-watchdog, 12-webster-hang-gate, 13-loom-watchdog-halt, 14-landing-watchdog-halt | 71492 | 74 | 255183 | 1.6M |
| run-robustness | unattributed | 78456 | 6 | 82883 | 0.1M |
| lyx-halt | 01-pause-record, 02-sleep-watch, 03-network-gate, 04-pause-geometry, 05-gitexec-network-hold, 06-github-network-hold | 63647 | 54 | 171148 | 0.8M |
| lyx-halt | 07-parentlock-covered, 08-shuttle-pause, 09-shuttle-api-continue, 10-shuttle-step-tie, 11-shuttle-pending-prompt, 12-runner-pause-wiring | 67794 | 113 | 326999 | 2.8M |
| lyx-halt | 13-shuttle-send-refusal, 14-reed-resume-hold, 15-start-refusals, 16-batten-sleep-clock, 17-batten-pause-reader, 18-loom-idle-release-pause | 70789 | 98 | 284993 | 2.2M |
| lyx-halt | 19-watchdog-awake, 20-watchdog-kill-record, 21-orch-pause, 22-pause-engine, 23-pause-cli | 73379 | 142 | 410711 | 4.0M |
| lyx-halt | 24-status-paused-since, 25-loom-bare-pause-refusal, 26-shed-pause-way-forward, 27-lock-wait-pause-after | 75931 | 68 | 240033 | 1.4M |
| lyx-halt | 28-webster-pause-retire | 78647 | 37 | 165454 | 0.6M |

