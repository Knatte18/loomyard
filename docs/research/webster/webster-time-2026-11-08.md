# Where a Webster run spends its time, 2026-11-08

Webster stays serial, because parallel batches buy little (`docs/research/webster/parallel-batches-2026-10-17/report.md`).
This report measures where a serial run's time goes today, so the levers rest on records and not on guesses.
It reads records and code only: no agent, test or gate ran for it.
Every figure is a measurement of past runs at one point in time, so it goes stale when new runs land.

## Sample and method

- **Window.** Webster runs that started on or after 2026-10-25 and ended by 2026-11-08.
  `lym report timing --since 2026-10-25 --format json`, and the same with `--by week`, list 330 runs in it:
  314 darn runs and 16 tapestry runs.
  Only the tapestry runs hold a Webster step.
  Machine sleep is left out of every interval of the timing report, as its sleep note says.
- **Webster step.** The span from a run's `loom: Webster -> running` commit to the next phase change in the records repo history.
  A reroute to plan review in the middle (two runs) ends the span and a later `Webster -> running` opens another;
  the plan-review time between them is not Webster's and is not counted.
- **Timing phase versus Webster step.** `lym report timing` files the Code-Bouncer, Code-Burler, Drift-Verify and Describe rows
  under its `webster` phase too (`internal/reportengine/doc.go`).
  Its `webster` phase therefore runs longer than the Webster step;
  the 14 runs analysed below hold 83,926 s of timing's `webster` phase against 72,312 s of Webster step.
- **Records read.**
  - Run state: `_lym/webster/state.json` in each run's archive tag (the batches, their cards, kind, recoveries and recorded partition),
    plus `outcome.yaml` and `reports/verify-gate.yaml`.
  - Timestamps: the commit times of `webster: begin-batch`, `record-batch`, `recover-batch` and `fix-plan` commits in that history.
  - Fork and Master transcripts: `~/.claude/projects/<worktree>/<session>/subagents/` for the forks, with tool-call start and result times.
  - Gate time: the hub's gate run records (`.lym/gate/runs/*.json`): per step the `gate_key`, `wait_start`, `start`, `end` and `wall_seconds`.
    The wait for a gate slot is `start - wait_start`; the run wall is `wall_seconds`.
- **Analysed runs.** 14 of the 16.
  `interactive-recipe` (33,427 s of machine sleep) and `cold-gate-fixer` (23,997 s) slept inside the Webster step,
  and the commit times cannot separate the sleep from the work.
  Their timing-report `webster` phase is 2,865 s and 988 s.
  They are left out of every split and kept in the batcher comparison, which needs only their plans.
- **Fork time.** A fork's span is its transcript's first to last timestamp.
  Time waiting on a gate is the gate run records' wall plus slot wait for the run in the Webster step.
  The rest of a fork's span is called agent work: model turns and the non-gate tools.

## Per run

Minutes.
Columns: wall of the Webster step; batches; cards; start-up (Webster running to the first `begin-batch`);
agent work in the forks; card gate wall, of which the guard step is most (see the summary); batch gate wall;
tests the fork started itself; waits for a gate slot (card gate, batch gate and own tests together);
overhead between a batch's `begin-batch` and its recorded end other than the fork and recovery;
recovery sessions; the plan-level verify gate; the verify-fix fork (which includes its own gate run);
Master wrap-up and unattributed time; batches that ran a fix round (a failed card or batch gate step).

