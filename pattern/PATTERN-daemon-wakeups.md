# PATTERN-daemon-wakeups

An unbounded wait wakes at most once a second and backs off while idle, and a wait an agent blocks on takes its cap from the keep-warm resolver.

## Unbounded waits

Rule: Unbounded wait wakes at most once a second and backs off while idle. (test)

A process that waits without a bound wakes cheaply:
at most once a second, backing off while nothing changes, and on a file event wherever its substrate offers one.

### What counts

An unbounded wait is a loop that lives as long as a run or a session and has no deadline of its own:

- the detached daemons `lym reed watchdog` and `lym orch watch`,
- the resize watch loop's poll mode in `internal/reedengine/watchloop.go`:
  its timer starts at a two-second base and backs off to a minute while nothing changes,
- dobby's loop in `internal/dobbyengine/loop.go`:
  it wakes on a file event or its timer, at most one tick a second,
  and it doubles the timer from batten's probe interval to a minute while no run is in flight,
- batten's Land row land-lock wait in `internal/battenshed/land.go`:
  it polls from one second and doubles to thirty seconds,
- the parent lock wait in `internal/parentlock/acquire.go`:
  it polls from one second and doubles to thirty seconds;
  an `internal/fswatch` event on the branch's lock record also wakes it, never within a second of an attempt's start,
- the hibernate request's trigger wait in `internal/hibernateengine/watch.go`, run in the prime's dobby:
  it polls from one second and doubles to thirty seconds while its reading is unchanged,
- the capture wait in `internal/driftengine/wait.go`:
  it polls the capture lock from one second and doubles to thirty seconds while the lock stays held,
- the current capture's lock wait in `internal/driftengine/capture.go`:
  it polls the capture lock from one second and takes it once free,
  and it doubles to thirty seconds while the lock stays held,
- the pause hold in `internal/pauserecord/hold.go`:
  it polls from one second and doubles to five seconds while the covering entry is unchanged,
- shuttle's `Wait`,
- the load turn's API-error hold in `internal/shuttleengine/loadturn.go`:
  it wakes once per back-off, from 30 seconds doubling to five minutes while the API keeps failing the load;
  between turns it reads the events file and reed's status every `pollInterval`, floored at one second like every poll,
- webster's long poll,
- `lym shed status --watch`,
- the gate test's CPU watchdog in `internal/gatecli/watchdog.go`:
  it samples once per period, 5 seconds in production, for as long as go test runs;
  it spawns nothing,
- the detached gate runner's lease read in `internal/gatecli/detachrun.go`:
  it wakes once a second for as long as the run's lease lasts,
- the capped gate call's front in `internal/gatecli/detachfront.go`:
  it tries the key directory's lock, looks for the runner's claim and polls the run's exit file once a second each;
  the call's keep-warm cap bounds it,
- the `--until-stop` waiter in `internal/shedverbs/loopwait.go`:
  its event source is `internal/fswatch` on the run's steps directory, and its timer starts at the one-second floor,
- the PR-Review row's wait in `internal/landingshed/prreview.go`:
  it wakes from one second, doubling to a minute while nothing changes;
  an `internal/fswatch` event on the run's pull-request record also wakes it, never within a second of a wake's start,
- the Sampler-Session row's wait in `internal/loomshed/samplersession.go`:
  it wakes from one second, doubling to a minute while nothing changes;
  an `internal/fswatch` event on the run's sampler record also wakes it, never within a second of a wake's start,
- the step watchdog in `internal/shedverbs/loopwatchdog.go`:
  it wakes at most once a second and backs off to a minute while the child shows no change,
- the status panes' loop in `internal/statuspane/pane.go`:
  it polls from one second and backs off to eight seconds while the text shows no change,
- creel's delivery workers in `internal/creelengine/deliver.go`:
  they probe from one second and double to eight seconds after every wake that delivers nothing,
  and they return to one second on a new post, reply, forward, inject or release and on a wake that delivers,
  and they cap a live, unpaused receiver's wait at its soonest queued inject's remaining TTL, never below one second,
