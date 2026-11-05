# Token usage and time by role

Generated 2026-10-16 13:50 for twb-btf-is, twb-btf-it, twb-btf-rs, twb-btf-rt, twb-btf-ks, twb-btf-kt.
Machine sleep, read from the systemd journal, is left out of every time figure.

## All runs

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan-burler-review | 4 | 91282 | 483648 | 13416174 | 300 | 2.4M | 23.2% | 16m5s | 13.8% | $6.93 | claude-opus-5-5: 150 |
| plan-burler-fix | 4 | 47275 | 256914 | 5589465 | 228 | 1.1M | 10.8% | 8m55s | 7.6% | $3.35 | claude-opus-5-5: 110 |
| discussion-burler-fix | 2 | 36807 | 138541 | 2926552 | 104 | 0.6M | 6.3% | 5m58s | 5.1% | $2.01 | claude-opus-5-5: 51 |
| discussion-burler-review | 2 | 23783 | 153541 | 2333309 | 88 | 0.5M | 5.2% | 5m1s | 4.3% | $1.71 | claude-opus-5-5: 44 |
| plan-bouncer-judge | 4 | 9054 | 148749 | 623729 | 48 | 0.3M | 2.8% | 1m44s | 1.5% | $0.59 | claude-sonnet-5-5: 24 |
| plan-burler-fix+sub | 6 | 10882 | 17352 | 754190 | 36 | 0.2M | 1.5% | 49s | 0.7% | $0.46 | claude-opus-5-5: 12 |
| discussion-bouncer-judge | 2 | 6043 | 70587 | 305135 | 24 | 0.1M | 1.4% | 1m1s | 0.9% | $0.30 | claude-sonnet-5-5: 12 |
| discussion-burler-fix+sub | 3 | 7099 | 10001 | 600626 | 20 | 0.1M | 1.0% | 30s | 0.4% | $0.31 | claude-opus-5-5: 7 |
| idle | | | | | | | | 1h17m10s | 66.1% | | |
| orch | | | | | | 5.0M | 47.8% | | | $13.63 | |

Total weight 10.4M; 439 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
Time shares can sum past 100% where sessions overlap, a `+sub` row overlapping its parent.
The union of working time plus idle is the span.

## Orchestrator