| Run | Wall | Batches | Cards | Start-up | Agent | Card gate | Batch gate | Own tests | Slot wait | Overhead | Recovery | Plan verify | Verify fix | Wrap-up | Fix batches |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `run-verbs-shared` | 97.8 | 11 | 20 | 0.4 | 71.2 | 11.1 | 4.3 | 0.1 | 0.0 | 1.5 | 0.0 | 5.5 | 2.6 | 1.1 | 6 |
| `warp-visibility` | 66.4 | 11 | 20 | 0.5 | 38.9 | 8.7 | 10.4 | 2.0 | 0.0 | 1.6 | 0.0 | 3.2 | 0.0 | 1.1 | 1 |
| `plan-split-producers` | 64.3 | 5 | 20 | 0.4 | 38.9 | 10.4 | 2.1 | 0.2 | 0.0 | 0.7 | 0.0 | 7.4 | 3.6 | 0.6 | 3 |
| `machine-switch` | 72.9 | 6 | 14 | 0.5 | 44.9 | 7.7 | 4.5 | 2.0 | 0.0 | 0.8 | 0.0 | 9.0 | 2.6 | 1.0 | 2 |
| `reed-status-buttons` | 41.0 | 2 | 9 | 0.4 | 18.2 | 5.2 | 0.7 | 12.8 | 0.0 | 0.2 | 0.0 | 3.2 | 0.0 | 0.2 | 1 |
| `gate-run-keeps-cache-warm` | 65.0 | 4 | 15 | 0.5 | 38.6 | 8.3 | 2.9 | 2.9 | 0.0 | 0.6 | 0.0 | 7.0 | 3.7 | 0.4 | 2 |
| `decision-record-on-board` | 41.5 | 2 | 9 | 0.4 | 14.3 | 6.6 | 2.7 | 0.2 | 2.1 | 0.3 | 0.0 | 10.7 | 3.7 | 0.5 | 2 |
| `shed-sub-run-block` | 44.2 | 4 | 9 | 0.4 | 21.8 | 6.3 | 1.6 | 0.0 | 2.4 | 0.5 | 0.0 | 9.1 | 1.3 | 0.8 | 1 |
| `batten-remote-order` | 121.2 | 5 | 14 | 0.5 | 56.2 | 18.1 | 12.7 | 0.7 | 27.1 | 0.8 | 0.0 | 4.7 | 0.0 | 0.6 | 4 |
| `pawl-orch-landing` | 92.9 | 7 | 18 | 0.7 | 69.0 | 11.7 | 3.5 | 2.9 | 0.0 | 1.2 | 0.0 | 3.3 | 0.0 | 0.7 | 5 |
| `raddle-recipe` | 126.5 | 8 | 27 | 1.0 | 88.0 | 15.5 | 4.4 | 0.3 | 0.0 | 2.5 | 2.2 | 8.6 | 3.3 | 0.6 | 5 |
| `drift-rework` | 169.3 | 14 | 27 | 1.4 | 124.8 | 17.3 | 6.5 | 6.5 | 0.0 | 1.9 | 0.0 | 5.7 | 3.5 | 1.9 | 8 |
| `slack-relay` | 68.4 | 3 | 14 | 0.4 | 42.4 | 8.0 | 1.3 | 0.0 | 9.7 | 0.6 | 0.0 | 5.6 | 0.0 | 0.4 | 2 |
| `creel-cross-hub-build` | 133.7 | 3 | 10 | 0.5 | 81.1 | 17.1 | 1.8 | 0.1 | 0.0 | 28.1 | 0.0 | 4.7 | 0.0 | 0.3 | 3 |
| **Total** | 1205.2 | 85 | 226 | 7.9 | 748.2 | 151.9 | 59.3 | 30.8 | 41.4 | 41.2 | 2.2 | 87.8 | 24.4 | 10.1 | 45 |
| **Mean per run** | 86.1 | 6.1 | 16.1 | 0.6 | 53.4 | 10.9 | 4.2 | 2.2 | 3.0 | 2.9 | 0.2 | 6.3 | 1.7 | 0.7 | 3.2 |

The two runs left out of the split:

| Run | Batches | Cards | Machine sleep in the run (min) | Timing `webster` phase (min) |
|---|---|---|---|---|
| `interactive-recipe` | 5 | 12 | 557 | 47.8 |
| `cold-gate-fixer` | 5 | 14 | 400 | 16.5 |

## Where the time goes

Shares of the 14 runs' Webster step (72,312 s), slowest part first.

| Part | Minutes | Share |
|---|---|---|
| Agent work in the forks (model turns and non-gate tools) | 748.2 | 62.1% |
| Card gate (run time; the guard step is 117.4 of the 151.9 min) | 151.9 | 12.6% |
| Plan-level verify gate | 87.8 | 7.3% |
| Batch gate | 59.3 | 4.9% |
| Waits for a gate slot (card gate 28.6, batch gate 6.4, own tests 6.4) | 41.4 | 3.4% |
| Overhead inside a batch (27.9 of it is one stalled batch, below) | 41.2 | 3.4% |
| Tests the forks started themselves | 30.8 | 2.6% |
| Verify-fix fork | 24.4 | 2.0% |
| Master wrap-up and unattributed | 10.1 | 0.8% |
| Start-up | 7.9 | 0.7% |
| Recovery sessions | 2.2 | 0.2% |

- **Start-up.** Between `Webster -> running` and the Master's first transcript line pass 3 to 9 s:
  the partition, the batchifier and the spawn, which the records cannot split further.
  The Master then takes 18 to 31 s to its first `begin-batch`.
  Together about 34 s per run on average, 0.7% of the step.
- **Between batches.** The gap from one `record-batch` to the next `begin-batch` is 2 to 10 s,
  and sums to 278 s over the 14 runs, under the unattributed row.
