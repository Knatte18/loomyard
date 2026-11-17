# Token usage and time by role

Generated 2026-11-17 07:47 for adopt-worker-record, gate-scope-by-coverage-map, decision-record-replace, testmap-upkeep-design, creel-notice-hygiene, plan-writer-context, adopt-redesign, test-only-functions-in-prod, weaver, codex-same-as-claude.
Machine sleep, read from the systemd journal, is left out of every time figure.
Not counted, no transcript reader: codex.

## Most expensive parts

| part | cost | share | per run |
|---|---|---|---|
| Darn writer | $16.26 | 16.2% | $1.63 |
| Webster forks | $12.12 | 12.1% | $1.21 |
| Other raddle | $11.57 | 11.5% | $1.16 |
| Darn writer forks | $10.75 | 10.7% | $1.08 |
| Code-Review fixer | $9.05 | 9.0% | $0.90 |
| Code-Review reviewer | $7.05 | 7.0% | $0.71 |
| Plan reviewer | $6.49 | 6.5% | $0.65 |
| Plan writer forks | $4.12 | 4.1% | $0.41 |
| Plan writer | $4.01 | 4.0% | $0.40 |
| Plan fixer | $3.23 | 3.2% | $0.32 |

## Cost per run

| run | recipe | Plan | Webster | Darn | Code-Review | Landing | Orch | Other | Total |
|---|---|---|---|---|---|---|---|---|---|
| adopt-worker-record | tapestry | $20.72 | $13.47 | $0.00 | $4.12 | $0.58 | $0.00 | $2.00 | $40.90 |
| gate-scope-by-coverage-map | raddle | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $1.83 | $1.83 |
| decision-record-replace | raddle | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $1.28 | $1.28 |
| testmap-upkeep-design | raddle | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $2.20 | $2.20 |
| creel-notice-hygiene | darn | $0.00 | $0.00 | $6.52 | $2.85 | $0.14 | $0.00 | $0.17 | $9.68 |
| plan-writer-context | darn | $0.00 | $0.00 | $2.62 | $0.00 | $0.00 | $0.00 | $0.00 | $2.62 |
| adopt-redesign | raddle | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $3.20 | $3.20 |
| test-only-functions-in-prod | darn | $0.00 | $0.00 | $17.87 | $13.89 | $0.18 | $0.00 | $1.10 | $33.04 |
| weaver | raddle | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $2.06 | $2.06 |
| codex-same-as-claude | raddle | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $0.00 | $3.53 | $3.53 |
| average |  | $2.07 | $1.35 | $2.70 | $2.09 | $0.09 | $0.00 | $1.74 | $10.03 |

## Recipes

| recipe | runs | cost per run | span per run | landed runs | changed lines per run | cost per changed line |
|---|---|---|---|---|---|---|
| darn | 3 | $15.11 | 1h1m38s | 3 | 2311 | $0.007 |
| raddle | 6 | $2.35 | 40m46s | 0 | n/a | n/a |
| tapestry | 1 | $40.90 | 2h29m30s | 1 | 6024 | $0.007 |

A run's landed commit is the newest commit titled after the change description archived at its tag and committed within its span; its changed lines are the lines it added and deleted.
No landed commit: gate-scope-by-coverage-map (no change description at its tag), decision-record-replace (no change description at its tag), testmap-upkeep-design (no change description at its tag), adopt-redesign (no change description at its tag), weaver (no change description at its tag), codex-same-as-claude (no change description at its tag).

## All runs

