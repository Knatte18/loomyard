// Package dobbyengine is the engine of dobby, the daemon that drives the batten runs of one beam worktree.
// It runs behind the hidden `lym beam dobby` verb, one process per beam worktree on its owning machine, and is told every path it touches.
//
// The prime is the root beam: its dobby has an empty slug in Geometry, and the dobby of any other beam carries that beam's slug.
//
// The package holds:
//
//   - Geometry is the absolute paths and names the hub wiring tells the engine.
//   - State is dobby's tolerant JSON record under the worktree's `.lym/beam/` directory, read with ReadState and rewritten under its lock with UpdateState.
//     A record that does not decode reads as absent with one Warn, so a hand-damaged file never wedges the beam.
//   - The PassExit codes are what a `lym batten run` pass exits with for a refusal dobby reads by code.
//   - EnsureLive starts the dobby of each worktree it is named, the prime's name or beam slugs, each only when Live reads its lock free, through SpawnDetached.
//     It records this machine as the prime owner first when the prime is named and none is recorded, and reports per worktree; it is idempotent.
//     A named prime or beam another machine owns is refused naming `lym adopt root` or `lym adopt <slug>` with its owner.
//     A named beam whose landing hold stands is refused naming `lym batten status <slug>` and spawns nothing, through StartDeps.LandingHeld, and a nil one reads as no hold standing.
//     OwnedWorktrees lists the prime and every local beam this machine may start, the set a caller that narrows nothing names.
//     A worktree with an anchor change standing on its own records worktree or the prime's is refused with the change's way forward and starts nothing, through StartDeps.MoveInProgress, and a nil one reads as no change standing.
//   - Config is beam.yaml, loaded strictly because dobby only ever runs inside a hub.
//   - Run is the daemon loop, with every seam it reads in Deps.
//   - `dobby.log` holds the dobby's own lines only, each appended by AppendLog through its own open, which creates no directory, so a removed worktree gets nothing back.
//     Only the holder of the file's dobby lock passes `rotate`: a write that would take the file past 1 MiB first renames it to `dobby.log.1`, replacing an older one.
//     SpawnDetached points the child's raw standard output and error at `dobby.out`, which the holder of the lock rotates the same way.
//     The one rotation a process without the lock makes is on Windows, where RotatePastBound renames `dobby.out` before a spawn only when Live reads the lock free.
//     Bound: `dobby.out` is rotated at the start of the lock holder, so between starts it grows by what the dobby's standard error alone writes.
//   - BuildView is the View of one dobby a reader outside its process sees, built from what it is told: its liveness, its state record, beam.yaml, its runs and the board.
//
// The loop:
//
//   - The lock at Geometry.LockPath is the one decision whether this process is the dobby.
//     ReadsAsReexec reads the state record first: a record holding the process's own pid and its start ticks, or no start ticks, is a re-exec.
//     A re-exec waits Deps.ReexecLockWait (ten seconds) for the lock, the process it replaced releasing it as it execs; any other start waits lock.ProbeGrace.
//     A fresh dobby that reused the pid of a record without ticks, after a reboot, reads as a re-exec.
//   - A lock that stays held returns ErrDobbyRunning and logs `dobby: start refused: another dobby holds the lock` without rotating.
//     A refused re-exec whose record still holds the pid posts a notice naming `lym beam start`, and a refused fresh start posts none.
//   - Holding the lock, Run calls Deps.Locked, which the CLI fills with the run-process registration, then records the start and logs `start fresh pid` or `start re-exec pid`.
//   - A record read error before the lock, or an error out of Run after it other than ErrDobbyRunning, logs `dobby: stopped on error` with its way forward, rotating only while the lock is held, and an error after a re-exec posts a notice.
//   - It reads its beam's owner at start and on each probe tick, and exits with a logged reason, `stop: another machine owns this beam`, naming `lym adopt root` or `lym adopt <slug>`, when another machine owns it; a cancelled context logs `stop: the context ended`.
//   - A record with the process's own pid and start ticks owes no new full pass; a record with the pid and no ticks gains the ticks and owes one; any other start rewrites the pid, the start time and the ticks and owes one.
//     A start, a machine sleep the sleep watch sees, and a tick that finds the record without a pid each owe every run a full pass until a tick succeeds.
//   - It wakes on a watch event and on its timer, coalescing events to at most one tick a second.
//     The timer starts at the probe interval and doubles to a minute while no run is in flight, and returns to the probe interval on an event or while a run is in flight.
//     Where the watch does not open, as on WSL or through a junction, the timer alone drives the loop at the probe cadence, and no wake is shorter than a second.
//   - A beam dobby ends when its worktree is gone, tested as the absence of the worktree's `.git` and never of its root, since every state write recreates the record's directory.
//     The loop tests it at the top of every iteration and again after each tick, before the binary check; the prime's dobby never takes the exit.
//     The exit appends `stop: the worktree of beam <slug> is gone` to Geometry.PrimeLogPath without rotating, and stops the tidy agent the last tick with the worktree present read in the state record through Deps.PrimeTidyShuttle, logging the stop or its failure with `lym reed remove <guid>` the same way.
//     It writes nothing at the removed worktree's path.
//     Bound: a tick in flight when the worktree is removed can leave an empty `.lym/beam` stub holding the `state.json` and lock file its state access wrote, and the rest of that tick runs over the fresh empty State it read there; a tidy that tick spawned is not stopped, and one it settled is stopped again, which removes a strand only while reed still tracks it and logs a failed stop when reed's status probe fails.
//   - Between ticks it calls the binary check, which re-execs onto a changed lym binary.
//     The check logs `re-exec onto the changed lym binary` right before the exec; an exec that fails logs `re-exec failed` and posts a notice naming `lym beam stop` and `lym beam start`, once per binary identity, and the dobby keeps running.
//   - A tick's failure is logged once per episode and the loop goes on; a cancelled context ends it after the tick in flight.
//   - While any path of Geometry.MoveRecords exists, an anchor change stands on the dobby's own records worktree or the prime's, and a tick spawns no pass, starts no task and runs no board-tidy step, the tidy agent being a strand whose state sits at the anchor.
//     The state record's upkeep and the landing count go on, a full pass the dobby owes stays owed, and the first tick of the episode logs it once.
//     An empty list, as a hand-built Geometry leaves it, gates nothing.
//   - The log records changes only, each one line through the Log seam stamped with its time, and a heartbeat.
//     A task start logs `start <slug>`; a pass spawn logs nothing.
//     A run's batten state or producer changing logs `run <slug>: <old> -> <new>`, the state alone for running and done and with ` at <producer>` for any other, and a run first seen logs its state once; the memory is in process only, so a re-exec logs no change it did not see.
//     The first failing pass of an episode logs `pass <slug> failing: <error>` and the exit 0 that ends it `pass <slug> succeeds again`.
//     A tick failing, and the beam's owner being unreadable, log their first line of an episode and the line that ends it.
//     A board-tidy agent starting, posting its report and timing out each log one line.
//   - A heartbeat, `heartbeat: <slug> <state> <producer>; …` per run still driven or `heartbeat: no runs`, is written right after the start line, which also records the logged states, and by the first tick at least ten minutes after the last one, whatever that tick did.
//     A run listing that fails writes `heartbeat: runs unreadable: <error>` in its place.
//     It rides the loop's ticks with no wait or wake of its own, so a dead dobby shows in its log within ten minutes plus the longest gap between ticks.
//
// The passes:
//
//   - Each tick lists the runs whose parent worktree is this dobby's, from the prime's local batten status files and never the board, so a run whose task Land already pruned is still driven through its teardown.
//   - A run gets a pass only while batten's state is running and its cancel marker is absent, never while a pass of it is in flight; a held driver lock reads as in flight, so a pass left by a dead dobby is waited for and never doubled.
//     A halted or paused batten run gets no pass; the orch or the operator resumes it with a hand pass.
//   - A done batten run is driven no further: once no pass of it is in flight, it takes no slot under `max_runs`, gets no pass, and its entry leaves the state record.
//     The board tidy still reads its history.
//   - A pass is triggered by a write of the child's status file, by a write of batten's own status file later than the run's last pass exit, by a probe tick once per probe interval, by the dobby's start and by the lift of a pause that covered the run.
//     A lift while a pass is in flight gives the run its pass once that one exits.
//   - A continue pass needs the run at Run-Shed with no full pass owed and no lift to answer; every other pass is full, and runs with `--no-reopen` while the run's origin-owed marker stands.
//     A start and each recorded sleep owe every run a full pass, kept in the state record until a full pass exits 0 with no origin-owed marker standing.
//   - While a pause covers the run, only continue passes run, and the full pass at the lift.
//   - A refused continue pass owes a full pass; a covered pass counts as no failure; any other non-zero exit counts a failure by its error text, retried on the next probe tick.
//     The third failure in a row with one text posts one notice to the nearest live orch, and an exit 0 clears the count and the noticed texts.
//   - A pass that exits as owned by another machine keeps the run's full pass owed and marks it owned elsewhere in the state record.
//     Dobby pauses the run under the owner caller, releases the parent lock record naming its child, and posts one notice naming `lym adopt <subject>` per owner episode, the subject read through `Deps.AdoptSubject` and the slug when that seam is nil or fails, with no failing-pass notice.
//     A failed pause warns once per episode and is recorded; it is never retried blind, since while the mark stands and no pause covers the run, each probe tick gives it a full pass, which is refused again or exits 0 and clears the mark.
//     While the mark stands and a pause covers the run, it gets no pass, a start's and a wake's included, but the full pass at the lift of the covering pause.
//     Once the owner pause stands, each probe tick reads the run's effective owner from the local Bolt with no pull, and when it no longer reads another machine, dobby lifts its own owner pause and runs a full pass.
//     An owner that a continue pass's own check finds sets no mark: that check pauses the run itself and dobby goes on passing it.
//     A marked run is in flight for the backoff only while no pause covers it.
//   - A task waits on the board as `draft` until released, and dobby starts it from an empty status.
//     It starts a task when the entry is a task whose dependencies are all done, its parent is this dobby's worktree, no other machine owns it, and no pause entry covers the hub or the slug.
//     Each tick reads the local board, which a write of `board.json` and the probe tick both cause, and starts each ready task while fewer than `max_runs` runs it drives are in flight: halted runs count, done runs, runs recorded owned elsewhere and pawl runs with no landing hold do not, and `max_runs: 0` starts nothing.
//   - Starting is the run's first pass, a full pass, which claims the task as batten does; a slug with a pass in flight is never started twice.
//     A first pass refused for a covering pause is no refused start: no notice, no marker and no backoff, and the start waits for the lift.
//     Any other first pass that exits non-zero and leaves no status file, a refusal as owned elsewhere included, is a refused start: one notice per refusal text, and a retry from one probe interval, doubling up to an hour, or at once when the entry changes.
//     The backoff, the last refusal text and the noticed texts are kept in the state record, and cleared when a pass claims the task or its board entry changes.
//   - A task is started only from an empty status, by its parent's dobby or a hand-run `lym batten run`, and never twice while a pass of it is in flight.
//     Across machines only the owner machine's dobby of the parent runs, and a release written on another machine reaches this dobby when this machine's board is next pulled.
//   - Bound: per run at most one pass per probe interval plus one per write of the child's status file, and at most one in flight.
//     A run counts as in flight for the loop's backoff while the planner could give it a pass on the next probe tick.
//
// The slot rule:
//
//   - A run whose RunView carries NoSlot takes no slot under `max_runs`, a pawl run whose landing hold does not stand, which only waits on its members at Worktree-Create, Beam-Up and Beam-Await; it takes one from Parent-Align on.
//     The parent's dobby still schedules the run's passes, owed full passes and probes as for every run it drives, and the view's `max_runs` reasons follow, since BuildView counts through the same rule.
//
// The "can land" notice:
//
//   - After the scheduling steps, each tick of a beam's dobby reads Deps.CanLand, whose LandReading says whether nothing works under the beam any more and which other machine, if any, owns the beam's parent.
//     The prime's dobby and a nil CanLand post none; the tick runs on every board change the dobby watches and on every pass exit, so a member's teardown, a cancel with `--remove`, a member moved off the project and one marked done by hand each trigger it.
//   - A passing reading with no sent episode in State.LandNotice posts one notice to the beam's own orch naming `lym beam land <slug>`, or, when another machine owns the parent, that it does and `lym adopt <subject>` instead, and records the episode as sent.
//     A reading that does not pass clears the episode, so a member opened again re-arms the notice; a restarted dobby reads the record and sends no second notice.
//   - A failed read or post is logged once per episode and changes nothing: a failed post leaves the episode unsent for the next tick.
//
// The board tidy:
//
//   - Each tick reads the batten history of every run whose parent worktree is this dobby's, whatever the run's state, and records each Land Done that names this machine as counted in the state record, so a landing a hand pass or a pass a dead dobby left made is counted and a restart counts none twice.
//     A record read for the first time, or rewritten fresh over one that did not decode, counts every landing already there as seen, so run records left from before the dobby trigger no tidy.
//     While a tidy agent is in flight nothing is counted, so the record's slugs since the last tidy are exactly the agent's.
//     A record written before the count read machines has its landed-since-tidy list emptied once, the first count after the deploy that is not held by a tidy in flight, which also drops this machine's own landings counted before the deploy and not yet tidied.
//   - At every `board_tidy_every` landings, 0 meaning never, dobby spawns the board-tidy agent unless one is in flight: a fresh shuttle run in a window of the worktree's reed session, role BoardTidyRole, its prompt TidyPrompt over the landed slugs and a parent directive naming the nearest live orch, its one output the report `.lym/beam/tidy/<UTC timestamp>.md`.
//     The agent in flight is kept in the state record, so a restart or a re-exec spawns no second.
//   - Once the report exists dobby posts its path to the nearest live orch and ends the agent through shuttle's stop verb.
//     Past `board_tidy_timeout_min` it ends the agent the same way, posts a no-report line naming the slugs, and keeps them in the state record for the next tidy.
//   - Bound: a fresh record loses a tidy in flight and the slugs landed since the last tidy, and the next tidy may spawn while the old agent runs until its timeout.
//     A landing whose run records `lym batten cleanup --apply` removed before dobby read them is not counted.
//
// The view:
//
//   - The runs are the listed ones that are not done, each with batten's state and producer, a pass in flight while its driver lock is held, and its last pass exit, owed full pass, owner mark and failing count from the state record.
//   - A task waits while it is released to the worktree, a task with an empty status whose parent is the worktree, and not started.
//     Its reasons are the start step's own conditions: the dependencies not done, another machine's ownership, a covering pause, a refused start whose backoff has not run out for the entry as it reads now, and `max_runs`.
//     The `max_runs` reason falls, in board order as the start step walks, on the tasks nothing else holds once the runs that take a slot fill it; a task with no reason starts on the dobby's next tick.
//   - Bound: a first pass in flight that has left no status file is in no file, so its task reads as ready and its slot as free until the status appears.
//
// Told-geometry tier: the package takes every absolute path from its caller and imports neither internal/lymcwd nor any CLI package, per PATTERN-cwd-resolution.
// Its seam_enforcement_test.go enforces that membership mechanically.
package dobbyengine