Orch sessions of the prime from 2026-10-16 13:02 to 2026-10-16 13:50, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| orch | 1 | 77 | 8.5M | $3.22 |
| orch+sub | 6 | 166 | 25.8M | $10.67 |
| total | 7 | 243 | 34.3M | $13.89 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| twb-btf-is | 7.2M | $2.10 | $2.52 | 45.5% |
| twb-btf-it | 7.3M | $2.14 | $3.20 | 40.1% |
| twb-btf-rs | 1.6M | $1.02 | $2.98 | 25.4% |
| twb-btf-rt | 1.2M | $0.65 | $2.62 | 19.8% |
| twb-btf-ks | 8.1M | $3.91 | $1.60 | 71.0% |
| twb-btf-kt | 7.9M | $3.82 | $2.74 | 58.2% |
| unattributed | 0.9M | $0.25 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## twb-btf-is

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan-burler-review | 1 | 22241 | 105232 | 2419009 | 62 | 0.5M | 27.8% | 3m58s | 46.8% | $1.46 | claude-opus-5-5: 31 |
| plan-burler-fix | 1 | 13250 | 64768 | 1679751 | 68 | 0.3M | 18.1% | 3m22s | 39.9% | $0.93 | claude-opus-5-5: 34 |
| plan-bouncer-judge | 1 | 1965 | 35638 | 157274 | 12 | 0.1M | 4.0% | 24s | 4.7% | $0.14 | claude-sonnet-5-5: 6 |
| idle | | | | | | | | 44s | 8.7% | | |
| orch | | | | | | 0.9M | 50.2% | | | $2.10 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-btf-it

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan-burler-review | 1 | 25610 | 114802 | 2626616 | 66 | 0.5M | 26.9% | 4m27s | 49.7% | $1.61 | claude-opus-5-5: 33 |
| plan-burler-fix | 1 | 16651 | 78832 | 2221273 | 82 | 0.4M | 20.3% | 2m52s | 32.0% | $1.17 | claude-opus-5-5: 38 |
| plan-burler-fix+sub | 3 | 7060 | 10142 | 368308 | 18 | 0.1M | 4.3% | 32s | 5.9% | $0.27 | claude-opus-5-5: 6 |
| plan-bouncer-judge | 1 | 2060 | 38145 | 154864 | 12 | 0.1M | 3.7% | 24s | 4.4% | $0.15 | claude-sonnet-5-5: 6 |
| idle | | | | | | | | 53s | 9.9% | | |
| orch | | | | | | 0.9M | 44.9% | | | $2.14 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-btf-rs

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan-burler-review | 1 | 26611 | 144837 | 5303225 | 102 | 0.8M | 60.5% | 4m38s | 36.9% | $2.32 | claude-opus-5-5: 51 |
| plan-burler-fix | 1 | 7135 | 50379 | 583314 | 30 | 0.2M | 11.3% | 1m4s | 8.5% | $0.51 | claude-opus-5-5: 15 |
| plan-bouncer-judge | 1 | 2713 | 38885 | 154573 | 12 | 0.1M | 5.6% | 24s | 3.2% | $0.16 | claude-sonnet-5-5: 6 |
| idle | | | | | | | | 6m25s | 51.3% | | |
| orch | | | | | | 0.3M | 22.7% | | | $1.02 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-btf-rt

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan-burler-review | 1 | 16820 | 118777 | 3067324 | 70 | 0.5M | 47.8% | 3m3s | 27.9% | $1.54 | claude-opus-5-5: 35 |
| plan-burler-fix | 1 | 10239 | 62935 | 1105127 | 48 | 0.2M | 21.3% | 1m36s | 14.6% | $0.74 | claude-opus-5-5: 23 |
| plan-bouncer-judge | 1 | 2316 | 36081 | 157018 | 12 | 0.1M | 6.4% | 32s | 4.8% | $0.14 | claude-sonnet-5-5: 6 |
| plan-burler-fix+sub | 3 | 3822 | 7210 | 385882 | 18 | 0.1M | 5.9% | 18s | 2.7% | $0.19 | claude-opus-5-5: 6 |
| idle | | | | | | | | 5m38s | 51.5% | | |
| orch | | | | | | 0.2M | 18.6% | | | $0.65 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-btf-ks

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| discussion-burler-review | 1 | 13323 | 75606 | 1211561 | 46 | 0.3M | 15.1% | 2m37s | 6.9% | $0.89 | claude-opus-5-5: 23 |
| discussion-burler-fix | 1 | 11158 | 49914 | 437786 | 24 | 0.2M | 8.7% | 1m52s | 4.9% | $0.56 | claude-opus-5-5: 12 |
| discussion-bouncer-judge | 1 | 3051 | 35368 | 152570 | 12 | 0.1M | 4.0% | 30s | 1.3% | $0.15 | claude-sonnet-5-5: 6 |
| idle | | | | | | | | 33m0s | 86.8% | | |
| orch | | | | | | 1.3M | 72.2% | | | $3.91 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-btf-kt

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| discussion-burler-fix | 1 | 25649 | 88627 | 2488766 | 80 | 0.5M | 21.7% | 4m6s | 10.8% | $1.45 | claude-opus-5-5: 39 |
| discussion-burler-review | 1 | 10460 | 77935 | 1121748 | 42 | 0.3M | 11.6% | 2m24s | 6.3% | $0.82 | claude-opus-5-5: 21 |
| discussion-burler-fix+sub | 3 | 7099 | 10001 | 600626 | 20 | 0.1M | 4.8% | 30s | 1.3% | $0.31 | claude-opus-5-5: 7 |
| discussion-bouncer-judge | 1 | 2992 | 35219 | 152565 | 12 | 0.1M | 3.3% | 31s | 1.4% | $0.15 | claude-sonnet-5-5: 6 |
| idle | | | | | | | | 30m30s | 80.7% | | |
| orch | | | | | | 1.3M | 58.5% | | | $3.82 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|

