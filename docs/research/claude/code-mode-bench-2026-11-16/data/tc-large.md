# Token usage and time by role

Generated 2026-11-16 13:01 for cm-la, cm-lb, cm-lc.
Machine sleep, read from the systemd journal, is left out of every time figure.
Not counted, no transcript reader: codex.

## Most expensive parts

| part | cost | share | per run |
|---|---|---|---|
| Plan reviewer | $17.90 | 43.0% | $5.97 |
| Plan writer | $16.04 | 38.5% | $5.35 |
| Plan reviewer forks | $4.34 | 10.4% | $1.45 |
| Plan writer forks | $2.47 | 5.9% | $0.82 |
| Other gate-fixer | $0.60 | 1.4% | $0.20 |
| Plan seed | $0.32 | 0.8% | $0.11 |
| Orch forks | $0.00 | 0.0% | $0.00 |
| Orch session | $0.00 | 0.0% | $0.00 |
| Orch unattributed | $0.00 | 0.0% | $0.00 |

## Cost per run

| run | recipe | Plan | Orch | Other | Total |
|---|---|---|---|---|---|
| cm-la | unknown | $13.72 | $0.00 | $0.19 | $13.91 |
| cm-lb | unknown | $12.72 | $0.00 | $0.21 | $12.93 |
| cm-lc | unknown | $14.62 | $0.00 | $0.20 | $14.82 |
| average |  | $13.69 | $0.00 | $0.20 | $13.89 |

## Recipes

| recipe | runs | cost per run | span per run | landed runs | changed lines per run | cost per changed line |
|---|---|---|---|---|---|---|
| unknown | 3 | $13.89 | 36m51s | 0 | n/a | n/a |

A run's landed commit is the newest commit titled after the change description archived at its tag and committed within its span; its changed lines are the lines it added and deleted.
No landed commit: cm-la (no shed record read), cm-lb (no shed record read), cm-lc (no shed record read).

## All runs

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $41.06 | 98.6% | 1h45m24s | 95.3% | claude-opus-5-5: 347, <synthetic>: 291, claude-sonnet-5-5: 93 | 30 | 749304 | 2909518 | 69190801 | 916 | 14.3M |
| └ reviewer | $17.90 | 43.0% | 54m33s | 49.3% | claude-opus-5-5: 160 | 3 | 278331 | 1213338 | 31316831 | 326 | 6.0M |
| └ writer | $16.04 | 38.5% | 49m53s | 45.1% | <synthetic>: 173, claude-opus-5-5: 145 | 3 | 309281 | 946053 | 25592126 | 296 | 5.3M |
| └ reviewer forks | $4.34 | 10.4% | 5m0s | 4.5% | claude-opus-5-5: 42 | 9 | 92790 | 101413 | 9904944 | 102 | 1.6M |
| └ writer forks | $2.47 | 5.9% | 3m19s | 3.0% | <synthetic>: 112, claude-sonnet-5-5: 74 | 12 | 63920 | 572808 | 1997896 | 148 | 1.2M |
| └ seed | $0.32 | 0.8% | 58s | 0.9% | claude-sonnet-5-5: 19, <synthetic>: 6 | 3 | 4982 | 75906 | 379004 | 44 | 0.2M |
| **Other** | $0.60 | 1.4% | 2m23s | 2.2% | claude-sonnet-5-5: 28, <synthetic>: 20 | 3 | 9535 | 132996 | 851935 | 62 | 0.3M |
| └ gate-fixer | $0.60 | 1.4% | 2m23s | 2.2% | claude-sonnet-5-5: 28, <synthetic>: 20 | 3 | 9535 | 132996 | 851935 | 62 | 0.3M |
| **Orch**, $0.00 per run | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ unattributed | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 2m48s | 2.5% | | | | | | | |

Total cost $41.66, total weight 14.6M; 745 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
A segment's time is the union of its parts' working time, and time shares can sum past 100% where sessions overlap, a forks row overlapping its parent.
The union of working time plus idle is the span.

## Orchestrator

