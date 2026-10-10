# PATTERN-daemon-wakeups

A process that waits without a bound wakes cheaply: at most once a second, backing off while nothing changes, and on a file event wherever its substrate offers one.

## What counts

An unbounded wait is a loop that lives as long as a run or a session and has no deadline of its own:

- the detached daemons `lyx reed watchdog` and `lyx orch watch`,
- batten's Run-Shed wait,
- shuttle's `Wait`,
- webster's long poll,
- `lyx shed status --watch`,
- the `--until-stop` waiter in `internal/shedverbs/loopwait.go`, whose event source is `internal/fswatch` on the run's steps directory and whose timer starts at the one-second floor,
- the step watchdog in `internal/shedverbs/loopwatchdog.go`, which wakes at most once a second and backs off to a minute while the child shows no change.

A bounded wait that ends within a minute is outside the rule.

## The rule

- **One-second floor.** Any periodic wake is at least one second apart.
  A configured interval below the floor is floored with one Warn naming the key and the floor, and is never refused.
- **Backoff.** A wake that spawns a process or captures a pane doubles its wait from a base to a named ceiling while nothing changes, and returns to the base on a change or an event.
- **One event source.** `internal/fswatch` is the one file-event source.
  A tmux hook that touches a file is how tmux state becomes a file event.
- **A test never waits out an interval.** A test that wants a fast loop injects a clock or a timing struct, never a sub-second configured value.

## The guard

`cmd/lyx/daemonwakeups_test.go` holds the scan:

- Over a declared list of the unbounded-wait files, it fails on a sub-second duration literal and on an integer constant named `…MS` or `…Ms` below 1000.
- A line that must keep one, such as a bounded retry, carries `//lyx:one-shot <reason>`; a bare annotation without a reason does not let the line through.
- Over every production file under `internal/` and `cmd/`, it fails on a `time.NewTicker` or `time.Tick` call outside the declared list.
  The way forward is to list the loop or explain it in the same commit.

## Bound

The scan sees literal shapes only.
A computed sub-second period, or a `time.Sleep` loop in a file the list does not name, is caught by review.
The annotation lets any sub-second literal through on the line that carries it, so the guard enforces announcement and not absence; the reason sits on the line itself, where review of the commit reads it.