| segment | cost | share | time | time share | models (messages) |
|---|---|---|---|---|---|
| **Darn** | $27.02 | 26.9% | 45m22s | 7.8% | claude-sonnet-5-5: 291, claude-opus-5-5: 158 |
| └ writer | $16.26 | 16.2% | 38m58s | 6.7% | claude-opus-5-5: 158, claude-sonnet-5-5: 58 |
| └ writer forks | $10.75 | 10.7% | 13m20s | 2.3% | claude-sonnet-5-5: 233 |
| **Code-Review** | $20.87 | 20.8% | 57m7s | 9.9% | claude-sonnet-5-5: 305, claude-opus-5-5: 88 |
| └ fixer | $9.05 | 9.0% | 32m41s | 5.6% | claude-sonnet-5-5: 184 |
| └ reviewer | $7.05 | 7.0% | 21m26s | 3.7% | claude-opus-5-5: 72, claude-sonnet-5-5: 30 |
| └ reviewer forks | $2.54 | 2.5% | 3m34s | 0.6% | claude-sonnet-5-5: 20, claude-opus-5-5: 16 |
| └ fixer forks | $1.25 | 1.2% | 1m29s | 0.3% | claude-sonnet-5-5: 28 |
| └ judge | $0.62 | 0.6% | 1m53s | 0.3% | claude-sonnet-5-5: 24 |
| └ seed | $0.35 | 0.3% | 58s | 0.2% | claude-sonnet-5-5: 19 |
| **Plan** | $20.72 | 20.7% | 55m19s | 9.6% | claude-opus-5-5: 181, claude-sonnet-5-5: 65 |
| └ reviewer | $6.49 | 6.5% | 25m15s | 4.4% | claude-opus-5-5: 40, claude-sonnet-5-5: 28 |
| └ writer forks | $4.12 | 4.1% | 10m46s | 1.9% | claude-opus-5-5: 28, claude-sonnet-5-5: 18 |
| └ writer | $4.01 | 4.0% | 8m51s | 1.5% | claude-opus-5-5: 41 |
| └ fixer | $3.23 | 3.2% | 11m21s | 2.0% | claude-opus-5-5: 42 |
| └ fixer forks | $1.30 | 1.3% | 2m7s | 0.4% | claude-opus-5-5: 18 |
| └ reviewer forks | $1.19 | 1.2% | 1m31s | 0.3% | claude-opus-5-5: 12 |
| └ judge | $0.28 | 0.3% | 1m2s | 0.2% | claude-sonnet-5-5: 12 |
| └ seed | $0.11 | 0.1% | 21s | 0.1% | claude-sonnet-5-5: 7 |
| **Other** | $17.37 | 17.3% | 1h8m43s | 11.9% | claude-sonnet-5-5: 243, claude-opus-5-5: 165, claude-haiku-5-5: 4 |
| └ raddle | $11.57 | 11.5% | 38m28s | 6.6% | claude-opus-5-5: 165 |
| └ gate-fixer | $3.21 | 3.2% | 29m12s | 5.0% | claude-sonnet-5-5: 133 |
| └ raddle forks | $2.53 | 2.5% | 4m18s | 0.7% | claude-sonnet-5-5: 103 |
| └ friction | $0.06 | 0.1% | 19s | 0.1% | claude-sonnet-5-5: 7 |
| └ tackler | $0.00 | 0.0% | 12s | 0.0% | claude-haiku-5-5: 4 |
| **Webster** | $13.47 | 13.4% | 1h1m36s | 10.6% | claude-sonnet-5-5: 162 |
| └ forks | $12.12 | 12.1% | 1h0m0s | 10.4% | claude-sonnet-5-5: 137 |
| └ Master | $1.35 | 1.3% | 1m44s | 0.3% | claude-sonnet-5-5: 25 |
| **Landing** | $0.89 | 0.9% | 2m48s | 0.5% | claude-sonnet-5-5: 24, claude-opus-5-5: 14 |
| └ fixer | $0.48 | 0.5% | 1m11s | 0.2% | claude-opus-5-5: 14 |
| └ describe | $0.42 | 0.4% | 1m36s | 0.3% | claude-sonnet-5-5: 24 |
| **Orch**, $0.00 per run | $0.00 | 0.0% |  |  |  |
| └ forks | $0.00 | 0.0% |  |  |  |
| └ session | $0.00 | 0.0% |  |  |  |
| └ unattributed | $0.00 | 0.0% |  |  |  |
| idle | | | 4h58m18s | 51.5% | |

Total cost $100.34; 3663 repeated transcript lines skipped.

