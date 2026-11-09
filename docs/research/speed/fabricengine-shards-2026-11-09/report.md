# Sharding `internal/fabricengine` tier 1: a measurement (2026-11-09)

A measurement report on machine OSL-1046 (16 cores, Cortex XDR installed), run under the load the machine normally carries.
Each run was done once, so each figure is one sample.
No code or test changed.

## Answer in brief

- Sharding is not worth building from this evidence.
  The package's wall was 242 s as one process, 249 s as two shards and 292 s as four.
  The gate's own plain run of the one-process case took 256 s.
- Sharding did not fall toward serial time divided by the shard count.
  The fastest of the four shards, with a quarter of the tests, took 208 s, 86 percent of the one-process wall.
- The package is bound by neither CPU nor disk here.
  Its process tree used 1.23 cores on average of 16, the machine was 57 percent idle, and iowait was 1.5 percent.
- 59.5 percent of the one-process wall is a serial phase, the top-level tests that are not marked parallel.
  Inside it the package keeps about one core busy while the machine is two-thirds idle.
- The average git process took 6.8 ms of wall as one process, 8.0 ms with two shards and 10.6 ms with four, for the same 65 to 67 thousand processes.
  More concurrency slowed each spawn instead of finishing the same spawns sooner.
- The levers the data points to are fewer git processes in the serial phase and fewer serial top-level tests, the latter with an upper bound of about half the wall.
  More shards did not move the wall, and the hub copies are 3 percent of the spawns.
- The package ran 6 times slower than on 2026-11-08 (37 to 41 s at a one-minute load under 2), so every figure here sits on a loaded machine.
  Whether sharding helps on a quiet machine was not measured: the task runs under the machine's normal load and waits for no quiet.

## Method

All runs ran through `lym gate test --wait --tier 1 ./internal/fabricengine`, with `-count=1` and a `-timeout` that differs per run so the gate carries no green record forward.
The three sharding runs pass `-exec "shard-runner.sh <out dir> <shards>"` to go test.
go test then hands the script the compiled test binary and the flags it would have given the binary.
The script runs the binary once, or as N copies at once, each with a `-test.run` regex over its share of the top-level tests, and prints the shards' output to go test.
Whichever way, the run is one gate tier 1 run, so it holds one gate slot and the gate's build and vet run before it.
The run's wall is the script's start to end, which excludes the gate's build and vet.

- `shard-runner.sh` splits the tests longest-processing-time first, by the per-test serial times of `fabric-test-redundancy-2026-11-08/pertest.tsv`.
  A test with no row there weighs 0.05 s.
  A test that tier 1 skips is shared out like any other and costs nothing.
  The shards start together, one background job each, and the script waits for all of them.
- The same script samples the machine every second (`/proc/stat`, `/proc/diskstats`, the one-minute load) and the five busiest processes every 10 s.
  It runs each shard under `/usr/bin/time -v` and sets `GIT_TRACE2_EVENT` to a directory, so every git process leaves one trace2 file.
  The tracing and sampling are on in all three runs, so they cost the same in each.
- The one-process run also writes a timestamped `test2json` copy of its events, which `serial-share.sh` reads for item 1.
- `resources.sh` reads one run's samples, `time -v` files and trace2 files for item 3.

| Run | Arguments after `lym gate test --wait --tier 1 ./internal/fabricengine` |
|---|---|
| Plain | `-- -count=1 -timeout=21m` |
| One process | `-- -count=1 -timeout=22m -exec "<path>/shard-runner.sh <dir> 1"` |
| Two shards | `-- -count=1 -timeout=23m -exec "<path>/shard-runner.sh <dir> 2"` |
| Four shards | `-- -count=1 -timeout=24m -exec "<path>/shard-runner.sh <dir> 4"` |

The plain run has no events or samples.
It is a second sample of the one-process wall and ran first.
The four runs followed one another within about 25 minutes.

## The runs and their load

The load is the `/proc/loadavg` triple (1, 5 and 15 minutes) before and after each run.
The one-minute load before a run still holds the previous run's own work, because the runs followed one another.
"Machine busy" is the share of all jiffies on the 16 cores that were not idle or iowait, over the run.

| Run | Load before | Load after | Package time (`ok` line) | Run wall | Machine busy | Mean sampled load |
|---|---|---|---|---|---|---|
| Plain | 3.67 4.72 4.39 | 22.51 10.98 6.77 | 255.9 s | n/a | n/a | n/a |
| One process | 10.97 9.89 6.69 | 30.83 16.42 9.69 | 242.7 s | 242.0 s | 41.5 % | 14.0 |
| Two shards | 13.67 14.60 9.76 | 31.60 23.29 14.37 | 249.1 s | 248.1 s | 44.0 % | 24.6 |
| Four shards | 31.81 23.86 14.75 | 47.19 37.74 23.01 | 292.0 s | 290.5 s | 43.4 % | 42.0 |

