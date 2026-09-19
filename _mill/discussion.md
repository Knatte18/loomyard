# Discussion: reed: per-hub daemon reaps orphaned sessions

```yaml
task: 'reed: per-hub daemon reaps orphaned sessions'
slug: reed-per-hub-daemon-reap
status: discussing
parent: main
```

## Problem

A tmux session on a hub's shared socket outlives the worktree it belongs to whenever the worktree directory disappears without `lyx reed down` running first — a crash mid-teardown, an operator `rm -rf`, an aborted task, a `fabric` teardown whose session-shutdown row never ran.
Nothing reaps it.
The session keeps its panes, its shells, and every Claude/agent process inside them alive indefinitely, squatting on the hub socket under a name no reed verb can address any more, because every reed verb derives its session name from a worktree root that no longer exists.

The two mechanisms that look like they should cover this both structurally cannot:

- `Engine.Watch`'s dormant mode (`internal/reedengine/watchloop.go`) *detects* the condition — `errWorktreeRootGone` surfaces from `reapplyLayout`, the watcher logs once and drops to a 60s cadence — but dormancy is a wait for the directory to come back, not a teardown. It never kills anything.
- The per-hub daemon (`internal/reedcli/watchdog.go`) never even enters such a session: `enterSession` → `resolveWatchedSession` → `lyxcwd.ResolveWorktree` spawns git against the missing directory and fails, so the name is logged at Debug and skipped every cycle, forever.

So an orphan that existed before the daemon started is skipped forever, and one orphaned while the daemon watched it goes dormant forever.
Either way it never dies.

**Why now:** the per-hub daemon shipped with the header/Selvage item and is the only process on the machine that already enumerates a hub's live sessions from the outside, on a cadence, without needing any one worktree to exist.
It is the only place this check can live.
The roadmap entry names it explicitly as a *safety net* for when the `worktree spawn/teardown as Shed producers` item's deliberate teardown sequencing does not run — not a replacement for that sequencing.

## Scope

**In:**

- A per-cycle orphan check in `runWatchdogLoop` (`internal/reedcli/watchdog.go`): for every live session name, decide whether `filepath.Join(hub, name)` is still a live directory.
- A consecutive-cycle confirmation counter before any reap fires, so a transient stat failure or an in-flight rename cannot kill a healthy session.
- A pure, injectable decision seam (`planSessionReap`-shaped) so the whole rule is unit-testable with no tmux, no filesystem, and no fixture.
- Reaping a confirmed orphan: kill its tmux session by exact target, and reap its pane process subtrees so the agent processes inside it actually die rather than being re-parented.
- One new engine-less exported function in `internal/reedengine` (`ReapSession`) alongside the existing `ListSessions`, plus an `Engine.ShellPath()` accessor.
- A `--shell` flag on `lyx reed watchdog`, told by `ensureWatchdogSpawned`'s single `exec.Command` construction — the one `up`, `resume` and `attach` all reach — exactly as `--tmux` already is.
- Cancelling and dropping the daemon's own `watchedSession` entry for a reaped name, so no goroutine keeps polling a killed session.
- An in-flight-reap set and a `sync.WaitGroup` in `runWatchdogLoop`, so reaps run off-loop without racing each other and the daemon never exits mid-reap.
- A timing-injection seam for the daemon's loop, so no test waits on the production cadence: `runWatchdogLoop` gains a `watchdogTiming` struct parameter (`DiscoveryCycle`, `IdleCycles`, `OrphanGoneCycles`) with a `watchdogDefaultTiming()` constructor sourcing the existing package constants and nothing else, and `watchdogCmd` passes that. This is deliberately the same shape `internal/reedengine/watchloop.go` already uses for `watchTiming`/`watchDefaultTiming`, so the repo carries one idiom for it rather than two. The `watchdogHubDiscoveryCycle`/`watchdogHubIdleCycles` constants stay exactly where they are and keep their doc comments; only their consumer changes.
- Logging: `Warn` on an actual reap (a destructive, unattended action), `Debug` on per-cycle confirmation increments.
- Docs: the watchdog section of `manifest/designs/reed-header-selvage.md`, and moving the roadmap item out of Planned.

**Out:**

- `kill-server`. A reap never tears the hub's shared server down — sibling worktrees may be live or mid-boot, and the daemon's existing `watchdogHubIdleCycles` idle-exit already covers a hub going quiet.
- Deleting or repairing anything on disk. The worktree is gone; there is no `.lyx`, no `reed.json`, no lock to clean up. The reap is process-and-session-only.
- `Engine.Down`. It cannot be used here and must not be adapted to: `withOpLock`'s `validateToldWorktreeRootLive` pre-flight refuses with `errWorktreeRootGone` in exactly the case this task reaps, and relaxing that validator would re-open the leak it was added to close.
- Standalone reed. It runs `Engine.Watch` as an in-process goroutine off the `reedUp` seam and has no daemon and no hub — nothing here applies to it.
- Any change to `Engine.Watch`, dormant mode, or `errWorktreeRootGone`. Dormancy stays exactly as it is; it is the per-worktree watcher's own resize concern, and the reap is a hub-level concern that runs above it.
- Removing worktree directories, git worktree pruning, or anything `lyx fabric cleanup` owns.
- Reaping when no daemon is running. The daemon exists only because `up`, `resume` or `attach` spawned it (`ensureWatchdogSpawned`), and it retires itself after `watchdogHubIdleCycles` idle cycles. So an orphan on a hub whose every worktree is already gone, or on a hub whose daemon was killed, survives until an operator next runs an lyx command against that hub and re-spawns the daemon — at which point the never-entered-orphan path reaps it. This is an accepted limitation, not an oversight: giving the reap its own independent lifecycle means an OS service or login agent, which the daemon as a whole is explicitly not. The safety net covers the common shape — some worktrees on the hub are still in use while another's directory vanished — and the degenerate all-gone hub costs one squatting tmux server until the next lyx command.
- Turning the daemon into an OS service, launch agent, or systemd unit — explicitly out of scope for the daemon as a whole.