Time is turn time: a session works from a user or assistant line until the line that ends its turn.
An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.
Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.
A segment's time is the union of its parts' working time, and time shares can sum past 100% where sessions overlap, a forks row overlapping its parent.
The union of working time plus idle is the span.
Every time figure leaves out machine sleep, 33h5m31s over these runs.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Darn-Write | 3 | 1h59m42s | 56m54s | 1h2m48s | 22.4% |
| Webster | 1 | 1h7m11s | 4m59s | 1h2m12s | 22.2% |
| Raddle-Write | 6 | 4h1m11s | 3h22m12s | 38m59s | 13.9% |
| Code-Burler-Fix | 4 | 45m24s | 12m35s | 32m49s | 11.7% |
| Plan-Burler-Review | 2 | 25m37s | 22s | 25m15s | 9.0% |
| Code-Burler-Review | 4 | 21m51s | 25s | 21m26s | 7.6% |
| Plan-Write | 1 | 18m12s | 45s | 17m27s | 6.2% |
| Plan-Burler-Fix | 2 | 13m12s | 1m3s | 12m9s | 4.3% |
| Code-Bouncer | 8 | 3m43s | 52s | 2m51s | 1.0% |
| Describe | 4 | 1m50s | 14s | 1m36s | 0.6% |
| Plan-Bouncer | 3 | 1m44s | 21s | 1m23s | 0.5% |
| Parent-Align | 1 | 4m38s | 3m27s | 1m11s | 0.4% |
| Friction-Reflect | 4 | 35s | 16s | 19s | 0.1% |
| Raddle-File | 6 | 2m23s | 2m11s | 12s | 0.1% |
| PR-Review | 4 | 14s | 14s | 0s | 0.0% |
| Drift-Await | 4 | 11s | 11s | 0s | 0.0% |
| Drift-Baseline | 4 | 9s | 9s | 0s | 0.0% |
| Drift-Expect | 4 | 13s | 13s | 0s | 0.0% |
| Drift-Recheck | 4 | 12s | 12s | 0s | 0.0% |
| Drift-Verify | 4 | 9s | 9s | 0s | 0.0% |
| Finalize | 4 | 8m42s | 8m42s | 0s | 0.0% |
| Motif | 1 | 2s | 2s | 0s | 0.0% |
| Parts-Plan-Write | 1 | 2s | 2s | 0s | 0.0% |
| Parts-Write | 1 | 6s | 6s | 0s | 0.0% |
| Preflight | 4 | 34s | 34s | 0s | 0.0% |
| Record-Write | 9 | 1m10s | 1m10s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## Orchestrator

Orch sessions of the prime from 2026-11-09 20:48 to 2026-11-16 14:55, the span of the counted runs.

| part | sessions | messages | tokens | est. cost |
|---|---|---|---|---|
| session | 0 | 0 | 0.0M | $0.00 |
| forks | 0 | 0 | 0.0M | $0.00 |
| total | 0 | 0 | 0.0M | $0.00 |

| run | orch tokens | orch cost | run cost | orch share of run cost |
|---|---|---|---|---|
| adopt-worker-record | 0.0M | $0.00 | $40.90 | 0.0% |
| gate-scope-by-coverage-map | 0.0M | $0.00 | $1.83 | 0.0% |
| decision-record-replace | 0.0M | $0.00 | $1.28 | 0.0% |
| testmap-upkeep-design | 0.0M | $0.00 | $2.20 | 0.0% |
| creel-notice-hygiene | 0.0M | $0.00 | $9.68 | 0.0% |
| plan-writer-context | 0.0M | $0.00 | $2.62 | 0.0% |
| adopt-redesign | 0.0M | $0.00 | $3.20 | 0.0% |
| test-only-functions-in-prod | 0.0M | $0.00 | $33.04 | 0.0% |
| weaver | 0.0M | $0.00 | $2.06 | 0.0% |
| codex-same-as-claude | 0.0M | $0.00 | $3.53 | 0.0% |
| unattributed | 0.0M | $0.00 | | |

Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, and one outside every window is unattributed.

## adopt-worker-record

