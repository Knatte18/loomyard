// Package dobbyengine is the engine of dobby, the daemon that drives the batten runs of one beam worktree.
// It runs behind the hidden `lyx beam dobby` verb, one process per beam worktree on its owning machine, and is told every path it touches.
//
// The prime is the root beam: its dobby has an empty slug in Geometry, and the dobby of any other beam carries that beam's slug.
//
// The package holds:
//
//   - Geometry is the absolute paths and names the hub wiring tells the engine.
//   - State is dobby's tolerant JSON record under the worktree's `.lyx/beam/` directory, read with ReadState and rewritten under its lock with UpdateState.
//     A record that does not decode reads as absent with one Warn, so a hand-damaged file never wedges the beam.
//   - The PassExit codes are what a `lyx batten run` pass exits with for a refusal dobby reads by code.
//   - EnsureLive starts the dobby of the prime and of every local beam pair this machine owns, each only when Live reads its lock free, through SpawnDetached.
//     It records this machine as the prime owner first when none is recorded, and reports per worktree; it is idempotent.
//   - Config is beam.yaml, loaded strictly because dobby only ever runs inside a hub.
//   - Run is the daemon loop, with every seam it reads in Deps.
//
// The loop:
//
//   - It takes the lock at Geometry.LockPath without blocking, and returns ErrDobbyRunning at once when another dobby holds it.
//   - It reads its beam's owner at start and on each probe tick, and exits with a logged reason when another machine owns it.
//   - A state record holding the process's own pid is a re-exec and owes no new full pass; any other pid, or no record, is a start.
//     A start, a machine sleep the sleep watch sees, and a tick that finds the record without a pid each owe every run a full pass until a tick succeeds.
//   - It wakes on a watch event and on its timer, coalescing events to at most one tick a second.
//     The timer starts at the probe interval and doubles to a minute while no run is in flight, and returns to the probe interval on an event or while a run is in flight.
//     Where the watch does not open, as on WSL or through a junction, the timer alone drives the loop at the probe cadence, and no wake is shorter than a second.
//   - Between ticks it calls the binary check, which re-execs onto a changed lyx binary.
//   - A tick's failure is logged once per episode and the loop goes on; a cancelled context ends it after the tick in flight.
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
//     Dobby pauses the run under the owner caller, releases the parent lock record naming its child, and posts one notice naming `lyx adopt` per owner episode, with no failing-pass notice.
//     A failed pause warns once per episode and is recorded; it is never retried blind, since while the mark stands and no pause covers the run, each probe tick gives it a full pass, which is refused again or exits 0 and clears the mark.
//     While the mark stands and a pause covers the run, it gets no pass, a start's and a wake's included, but the full pass at the lift of the covering pause.
//     Once the owner pause stands, each probe tick reads the run's effective owner from the local Bolt with no pull, and when it no longer reads another machine, dobby lifts its own owner pause and runs a full pass.
//     An owner that a continue pass's own check finds sets no mark: that check pauses the run itself and dobby goes on passing it.
//     A marked run is in flight for the backoff only while no pause covers it.
//   - A task waits on the board as `draft` until released, and dobby starts it from an empty status.
//     It starts a task when the entry is a task whose dependencies are all done, its parent is this dobby's worktree, no other machine owns it, and no pause entry covers the hub or the slug.
//     Each tick reads the local board, which a write of `board.json` and the probe tick both cause, and starts each ready task while fewer than `max_runs` runs it drives are in flight: halted runs count, done runs and runs recorded owned elsewhere do not, and `max_runs: 0` starts nothing.
//   - Starting is the run's first pass, a full pass, which claims the task as batten does; a slug with a pass in flight is never started twice.
//     A first pass refused for a covering pause is no refused start: no notice, no marker and no backoff, and the start waits for the lift.
//     Any other first pass that exits non-zero and leaves no status file, a refusal as owned elsewhere included, is a refused start: one notice per refusal text, and a retry from one probe interval, doubling up to an hour, or at once when the entry changes.
//     The backoff and the noticed texts are kept in the state record, and cleared when a pass claims the task or its board entry changes.
//   - A task is started only from an empty status, by its parent's dobby or a hand-run `lyx batten run`, and never twice while a pass of it is in flight.
//     Across machines only the owner machine's dobby of the parent runs, and a release written on another machine reaches this dobby when this machine's board is next pulled.
//   - Bound: per run at most one pass per probe interval plus one per write of the child's status file, and at most one in flight.
//     A run counts as in flight for the loop's backoff while the planner could give it a pass on the next probe tick.
//
// The board tidy:
//
//   - Each tick reads the batten history of every run whose parent worktree is this dobby's, whatever the run's state, and records each Land Done as counted in the state record, so a landing a hand pass or a pass a dead dobby left made is counted and a restart counts none twice.
//     A record read for the first time, or rewritten fresh over one that did not decode, counts every landing already there as seen, so run records left from before the dobby trigger no tidy.
//     While a tidy agent is in flight nothing is counted, so the record's slugs since the last tidy are exactly the agent's.
//   - At every `board_tidy_every` landings, 0 meaning never, dobby spawns the board-tidy agent unless one is in flight: a fresh shuttle run in a window of the worktree's reed session, role BoardTidyRole, its prompt TidyPrompt over the landed slugs and a parent directive naming the nearest live orch, its one output the report under the marker directory.
//     The agent in flight is kept in the state record, so a restart or a re-exec spawns no second.
//   - Once the report exists dobby posts its path to the nearest live orch and ends the agent through shuttle's stop verb.
//     Past `board_tidy_timeout_min` it ends the agent the same way, posts a no-report line naming the slugs, and keeps them in the state record for the next tidy.
//   - Bound: a fresh record loses a tidy in flight and the slugs landed since the last tidy, and the next tidy may spawn while the old agent runs until its timeout.
//     A landing whose run records `lyx batten cleanup --apply` removed before dobby read them is not counted.
//
// Told-geometry tier: the package takes every absolute path from its caller and imports neither internal/lyxcwd nor any CLI package, per PATTERN-cwd-resolution.
// Its seam_enforcement_test.go enforces that membership mechanically.
package dobbyengine
