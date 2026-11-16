# Token usage and time by role

Generated 2026-11-16 12:31 for cm-sa, cm-sb, cm-sc.
Machine sleep, read from the systemd journal, is left out of every time figure.
Not counted, no transcript reader: codex.

## Most expensive parts

| part | cost | share | per run |
|---|---|---|---|
| Plan reviewer | $7.51 | 48.2% | $2.50 |
| Plan writer | $5.07 | 32.5% | $1.69 |
| Plan reviewer forks | $2.34 | 15.0% | $0.78 |
| Plan seed | $0.47 | 3.0% | $0.16 |
| Other gate-fixer | $0.19 | 1.2% | $0.06 |
| Orch forks | $0.00 | 0.0% | $0.00 |
| Orch session | $0.00 | 0.0% | $0.00 |
| Orch unattributed | $0.00 | 0.0% | $0.00 |

## Cost per run

| run | recipe | Plan | Orch | Other | Total |
|---|---|---|---|---|---|
| cm-sa | unknown | $5.10 | $0.00 | $0.19 | $5.29 |
| cm-sb | unknown | $5.02 | $0.00 | $0.00 | $5.02 |
| cm-sc | unknown | $5.28 | $0.00 | $0.00 | $5.28 |
| average |  | $5.13 | $0.00 | $0.06 | $5.20 |

## Recipes

| recipe | runs | cost per run | span per run | landed runs | changed lines per run | cost per changed line |
|---|---|---|---|---|---|---|
| unknown | 3 | $5.20 | 13m34s | 0 | n/a | n/a |

A run's landed commit is the newest commit titled after the change description archived at its tag and committed within its span; its changed lines are the lines it added and deleted.
No landed commit: cm-sa (no shed record read), cm-sb (no shed record read), cm-sc (no shed record read).

## All runs

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $15.40 | 98.8% | 38m47s | 95.3% | claude-opus-5-5: 221, <synthetic>: 79, claude-sonnet-5-5: 23 | 18 | 288494 | 1087635 | 22822495 | 532 | 5.1M |
| └ reviewer | $7.51 | 48.2% | 23m21s | 57.4% | claude-opus-5-5: 105 | 3 | 141000 | 497367 | 11023467 | 218 | 2.4M |
| └ writer | $5.07 | 32.5% | 13m42s | 33.7% | claude-opus-5-5: 81, <synthetic>: 69 | 3 | 82066 | 408984 | 6933332 | 174 | 1.6M |
| └ reviewer forks | $2.34 | 15.0% | 3m48s | 9.3% | claude-opus-5-5: 35 | 9 | 56544 | 67522 | 4361303 | 88 | 0.8M |
| └ seed | $0.47 | 3.0% | 1m44s | 4.2% | claude-sonnet-5-5: 23, <synthetic>: 10 | 3 | 8884 | 113762 | 504393 | 52 | 0.2M |
| **Other** | $0.19 | 1.2% | 32s | 1.3% | claude-sonnet-5-5: 12 | 1 | 2712 | 35645 | 372410 | 26 | 0.1M |
| └ gate-fixer | $0.19 | 1.2% | 32s | 1.3% | claude-sonnet-5-5: 12 | 1 | 2712 | 35645 | 372410 | 26 | 0.1M |
| **Orch**, $0.00 per run | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ unattributed | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 1m23s | 3.4% | | | | | | | |

Total cost $15.59, total weight 5.2M; 335 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
A segment's time is the union of its parts' working time, and time shares can sum past 100% where sessions overlap, a forks row overlapping its parent.
The union of working time plus idle is the span.

## Orchestrator