- **Operator waits.** The Webster step holds no blocked or paused state of the run.
  Two runs rerouted to plan review mid-step (`raddle-recipe` for 27 min, `drift-rework` for 10 min);
  that time is outside the step and is timing's `plan` phase.
  The timing report's `waits` of these 14 runs (escalation 1,466 s, halt 102 s) are not attributed to a phase in it.
- **Slot waits.** 2,481 s inside forks plus 103 s inside the plan-level verify, concentrated in four runs:
  `batten-remote-order` 27.1 min, `slack-relay` 9.7, `shed-sub-run-block` 2.4, `decision-record-on-board` 2.1.
  The other ten waited for no slot.
- **One stall.** `creel-cross-hub-build` batch 8: the first fork died on an API error (connection refused) after 47 min of work,
  its completion notification reached the Master at 11:07 UTC, and the Master recorded `no-report` only at 11:35.
  That 27.9 min is 21% of the run and 2.3% of all analysed time; nothing in Webster's code is slow there, so it is not a lever.
- **Timing report.** The week buckets list 56,532 s of `webster` phase in 2026-W45 and 31,247 s in 2026-W44, none in W43;
  the 16 runs sum to 87,779 s of 201,290 active seconds (44%), median 5,007 s per run.

## Fork time: agent versus gate

The 14 runs held 86 implementer forks (one re-fork after the stall), 61,902 s in all, median 581 s, mean 720 s.

| Part of fork time | Seconds | Share |
|---|---|---|
| Waiting on a gate or test the fork started (run wall) | 14,524 | 23.5% |
| Waiting for a gate slot | 2,481 | 4.0% |
| Agent work | 44,893 | 72.5% |

- Gate and test waits are an upper bound on blocking: a gate that Bash moved to the background after 180 s can overlap the fork's own work.
  Tool calls account for 19,365 s (31%) of fork time, of which gate waits are almost all, so the agent work is mostly model turns:
  4,581 turns (13.5 s each) and 9,360 tool calls.
- **Orientation.** The first `Edit` or `Write` comes a median 95 s into a fork, summing to 10,201 s over 83 forks (16.5% of fork time).
- **Gate content.** The card gate's 9,117 s of run time is 7,043 s of guard step and 2,074 s of card tests.
  The guard step ran 787 times in card gates (28 failed) and 176 times in batch gates (720 s, none failed).

## Batches, fix rounds and recoveries

- Cards per run: mean 16.1, range 9 to 27.
  Batches per run: mean 6.1, range 2 to 14.
  Cards per batch: mean 2.7, range 1 to 6.
- 45 of 85 batches ran at least one fix round, counted as a card or batch gate step that failed (91 failed steps: 85 in card gates, 6 in batch gates).
- 1 batch needed a recovery: `raddle-recipe` batch 7 (the fork reported failure; two recovery sessions, 131 s, the second succeeded).
  1 batch was re-forked: the `creel-cross-hub-build` stall above.
- 8 of the 14 runs failed the plan-level verify on its first attempt and got a verify-fix fork;
  the failing ids include `cmd/lym` in 5 of those 8.
  Counting the two runs left out, 10 of 16 left a `verify-gate.yaml`.
  A first-attempt failure costs about 6.8 min per affected run: the fork (183 s on average) plus a second verify (226 s above a single one).

## The batcher question

How many batches would a grouping by the card dependency graph give, against the recorded partition?

**Method.** A throwaway tool, not committed, parsed each recorded plan with `internal/planparser`
and took each card's size from the per-card components recorded in `state.json`'s partition breakdown
(read set, messages, write allowance, start-up context by position).
It derived the card edges as `deriveEdges` in `internal/websterengine/sequence.go` does:
an edge from a card's `Targets` to a later card's `Uses` or `Targets`, as `planparser.DependencyEdge` accepts it.
It kept the rename boundary of `internal/batcher/boundary.go`: a later card that covers an earlier card's target cannot share its batch.
Under the `wide` profile every recorded run used (`budget: 300000`, `max_cards: 6`) it formed batches in execution order,
each taking the ready cards, lowest card number first or longest dependency chain first, while the batch fits the budget at its position.
It reports the smaller count of the two orders.
The result is a heuristic: the true minimum is at most these counts.
Two checks hold on all 16 plans: the tool's recomputed peak equals the recorded estimate (to 12 tokens on one plan, exactly on 15),
and its contiguous grouping reproduces the recorded batch count.