The processes busy at the time, as the five busiest per 10 s sample, were the same set in every run:

- `pmd` and `piud` of `/opt/traps`, the Cortex XDR agent, in 18 to 22 of about 23 samples per run, at about 30 and 26 percent of a core.
- `dotnet` and `VBCSCompiler`, the other projects' C# builds.
- `compile`, `vet`, `link` and `go` of a second Go build, present in the two-shard run.
- `chrome`, `claude`, `Xorg` and `x2golistsession`.
- `fabric.test`, the package itself, in 10 to 18 samples.

The four-shard run started at a one-minute load of 31.8, which is mostly the two-shard run's own tail.
The runs therefore do not start at equal load, and the order (one process, two shards, four shards) is confounded with a rising start load.
The 5 percent difference between the plain run (255.9 s) and the one-process run (242.7 s) is the best measure here of the spread between two runs of one case.

## Item 1: the serial share and the longest serial chain

`serial-share.sh` on the one-process run's events, which ran 400 top-level tests and 1196 leaf tests at tier 1.
A test is marked parallel when it paused, which `t.Parallel()` causes.
Go runs the top-level tests that are not marked parallel one after another first, and resumes the paused ones together after the last of them (phase 1 and phase 2).

| Figure | Value |
|---|---|
| Package time, first test to last | 240.7 s |
| Top-level tests not marked parallel | 46 of 400 |
| Leaf tests not marked parallel | 355 of 1196 |
| Leaf time, summed | 1802.6 s |
| Leaf time of the leaves not marked parallel | 218.2 s, 12.1 percent |
| Elapsed of the 46 serial top-level tests, summed | 126.1 s |
| Phase 1, the serial chain | 143.2 s, 59.5 percent of the package |
| Phase 2 | 97.4 s |
| Top-level time inside phase 2 | 838.3 s, so 8.6 top-level tests ran at once on average |
| Longest top-level test | `TestPushPairAnchored_PushesBothSidesRetriesAndReportsEachSide`, 21.5 s, in phase 2 |

- The share of leaf time in tests not marked parallel is 12 percent, but the serial share of the wall is 59.5 percent.
  Leaf time adds the parallel tests' overlapping time, and the serial phase does not overlap.
- The longest serial chain is phase 1 itself.
  Phase 1 is 17 s longer than the serial tests' 126 s because it also holds the start of every parallel test up to its pause.
- The eight longest serial top-level tests are `TestPushFailureClasses` 19.4 s, `TestAnchorChange` 14.8 s, `TestReconcileOrigin_Judge` 10.4 s, `TestStoreHousekeeping` 8.5 s, `TestWorktreeSeam_Events` 8.3 s, `TestReconcileOrigin_Ended` 8.0 s, `TestReconcileOrigin_Moves` 7.8 s and `TestReconcile_MigratesMarkerlessWefts` 7.1 s.
  `fabricengine-per-test-times-2026-11-07.md` names a process-global notifier, hook or threshold as the reason several of them are serial.
- Phase 2 cannot end before its longest test, 21.5 s, and the tail of phase 2 is narrower than its mean width of 8.6.

## Item 2: the wall as one process, two shards and four shards

| Run | Tests per shard | Shard walls | Run wall | Against one process |
|---|---|---|---|---|
| One process | 573 | 241.0 s | 242.0 s | 1.00 |
| Two shards | 290, 283 | 161.0 s, 247.9 s | 248.1 s | 1.03 |
| Four shards | 140, 149, 142, 142 | 217.4 s, 288.7 s, 245.8 s, 207.6 s | 290.5 s | 1.20 |

The counts are the tests of the binary's `-test.list`, tier 1 or not.

- The wall is the slowest shard's wall plus the script's own start and end, about 1 s.
- The shards were unbalanced: the slowest was 1.5 times the fastest with two shards and 1.4 times with four.
  The weights are serial times from a quiet machine, a poor predictor under this load.
- With perfect balance the four shards would still average 240 s, the one-process wall.
  So the imbalance does not explain the result.
- Each shard ran its own `TestMain` and its own hub template builds, which the one-process run does once.
  The git processes grew from 64 970 to 65 996 and 67 240, 1.6 and 3.5 percent more, and the hub copies stayed at 979 in all three.
- Total CPU of the shards and their git children grew with the shard count: 296.7 s, 344.3 s and 382.4 s of user plus system time.
- All three runs passed: 1375 passed, 174 skipped, none failed, and no shard collided with another over a lock or a shared path.

## Item 3: CPU, waiting, git processes and hub copies in the one-process run

`resources.sh` on the one-process run.