## Decisions

### reap-check-lives-in-the-discovery-loop

- Decision: the orphan check runs in `runWatchdogLoop`'s own per-cycle body, over the **live session-name list** returned by `reedengine.ListSessions`, not over the daemon's `known` map and not inside `Engine.Watch`.
- Rationale: the live list is the only input that covers both failure shapes. A session orphaned before the daemon started never enters `known` at all (`resolveWatchedSession` fails), so a `known`-driven check would miss precisely the case the operator hits after a crash-and-restart. `Engine.Watch` is per-worktree and every op it can reach refuses once the root is gone, so it has no reachable teardown path.
- Rejected: reaping from dormant mode inside `Engine.Watch` (cannot see never-entered sessions, and every engine op it could call is refused by the told-geometry pre-flight); iterating `known` (same blind spot); a separate `lyx reed reap` verb an operator runs by hand (the point of the item is that it is unattended).

### gone-predicate-mirrors-validateToldWorktreeRootLive

- Decision: a session name counts as gone for this cycle when `os.Stat(filepath.Join(hub, name))` returns `fs.ErrNotExist`, **or** succeeds with a non-directory. Every other stat outcome — success-and-is-a-directory, or any other error (EACCES, EIO, a network-filesystem hiccup) — counts as **not gone**, and the cycle makes no progress toward a reap for that name.
- Rationale: this is `validateToldWorktreeRootLive`'s own only-proven-gone-carries-the-sentinel contract (`internal/reedengine/server.go`), restated at the one layer that cannot call it. Treating an unreadable path as gone would let a momentary permission or I/O blip kill a healthy session full of live work.
- Rejected: treating any stat error as gone (turns a transient fault into destruction); calling `lyxcwd.ResolveWorktree` as the predicate (it spawns git per session per cycle, and it fails for reasons unrelated to the directory existing — a corrupt `.git`, a git binary problem — none of which prove the worktree is gone).

### direct-join-not-a-scan

- Decision: the session-name → worktree-path mapping is the exact join `filepath.Join(hub, sessionName)`, reusing `resolveWatchedSession`'s existing rationale rather than inventing a second mapping.
- Rationale: hub-mode `reedengine.SessionName(worktreeRoot)` is `filepath.Base(worktreeRoot)` verbatim, and `validateToldTmuxIdentity` refuses an unusable name instead of sanitizing it, so with `hub == filepath.Dir(worktreeRoot)` the join recovers the worktree root exactly. Two different derivations of the same mapping in the same file would be free to drift.
- Rejected: scanning the hub for directories and matching names (same answer, more I/O, and it makes an absent directory indistinguishable from an unreadable hub).

### three-consecutive-cycles-before-a-reap

- Decision: a name must be observed gone on `watchdogOrphanGoneCycles` (3) **consecutive** discovery cycles before it is reaped — roughly 15s at the existing 5s `watchdogHubDiscoveryCycle`. Any cycle in which the name is not-gone resets its counter to zero, and any name absent from the live list is dropped from the counter map entirely.
- Rationale: the reap is destructive and unattended, and a directory can be legitimately absent for a moment — a `git worktree move`, an editor or backup tool swapping a directory, a filesystem remount. Three cycles is the same figure and the same reasoning `watchdogHubIdleCycles` already uses for the daemon's own exit, so the file carries one cadence idiom rather than two.
- Decision — what a non-affirmative cycle does to the counters: a cycle whose `ListSessions` call errored, or returned an empty list, `continue`s before the reap pass, so it advances no counter, resets no counter, and prunes no counter. "Three consecutive cycles" therefore means three consecutive *affirmative* cycles, which may be separated by a tmux outage.
- Rationale: the alternative readings are both worse. Resetting on a non-affirmative cycle would let a flapping socket indefinitely postpone reaping a genuinely dead worktree; advancing on one would count cycles that observed nothing at all as evidence the worktree is gone. Leaving them untouched keeps every counter increment backed by an actual observation, which is the same only-proven-gone posture the predicate itself takes. Pruning is skipped on those cycles for the same reason — a name absent from a listing that never happened is not evidence the session left.
- Rejected: reaping on the first observation (one blip destroys live agent work); a wall-clock grace period (a second time base to reason about, when the loop already has a cycle counter); a much longer grace such as minutes (this is a safety net for an already-broken state — dragging it out only extends how long the leak runs).

