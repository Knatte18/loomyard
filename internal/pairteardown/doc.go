// Package pairteardown is the one sequence that ends a pair: it waits for the pair's loom driver to go quiet, probes for removal refusals, kills the pair's step loop, ends the pair's reed session, and only then removes the pair.
// `lyx fabric remove` calls Teardown.Run, and batten's Worktree-Teardown row calls the two phases, Teardown.EndSession and Teardown.RemovePair, so the two callers cannot drift.
//
// The sequence lives in its own package because reedengine imports fabricengine: fabric cannot end a session itself without an import cycle.
// The package sits outside the Fabric Vocabulary owner set, so it says "pair" and "task worktree" and passes fabricengine.RemoveResult through whole.
//
// # Sequence
//
// EndSession runs five steps in order and stops at the first failure:
//
//  1. Quiet wait.
//     It polls until the pair's driver is quiet, bounded by Request.QuietWait.
//  2. Refusal probe.
//     Topology.RemoveRefusal answers whether Remove would refuse, with nothing touched.
//     A refusal is returned unchanged, so a refused removal leaves the session and its strands live.
//  3. Loop end.
//     With the task worktree present, the pair's detached step loop and its in-flight step tree are killed, and the loop lock is taken and held through the session end.
//     Taking the lock replaces the loop's pid file with the teardown's mark, which bars every later loop up to the removal, so no step starts once the session end begins.
//     A loop spawned between the kill and the take is killed again and the take retried, for at most runLockWait.
//  4. Run-lock wait.
//     It waits, for at most runLockWait, for the run lock to be released, so the session ends only after the in-flight step is dead.
//     A spent bound names the pid of the run's newest unfinished step as the holder, or an unknown holder.
//  5. Session end.
//     With the task worktree present it is Engine.Down, which also tears down the hub's tmux server after the last session.
//     With the task worktree gone it is reedengine.EndSessionByName, so no path under the gone worktree is recreated.
//     The loop lock is released after it; a failure before the session end puts the pid file back first, or removes the mark when there was no record.
//
// RemovePair then calls Topology.Remove.
// It repeats the probe's checks, so a refusal raised only by Remove means the pair changed between probe and removal, and the session is already ended.
//
// # Board claim
//
// After a successful removal, RemovePair settles the board entry of the removed pair's slug, so no route leaves a run's claim behind.
// Whether the work landed is read before the removal, since the task worktree's status file goes with it: the request's Landed flag, or a Finalize with outcome Done in that status file's history.
// A task worktree already gone is not landed, and a failed read of the answer leaves the board alone.
// An entry with no status, a done status or a status without the run form is kept.
// A landed run's entry is marked done.
// Otherwise an entry whose run batten never seeded, or whose batten run is paused, blocked, failed or awaiting, is cleared; a batten run that is done marks it done; a running one holds it, and the settle only warns, naming `lyx batten status <slug>` and `lyx batten pause <slug>`.
// A board or status read failure warns and writes nothing, and a failed removal settles nothing.
// A run whose Finalize merged but failed its done mark and stopped on its push holds no Finalize Done, so its entry ends cleared, for the operator to mark done with `lyx board set-status`.
//
// # Quiet rule
//
// A pair is quiet when its run lock and its loop lock are free and its driver strand is absent, dead, retiring or parked.
// A held loop lock reads as busy, and the refusal names the live loop and the way forward of pausing the run or retrying once it stops.
// A pair whose task worktree is gone is quiet by exception: neither the run lock nor reed's strand table can be probed there, and the session is ended by exact name with no wait.
// "Gone" means the task worktree path is not a registered linked worktree.
//
// # Caller policies
//
// `fabric remove` passes RemoveQuietWait with Request.RefuseWhenBusy: a driver still busy when the bound is spent is never ended, and EndSession returns ErrDriverBusy.
// The bound keeps the operator path from killing a working driver without letting an unbounded wait hang the hub's land step; `--force` answers dirtiness only, never the wait.
// Batten passes a zero wait without RefuseWhenBusy, since it already waited its own driver exit grace: a driver still busy is ended anyway, and SessionResult.DriverWasLive reports it.
// A parked driver is ended without refusal, since its park command already committed its records.
//
// ErrPairNotFound is the done post-condition of both phases: the refusal probe raises it once nothing of the pair remains, and so does RemovePair.
// A teardown re-entered after a completed removal therefore reports done instead of halting at session shutdown,
// and one re-entered after an interruption relies on Remove finishing a half-removed pair instead of refusing it.
package pairteardown
