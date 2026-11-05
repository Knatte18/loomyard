# Token usage and time by role

Generated 2026-10-12 08:47 for twb-sm-s-1, twb-sm-s-2, twb-sm-r-1, twb-sm-r-2, twb-fab-s-1, twb-fab-s-2, twb-fab-r-1, twb-fab-r-2.

## All runs

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 8 | 410765 | 1642766 | 33233726 | 2540 | 7.4M | 72.2% | 1h10m8s | 95.1% | $31.56 | claude-sonnet-5-5: 188, claude-fable-5-1: 78 |
| plan+sub | 16 | 12920 | 300974 | 10945070 | 196 | 1.5M | 14.9% | 20m51s | 28.3% | $3.07 | claude-sonnet-5-5: 82 |
| idle | | | | | | | | 7s | 0.2% | | |
| orch | | | | | | 1.3M | 12.9% | | | $3.11 | |

Total weight 10.3M; 559 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
Time shares can sum past 100% where sessions overlap, a `+sub` row overlapping its parent.
The union of working time plus idle is the span.

## Orchestrator

Orch sessions of the prime from 2026-10-12 08:23 to 2026-10-12 08:46, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| orch | 1 | 29 | 3.1M | $1.06 |
| orch+sub | 3 | 59 | 7.1M | $2.41 |
| total | 4 | 88 | 10.2M | $3.48 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| twb-sm-s-1 | 0.7M | $0.24 | $3.01 | 7.2% |
| twb-sm-s-2 | 1.1M | $0.33 | $2.66 | 11.2% |
| twb-sm-r-1 | 0.7M | $0.24 | $2.64 | 8.2% |
| twb-sm-r-2 | 1.1M | $0.33 | $3.67 | 8.3% |
| twb-fab-s-1 | 0.8M | $0.24 | $5.46 | 4.2% |
| twb-fab-s-2 | 1.2M | $0.37 | $5.27 | 6.6% |
| twb-fab-r-1 | 1.9M | $1.02 | $5.94 | 14.7% |
| twb-fab-r-2 | 1.1M | $0.33 | $5.99 | 5.3% |
| unattributed | 1.6M | $0.37 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## twb-sm-s-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 51002 | 190533 | 5792283 | 102 | 1.1M | 66.6% | 6m58s | 91.7% | $2.15 | claude-sonnet-5-5: 48 |
| plan+sub | 4 | 2475 | 89369 | 3104650 | 58 | 0.4M | 27.0% | 5m29s | 72.3% | $0.87 | claude-sonnet-5-5: 25 |
| idle | | | | | | | | 1s | 0.2% | | |
| orch | | | | | | 0.1M | 6.5% | | | $0.24 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-sm-s-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 47367 | 183101 | 4966301 | 92 | 1.0M | 65.7% | 6m14s | 85.5% | $1.92 | claude-sonnet-5-5: 42 |
| plan+sub | 4 | 2482 | 63590 | 2743757 | 46 | 0.4M | 25.0% | 3m48s | 52.1% | $0.73 | claude-sonnet-5-5: 19 |
| idle | | | | | | | | 1s | 0.2% | | |
| orch | | | | | | 0.1M | 9.3% | | | $0.33 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-sm-r-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 51132 | 173588 | 5470849 | 100 | 1.0M | 71.6% | 6m34s | 87.7% | $2.04 | claude-sonnet-5-5: 46 |
| plan+sub | 4 | 5086 | 64566 | 1941316 | 40 | 0.3M | 21.1% | 4m36s | 61.6% | $0.60 | claude-sonnet-5-5: 16 |
| idle | | | | | | | | 0s | 0.1% | | |
| orch | | | | | | 0.1M | 7.3% | | | $0.24 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-sm-r-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 77539 | 210042 | 7485676 | 110 | 1.4M | 71.1% | 9m52s | 91.6% | $2.80 | claude-sonnet-5-5: 52 |
| plan+sub | 4 | 2877 | 83449 | 3155347 | 52 | 0.4M | 22.1% | 6m57s | 64.4% | $0.87 | claude-sonnet-5-5: 22 |
| idle | | | | | | | | 1s | 0.1% | | |
| orch | | | | | | 0.1M | 6.9% | | | $0.33 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-fab-s-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 42307 | 222106 | 2240801 | 550 | 0.7M | 85.3% | 9m14s | 99.8% | $5.46 | claude-fable-5-1: 20 |
| idle | | | | | | | | 1s | 0.2% | | |
| orch | | | | | | 0.1M | 14.7% | | | $0.24 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-fab-s-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 42524 | 212142 | 1960654 | 486 | 0.7M | 81.7% | 10m23s | 99.8% | $5.27 | claude-fable-5-1: 18 |
| idle | | | | | | | | 1s | 0.2% | | |
| orch | | | | | | 0.2M | 18.3% | | | $0.37 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-fab-r-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 47991 | 227988 | 2728833 | 582 | 0.8M | 64.5% | 10m14s | 99.8% | $5.94 | claude-fable-5-1: 21 |
| idle | | | | | | | | 1s | 0.2% | | |
| orch | | | | | | 0.4M | 35.5% | | | $1.02 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-fab-r-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 50903 | 223266 | 2588329 | 518 | 0.8M | 85.4% | 10m39s | 99.8% | $5.99 | claude-fable-5-1: 19 |
| idle | | | | | | | | 1s | 0.2% | | |
| orch | | | | | | 0.1M | 14.6% | | | $0.33 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|