### counter-resets-after-a-reap

- Decision: when a reap is dispatched for a name, that name is deleted from the gone-counter map rather than left at its threshold value.
- Rationale: `kill-session` teardown is asynchronous, so the name can still appear in the next cycle's listing. Resetting means a still-listed name has to re-confirm across three more cycles before a second kill is issued, which both gives the first kill time to land and keeps the repeat kill idempotent rather than fired every 5s. The in-flight set (see `the-reap-runs-off-loop-not-inline`) covers the shorter window the reset alone does not: while the goroutine is still running, the name is not even eligible for re-selection.
- Rejected: leaving the counter at threshold (a lagging teardown means a kill every cycle); tracking a separate "already reaped" set (more state for a case the reset already answers).

### reap-kills-the-session-and-its-pane-subtrees

- Decision: reaping means (1) `kill-session` against the exact target `=<name>`, and (2) capturing the session's safe pane reap roots and their descendant closure **before** the kill and then reaping them with the existing `reapPaneChildren` wait-then-force-kill sequence.
- Rationale: killing only the tmux session is the leak this task exists to close, not a fix for it. tmux terminates pane children asynchronously and a process that traps or escapes SIGHUP — a detached agent, a `claude` session — survives it, which is exactly why `Engine.Down` captures roots before `kill-session` and reaps after. An orphan reap that skipped this would leave the expensive processes running and only remove the handle the operator could have used to find them.
- Rejected: `kill-session` alone (leaks the processes the reap is for); also issuing `kill-server` when the reap empties the socket (a sibling may be mid-boot; the daemon's idle-exit already handles a quiet hub).

### ReapSession-is-a-second-engine-less-exported-function

- Decision: `internal/reedengine` gains exactly one new exported symbol, engine-less and mirroring `ListSessions`: `ReapSession(tmuxPath, shellPath, socketKey, sessionName string) error`. It builds a throwaway `*Engine` internally purely to reach the existing pane-reap helpers, acquires **no** lock, touches **no** state directory, and reads **no** geometry field beyond `SocketKey`/`SessionName`. It never calls `withOpLock`/`withTryOpLock`, so it never reaches a told-geometry validator.
- Decision — the split point, named: the tmux half is `reapSessionTmux(cmd TmuxCmd, session string) (roots []int, err error)`, which takes a `TmuxCmd` by value exactly as `listSessionsVia` does. It calls `cmd.listPanes(session)`, derives `sessionReapRoots(live)`, then issues `cmd.run("kill-session", "-t", exactSessionTarget(session))` and returns the roots it captured. It spawns no process of its own beyond tmux and waits on nothing, so it is fully drivable through `TmuxCmd`'s `execHook` seam with no live server. The process half — `(*Engine).descendantClosurePIDs(roots)` followed by `reapPaneChildren(pids, reapExitTimeout)` — stays on `*Engine`, because `descendantClosurePIDs` reads `e.cfg.Shell` on Windows. `ReapSession` is the two-line composition of the two halves, in that order.
- Rationale for the split landing there: it is the same seam `Engine.Down` already has implicitly — capture-roots-and-kill is pure tmux round trips, and everything after it touches real OS processes. Putting the boundary anywhere later would drag `waitProcessExit`/`proc.KillPID` into whatever covers the first half.
- Test consequence, stated so it is not discovered at code time: the **untagged** test covers `reapSessionTmux` only, through `execHook`. The process half is never exercised by an untagged test — it kills and waits on real pids, which the Test Tier Purity Invariant forbids outside `integration`/`smoke`-tagged files, and it is covered there instead by the end-to-end orphan reap.
- A `listPanes` failure does not abort the reap: `reapSessionTmux` returns nil roots and still issues `kill-session`, because a session whose panes cannot be listed is exactly the one most worth killing. The error is logged, not propagated as a reason to skip the kill.
- Rationale: `TmuxCmd.run`/`output`, `sessionReapRoots`, `descendantClosurePIDs` and `reapPaneChildren` are all unexported package internals, so the reap physically cannot be written in `internal/reedcli`. `ListSessions` already established the precedent and the justification for exactly this shape: the daemon acts across sessions before it has any Engine to bind a method to. Keeping it lock-free and state-free is what makes it legal for a worktree that no longer exists — there is nothing left to lock or persist.
- Rejected: exporting `TmuxCmd.run` (opens the whole tmux surface to every caller for one use); having `reedcli` shell out to tmux itself (a second tmux invocation site outside `reedengine`, which is the boundary `reedengine` exists to hold); building a real `Engine` in `reedcli` and calling `Down` (refused by `validateToldWorktreeRootLive`, and `Down` also deletes state files and tidies the shared server — neither of which this path may do).

### the-daemon-is-told-its-shell

- Decision: `lyx reed watchdog` gains a `--shell <path>` flag alongside `--hub-path` and `--tmux`, validated non-empty in the same pre-flight, and `ensureWatchdogSpawned` passes `c.eng.ShellPath()` for it. `Engine` gains a `ShellPath()` accessor beside the existing `TmuxPath()`.
- Rationale: `descendantClosurePIDs` on Windows (`internal/reedengine/proctree_windows.go`) spawns `e.cfg.Shell` to walk the process tree, so the reap needs a shell path, and the orphan's own config is unreachable by construction — it lived in the deleted worktree. Telling the daemon its shell on the command line is exactly the posture it already takes for its tmux binary and its hub path: it opts out of cwd/location/config resolution entirely and must never derive either from its own environment.
- Decision: `--shell` is **accepted on every GOOS but never required**, and its pre-flight does **not** reject an empty value — unlike `--hub-path` and `--tmux`, which stay hard requirements. `ensureWatchdogSpawned` passes `c.eng.ShellPath()` unconditionally, empty or not, and the daemon logs a `Warn` naming the hub when it starts with an empty shell.
- Rationale — this supersedes an earlier round's "required on every GOOS" reading, which was wrong on consequences: `ensureWatchdogSpawned` is best-effort and its child's stderr is discarded, so a hard pre-flight rejection would silently cost the hub its **entire** watchdog daemon — resize self-heal for every worktree included — over one empty config value, with the only trace in the hub's durable log. The degradation from an empty shell is far smaller than that: `proctree_linux.go` never reads `cfg.Shell` at all, and `proctree_windows.go`'s `descendantClosurePIDs` already returns `roots` unchanged when its probe command fails, so an empty shell degrades the reap to killing the pane root pids without their descendants. A bounded reap degradation on one platform beats losing the daemon outright on both.
- Reachability, stated so the case is not over-estimated: `reedengine.LoadConfig` → `configengine.LoadOrTemplate` refuses a `reed.yaml` whose `shell:` key is **absent** (`MissingKeys` → `missing keys: shell; run "lyx config reconcile"`), and that refusal fires long before `ensureWatchdogSpawned` runs. The only reachable empty value is a `shell:` key present with an empty value, or an env expansion resolving to empty. That is exactly why the answer is a logged degradation rather than a validator: the strict path already covers the common shape.
- Consequence to carry through: `watchdogCmd`'s own `Long` text advertises running the daemon by hand as "a real diagnosis path" and prints an example carrying only `--hub-path` and `--tmux`. Both gain `--shell` — it is what a hand-run daemon should be told for its reap to be complete, even though it is not enforced — so the `Long` text and its example are updated in the same change; they are part of this task's documentation surface, not incidental help prose.
- Rejected: defaulting to a per-GOOS shell inside `ReapSession` or at the spawn site (an unrelated second source of truth for the shell, duplicating the `shell:` default that `template_posix.yaml`/`template_windows.yaml` already own, and diverging silently the moment either moves); loading some other worktree's config to borrow its shell (arbitrary, and wrong the moment two worktrees differ); validating or defaulting `Shell` inside `LoadConfig` (it would change config strictness for every reed consumer to fix one flag's edge case); a hard non-empty `--shell` pre-flight (loses the whole daemon, per the rationale above).