Orch sessions of the prime from 2026-11-16 12:11 to 2026-11-16 13:01, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| session | 0 | 0 | 0.0M | $0.00 |
| forks | 0 | 0 | 0.0M | $0.00 |
| total | 0 | 0 | 0.0M | $0.00 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| cm-la | 0.0M | $0.00 | $13.91 | 0.0% |
| cm-lb | 0.0M | $0.00 | $12.93 | 0.0% |
| cm-lc | 0.0M | $0.00 | $14.82 | 0.0% |
| unattributed | 0.0M | $0.00 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## cm-la

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $13.72 | 98.7% | 32m46s | 95.9% | claude-opus-5-5: 124, claude-sonnet-5-5: 29 | 10 | 256939 | 905922 | 24383136 | 318 | 4.9M |
| └ reviewer | $5.57 | 40.1% | 16m42s | 48.9% | claude-opus-5-5: 57 | 1 | 98807 | 293761 | 10644742 | 116 | 1.9M |
| └ writer | $5.47 | 39.3% | 15m46s | 46.1% | claude-opus-5-5: 49 | 1 | 99833 | 337239 | 8947898 | 100 | 1.8M |
| └ reviewer forks | $1.70 | 12.2% | 1m59s | 5.8% | claude-opus-5-5: 18 | 3 | 33978 | 43679 | 4017230 | 42 | 0.6M |
| └ writer forks | $0.86 | 6.2% | 54s | 2.6% | claude-sonnet-5-5: 23 | 4 | 22486 | 201809 | 652836 | 46 | 0.4M |
| └ seed | $0.12 | 0.8% | 18s | 0.9% | claude-sonnet-5-5: 6 | 1 | 1835 | 29434 | 120430 | 14 | 0.1M |
| **Other** | $0.19 | 1.3% | 51s | 2.5% | claude-sonnet-5-5: 10 | 1 | 3316 | 37079 | 306350 | 22 | 0.1M |
| └ gate-fixer | $0.19 | 1.3% | 51s | 2.5% | claude-sonnet-5-5: 10 | 1 | 3316 | 37079 | 306350 | 22 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 34s | 1.7% | | | | | | | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## cm-lb

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $12.72 | 98.4% | 30m57s | 94.9% | <synthetic>: 291, claude-opus-5-5: 105, claude-sonnet-5-5: 38 | 11 | 238065 | 926247 | 21653859 | 298 | 4.5M |
| └ reviewer | $5.94 | 46.0% | 14m33s | 44.6% | claude-opus-5-5: 51 | 1 | 90641 | 355750 | 11763155 | 104 | 2.1M |
| └ writer | $4.47 | 34.6% | 16m4s | 49.2% | <synthetic>: 173, claude-opus-5-5: 46 | 1 | 92410 | 256969 | 6675934 | 94 | 1.5M |
| └ reviewer forks | $1.11 | 8.6% | 1m26s | 4.4% | claude-opus-5-5: 8 | 3 | 28263 | 20389 | 2213089 | 22 | 0.4M |
| └ writer forks | $1.10 | 8.5% | 1m9s | 3.5% | <synthetic>: 112, claude-sonnet-5-5: 31 | 5 | 25231 | 269200 | 852606 | 62 | 0.5M |
| └ seed | $0.10 | 0.8% | 21s | 1.1% | claude-sonnet-5-5: 7, <synthetic>: 6 | 1 | 1520 | 23939 | 149075 | 16 | 0.1M |
| **Other** | $0.21 | 1.6% | 42s | 2.2% | <synthetic>: 20, claude-sonnet-5-5: 9 | 1 | 2569 | 50861 | 269832 | 20 | 0.1M |
| └ gate-fixer | $0.21 | 1.6% | 42s | 2.2% | <synthetic>: 20, claude-sonnet-5-5: 9 | 1 | 2569 | 50861 | 269832 | 20 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 57s | 2.9% | | | | | | | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## cm-lc

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $14.62 | 98.6% | 41m40s | 95.2% | claude-opus-5-5: 118, claude-sonnet-5-5: 26 | 9 | 254300 | 1077349 | 23153806 | 300 | 4.9M |
| └ reviewer | $6.38 | 43.0% | 23m19s | 53.3% | claude-opus-5-5: 52 | 1 | 88883 | 563827 | 8908934 | 106 | 2.0M |
| └ writer | $6.09 | 41.1% | 18m3s | 41.2% | claude-opus-5-5: 50 | 1 | 117038 | 351845 | 9968294 | 102 | 2.0M |
| └ reviewer forks | $1.53 | 10.3% | 1m35s | 3.6% | claude-opus-5-5: 16 | 3 | 30549 | 37345 | 3674625 | 38 | 0.6M |
| └ writer forks | $0.52 | 3.5% | 1m16s | 2.9% | claude-sonnet-5-5: 20 | 3 | 16203 | 101799 | 492454 | 40 | 0.3M |
| └ seed | $0.09 | 0.6% | 18s | 0.7% | claude-sonnet-5-5: 6 | 1 | 1627 | 22533 | 109499 | 14 | 0.0M |
| **Other** | $0.20 | 1.4% | 49s | 1.9% | claude-sonnet-5-5: 9 | 1 | 3650 | 45056 | 275753 | 20 | 0.1M |
| └ gate-fixer | $0.20 | 1.4% | 49s | 1.9% | claude-sonnet-5-5: 9 | 1 | 3650 | 45056 | 275753 | 20 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 1m17s | 2.9% | | | | | | | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|