| Run | Cards | Recorded | Contiguous, recomputed | Dependency graph | Edges | Longest chain |
|---|---|---|---|---|---|---|
| `run-verbs-shared` | 20 | 11 | 11 | 10 | 99 | 16 |
| `interactive-recipe` | 12 | 5 | 5 | 4 | 33 | 8 |
| `warp-visibility` | 20 | 11 | 11 | 10 | 40 | 7 |
| `plan-split-producers` | 20 | 5 | 5 | 4 | 42 | 9 |
| `machine-switch` | 14 | 6 | 6 | 5 | 27 | 8 |
| `reed-status-buttons` | 9 | 2 | 2 | 2 | 31 | 8 |
| `gate-run-keeps-cache-warm` | 15 | 4 | 4 | 4 | 23 | 6 |
| `cold-gate-fixer` | 14 | 5 | 5 | 5 | 22 | 8 |
| `decision-record-on-board` | 9 | 2 | 2 | 2 | 15 | 4 |
| `shed-sub-run-block` | 9 | 4 | 4 | 3 | 13 | 6 |
| `batten-remote-order` | 14 | 5 | 5 | 5 | 27 | 8 |
| `pawl-orch-landing` | 18 | 7 | 7 | 7 | 70 | 10 |
| `raddle-recipe` | 27 | 8 | 8 | 7 | 54 | 8 |
| `drift-rework` | 27 | 14 | 14 | 13 | 176 | 17 |
| `slack-relay` | 14 | 3 | 3 | 3 | 24 | 7 |
| `creel-cross-hub-build` | 10 | 3 | 3 | 3 | 27 | 8 |
| **Total** | 252 | 95 | 95 | 87 | | |

- Eight plans lose one batch each, and none loses more: 95 batches become 87 (−8.4%);
  for the 14 analysed runs 85 become 78 (−8.2%).
- The graph is dense (13 to 176 edges over 9 to 27 cards) with chains of 4 to 17 cards,
  so grouping by it frees little that the contiguous run did not already take.
  The note `batcher-dag-order` is sized at about one batch per plan on one plan in two.
- A regrouped batch runs its cards out of card-number order, and `internal/batcher` returns batches in plan card order today,
  which `CheckBatchOrder` only asserts; the note has to change that invariant, not only the grouping.

## Levers

Estimated saving is minutes per run, averaged over the 14 analysed runs (mean Webster step 86.1 min).
The levers overlap a little, so their sum is not exact.

1. **Run the guard step once per batch, not in every card gate.**
   Saves 6 to 8 min.
   The guard step is 7,043 of the card gate's 9,117 s (77%), run 787 times for 226 cards;
   the batch gate already runs it (176 times, 720 s).
   Dropping it from the card gate saves up to 8.4 min per run;
   the 28 card-gate guard failures would surface at the batch gate instead, about 60 s a fix round, leaving 6.4 min.
2. **Shorten the fork's orientation read.**
   Saves 3 to 6 min.
   A fork's first edit comes a median 95 s in, and orientation is 10,201 s (16.5% of fork time), 12.1 min per run.
   Handing the fork the file and symbol list its cards name, or having it read them in one batched turn, should halve it;
   the halving is an assumption, the 12.1 min is measured.
3. **Catch `cmd/lym` and cross-package failures before the plan-level verify.**
   Saves 2 to 4 min.
   8 of 14 runs failed verify on attempt 1, the failing ids hold `cmd/lym` in 5 of them,
   and each such run paid a verify-fix fork plus a second verify: 3,269 s in all, 3.9 min per run, 6.8 per affected run.
   Adding `cmd/lym` to the batch gate's scope, or to the last batch's gate, moves that failure into a fork that can fix it at once;
   the cost of the added gate run is not measured.
4. **Relieve gate-slot contention for Webster forks.**
   Saves about 3 min, 0 to 27 min by run.
   Forks and the verify waited 2,584 s for a slot, 3.1 min per run, almost all in four runs;
   `batten-remote-order` lost 27.1 min of its 121 while other worktrees held the slots (`gate_key`, `wait_start` and `start` in the gate run records).
5. **Regroup by the dependency graph (`batcher-dag-order`).**
   Saves 1 to 1.5 min.
   85 batches become 78 in the analysed runs, 0.5 per run;
   each removed batch saves one batch gate (46 s on average with its slot wait), about 9 s of batch overhead
   and part of a fork's 119 s average orientation, 2 to 3 min in all.

Together the levers are 15 to 23 min of the 86 min mean, 17% to 27%.
The largest part, agent work at 53.4 min per run, is not a lever of this report:
the records do not split it into work the cards need and work they do not.

## Limits

- Gate times are exact (the gate run records); fork spans and turns come from transcripts, and batch times from commit times.
- A fix round is any failed card or batch gate step, so one failing call that fails at two tiers counts once per step.
- Agent work is a residual, so every error in the gate columns lands in it.
- 14 runs on one codebase in two weeks is a small sample, and they span 41 to 169 min.
- The dependency-graph count is a greedy upper bound on the minimum, and prices cards by the recorded estimate, not by a measured fork peak.