| segment | cost | share | time | time share | models (messages) |
|---|---|---|---|---|---|
| **Plan** | $20.72 | 50.7% | 55m19s | 37.0% | claude-opus-5-5: 181, claude-sonnet-5-5: 65 |
| └ reviewer | $6.49 | 15.9% | 25m15s | 16.9% | claude-opus-5-5: 40, claude-sonnet-5-5: 28 |
| └ writer forks | $4.12 | 10.1% | 10m46s | 7.2% | claude-opus-5-5: 28, claude-sonnet-5-5: 18 |
| └ writer | $4.01 | 9.8% | 8m51s | 5.9% | claude-opus-5-5: 41 |
| └ fixer | $3.23 | 7.9% | 11m21s | 7.6% | claude-opus-5-5: 42 |
| └ fixer forks | $1.30 | 3.2% | 2m7s | 1.4% | claude-opus-5-5: 18 |
| └ reviewer forks | $1.19 | 2.9% | 1m31s | 1.0% | claude-opus-5-5: 12 |
| └ judge | $0.28 | 0.7% | 1m2s | 0.7% | claude-sonnet-5-5: 12 |
| └ seed | $0.11 | 0.3% | 21s | 0.2% | claude-sonnet-5-5: 7 |
| **Webster** | $13.47 | 32.9% | 1h1m36s | 41.2% | claude-sonnet-5-5: 162 |
| └ forks | $12.12 | 29.6% | 1h0m0s | 40.1% | claude-sonnet-5-5: 137 |
| └ Master | $1.35 | 3.3% | 1m44s | 1.2% | claude-sonnet-5-5: 25 |
| **Code-Review** | $4.12 | 10.1% | 12m25s | 8.3% | claude-sonnet-5-5: 99 |
| └ fixer | $2.13 | 5.2% | 7m25s | 5.0% | claude-sonnet-5-5: 53 |
| └ reviewer | $0.85 | 2.1% | 3m56s | 2.6% | claude-sonnet-5-5: 15 |
| └ reviewer forks | $0.58 | 1.4% | 1m19s | 0.9% | claude-sonnet-5-5: 13 |
| └ fixer forks | $0.30 | 0.7% | 30s | 0.3% | claude-sonnet-5-5: 6 |
| └ seed | $0.13 | 0.3% | 22s | 0.2% | claude-sonnet-5-5: 6 |
| └ judge | $0.13 | 0.3% | 34s | 0.4% | claude-sonnet-5-5: 6 |
| **Other** | $2.00 | 4.9% | 12m5s | 8.1% | claude-sonnet-5-5: 100 |
| └ gate-fixer | $1.94 | 4.7% | 11m45s | 7.9% | claude-sonnet-5-5: 93 |
| └ friction | $0.06 | 0.2% | 19s | 0.2% | claude-sonnet-5-5: 7 |
| **Landing** | $0.58 | 1.4% | 1m43s | 1.1% | claude-opus-5-5: 14, claude-sonnet-5-5: 6 |
| └ fixer | $0.48 | 1.2% | 1m11s | 0.8% | claude-opus-5-5: 14 |
| └ describe | $0.10 | 0.3% | 31s | 0.3% | claude-sonnet-5-5: 6 |
| **Orch** | $0.00 | 0.0% |  |  |  |
| └ forks | $0.00 | 0.0% |  |  |  |
| └ session | $0.00 | 0.0% |  |  |  |
| idle | | | 16m37s | 11.1% | |

Machine sleep left out of the span: 33h5m31s.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Webster | 1 | 1h7m11s | 4m59s | 1h2m12s | 46.8% |
| Plan-Burler-Review | 2 | 25m37s | 22s | 25m15s | 19.0% |
| Plan-Write | 1 | 18m12s | 45s | 17m27s | 13.1% |
| Plan-Burler-Fix | 2 | 13m12s | 1m3s | 12m9s | 9.1% |
| Code-Burler-Fix | 1 | 9m50s | 2m16s | 7m34s | 5.7% |
| Code-Burler-Review | 1 | 4m1s | 5s | 3m56s | 3.0% |
| Plan-Bouncer | 3 | 1m44s | 21s | 1m23s | 1.0% |
| Parent-Align | 1 | 4m38s | 3m27s | 1m11s | 0.9% |
| Code-Bouncer | 2 | 1m8s | 12s | 56s | 0.7% |
| Describe | 1 | 35s | 4s | 31s | 0.4% |
| Friction-Reflect | 1 | 23s | 4s | 19s | 0.2% |
| Drift-Await | 1 | 2s | 2s | 0s | 0.0% |
| Drift-Baseline | 1 | 3s | 3s | 0s | 0.0% |
| Drift-Expect | 1 | 7s | 7s | 0s | 0.0% |
| Drift-Recheck | 1 | 2s | 2s | 0s | 0.0% |
| Drift-Verify | 1 | 2s | 2s | 0s | 0.0% |
| Finalize | 1 | 2m20s | 2m20s | 0s | 0.0% |
| Motif | 1 | 2s | 2s | 0s | 0.0% |
| PR-Review | 1 | 3s | 3s | 0s | 0.0% |
| Parts-Plan-Write | 1 | 2s | 2s | 0s | 0.0% |
| Parts-Write | 1 | 6s | 6s | 0s | 0.0% |
| Preflight | 1 | 10s | 10s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## gate-scope-by-coverage-map

