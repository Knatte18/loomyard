# Token usage and time by role

Generated 2026-10-11 20:56 for twb-son-s-1, twb-son-s-2, twb-son-r-1, twb-son-r-2.

## All runs

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 4 | 331419 | 1039305 | 41782369 | 508 | 7.1M | 57.7% | 41m47s | 87.0% | $14.27 | claude-sonnet-5-5: 238 |
| plan+sub | 16 | 10851 | 384088 | 18740819 | 226 | 2.4M | 19.5% | 24m28s | 50.9% | $4.82 | claude-sonnet-5-5: 97 |
| idle | | | | | | | | 3s | 0.1% | | |
| orch | | | | | | 2.8M | 22.8% | | | $7.83 | |

Total weight 12.4M; 409 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
Time shares can sum past 100% where sessions overlap, a `+sub` row overlapping its parent.
The union of working time plus idle is the span.

## Orchestrator

Orch sessions of the prime from 2026-10-11 20:28 to 2026-10-11 20:56, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| orch | 1 | 72 | 6.5M | $2.31 |
| orch+sub | 6 | 87 | 11.8M | $6.18 |
| total | 7 | 159 | 18.3M | $8.49 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| twb-son-s-1 | 4.0M | $1.51 | $4.07 | 27.1% |
| twb-son-s-2 | 3.3M | $1.54 | $4.86 | 24.0% |
| twb-son-r-1 | 5.1M | $1.81 | $5.22 | 25.8% |
| twb-son-r-2 | 5.5M | $2.97 | $4.94 | 37.6% |
| unattributed | 0.4M | $0.66 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## twb-son-s-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 67291 | 238057 | 8526038 | 122 | 1.5M | 57.1% | 8m48s | 84.5% | $2.97 | claude-sonnet-5-5: 57 |
| plan+sub | 4 | 2473 | 85620 | 4267545 | 54 | 0.5M | 21.0% | 4m38s | 44.4% | $1.09 | claude-sonnet-5-5: 23 |
| idle | | | | | | | | 1s | 0.2% | | |
| orch | | | | | | 0.6M | 22.0% | | | $1.51 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-son-s-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 65010 | 269511 | 11210039 | 128 | 1.8M | 60.0% | 8m35s | 86.0% | $3.57 | claude-sonnet-5-5: 60 |
| plan+sub | 4 | 2858 | 98944 | 5098108 | 52 | 0.6M | 21.8% | 4m56s | 49.6% | $1.30 | claude-sonnet-5-5: 22 |
| idle | | | | | | | | 1s | 0.1% | | |
| orch | | | | | | 0.5M | 18.2% | | | $1.54 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-son-r-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 107342 | 279690 | 10699038 | 122 | 2.0M | 59.1% | 13m11s | 88.9% | $3.91 | claude-sonnet-5-5: 57 |
| plan+sub | 4 | 3366 | 103412 | 5060134 | 60 | 0.7M | 19.7% | 8m3s | 54.3% | $1.30 | claude-sonnet-5-5: 26 |
| idle | | | | | | | | 1s | 0.1% | | |
| orch | | | | | | 0.7M | 21.2% | | | $1.81 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-son-r-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 91776 | 252047 | 11347254 | 136 | 1.9M | 55.0% | 11m13s | 87.6% | $3.82 | claude-sonnet-5-5: 64 |
| plan+sub | 4 | 2154 | 96112 | 4315032 | 60 | 0.6M | 16.2% | 6m51s | 53.4% | $1.12 | claude-sonnet-5-5: 26 |
| idle | | | | | | | | 1s | 0.1% | | |
| orch | | | | | | 1.0M | 28.8% | | | $2.97 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|