### reap-ignores-the-watchdog-config-key

- Decision: an orphan is reaped regardless of any `watchdog:` value, and the reap path never attempts `reedengine.LoadConfig`.
- Rationale: forced, not chosen — the worktree holding `reed.yaml` is gone, so there is no value to read. It is also correct on the merits: `watchdog: off` disables the *resize* self-heal for a live worktree, an entirely different mechanism from a hub-level safety net cleaning up after a worktree that no longer exists.
- Rejected: skipping `watchdog: off` worktrees (unimplementable — the config is in the deleted directory); adding a separate opt-out key (a key that can only be read from a directory that must be absent for the code to run).

### reap-any-live-session-whose-join-is-gone

- Decision: the rule is applied to every session name on the hub socket, with no allowlist of names the daemon has previously resolved.
- Rationale: every session *lyx itself* creates on this socket is worktree-named by construction (`SessionName(worktreeRoot)` is `filepath.Base(worktreeRoot)`), so the join is exact for every session the daemon is actually responsible for, and the kill uses the exact `=<name>` target, so a prefix-sharing sibling can never be hit by collateral. Restricting to previously-resolved names would reintroduce the never-entered blind spot this design is built to close.
- Accepted collateral, stated rather than assumed away: `ServerName(hub)` only makes the socket lyx-*named*, it does not stop an operator running `tmux -L lyx-<base>-<hash> new-session -s scratch` by hand on it. Such a session has no matching directory under the hub and would be reaped ~15s later. That is accepted: a hand-made session on a socket keyed to lyx's own hub hash is squatting on lyx's substrate, the reap logs it by name at `Warn` in the hub's durable log, and the alternative (a persisted allowlist) costs the never-entered case this whole design exists for.
- Rejected: reaping only names the daemon once resolved successfully (the blind spot); maintaining a persisted registry of lyx-created session names (durable state for a daemon deliberately built to hold none).

### ordering-within-a-cycle

