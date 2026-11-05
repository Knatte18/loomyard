# PATTERN-daemon-wakeups

An unbounded wait wakes at most once a second and backs off while idle, and a wait an agent blocks on takes its cap from the keep-warm resolver.

## Unbounded waits

Rule: Unbounded wait wakes at most once a second and backs off while idle. (test)

A process that waits without a bound wakes cheaply: at most once a second, backing off while nothing changes, and on a file event wherever its substrate offers one.

### What counts

An unbounded wait is a loop that lives as long as a run or a session and has no deadline of its own:

- the detached daemons `lym reed watchdog` and `lym orch watch`,
- the resize watch loop's poll mode in `internal/reedengine/watchloop.go`, whose timer starts at a two-second base and backs off to a minute while nothing changes,
- dobby's loop in `internal/dobbyengine/loop.go`, which wakes on a file event or its timer, at most one tick a second, and doubles the timer from batten's probe interval to a minute while no run is in flight,
- batten's Land row land-lock wait in `internal/battenshed/land.go`, which polls from one second and doubles to thirty seconds,
- the parent lock wait in `internal/parentlock/acquire.go`, which polls from one second and doubles to thirty seconds, and on an `internal/fswatch` event on the branch's lock record, still a second after its last attempt began,
- the hibernate request's trigger wait in `internal/hibernateengine/watch.go`, run in the prime's dobby, which polls from one second and doubles to thirty seconds while its reading is unchanged,
- the capture wait in `internal/driftengine/wait.go`, which polls the capture lock from one second and doubles to thirty seconds while the lock stays held,
- the pause hold in `internal/pauserecord/hold.go`, which polls from one second and doubles to five seconds while the covering entry is unchanged,
- shuttle's `Wait`,
- webster's long poll,
- `lym shed status --watch`,
- the gate test's CPU watchdog in `internal/gatecli/watchdog.go`, which samples once per period, 5 seconds in production, for as long as go test runs and spawns nothing,
- the detached gate runner's lease read in `internal/gatecli/detachrun.go`, which wakes once a second for as long as the run's lease lasts,
- the capped gate call's front in `internal/gatecli/detachfront.go`, which tries the key directory's lock, looks for the runner's claim and polls the run's exit file, each once a second and bounded by the call's keep-warm cap,
- the `--until-stop` waiter in `internal/shedverbs/loopwait.go`, whose event source is `internal/fswatch` on the run's steps directory and whose timer starts at the one-second floor,
- the PR-Review row's wait in `internal/landingshed/prreview.go`, which wakes from one second, doubling to a minute while nothing changes, and on an `internal/fswatch` event on the run's pull-request record, still a second after its last wake began,
- the Sampler-Session row's wait in `internal/loomshed/samplersession.go`, which wakes from one second, doubling to a minute while nothing changes, and on an `internal/fswatch` event on the run's sampler record, still a second after its last wake began,
- the step watchdog in `internal/shedverbs/loopwatchdog.go`, which wakes at most once a second and backs off to a minute while the child shows no change,
- the status panes' loop in `internal/statuspane/pane.go`, which polls from one second and backs off to eight seconds while the text shows no change,
- creel's delivery workers in `internal/creelengine/deliver.go`, which probe from one second and double to eight seconds after every wake that delivers nothing, returning to one second on a new post, reply, forward, inject or release and on a wake that delivers, and capping a live, unpaused receiver's wait at its soonest queued inject's remaining TTL, never below one second,
- creel's removed-holder sweep in `internal/creelengine/sweep.go`, which runs while a strand holds unread mail, a queued inject or a read, marked, unanswered message, from 30 seconds doubling to 50 seconds while the strand-name set is unchanged, with a 10-second confirm after a first miss,
- creel's chase timer in `internal/creelengine/chase.go`, which waits one `agedPersistInterval` while the holder's clock runs and doubles to `chaseProbeCeiling` while it is stopped, returning to the base on a running tick or a wake of the holder, and ends when the holder holds nothing chaseable,
- creel's daemon poll in `internal/creelengine/server.go`, which asks once a second whether its binary was replaced and whether the UTC day changed,
- creel's waiting verbs in `internal/creelengine/client.go` and `server.go` (`post --wait-reply`, `inject --wait`, `read --wait` and `hook-wait`), which wait on their connection and reconnect from one second; the client's connect wait after a spawn also ends on the exit of the daemon it started, other than a lost start race.

A bounded wait that ends within a minute is outside the rule.

### The rule

- **One-second floor.** Any periodic wake is at least one second apart.
  A configured interval below the floor is floored with one Warn naming the key and the floor, and is never refused.
- **Backoff.** A wake that spawns a process or captures a pane doubles its wait from a base to a named ceiling while nothing changes, and returns to the base on a change or an event.
- **One event source.** `internal/fswatch` is the one file-event source.
  A tmux hook that touches a file is how tmux state becomes a file event.
- **A test never waits out an interval.** A test that wants a fast loop injects a clock or a timing struct, never a sub-second configured value.

### The guard

`cmd/lym/daemonwakeups_test.go` holds the scan:

- Over a declared list of the unbounded-wait files, it fails on a sub-second duration literal and on an integer constant named `…MS` or `…Ms` below 1000.
- A line that must keep one, such as a bounded retry, carries `//lym:one-shot <reason>`; a bare annotation without a reason does not let the line through.
- Over every production file under `internal/` and `cmd/`, it fails on a `time.NewTicker` or `time.Tick` call outside the declared list.
  The way forward is to list the loop or explain it in the same commit.

### Bound

The scan sees literal shapes only.
A computed sub-second period, or a `time.Sleep` loop in a file the list does not name, is caught by review.
The annotation lets any sub-second literal through on the line that carries it, so the guard enforces announcement and not absence; the reason sits on the line itself, where review of the commit reads it.

## Keep-warm cap

Rule: Bounding a wait an agent blocks on: the cap from `shuttleengines`' one keep-warm resolver for the caller's role, never a constant bar its fallback; a long gate run goes on in a detached runner the same command joins.

Held by review alone; no test enforces it.
