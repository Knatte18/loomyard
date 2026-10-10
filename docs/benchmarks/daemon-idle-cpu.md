# Idle CPU of `lyx reed watchdog`, before and after the daemon-wakeups task

Measured 2026-10-10 on a Ryzen AI 7 445 (12 threads), Linux 7.0.0-31, tmux 3.6.
One run per binary, ten minutes each, so every number is a single sample.

## Binaries

| Figure | Binary | Commit |
|---|---|---|
| Before | `go build ./cmd/lyx` over `git archive main` | `7648e8b27`, `main`'s tip before the task landed |
| After | `./deploy-dev` build of the task branch | `03d101522`, the branch at card 13 |

## Procedure

Shell only.
Every command below ran with `TMUX_TMPDIR` pointing at a short scratch directory and `TMUX` unset, so no tmux server of the operator's was reached.

1. Build a scratch hub from local bare repositories in a temporary directory.
   This run built it through the `hubforge` fixture, which drives `fabriccli.CloneAndWire` over copied bare repos, and held it open for the run.
   Add three worktrees to it.
2. In each of the three worktrees, run `lyx reed up`, and leave the sessions idle with no strands.
3. End the daemon that `reed up` spawned for the scratch hub: `pgrep -f "lyx reed watchdog --hub-path <scratch hub>"`, then `kill` that pid.
   The match carries the scratch hub's path, so no daemon of another hub is matched.
4. Start the daemon in the foreground, in the background of the shell, with `LYX_LOG_LEVEL=debug`: `lyx reed watchdog --hub-path <scratch hub> --tmux tmux --shell bash`.
5. Three seconds later read fields 14 to 17 of `/proc/<pid>/stat` (`utime`, `stime`, `cutime`, `cstime`, in clock ticks of 1/100 s).
   Read them again after 600 seconds, then `kill` the pid.
6. Repeat steps 4 and 5 with the other binary.
7. For the spawn rate, run each binary again for 300 seconds with `--tmux` pointing at a shell shim that appends the first non-flag argument of every invocation to a file and then `exec`s `tmux`; count the file's lines by subcommand.
8. End the scratch hub's sessions and remove its directory.

The report states CPU seconds per hour as the 600-second difference times six.
`cutime` and `cstime` are the CPU of the children the daemon waited for, which is where the tmux spawns land.

## Results

| Figure | Daemon CPU (utime + stime) | Children CPU (cutime + cstime) | Total | `list-sessions` per minute |
|---|---|---|---|---|
| Before | 2.68 s per 600 s, 16.1 s per hour | 14.03 s per 600 s, 84.2 s per hour | 100.3 s per hour | 11.8 |
| After | 2.62 s per 600 s, 15.7 s per hour | 14.44 s per 600 s, 86.6 s per hour | 102.4 s per hour | 1.6 |

The spawn counts come from the 300-second shim runs: 59 `list-sessions` before and 8 after.
The shim saw no other subcommand in either run.

## What the numbers say

The discovery loop's tmux spawns fell from one every five seconds to eight in five minutes, the doubling to the 60-second ceiling working as designed, yet the daemon's CPU did not move: 100 s per hour before and 102 s after, a difference inside the noise of a single sample.
The daemon's own CPU is unchanged at about 16 s per hour in both rows; nearly all the CPU is in children the daemon waits for, and those children are not the discovery loop's `list-sessions`, which the shim counts and which fell by a factor of seven.
This run does not identify them: the shim sees only invocations through `--tmux`, and the three idle sessions' own watchers may spawn through another path.
The task's spawn reduction is real on the discovery loop; its effect on idle CPU is not visible in this measurement, and the remaining cost needs its own measurement.

## Deviations from the card's procedure

- The scratch hub was built from local bare repositories through `hubforge`, not cloned with `lyx fabric clone --into`, so that no verb ran against the operator's hub or its remotes.
- The durable log holds Info and above only, and the Debug spawn lines never appear in it; a run at `LYX_LOG_LEVEL=debug` with `LYX_LOG_FILE` wrote five warning lines and no tmux lines, so the spawn counts come from the shim instead.
