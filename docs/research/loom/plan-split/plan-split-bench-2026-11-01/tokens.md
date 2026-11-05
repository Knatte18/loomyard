# Token usage and time by role

Generated 2026-11-01 19:03 for psb-wv-plan, psb-wv-split, psb-rv-plan, psb-rv-split.
Machine sleep, read from the systemd journal, is left out of every time figure.

## Most expensive parts

| part | cost | share | per run |
|---|---|---|---|
| Plan writer | $24.80 | 27.9% | $6.20 |
| Plan writer forks | $18.16 | 20.4% | $4.54 |
| Plan reviewer | $11.72 | 13.2% | $2.93 |
| Plan reviewer forks | $4.59 | 5.2% | $1.15 |
| Other parts-plan | $3.73 | 4.2% | $0.93 |
| Other parts-plan-fabric-warp-seam | $3.35 | 3.8% | $0.84 |
| Other parts-plan-heddle | $3.07 | 3.5% | $0.77 |
| Other parts-plan-bolt-replay-seam | $2.72 | 3.1% | $0.68 |
| Other parts-plan-bolt-dir-name | $2.72 | 3.1% | $0.68 |
| Other parts-plan-config-split | $2.20 | 2.5% | $0.55 |

## Cost per run

| run | Discussion | Plan | Webster | Merge | Alignment | Orch | Other | Total |
|---|---|---|---|---|---|---|---|---|
| psb-wv-plan | $0.00 | $19.88 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $19.88 |
| psb-wv-split | $0.00 | $5.74 | $0.00 | $0.00 | $0.00 | $0.00 | $16.69 | $22.43 |
| psb-rv-plan | $0.00 | $34.09 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $34.09 |
| psb-rv-split | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $12.52 | $12.52 |
| average | $0.00 | $14.93 | $0.00 | $0.00 | $0.00 | $0.00 | $7.30 | $22.23 |

## All runs

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $59.70 | 67.2% | 1h28m57s | 79.7% | claude-opus-5-5: 427, claude-sonnet-5-5: 18 | 29 | 738357 | 3281005 | 144349588 | 952 | 22.2M |
| └ writer | $24.80 | 27.9% | 31m2s | 27.8% | claude-opus-5-5: 157 | 2 | 169926 | 1881125 | 59972451 | 322 | 9.2M |
| └ writer forks | $18.16 | 20.4% | 29m35s | 26.5% | claude-opus-5-5: 116 | 12 | 257480 | 376110 | 55640317 | 256 | 7.3M |
| └ reviewer | $11.72 | 13.2% | 31m47s | 28.5% | claude-opus-5-5: 107 | 3 | 204966 | 778911 | 18627520 | 220 | 3.9M |
| └ reviewer forks | $4.59 | 5.2% | 5m36s | 5.0% | claude-opus-5-5: 47 | 9 | 100344 | 130120 | 9665715 | 112 | 1.6M |
| └ seed | $0.43 | 0.5% | 1m3s | 0.9% | claude-sonnet-5-5: 18 | 3 | 5641 | 114739 | 443585 | 42 | 0.2M |
| **Other** | $29.21 | 32.8% | 21m25s | 19.2% | claude-opus-5-5: 375 | 15 | 398757 | 2511043 | 43357305 | 848 | 9.5M |
| └ parts-plan | $3.73 | 4.2% | 8m38s | 7.7% | claude-opus-5-5: 82 | 2 | 47618 | 262561 | 7297599 | 214 | 1.3M |
| └ parts-plan-fabric-warp-seam | $3.35 | 3.8% | 7m46s | 7.0% | claude-opus-5-5: 37 | 1 | 48683 | 234640 | 6013789 | 78 | 1.1M |
| └ parts-plan-heddle | $3.07 | 3.5% | 7m51s | 7.0% | claude-opus-5-5: 37 | 1 | 51898 | 193098 | 5329480 | 80 | 1.0M |
| └ parts-plan-bolt-replay-seam | $2.72 | 3.1% | 8m11s | 7.3% | claude-opus-5-5: 26 | 1 | 51694 | 205925 | 3301772 | 56 | 0.8M |
| └ parts-plan-bolt-dir-name | $2.72 | 3.1% | 6m48s | 6.1% | claude-opus-5-5: 29 | 1 | 44802 | 200192 | 4119324 | 60 | 0.9M |
| └ parts-plan-config-split | $2.20 | 2.5% | 3m16s | 2.9% | claude-opus-5-5: 22 | 1 | 20263 | 235040 | 3094981 | 48 | 0.7M |
| └ parts-plan-loom-wiring | $2.20 | 2.5% | 2m57s | 2.7% | claude-opus-5-5: 18 | 1 | 18017 | 255724 | 2781081 | 38 | 0.7M |
| └ parts-plan-record-write | $1.85 | 2.1% | 2m54s | 2.6% | claude-opus-5-5: 16 | 1 | 17517 | 217612 | 2051534 | 36 | 0.6M |
| └ parts-plan-preflight | $1.68 | 1.9% | 3m17s | 2.9% | claude-opus-5-5: 22 | 1 | 20084 | 160182 | 2408503 | 48 | 0.5M |
| └ parts-plan-recipe-contract | $1.57 | 1.8% | 3m24s | 3.0% | claude-opus-5-5: 17 | 1 | 21168 | 158618 | 1756016 | 36 | 0.5M |
| └ parts | $1.56 | 1.7% | 4m29s | 4.0% | claude-opus-5-5: 30 | 2 | 29595 | 132177 | 1513755 | 64 | 0.5M |
| └ parts-plan-landing-describe | $1.37 | 1.5% | 2m45s | 2.5% | claude-opus-5-5: 24 | 1 | 16376 | 117230 | 2273161 | 58 | 0.5M |
| └ parts-plan-prompt-seam | $1.19 | 1.3% | 1m48s | 1.6% | claude-opus-5-5: 15 | 1 | 11042 | 138044 | 1416310 | 32 | 0.4M |
| **Orch**, $0.00 per run | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ unattributed | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 1m16s | 1.1% | | | | | | | |