- creel's removed-holder sweep in `internal/creelengine/sweep.go`:
  it runs while a strand holds unread mail, a queued inject or a read, marked, unanswered message;
  its wait runs from 30 seconds, doubling to 50 seconds while the strand-name set is unchanged,
  and a first miss gets a 10-second confirm,
- creel's chase timer in `internal/creelengine/chase.go`:
  it waits one `agedPersistInterval` while the holder's clock runs,
  and it doubles to `chaseProbeCeiling` while the clock is stopped;
  it returns to the base on a running tick or a wake of the holder,
  and it ends when the holder holds nothing chaseable,
- creel's outbound workers in `internal/creelengine/outbound.go`:
  there is one per receiving hub with a pending message to another hub;
  each retries a transient answer from a one-second base, doubling to a one-minute ceiling,
  and the wait is capped at what is left of the message's hour;
  the worker ends at that hour, and a wake changes nothing during the wait,
- creel's daemon poll in `internal/creelengine/server.go`:
  it asks once a second whether its binary was replaced and whether the UTC day changed,
- creel's waiting verbs in `internal/creelengine/client.go` and `server.go`:
  `post --wait-reply`, `inject --wait`, `read --wait` and `hook-wait` wait on their connection;
  they reconnect from one second;
  the client's connect wait after a spawn also ends when the daemon it started exits, except a lost start race,
- the `lym adopt` follow in `internal/adoptcli/follow.go`:
  it reads the adopt record from one second, doubling to eight seconds while the record is unchanged,
  and it returns to one second on a change;
  an agent session's keep-warm cap bounds it, and a plain shell follows until the worker ends,
- the Slack relay's loops:
  the outbound read in `internal/slackengine/relay.go`, with the binary check and the retried work after it, from five seconds doubling to sixty;
  the holder in `internal/slackengine/holder.go`, its lock wait blocking without waking, its 30-second ping spawning nothing and its reconnect from one second doubling to sixty;
  and the inbox and presence watch in `internal/slackengine/relay.go`, never within a second of the previous wake's start, with its on-disk fallback from one second to sixty.

A bounded wait that ends within a minute is outside the rule.

### The rule

- **One-second floor.** Any periodic wake is at least one second apart.
  A configured interval below the floor is floored with one Warn naming the key and the floor, and is never refused.
- **Backoff.**
  A wake that spawns a process or captures a pane doubles its wait from a base to a named ceiling while nothing changes,
  and it returns to the base on a change or an event.
- **One event source.** `internal/fswatch` is the one file-event source.
  A tmux hook that touches a file is how tmux state becomes a file event.
- **A test never waits out an interval.**
  A test that wants a fast loop injects a clock or a timing struct, never a sub-second configured value.

### The guard

`cmd/lym/daemonwakeups_test.go` holds the scan:

- Over a declared list of the unbounded-wait files, it fails on a sub-second duration literal,
  and it fails on an integer constant named `…MS` or `…Ms` below 1000.
- A line that must keep one, such as a bounded retry, carries `//lym:one-shot <reason>`;
  a bare annotation without a reason does not let the line through.
- It scans every production file under `internal/` and `cmd/`.
  A `time.NewTicker` or `time.Tick` call outside the declared list fails it.
  The way forward is to list the loop or explain it in the same commit.

### Bound

The scan sees literal shapes only.
A computed sub-second period, or a `time.Sleep` loop in a file the list does not name, is caught by review.
The annotation lets any sub-second literal through on the line that carries it,
so the guard enforces announcement and not absence;
the reason sits on the line itself, where review of the commit reads it.

## Keep-warm cap

Rule: Bounding a wait an agent blocks on: the cap from `shuttleengines`' one keep-warm resolver for the caller's role, never a constant bar its fallback; a long gate run goes on in a detached runner the same command joins.

Held by review alone; no test enforces it.
