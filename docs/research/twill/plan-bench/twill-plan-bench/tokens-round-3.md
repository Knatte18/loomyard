# Token usage and time by role

Generated 2026-10-11 20:47 for twb-p3s-th-1, twb-p3s-th-2, twb-p3r-th-1, twb-p3r-th-2.

## All runs

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 4 | 295744 | 926423 | 38243086 | 560 | 6.5M | 58.9% | 49m55s | 90.3% | $18.20 | claude-opus-5-5: 264 |
| plan+sub | 16 | 8516 | 326815 | 17192271 | 222 | 2.2M | 19.8% | 22m14s | 40.2% | $5.24 | claude-opus-5-5: 95 |
| idle | | | | | | | | 5s | 0.1% | | |
| orch | | | | | | 2.3M | 21.4% | | | $6.79 | |

Total weight 11.0M; 374 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
Time shares can sum past 100% where sessions overlap, a `+sub` row overlapping its parent.
The union of working time plus idle is the span.

## Orchestrator

Orch sessions of the prime from 2026-10-11 20:16 to 2026-10-11 20:47, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| orch | 1 | 62 | 5.2M | $1.99 |
| orch+sub | 5 | 95 | 9.5M | $5.60 |
| total | 6 | 157 | 14.8M | $7.59 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| twb-p3s-th-1 | 1.6M | $0.87 | $5.36 | 14.0% |
| twb-p3s-th-2 | 3.2M | $1.86 | $6.76 | 21.6% |
| twb-p3r-th-1 | 5.8M | $2.24 | $5.32 | 29.7% |
| twb-p3r-th-2 | 3.1M | $1.82 | $6.01 | 23.2% |
| unattributed | 1.1M | $0.80 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## twb-p3s-th-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 61151 | 217120 | 9070398 | 142 | 1.5M | 64.8% | 9m53s | 89.4% | $4.12 | claude-opus-5-5: 67 |
| plan+sub | 4 | 1526 | 74782 | 4168725 | 56 | 0.5M | 22.6% | 4m14s | 38.4% | $1.24 | claude-opus-5-5: 24 |
| idle | | | | | | | | 1s | 0.2% | | |
| orch | | | | | | 0.3M | 12.6% | | | $0.87 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-p3s-th-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 76388 | 252025 | 12926885 | 174 | 2.0M | 62.5% | 12m23s | 89.9% | $5.37 | claude-opus-5-5: 83 |
| plan+sub | 4 | 1497 | 82858 | 4685847 | 52 | 0.6M | 18.2% | 5m9s | 37.3% | $1.38 | claude-opus-5-5: 22 |
| idle | | | | | | | | 2s | 0.2% | | |
| orch | | | | | | 0.6M | 19.3% | | | $1.86 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-p3r-th-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 78591 | 225957 | 7002667 | 110 | 1.4M | 50.7% | 15m6s | 91.8% | $4.10 | claude-opus-5-5: 51 |
| plan+sub | 4 | 3166 | 78030 | 3812228 | 52 | 0.5M | 18.2% | 6m32s | 39.7% | $1.22 | claude-opus-5-5: 22 |
| idle | | | | | | | | 1s | 0.1% | | |
| orch | | | | | | 0.8M | 31.0% | | | $2.24 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-p3r-th-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 79614 | 231321 | 9243136 | 134 | 1.6M | 57.8% | 12m33s | 89.8% | $4.60 | claude-opus-5-5: 63 |
| plan+sub | 4 | 2327 | 91145 | 4525471 | 62 | 0.6M | 20.7% | 6m19s | 45.2% | $1.41 | claude-opus-5-5: 27 |
| idle | | | | | | | | 1s | 0.1% | | |
| orch | | | | | | 0.6M | 21.5% | | | $1.82 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|

