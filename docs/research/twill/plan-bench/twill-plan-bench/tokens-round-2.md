# Token usage and time by role

Generated 2026-10-11 20:05 for twb-p2r-tw-1, twb-p2r-tw-2, twb-p2r-oh-1, twb-p2r-oh-2, twb-p2s-tw-1, twb-p2s-tw-2, twb-p2s-oh-2.

## All runs

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 7 | 380220 | 1333397 | 48171622 | 808 | 8.4M | 73.5% | 1h1m32s | 91.5% | $23.91 | claude-opus-5-5: 389 |
| plan+sub | 16 | 9383 | 321155 | 15590606 | 218 | 2.0M | 17.6% | 22m49s | 34.0% | $4.91 | claude-opus-5-5: 93 |
| idle | | | | | | | | 13s | 0.3% | | |
| orch | | | | | | 1.0M | 9.0% | | | $2.56 | |

Total weight 11.4M; 509 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
Time shares can sum past 100% where sessions overlap, a `+sub` row overlapping its parent.
The union of working time plus idle is the span.

## Orchestrator

Orch sessions of the prime from 2026-10-11 19:44 to 2026-10-11 20:05, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| orch | 1 | 18 | 1.3M | $0.81 |
| orch+sub | 2 | 37 | 7.2M | $2.87 |
| total | 3 | 55 | 8.4M | $3.68 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| twb-p2r-tw-1 | 0.6M | $0.25 | $4.89 | 4.9% |
| twb-p2r-tw-2 | 0.6M | $0.25 | $4.96 | 4.8% |
| twb-p2r-oh-1 | 0.5M | $0.14 | $2.41 | 5.5% |
| twb-p2r-oh-2 | 2.4M | $0.72 | $2.95 | 19.5% |
| twb-p2s-tw-1 | 0.6M | $0.25 | $5.73 | 4.2% |
| twb-p2s-tw-2 | 0.6M | $0.24 | $5.39 | 4.2% |
| twb-p2s-oh-2 | 2.4M | $0.72 | $2.48 | 22.4% |
| unattributed | 0.6M | $1.13 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## twb-p2r-tw-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 64444 | 199837 | 6731656 | 114 | 1.2M | 67.7% | 10m12s | 89.9% | $3.63 | claude-opus-5-5: 53 |
| plan+sub | 4 | 2018 | 95025 | 3700148 | 56 | 0.5M | 27.1% | 5m38s | 49.7% | $1.26 | claude-opus-5-5: 24 |
| idle | | | | | | | | 1s | 0.1% | | |
| orch | | | | | | 0.1M | 5.1% | | | $0.25 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-p2r-tw-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 70418 | 202680 | 7273030 | 126 | 1.3M | 71.9% | 11m10s | 90.6% | $3.88 | claude-opus-5-5: 59 |
| plan+sub | 4 | 2107 | 83248 | 3130342 | 54 | 0.4M | 23.1% | 6m40s | 54.1% | $1.08 | claude-opus-5-5: 23 |
| idle | | | | | | | | 0s | 0.1% | | |
| orch | | | | | | 0.1M | 5.1% | | | $0.25 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-p2r-oh-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 41524 | 159653 | 3927614 | 76 | 0.8M | 93.1% | 6m48s | 99.7% | $2.41 | claude-opus-5-5: 38 |
| idle | | | | | | | | 1s | 0.3% | | |
| orch | | | | | | 0.1M | 6.9% | | | $0.14 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-p2r-oh-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 49488 | 170675 | 5547845 | 98 | 1.0M | 77.4% | 8m8s | 99.7% | $2.95 | claude-opus-5-5: 49 |
| idle | | | | | | | | 2s | 0.3% | | |
| orch | | | | | | 0.3M | 22.6% | | | $0.72 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-p2s-tw-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 60359 | 240051 | 9849254 | 134 | 1.6M | 70.5% | 9m41s | 84.8% | $4.38 | claude-opus-5-5: 64 |
| plan+sub | 4 | 1293 | 78934 | 4653295 | 54 | 0.6M | 25.3% | 5m28s | 47.9% | $1.35 | claude-opus-5-5: 23 |
| idle | | | | | | | | 2s | 0.3% | | |
| orch | | | | | | 0.1M | 4.2% | | | $0.25 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-p2s-tw-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 58568 | 207573 | 9798942 | 158 | 1.5M | 71.9% | 9m38s | 85.9% | $4.17 | claude-opus-5-5: 75 |
| plan+sub | 4 | 3965 | 63948 | 4106821 | 54 | 0.5M | 23.9% | 5m3s | 45.0% | $1.22 | claude-opus-5-5: 23 |
| idle | | | | | | | | 5s | 0.7% | | |
| orch | | | | | | 0.1M | 4.2% | | | $0.24 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-p2s-oh-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 35419 | 152928 | 5043281 | 102 | 0.9M | 74.7% | 5m56s | 99.6% | $2.48 | claude-opus-5-5: 51 |
| idle | | | | | | | | 2s | 0.4% | | |
| orch | | | | | | 0.3M | 25.3% | | | $0.72 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|