| segment | cost | share | time | time share | models (messages) |
|---|---|---|---|---|---|
| **Other** | $1.83 | 100.0% | 5m35s | 25.2% | claude-opus-5-5: 23, claude-sonnet-5-5: 16, claude-haiku-5-5: 4 |
| └ raddle | $1.43 | 78.1% | 5m23s | 24.3% | claude-opus-5-5: 23 |
| └ raddle forks | $0.40 | 21.7% | 48s | 3.6% | claude-sonnet-5-5: 16 |
| └ tackler | $0.00 | 0.2% | 12s | 0.9% | claude-haiku-5-5: 4 |
| **Orch** | $0.00 | 0.0% |  |  |  |
| └ forks | $0.00 | 0.0% |  |  |  |
| └ session | $0.00 | 0.0% |  |  |  |
| idle | | | 16m35s | 74.8% | |

Session transcripts left out: 13; they started before the slug's previous run ended, at 2026-11-11T09:27:45+01:00.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Raddle-Write | 1 | 20m1s | 14m38s | 5m23s | 96.5% |
| Raddle-File | 1 | 2m0s | 1m48s | 12s | 3.5% |
| Record-Write | 1 | 9s | 9s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## decision-record-replace

| segment | cost | share | time | time share | models (messages) |
|---|---|---|---|---|---|
| **Other** | $1.28 | 100.0% | 3m25s | 82.4% | claude-opus-5-5: 22, claude-sonnet-5-5: 14 |
| └ raddle | $1.00 | 78.2% | 3m9s | 75.8% | claude-opus-5-5: 22 |
| └ raddle forks | $0.28 | 21.8% | 31s | 12.3% | claude-sonnet-5-5: 14 |
| **Orch** | $0.00 | 0.0% |  |  |  |
| └ forks | $0.00 | 0.0% |  |  |  |
| └ session | $0.00 | 0.0% |  |  |  |
| idle | | | 44s | 17.6% | |

Session transcripts left out: 11; they started before the slug's previous run ended, at 2026-11-11T16:01:37+01:00.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Raddle-Write | 1 | 3m56s | 31s | 3m25s | 100.0% |
| Raddle-File | 1 | 2s | 2s | 0s | 0.0% |
| Record-Write | 1 | 11s | 11s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## testmap-upkeep-design

| segment | cost | share | time | time share | models (messages) |
|---|---|---|---|---|---|
| **Other** | $2.20 | 100.0% | 7m0s | 33.1% | claude-opus-5-5: 27, claude-sonnet-5-5: 13 |
| └ raddle | $1.88 | 85.2% | 7m0s | 33.1% | claude-opus-5-5: 27 |
| └ raddle forks | $0.33 | 14.8% | 36s | 2.8% | claude-sonnet-5-5: 13 |
| **Orch** | $0.00 | 0.0% |  |  |  |
| └ forks | $0.00 | 0.0% |  |  |  |
| └ session | $0.00 | 0.0% |  |  |  |
| idle | | | 14m9s | 66.9% | |

Session transcripts left out: 9; they started before the slug's previous run ended, at 2026-11-11T16:04:07+01:00.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Raddle-Write | 1 | 20m59s | 13m59s | 7m0s | 100.0% |
| Raddle-File | 1 | 1s | 1s | 0s | 0.0% |
| Record-Write | 1 | 9s | 9s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## creel-notice-hygiene