| Figure | Whole run | Phase 1 | Phase 2 |
|---|---|---|---|
| Machine CPU busy (user, system and irq) | 41.5 % | 31.0 % | 56.8 % |
| Machine user CPU | 26.5 % | 18.4 % | 38.2 % |
| Machine system CPU | 14.9 % | 12.5 % | 18.5 % |
| Machine iowait | 1.5 % | 1.9 % | 0.8 % |
| Machine idle | 57.1 % | 67.1 % | 42.4 % |
| Disk busy | 238 ms/s | 270 ms/s | 193 ms/s |
| Disk reads and writes | 3037 and 268 per s | 3315 and 153 per s | 2652 and 435 per s |

- The binary and its git children together used 97.4 s of user and 199.3 s of system time, 296.7 s over a 241 s wall: 123 percent of one core on average.
  Two-thirds of that CPU is system time, which is process creation and file calls.
- The disk was busy a quarter of the time (238 ms/s) with iowait near 1.5 percent, so the package did not wait on the disk.
- With 16 cores and the machine 57 percent idle, the package is not CPU bound either.
  It sat at a one-minute load of 8 to 32 while the CPU was mostly idle, so many tasks were runnable or blocked while no core was short.
  The samples do not say which: `procs_blocked` was not sampled.
- The package started 64 970 git processes in 241 s, about 270 a second, with 444 s of git wall time summed over them, 6.8 ms each.
- The most common commands were `config` 6050, `rev-list` 5664, `rev-parse` 4935, `commit` 3295, `for-each-ref` 3183, `pack-objects` 3053, `gc` 2857, `unpack-objects` 2851, `push` 2775, `receive-pack` 2774, `upload-pack` 2685 and `worktree` 2616.
- The package made 979 hub copies, each ending in two git processes, so about 2000 of the 64 970 processes (3 percent), and 349 `git init` processes, which are scratch hub builds, bare repositories and clones together.
  Hub copies are not where the spawns are.
- 55 of the 220 test files have no `t.Parallel()`.
  The events say 354 of the 400 top-level tests are marked parallel: the serial tests are few and long, not many.

### Per-spawn cost rose with concurrency

| Run | Git processes | Git wall summed | Per process | Machine system CPU |
|---|---|---|---|---|
| One process | 64 970 | 444 s | 6.8 ms | 14.9 % |
| Two shards | 65 996 | 531 s | 8.0 ms | 16.4 % |
| Four shards | 67 240 | 713 s | 10.6 ms | 18.7 % |

The git work stayed constant and each process took longer as more ran at once, with idle cores left.
A shard's own CPU fell to a quarter (96 s of user plus system time against 297 s), but its CPU share of its wall fell from 123 percent to 31 to 47 percent.
So a shard spends most of its wall waiting.
The data fits a cost that each spawn pays and that grows with concurrent spawns, such as the endpoint agent inspecting each new process and file, or kernel contention on process creation.
It does not isolate which.

## What would move the wall

- More shards did not, as shown above.
- Fewer git processes would, because the cost sits in spawns: 65 thousand of them at 270 a second, each slower when many run at once.
  The serial phase matters most, because it is the 59.5 percent of the wall that no overlap hides.
  A spawn removed from a serial test shortens the chain by its whole latency.
- Fewer serial top-level tests would, in principle.
  The 46 serial tests run for 126 s of the 143 s phase 1.
  If all of them ran at phase 2's mean width of 8.6, they would add about 15 s to phase 2 and take their 126 s out of phase 1, leaving about 129 s of the 241 s.
  That is an upper bound: per-process cost rose with concurrency in the shard runs, so the real gain is smaller.
  Several of the serial tests are serial for process-global state, so each needs the state injected before it can be marked parallel.
- More parallel tests among the 354 already marked would not help: phase 2 is 97 s and already runs 8.6 tests at once.
- Fewer hub copies would move about 3 percent of the spawns and the copies' file work, so it is not the first lever.

## Is sharding worth building?

Not on this evidence.
On this machine under its normal load, two shards equalled one process and four shards were 20 percent slower, with 16 to 29 percent more CPU.
A sharded gate run would also have to merge the shards' green record and failure report, a cost with no wall saving to pay for it.

This does not settle a quiet machine.
The earlier reports measured the package at 37 to 41 s at a load under 2, where one process already ran three times faster than serial, and no run here waited for that state.
A build task would first need one sharding run on a quiet machine, and a `procs_blocked` sample in `shard-runner.sh`, to tell a spawn-bound machine from a CPU-bound one.

## Reproduce

From the worktree, with `<dir>` an absolute scratch directory that does not exist yet:

```sh
lym gate test --wait --tier 1 ./internal/fabricengine -- -count=1 -timeout=22m \
  -exec "$PWD/docs/research/speed/fabricengine-shards-2026-11-09/shard-runner.sh <dir> 1"
docs/research/speed/fabricengine-shards-2026-11-09/serial-share.sh <dir>/events.json
docs/research/speed/fabricengine-shards-2026-11-09/resources.sh <dir>
```

Use shard count 2 or 4 and a different `-timeout` for the other runs.
`serial-share.sh` needs the one-process run's `events.json`; `resources.sh` reads any run.