- Decision: one cycle runs: list sessions → update gone-counters and select reaps → for each selected name, cancel and delete its `known` entry, mark it in-flight, and **dispatch its reap to its own goroutine** → remove selected names from the working live list → run the existing `planSessionDiff` appeared/departed pass against what remains.
- Rationale: reaping before the diff is what stops the daemon from entering a session in the same cycle it is about to kill, and stops the kill from immediately registering as a "departure" it would then have to unwind. Removing selected names from the live list before the diff keeps `planSessionDiff` pure and unchanged.
- Rejected: reaping after the diff (a pointless enter-then-tear-down in the same cycle); folding the reap decision into `planSessionDiff` (it would stop being the pure two-set comparison it is, and its existing tests would have to absorb an unrelated concern).

### the-reap-runs-off-loop-not-inline

- Decision: a reap runs in its own goroutine, never inline in `runWatchdogLoop`'s tick body. The loop dispatches it and moves on within the same tick. Concurrency is bounded by two pieces of loop-owned bookkeeping: an **in-flight set** of names currently being reaped, which excludes a name from reap selection *and* from the appeared set until its goroutine returns, and a `sync.WaitGroup` the existing deferred cleanup waits on before `runWatchdogLoop` returns, so the daemon never exits mid-reap and never force-kills its own reaper.
- Rationale: a reap is not cheap and not bounded by anything small. `reapPaneChildren(pids, reapExitTimeout)` waits up to `reapExitTimeout` (15s) for a graceful exit and then up to `forceKillExitGrace` (5s) per straggler after `proc.KillPID`, so one orphan stalls an inline tick body for ~20s and several stall it for multiples of that. Run inline, that stall would silently corrupt three things the rest of this design depends on: the "~15s" the three-cycle confirmation rule is quoted at stops being true (a `time.Ticker` coalesces at most one pending tick, so the loop simply skips cycles it slept through), a newly-appeared healthy session waits up to 20s to be entered and start self-healing, and `ctx.Done()` — the daemon's only shutdown signal — goes unread for the whole stall. Off-loop, the discovery cadence, the idle accounting, and shutdown responsiveness are all exactly what they were before this task, and the reap's cost is paid where nothing is waiting on it.
- The in-flight set is the piece that makes this safe rather than merely fast: without it, the next tick 5s later would see the still-listed name (teardown is asynchronous), and — because the gone-counter was reset at dispatch — would eventually dispatch a second concurrent reap for the same session, two goroutines racing to force-kill the same pids.
- Decision — the in-flight set's synchronization, named: it is a **plain `map[string]bool` owned exclusively by the loop goroutine**, never touched by a reap goroutine. Insertion happens on the loop at dispatch. Removal happens on the loop too: each reap goroutine's only cross-goroutine act is a **non-blocking send of its session name on a buffered `chan string`** (`select { case done <- name: default: }`) just before it returns, and the loop **drains that channel non-blockingly at the top of every tick**, before the reap-selection pass, deleting each drained name from the set.
- Rationale: `runWatchdogLoop` already owns three mutable structures single-threaded — `known`, the gone-counter map, and now this set. A mutex would make all three reachable from N reap goroutines to solve a problem only one of them has, and it is the shape most likely to grow a second lock later. The channel keeps every map mutation on the loop goroutine, so `-race` has nothing to find, and it makes the removal point deterministic and assertable instead of "whenever the goroutine happened to finish".
- Removal timing, accepted explicitly: a name leaves the set at **tick granularity** — up to one `DiscoveryCycle` after its reap actually finished, not the instant it did. That is harmless and arguably right: the reap-selection pass is the only reader, it runs once per tick, and the gone-counter was already reset at dispatch, so the extra cycle costs nothing but a slightly later re-eligibility.
- The send is non-blocking, and the buffer generous, so a reap goroutine can never block on a loop that has already returned — which is what would otherwise deadlock against the deferred `WaitGroup` wait, since nothing drains the channel once the loop is out of its `for`. A dropped send is inert: the loop is gone, and with it the set.
- Rejected: a mutex-guarded set (shared mutability for one writer, as above); having the reap goroutine delete from the map directly (a data race the specified `-race` integration tests would trip); an unbuffered or blocking send (deadlocks against the deferred `WaitGroup` wait at daemon exit).
- Rejected: reaping inline and accepting the stall (the three consequences above, none of which the confirmation rule survives); a single serialized reaper goroutine fed by a channel (an unbounded queue and a second lifetime to reason about, for a case where concurrent orphans are already rare); plumbing `ctx` into `reapPaneChildren` so an inline reap stays interruptible (it would change a shared teardown helper `Engine.Down` also depends on, to work around a problem the goroutine removes outright).

### idle-accounting-is-untouched