| segment | cost | share | time | time share | models (messages) |
|---|---|---|---|---|---|
| **Darn** | $6.52 | 67.4% | 12m59s | 36.1% | claude-opus-5-5: 59, claude-sonnet-5-5: 40 |
| └ writer | $5.10 | 52.7% | 11m56s | 33.2% | claude-opus-5-5: 59 |
| └ writer forks | $1.42 | 14.7% | 1m55s | 5.3% | claude-sonnet-5-5: 40 |
| **Code-Review** | $2.85 | 29.5% | 8m20s | 23.2% | claude-sonnet-5-5: 77 |
| └ fixer | $1.48 | 15.3% | 5m21s | 14.9% | claude-sonnet-5-5: 37 |
| └ reviewer | $0.58 | 5.9% | 2m26s | 6.8% | claude-sonnet-5-5: 15 |
| └ reviewer forks | $0.35 | 3.6% | 35s | 1.6% | claude-sonnet-5-5: 7 |
| └ fixer forks | $0.24 | 2.5% | 17s | 0.8% | claude-sonnet-5-5: 6 |
| └ judge | $0.12 | 1.3% | 19s | 0.9% | claude-sonnet-5-5: 6 |
| └ seed | $0.08 | 0.9% | 14s | 0.6% | claude-sonnet-5-5: 6 |
| **Other** | $0.17 | 1.7% | 4m27s | 12.4% | claude-sonnet-5-5: 8 |
| └ gate-fixer | $0.17 | 1.7% | 4m27s | 12.4% | claude-sonnet-5-5: 8 |
| **Landing** | $0.14 | 1.4% | 26s | 1.2% | claude-sonnet-5-5: 8 |
| └ describe | $0.14 | 1.4% | 26s | 1.2% | claude-sonnet-5-5: 8 |
| **Orch** | $0.00 | 0.0% |  |  |  |
| └ forks | $0.00 | 0.0% |  |  |  |
| └ session | $0.00 | 0.0% |  |  |  |
| idle | | | 9m44s | 27.1% | |

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Darn-Write | 1 | 20m24s | 2m58s | 17m26s | 66.5% |
| Code-Burler-Fix | 1 | 8m14s | 2m53s | 5m21s | 20.4% |
| Code-Burler-Review | 1 | 2m31s | 5s | 2m26s | 9.3% |
| Code-Bouncer | 2 | 44s | 11s | 33s | 2.1% |
| Describe | 1 | 31s | 5s | 26s | 1.7% |
| Drift-Await | 1 | 3s | 3s | 0s | 0.0% |
| Drift-Baseline | 1 | 2s | 2s | 0s | 0.0% |
| Drift-Expect | 1 | 2s | 2s | 0s | 0.0% |
| Drift-Recheck | 1 | 5s | 5s | 0s | 0.0% |
| Drift-Verify | 1 | 3s | 3s | 0s | 0.0% |
| Finalize | 1 | 2m58s | 2m58s | 0s | 0.0% |
| Friction-Reflect | 1 | 5s | 5s | 0s | 0.0% |
| PR-Review | 1 | 5s | 5s | 0s | 0.0% |
| Preflight | 1 | 6s | 6s | 0s | 0.0% |
| Record-Write | 1 | 3s | 3s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## plan-writer-context