Total cost $88.91, total weight 31.7M; 1504 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
A segment's time is the union of its parts' working time, and time shares can sum past 100% where sessions overlap, a forks row overlapping its parent.
The union of working time plus idle is the span.

## Orchestrator

Orch sessions of the prime from 2026-11-01 18:18 to 2026-11-01 19:03, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| session | 0 | 0 | 0.0M | $0.00 |
| forks | 0 | 0 | 0.0M | $0.00 |
| total | 0 | 0 | 0.0M | $0.00 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| psb-wv-plan | 0.0M | $0.00 | $19.88 | 0.0% |
| psb-wv-split | 0.0M | $0.00 | $22.43 | 0.0% |
| psb-rv-plan | 0.0M | $0.00 | $34.09 | 0.0% |
| psb-rv-split | 0.0M | $0.00 | $12.52 | 0.0% |
| unattributed | 0.0M | $0.00 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## psb-wv-plan

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $19.88 | 100.0% | 35m2s | 99.1% | claude-opus-5-5: 151, claude-sonnet-5-5: 6 | 12 | 288683 | 1197246 | 41071185 | 340 | 7.0M |
| └ writer forks | $7.72 | 38.8% | 14m14s | 40.2% | claude-opus-5-5: 58 | 6 | 119430 | 200045 | 21645925 | 128 | 3.0M |
| └ writer | $7.21 | 36.3% | 11m54s | 33.7% | claude-opus-5-5: 48 | 1 | 68490 | 693303 | 11857760 | 100 | 2.4M |
| └ reviewer | $3.48 | 17.5% | 10m1s | 28.3% | claude-opus-5-5: 30 | 1 | 67816 | 237426 | 4670468 | 62 | 1.1M |
| └ reviewer forks | $1.35 | 6.8% | 1m39s | 4.6% | claude-opus-5-5: 15 | 3 | 31224 | 33899 | 2756470 | 36 | 0.5M |
| └ seed | $0.13 | 0.6% | 18s | 0.8% | claude-sonnet-5-5: 6 | 1 | 1723 | 32573 | 140562 | 14 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 20s | 0.9% | | | | | | | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## psb-wv-split

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Other** | $16.69 | 74.4% | 14m56s | 59.8% | claude-opus-5-5: 219 | 7 | 258446 | 1229177 | 26852798 | 496 | 5.5M |
| └ parts-plan-fabric-warp-seam | $3.35 | 14.9% | 7m46s | 31.1% | claude-opus-5-5: 37 | 1 | 48683 | 234640 | 6013789 | 78 | 1.1M |
| └ parts-plan-heddle | $3.07 | 13.7% | 7m51s | 31.4% | claude-opus-5-5: 37 | 1 | 51898 | 193098 | 5329480 | 80 | 1.0M |
| └ parts-plan | $2.94 | 13.1% | 7m0s | 28.0% | claude-opus-5-5: 61 | 1 | 38399 | 192716 | 6021667 | 160 | 1.0M |
| └ parts-plan-bolt-replay-seam | $2.72 | 12.1% | 8m11s | 32.8% | claude-opus-5-5: 26 | 1 | 51694 | 205925 | 3301772 | 56 | 0.8M |
| └ parts-plan-bolt-dir-name | $2.72 | 12.1% | 6m48s | 27.2% | claude-opus-5-5: 29 | 1 | 44802 | 200192 | 4119324 | 60 | 0.9M |
| └ parts-plan-prompt-seam | $1.19 | 5.3% | 1m48s | 7.2% | claude-opus-5-5: 15 | 1 | 11042 | 138044 | 1416310 | 32 | 0.4M |
| └ parts | $0.69 | 3.1% | 1m49s | 7.3% | claude-opus-5-5: 14 | 1 | 11928 | 64562 | 650456 | 30 | 0.2M |
| **Plan** | $5.74 | 25.6% | 9m34s | 38.3% | claude-opus-5-5: 54, claude-sonnet-5-5: 6 | 5 | 97967 | 353456 | 10605024 | 130 | 2.0M |
| └ reviewer | $3.94 | 17.6% | 9m12s | 36.8% | claude-opus-5-5: 38 | 1 | 59596 | 273697 | 6918018 | 78 | 1.3M |
| └ reviewer forks | $1.66 | 7.4% | 1m50s | 7.3% | claude-opus-5-5: 16 | 3 | 36401 | 44242 | 3543770 | 38 | 0.6M |
| └ seed | $0.14 | 0.6% | 22s | 1.4% | claude-sonnet-5-5: 6 | 1 | 1970 | 35517 | 143236 | 14 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 29s | 2.0% | | | | | | | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## psb-rv-plan

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $34.09 | 100.0% | 44m22s | 99.3% | claude-opus-5-5: 222, claude-sonnet-5-5: 6 | 12 | 351707 | 1730303 | 92673379 | 482 | 13.2M |
| └ writer | $17.59 | 51.6% | 19m8s | 42.8% | claude-opus-5-5: 109 | 1 | 101436 | 1187822 | 48114691 | 222 | 6.8M |
| └ writer forks | $10.44 | 30.6% | 15m21s | 34.4% | claude-opus-5-5: 58 | 6 | 138050 | 176065 | 33994392 | 128 | 4.3M |
| └ reviewer | $4.30 | 12.6% | 12m34s | 28.1% | claude-opus-5-5: 39 | 1 | 77554 | 267788 | 7039034 | 80 | 1.4M |
| └ reviewer forks | $1.59 | 4.7% | 2m8s | 4.8% | claude-opus-5-5: 16 | 3 | 32719 | 51979 | 3365475 | 38 | 0.6M |
| └ seed | $0.17 | 0.5% | 23s | 0.9% | claude-sonnet-5-5: 6 | 1 | 1948 | 46649 | 159787 | 14 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 18s | 0.7% | | | | | | | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## psb-rv-split

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Other** | $12.52 | 100.0% | 6m29s | 97.9% | claude-opus-5-5: 156 | 8 | 140311 | 1281866 | 16504507 | 352 | 4.0M |
| └ parts-plan-config-split | $2.20 | 17.6% | 3m16s | 49.3% | claude-opus-5-5: 22 | 1 | 20263 | 235040 | 3094981 | 48 | 0.7M |
| └ parts-plan-loom-wiring | $2.20 | 17.5% | 2m57s | 44.7% | claude-opus-5-5: 18 | 1 | 18017 | 255724 | 2781081 | 38 | 0.7M |
| └ parts-plan-record-write | $1.85 | 14.8% | 2m54s | 43.8% | claude-opus-5-5: 16 | 1 | 17517 | 217612 | 2051534 | 36 | 0.6M |
| └ parts-plan-preflight | $1.68 | 13.5% | 3m17s | 49.7% | claude-opus-5-5: 22 | 1 | 20084 | 160182 | 2408503 | 48 | 0.5M |
| └ parts-plan-recipe-contract | $1.57 | 12.5% | 3m24s | 51.3% | claude-opus-5-5: 17 | 1 | 21168 | 158618 | 1756016 | 36 | 0.5M |
| └ parts-plan-landing-describe | $1.37 | 10.9% | 2m45s | 41.6% | claude-opus-5-5: 24 | 1 | 16376 | 117230 | 2273161 | 58 | 0.5M |
| └ parts | $0.86 | 6.9% | 2m39s | 40.1% | claude-opus-5-5: 16 | 1 | 17667 | 67615 | 863299 | 34 | 0.3M |
| └ parts-plan | $0.79 | 6.3% | 1m38s | 24.8% | claude-opus-5-5: 21 | 1 | 9219 | 69845 | 1275932 | 54 | 0.3M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 8s | 2.1% | | | | | | | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|

