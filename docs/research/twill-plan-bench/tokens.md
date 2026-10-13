# Token usage and time by role

Generated 2026-10-11 19:08 for twb-pl-oh-1, twb-pl-om-1, twb-pl-tw-1, twb-pl-tw-2.

## All runs

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 4 | 172638 | 706069 | 28365987 | 520 | 4.6M | 69.4% | 29m11s | 95.5% | $12.66 | claude-opus-5-5: 251 |
| plan+sub | 10 | 4064 | 55809 | 7545306 | 106 | 0.8M | 12.8% | 2m27s | 8.0% | $1.87 | claude-opus-5-5: 43 |
| idle | | | | | | | | 6s | 0.4% | | |
| orch | | | | | | 1.2M | 17.8% | | | $2.76 | |

Total weight 6.6M; 305 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
Time shares can sum past 100% where sessions overlap, a `+sub` row overlapping its parent.
The union of working time plus idle is the span.

## Orchestrator

Orch sessions of the prime from 2026-10-11 18:51 to 2026-10-11 19:07, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| orch | 1 | 9 | 0.9M | $0.69 |
| orch+sub | 1 | 45 | 9.0M | $2.07 |
| total | 2 | 54 | 9.9M | $2.76 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| twb-pl-oh-1 | 1.0M | $0.38 | $2.67 | 12.4% |
| twb-pl-om-1 | 0.7M | $0.31 | $2.77 | 9.9% |
| twb-pl-tw-1 | 4.5M | $1.18 | $4.44 | 21.0% |
| twb-pl-tw-2 | 3.7M | $0.89 | $4.65 | 16.1% |
| unattributed | 0.0M | $0.00 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## twb-pl-oh-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 37631 | 160413 | 5549924 | 110 | 0.9M | 86.9% | 6m24s | 99.3% | $2.67 | claude-opus-5-5: 55 |
| idle | | | | | | | | 3s | 0.7% | | |
| orch | | | | | | 0.1M | 13.1% | | | $0.38 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-pl-om-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 35552 | 157670 | 6335109 | 122 | 1.0M | 90.2% | 6m12s | 99.5% | $2.77 | claude-opus-5-5: 61 |
| idle | | | | | | | | 2s | 0.5% | | |
| orch | | | | | | 0.1M | 9.8% | | | $0.31 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-pl-tw-1

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 49427 | 188875 | 7695873 | 142 | 1.3M | 56.8% | 8m16s | 91.6% | $3.47 | claude-opus-5-5: 66 |
| plan+sub | 5 | 2964 | 27361 | 3877931 | 54 | 0.4M | 19.8% | 1m17s | 14.2% | $0.97 | claude-opus-5-5: 22 |
| idle | | | | | | | | 1s | 0.1% | | |
| orch | | | | | | 0.5M | 23.4% | | | $1.18 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## twb-pl-tw-2

| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| plan | 1 | 50028 | 199111 | 8785081 | 146 | 1.4M | 62.8% | 8m19s | 94.1% | $3.75 | claude-opus-5-5: 69 |
| plan+sub | 5 | 1100 | 28448 | 3667375 | 52 | 0.4M | 18.6% | 1m9s | 13.1% | $0.90 | claude-opus-5-5: 21 |
| idle | | | | | | | | 1s | 0.2% | | |
| orch | | | | | | 0.4M | 18.6% | | | $0.89 | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|