| segment | cost | share | time | time share | models (messages) |
|---|---|---|---|---|---|
| **Darn** | $2.62 | 100.0% | 9m18s | 15.3% | claude-sonnet-5-5: 58 |
| └ writer | $2.62 | 100.0% | 9m18s | 15.3% | claude-sonnet-5-5: 58 |
| **Orch** | $0.00 | 0.0% |  |  |  |
| └ forks | $0.00 | 0.0% |  |  |  |
| └ session | $0.00 | 0.0% |  |  |  |
| idle | | | 51m28s | 84.7% | |

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Darn-Write | 1 | 1h0m0s | 50m42s | 9m18s | 100.0% |
| Code-Bouncer | 1 | 4s | 4s | 0s | 0.0% |
| Describe | 1 | 2s | 2s | 0s | 0.0% |
| Drift-Await | 1 | 2s | 2s | 0s | 0.0% |
| Drift-Baseline | 1 | 3s | 3s | 0s | 0.0% |
| Drift-Expect | 1 | 1s | 1s | 0s | 0.0% |
| Drift-Recheck | 1 | 2s | 2s | 0s | 0.0% |
| Drift-Verify | 1 | 2s | 2s | 0s | 0.0% |
| Finalize | 1 | 8s | 8s | 0s | 0.0% |
| Friction-Reflect | 1 | 4s | 4s | 0s | 0.0% |
| PR-Review | 1 | 2s | 2s | 0s | 0.0% |
| Preflight | 1 | 11s | 11s | 0s | 0.0% |
| Record-Write | 1 | 5s | 5s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## adopt-redesign

| segment | cost | share | time | time share | models (messages) |
|---|---|---|---|---|---|
| **Other** | $3.20 | 100.0% | 9m10s | 19.8% | claude-opus-5-5: 26, claude-sonnet-5-5: 18 |
| └ raddle | $2.66 | 83.2% | 9m10s | 19.8% | claude-opus-5-5: 26 |
| └ raddle forks | $0.54 | 16.8% | 55s | 2.0% | claude-sonnet-5-5: 18 |
| **Orch** | $0.00 | 0.0% |  |  |  |
| └ forks | $0.00 | 0.0% |  |  |  |
| └ session | $0.00 | 0.0% |  |  |  |
| idle | | | 37m10s | 80.2% | |

Session transcripts left out: 14; they started before the slug's previous run ended, at 2026-11-15T15:54:31+01:00.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Raddle-Write | 1 | 46m12s | 37m2s | 9m10s | 100.0% |
| Raddle-File | 1 | 2s | 2s | 0s | 0.0% |
| Record-Write | 1 | 6s | 6s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## test-only-functions-in-prod

| segment | cost | share | time | time share | models (messages) |
|---|---|---|---|---|---|
| **Darn** | $17.87 | 54.1% | 23m5s | 26.2% | claude-sonnet-5-5: 193, claude-opus-5-5: 99 |
| └ writer forks | $9.33 | 28.2% | 11m25s | 13.0% | claude-sonnet-5-5: 193 |
| └ writer | $8.54 | 25.8% | 17m44s | 20.1% | claude-opus-5-5: 99 |
| **Code-Review** | $13.89 | 42.0% | 36m22s | 41.2% | claude-sonnet-5-5: 129, claude-opus-5-5: 88 |
| └ reviewer | $5.63 | 17.0% | 15m4s | 17.1% | claude-opus-5-5: 72 |
| └ fixer | $5.44 | 16.5% | 19m54s | 22.6% | claude-sonnet-5-5: 94 |
| └ reviewer forks | $1.61 | 4.9% | 1m41s | 1.9% | claude-opus-5-5: 16 |
| └ fixer forks | $0.70 | 2.1% | 42s | 0.8% | claude-sonnet-5-5: 16 |
| └ judge | $0.37 | 1.1% | 1m0s | 1.1% | claude-sonnet-5-5: 12 |
| └ seed | $0.14 | 0.4% | 23s | 0.4% | claude-sonnet-5-5: 7 |
| **Other** | $1.10 | 3.3% | 13m0s | 14.7% | claude-sonnet-5-5: 32 |
| └ gate-fixer | $1.10 | 3.3% | 13m0s | 14.7% | claude-sonnet-5-5: 32 |
| **Landing** | $0.18 | 0.5% | 39s | 0.7% | claude-sonnet-5-5: 10 |
| └ describe | $0.18 | 0.5% | 39s | 0.7% | claude-sonnet-5-5: 10 |
| **Orch** | $0.00 | 0.0% |  |  |  |
| └ forks | $0.00 | 0.0% |  |  |  |
| └ session | $0.00 | 0.0% |  |  |  |
| idle | | | 15m7s | 17.1% | |

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Darn-Write | 1 | 39m18s | 3m13s | 36m5s | 49.4% |
| Code-Burler-Fix | 2 | 27m20s | 7m26s | 19m54s | 27.2% |
| Code-Burler-Review | 2 | 15m19s | 15s | 15m4s | 20.6% |
| Code-Bouncer | 3 | 1m47s | 24s | 1m23s | 1.9% |
| Describe | 1 | 42s | 4s | 38s | 0.9% |
| PR-Review | 1 | 4s | 4s | 0s | 0.0% |
| Drift-Await | 1 | 4s | 4s | 0s | 0.0% |
| Drift-Baseline | 1 | 1s | 1s | 0s | 0.0% |
| Drift-Expect | 1 | 3s | 3s | 0s | 0.0% |
| Drift-Recheck | 1 | 3s | 3s | 0s | 0.0% |
| Drift-Verify | 1 | 2s | 2s | 0s | 0.0% |
| Finalize | 1 | 3m16s | 3m16s | 0s | 0.0% |
| Friction-Reflect | 1 | 3s | 3s | 0s | 0.0% |
| Preflight | 1 | 7s | 7s | 0s | 0.0% |
| Record-Write | 1 | 3s | 3s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## weaver