- Decision: `sessionsAreIdle` and the idle-exit counter are not changed. A cycle whose listing was affirmative resets `idleCycles` to zero even if every session in it was reaped.
- Rationale: the idle rule is about whether the hub socket answered with sessions, which it did. The hub going genuinely quiet after a reap is then observed by the *next* cycles on their own, and the daemon exits three cycles later through the existing path — no special case needed.
- Rejected: counting an all-reaped cycle as idle (couples two independent counters and would shorten the daemon's life on the exact cycle it did the most work).

## Technical context

**The daemon** — `internal/reedcli/watchdog.go`.
`runWatchdogLoop(ctx, hub, tmuxPath)` ticks at `watchdogHubDiscoveryCycle` (5s), calls `reedengine.ListSessions(tmuxPath, reedengine.ServerName(hub))`, feeds `sessionsAreIdle` for the exit counter, then `planSessionDiff(live, known)` for enter/depart.
`known` is `map[string]watchedSession`; `watchedSession{eng, cancel}` with `cancel == nil` for a `watchdog: off` worktree.
`enterSession` resolves via `resolveWatchedSession` (the `filepath.Join(hub, sessionName)` + `lyxcwd.ResolveWorktree` pair this task's predicate deliberately does **not** reuse), builds geometry through `hubgeom.ReedGeometry`, loads config via `reedengine.LoadConfig`, and starts `eng.Watch` in a goroutine.
`watchdogCmd` does every fallible thing pre-flight on the envelope (`--hub-path` absolute, `--tmux` non-empty), points `logger.SetDurableSinkDir(fabricengine.HubLogsDir(hubPath))` **before** `logger.SetOutput(io.Discard)`, then takes `reed-watchdog.lock` under `fabricengine.HubScratchDir(hubPath)` — contention exits 0, a lock *error* is a real failure.

**The spawn site** — `internal/reedcli/spawnwatchdog.go`.
`up`, `resume` and `attach` each call `ensureWatchdogSpawned`, but there is exactly one `exec.Command` construction for all three, so `--tmux` appears once in production code and `--shell` gets added once beside it.
`ensureWatchdogSpawned` re-execs `os.Executable()` as `lyx reed watchdog --hub-path <hub> --tmux <tmux>`, detached via `proc.Detach`, with `cmd.Dir` pinned to the hub (a held cwd handle on a worktree directory blocks deletion on Windows).
`suppressWatchdogSpawn` guards test binaries from recursive re-exec.
This is where `--shell` gets added.

**The engine-less seam** — `internal/reedengine/overlay.go`.
`ListSessions(tmuxPath, socketKey)` → `listSessionsVia(NewTmuxCmd(...))`; the split exists so a test can drive the parsing half through `TmuxCmd`'s `execHook` seam with no live server.
`ReapSession` should follow the same two-function shape for the same reason.
`exactSessionTarget(session)` returns `"=" + session` — load-bearing, since a bare `-t name` prefix-matches a sibling worktree's session.

**The teardown pieces to reuse, all in `internal/reedengine/lifecycle.go`** — `sessionReapRoots(live []LivePane) []int` (filters by `safeReapRoot`: `!p.Dead && p.PID > 0`, in `strand.go`), `(*Engine).descendantClosurePIDs(roots)` (per-GOOS: `/proc` walk on Linux, a `Get-CimInstance Win32_Process` closure through `e.cfg.Shell` on Windows), `reapPaneChildren(pids, reapExitTimeout)` (waits `reapExitTimeout` = 15s, then `proc.KillPID` + `forceKillExitGrace` = 5s).
`Engine.Down` at `lifecycle.go:864` is the canonical ordering to copy: capture roots while panes still exist → `kill-session` → `reapPaneChildren`.
Note `descendantClosurePIDs` is a method only because of Windows' `e.cfg.Shell`; a throwaway `Engine` carrying just `cfg.Tmux`, `cfg.Shell`, `geom.SocketKey` and `geom.SessionName` satisfies everything the reap needs.

**Why `Down` is unreachable** — `internal/reedengine/lock.go`.
`withOpLock` and `withTryOpLock` both run `validateToldTmuxIdentity` → `validateToldAnchorPath` → `validateToldWorktreeRootLive` before touching anything, and the third returns an `errWorktreeRootGone`-wrapped error exactly when the worktree root does not exist or is not a directory (`internal/reedengine/server.go:310-332`).
Both also `os.MkdirAll(e.stateDir())`, and `stateDir()` is `filepath.Join(e.geom.AnchorPath, lyxdirs.DotLyxDirName)` — inside the deleted worktree.
That third validator was added deliberately to stop the watcher conjuring state under a vanished path; nothing in this task may weaken it.

**The dormant path, for context only** — `internal/reedengine/watchloop.go`'s `watchModeDormant` and `handleWatchOutcome`. `errWorktreeRootGone` from `reapplyLayout` drops the watcher to `watchdogDormantCycle` (60s), logs once, remembers `dormantFrom`, and resumes when the directory returns. It is left untouched; a reaped session's goroutine simply gets cancelled by the reap's `known`-entry teardown.

**Gotchas**

- A reaped name can still appear in the next `list-sessions` answer — teardown is asynchronous. The counter reset is what keeps that from producing a kill every cycle.
- `ListSessions` returns `(nil, err)` for "no server running"; that is an idle cycle, and the reap pass must not run on it (there is no live list to reason about).
- `logger` output is discarded in the daemon; only the durable sink under `HubLogsDir` is observable, which is why every reap decision needs an explicit log line there.
- The daemon's cwd is the hub, never a worktree — `resolveWatchedSession`'s git spawn already relies on being told an absolute path rather than using cwd, and the reap's `os.Stat` must too.

## Constraints

From `CONSTRAINTS.md`:

- **Told-Geometry Invariant** — `internal/reedengine` must not import `internal/lyxcwd`. `ReapSession` takes its tmux path, shell path, socket key and session name as parameters and derives nothing. `internal/reedcli` holds the `*lyxcwd.Location` side and may import `internal/fabricengine`/`internal/hubgeom`, as it already does.
- **Live-Substrate Spawn Observability** — the reap kills real OS processes and *waits* for them, so both the kill and the wait are logged through `internal/logger`. The per-cycle `os.Stat` probe is not a spawn; the existing per-cycle git spawn in `resolveWatchedSession` is logged at `Debug` as a spawn inside a polling probe, and any new spawn on this path follows the same tier.
- **Durable-vs-Ephemeral State Invariant** — the reap persists nothing and creates no `.lyx`. The daemon's own lock stays at `fabricengine.HubScratchDir(hub)/reed-watchdog.lock`.
- **Lifecycle Bookend Invariant** — a producer that destroys a task worktree never runs from inside it. The daemon runs with `cmd.Dir` pinned to the hub and destroys no worktree at all, only sessions and processes; nothing here moves teardown sequencing out of the lifecycle Shed.
- **CLI / Cobra Invariant** — `--shell` is a new flag on an existing command, so the `Command()`/`RunCLI` seam and the existing `Short` are unchanged; the help-tree tests must still pass.
- **Test Tier Purity Invariant** — no `exec.Command`, no `gitexec`, no `hubforge.NewHub`, and no `time.Sleep` ≥ 1s in untagged files. The pure decision seam is untagged; anything touching a live tmux server or a real process tree is `integration`- or `smoke`-tagged.
- **Hermetic Git Test Environment Invariant** — any test package here that spawns git calls `gitkit.HermeticGitEnv()` in `TestMain`. `internal/reedcli` already has `testmain_test.go`.
- **Documentation Lifecycle** — see `docs/overview.md#documentation-lifecycle`.

Discovered during exploration:

- `reedengine`'s exported surface for this feature is capped at what the daemon provably cannot do without: one new function (`ReapSession`) and one new accessor (`ShellPath`). `TmuxCmd.run`/`output` stay unexported.
- The `=<name>` exact-match target is mandatory on every tmux call the reap makes.
- `validateToldWorktreeRootLive`'s only-proven-gone contract is the semantics the new predicate must match, restated rather than imported.

## Testing

**`internal/reedcli` — pure, untagged (TDD candidates, write these first):**

- The reap decision seam. Table-driven over an injected stat function, covering: a cycle whose listing was non-affirmative (every counter untouched — neither advanced, reset, nor pruned, so contiguity spans the outage); a live directory (never reaps, counter stays zero); a missing directory across fewer than the threshold of cycles (no reap yet); missing across exactly the threshold (reaps); missing, then present, then missing (counter reset — no reap); a path that exists but is a file (reaps, same as missing); a stat error that is neither (never reaps, no matter how many cycles); a name that leaves the live list (counter entry dropped, so its return starts from zero); several sessions on one hub progressing independently.
- Counter-map hygiene: no unbounded growth across cycles whose live set churns.
- `planSessionDiff` keeps its existing tests unchanged — proof the reap did not leak into it.
- The `--shell` pre-flight, asserted as the *inverse* of `--tmux`'s: an absent or empty `--shell` starts the daemon anyway (no `output.Err`, no non-zero exit), while an absent or empty `--tmux` or `--hub-path` still refuses exactly as today. This is the regression guard for `the-daemon-is-told-its-shell` — an implementer who copies `--tmux`'s validator fails it.
- The in-flight set's completion channel: a send on a full buffer does not block its goroutine, and a drained name leaves the set on the next tick rather than immediately.
- `watchdogDefaultTiming()` returns exactly the package constants — the guard against a test-only default silently becoming production's cadence, mirroring `watchDefaultTiming`'s own coverage in `watchloop_test.go`.
- In-flight bookkeeping: a name whose reap goroutine has not yet returned is selected neither for a second reap nor as an "appeared" session, even while it is still listed by tmux.

**`internal/reedengine` — pure, untagged:**

- `reapSessionTmux` driven through `TmuxCmd`'s `execHook` seam with no live server: it issues `kill-session` against `=<name>` (never a bare name), it lists panes before it kills, and a `list-panes` failure still results in a `kill-session` attempt and nil roots rather than an early return.
- `sessionReapRoots` already has coverage; assert `reapSessionTmux` feeds it the pane list it got, without re-testing its filter.
- Nothing untagged covers `ReapSession`'s process half (`descendantClosurePIDs` + `reapPaneChildren`) — it waits on and kills real pids, so the Test Tier Purity Invariant puts it in the integration/smoke tier below, not here.

**`internal/reedcli` — integration/smoke-tagged:**

- End-to-end orphan reap: boot a hub session in a temporary worktree, delete the worktree directory, drive the daemon loop with compressed timings, and assert the session is gone from `list-sessions` and its pane children have exited.
- Never-entered orphan: the worktree is already gone when the loop's first cycle runs (the case `enterSession` can never reach), and it is still reaped.
- Non-interference: a healthy sibling session on the same hub socket survives the orphan's reap untouched — this is the collateral-damage guard, and it belongs in the same test as the reap so a broken exact-match target fails loudly.
- The reap's process half: after the end-to-end reap, the orphan's pane child pids are confirmed exited, not merely SIGHUP'd — this is the only tier that can exercise `descendantClosurePIDs` + `reapPaneChildren` at all.
- Loop liveness during a reap: with a reap in flight, the discovery loop still ticks and still enters a newly-appeared session, and cancelling the daemon's context returns promptly rather than after the reap's full ~20s budget. This is the regression guard for `the-reap-runs-off-loop-not-inline`; an inline implementation fails it.
- Existing `watchdog_integration_test.go`, `smoke_teardown_test.go` and `smoke_lifecycle_test.go` are the fixture patterns to follow; every test here drives `runWatchdogLoop` through the `watchdogTiming` parameter (see Scope In) so none waits on the production 5s × 3.

Do not add a test that deletes a worktree while a live `Engine.Watch` goroutine is mid-`reapplyLayout` and asserts on dormancy — that is `watchloop`'s existing contract and is not what this task changes.

## Documentation

- `internal/reedcli/watchdog.go` — `watchdogCmd`'s `Long` help text and its `Example:` line, which today show only `--hub-path` and `--tmux` while advertising the hand-run foreground diagnosis path. Both gain `--shell`.
- `manifest/designs/reed-header-selvage.md` — extend the "Watchdog daemon → a detached, per-hub background process" section with the orphan-reap rule, and drop only the last `## Related` bullet — the one forward-referencing this item as future work. The other `## Related` bullet (the `loom-step.md` / `ly-drive` / `reed-born-as-strand.md` line above it) is untouched and the section stays.
- `manifest/roadmap.md` — move the item out of Planned on completion.
- `docs/overview.md` — no change expected: no new module, and the execution stack is unchanged. Touch it only if that stops being true.
- `CONSTRAINTS.md` — no new cross-cutting invariant expected. The rules here are module-local to reed and belong in the design doc.

## Q&A log

- **Q:** Where does the orphan check run — the daemon's discovery loop, the daemon's `known` map, or `Engine.Watch`'s dormant mode? **A:** [auto-pick] Discovery loop, over the live session-name list. **Why:** it is the only input covering both a session orphaned before the daemon started and one orphaned while it watched; `known` misses the first and `Engine.Watch` has no reachable teardown once the root is gone.
- **Q:** What predicate counts as "worktree gone"? **A:** [auto-pick] `fs.ErrNotExist` or exists-but-not-a-directory; every other stat outcome is not-gone. **Why:** it mirrors `validateToldWorktreeRootLive`'s only-proven-gone contract, so a transient EACCES/EIO cannot destroy live work.
- **Q:** Debounce before reaping, or reap on first observation? **A:** [auto-pick] Three consecutive gone cycles (~15s), counter reset on any not-gone observation. **Why:** the reap is destructive and unattended, and it reuses `watchdogHubIdleCycles`' existing figure and reasoning rather than introducing a second cadence idiom.
- **Q:** Does the reap kill the tmux session only, or also reap pane process subtrees? **A:** [auto-pick] Both, capturing roots before `kill-session` exactly as `Engine.Down` does. **Why:** session-only leaves the agent processes running, which is the leak the task exists to close.
- **Q:** How does the reap reach tmux, given `TmuxCmd.run`/`output` are unexported? **A:** [auto-pick] One new engine-less exported `reedengine.ReapSession`, mirroring `ListSessions`. **Why:** `ListSessions` set the precedent for exactly this shape, and it caps the new exported surface at one function instead of opening `TmuxCmd`.
- **Q:** Where does the daemon get the shell path Windows' process-tree walk needs? **A:** [auto-pick] A new `--shell` flag told by `ensureWatchdogSpawned`'s single spawn construction, alongside `--tmux`. **Why:** the orphan's own config is in the deleted worktree, and being told its inputs on the command line is the daemon's existing posture.
- **Q:** Should `watchdog: off` exempt a worktree from the reap? **A:** [auto-pick] No, and the reap never attempts to load config. **Why:** the config file is in the gone directory, so the question is unanswerable; a resize opt-out is also a different mechanism from a hub-level orphan safety net.
- **Q:** Restrict reaping to session names the daemon previously resolved? **A:** [auto-pick] No — apply the rule to every live session on the hub socket. **Why:** the socket is lyx-owned by construction and the kill uses an exact `=<name>` target, while an allowlist would restore the never-entered blind spot.
- **Q:** Should a reap that empties the socket also `kill-server`? **A:** [auto-pick] No. **Why:** a sibling may be mid-boot, and the existing idle-exit already retires the daemon when the hub goes quiet.
- **Q:** Does an all-reaped cycle count toward the daemon's idle-exit counter? **A:** [auto-pick] No — the listing was affirmative, so `sessionsAreIdle` is untouched. **Why:** the following cycles observe the quiet hub on their own through the existing path.