Orch sessions of the prime from 2026-11-16 12:10 to 2026-11-16 12:30, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| session | 0 | 0 | 0.0M | $0.00 |
| forks | 0 | 0 | 0.0M | $0.00 |
| total | 0 | 0 | 0.0M | $0.00 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| cm-sa | 0.0M | $0.00 | $5.29 | 0.0% |
| cm-sb | 0.0M | $0.00 | $5.02 | 0.0% |
| cm-sc | 0.0M | $0.00 | $5.28 | 0.0% |
| unattributed | 0.0M | $0.00 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## cm-sa

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $5.10 | 96.4% | 12m17s | 91.5% | claude-opus-5-5: 71, claude-sonnet-5-5: 7 | 6 | 95429 | 358259 | 7496541 | 170 | 1.7M |
| └ reviewer | $2.60 | 49.2% | 7m43s | 57.5% | claude-opus-5-5: 33 | 1 | 48858 | 176459 | 3699285 | 68 | 0.8M |
| └ writer | $1.49 | 28.2% | 4m0s | 29.8% | claude-opus-5-5: 24 | 1 | 24685 | 124947 | 1860189 | 52 | 0.5M |
| └ reviewer forks | $0.88 | 16.6% | 1m8s | 8.4% | claude-opus-5-5: 14 | 3 | 19187 | 26572 | 1795079 | 34 | 0.3M |
| └ seed | $0.13 | 2.5% | 35s | 4.3% | claude-sonnet-5-5: 7 | 1 | 2699 | 30281 | 141988 | 16 | 0.1M |
| **Other** | $0.19 | 3.6% | 32s | 3.9% | claude-sonnet-5-5: 12 | 1 | 2712 | 35645 | 372410 | 26 | 0.1M |
| └ gate-fixer | $0.19 | 3.6% | 32s | 3.9% | claude-sonnet-5-5: 12 | 1 | 2712 | 35645 | 372410 | 26 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 37s | 4.5% | | | | | | | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## cm-sb

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $5.02 | 100.0% | 13m17s | 97.2% | <synthetic>: 79, claude-opus-5-5: 72, claude-sonnet-5-5: 8 | 6 | 93833 | 369447 | 7237290 | 174 | 1.7M |
| └ reviewer | $2.59 | 51.6% | 8m9s | 59.7% | claude-opus-5-5: 41 | 1 | 47855 | 162608 | 4098874 | 84 | 0.9M |
| └ writer | $1.56 | 31.0% | 4m28s | 32.7% | <synthetic>: 69, claude-opus-5-5: 21 | 1 | 27007 | 138559 | 1629071 | 46 | 0.5M |
| └ reviewer forks | $0.69 | 13.7% | 1m2s | 7.5% | claude-opus-5-5: 10 | 3 | 15702 | 22211 | 1324987 | 26 | 0.2M |
| └ seed | $0.18 | 3.7% | 40s | 4.9% | <synthetic>: 10, claude-sonnet-5-5: 8 | 1 | 3269 | 46069 | 184358 | 18 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 23s | 2.8% | | | | | | | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## cm-sc

| segment | cost | share | time | time share | models (messages) | sessions | output | cache write | cache read | input | weight |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Plan** | $5.28 | 100.0% | 13m13s | 97.1% | claude-opus-5-5: 78, claude-sonnet-5-5: 8 | 6 | 99232 | 359929 | 8088664 | 188 | 1.8M |
| └ reviewer | $2.32 | 44.0% | 7m29s | 55.0% | claude-opus-5-5: 31 | 1 | 44287 | 158300 | 3225308 | 66 | 0.7M |
| └ writer | $2.02 | 38.3% | 5m14s | 38.5% | claude-opus-5-5: 36 | 1 | 30374 | 145478 | 3444072 | 76 | 0.7M |
| └ reviewer forks | $0.78 | 14.7% | 1m39s | 12.1% | claude-opus-5-5: 11 | 3 | 21655 | 18739 | 1241237 | 28 | 0.3M |
| └ seed | $0.16 | 3.0% | 29s | 3.6% | claude-sonnet-5-5: 8 | 1 | 2916 | 37412 | 178047 | 18 | 0.1M |
| **Orch** | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ forks | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| └ session | $0.00 | 0.0% |  |  |  | | | | | | 0.0M |
| idle | | | 24s | 2.9% | | | | | | | |

No shed status was read, so the span is the transcript window and no Producers table is printed.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|