| segment | cost | share | time | time share | models (messages) |
|---|---|---|---|---|---|
| **Other** | $2.06 | 100.0% | 5m27s | 15.0% | claude-opus-5-5: 27, claude-sonnet-5-5: 21 |
| └ raddle | $1.56 | 75.8% | 5m12s | 14.3% | claude-opus-5-5: 27 |
| └ raddle forks | $0.50 | 24.2% | 43s | 2.0% | claude-sonnet-5-5: 21 |
| **Orch** | $0.00 | 0.0% |  |  |  |
| └ forks | $0.00 | 0.0% |  |  |  |
| └ session | $0.00 | 0.0% |  |  |  |
| idle | | | 30m56s | 85.0% | |

Session transcripts left out: 13; they started before the slug's previous run ended, at 2026-11-16T10:34:54+01:00.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Raddle-Write | 1 | 36m6s | 30m39s | 5m27s | 100.0% |
| Raddle-File | 1 | 3s | 3s | 0s | 0.0% |
| Record-Write | 1 | 14s | 14s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## codex-same-as-claude

| segment | cost | share | time | time share | models (messages) |
|---|---|---|---|---|---|
| **Other** | $3.53 | 100.0% | 8m35s | 7.5% | claude-opus-5-5: 40, claude-sonnet-5-5: 21 |
| └ raddle | $3.04 | 86.0% | 8m35s | 7.5% | claude-opus-5-5: 40 |
| └ raddle forks | $0.49 | 14.0% | 46s | 0.7% | claude-sonnet-5-5: 21 |
| **Orch** | $0.00 | 0.0% |  |  |  |
| └ forks | $0.00 | 0.0% |  |  |  |
| └ session | $0.00 | 0.0% |  |  |  |
| idle | | | 1h45m47s | 92.5% | |

Session transcripts left out: 14; they started before the slug's previous run ended, at 2026-11-16T10:43:20+01:00.

### Producers

| producer | entries | elapsed | idle | active | share |
|---|---|---|---|---|---|
| Raddle-Write | 1 | 1h53m57s | 1h45m22s | 8m35s | 100.0% |
| Raddle-File | 1 | 15s | 15s | 0s | 0.0% |
| Record-Write | 1 | 10s | 10s | 0s | 0.0% |

A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.
Elapsed and idle leave out machine sleep; active is elapsed minus idle, and the share is a row's active time over the table's summed active time.

## Webster forks

| run | cards | start context | messages | peak context | weight |
|---|---|---|---|---|---|
| adopt-worker-record | 01-adopt-pause-caller, 02-beam-up-skip-orch, 03-adopt-record | 73320 | 24 | 184910 | 0.6M |
| adopt-worker-record | 04-green-end-reports, 05-run-report-paused-by-adopt | 76031 | 25 | 223883 | 0.8M |
| adopt-worker-record | 06-adopt-step-order | 77899 | 35 | 354901 | 2.0M |
| adopt-worker-record | 07-adopt-worker, 08-adopt-caller-follow, 09-beam-status-adopt, 10-adopt-docs-and-pattern | 80777 | 53 | 410238 | 2.6M |

