// Package pairteardown is the one sequence that ends a pair: it waits for the pair's loom driver to go quiet, probes for removal refusals, ends the pair's reed session, and only then removes the pair.
// `lyx fabric remove` calls Teardown.Run, and batten's Worktree-Teardown row calls the two phases, Teardown.EndSession and Teardown.RemovePair, so the two callers cannot drift.
//
// The sequence lives in its own package because reedengine imports fabricengine: fabric cannot end a session itself without an import cycle.
// The package sits outside the Fabric Vocabulary owner set, so it says "pair" and "task worktree" and passes fabricengine.RemoveResult through whole.
//
// # Sequence
//
// EndSession runs three steps in order and stops at the first failure:
//
//  1. Quiet wait.
//     It polls until the pair's driver is quiet, bounded by Request.QuietWait.
//  2. Refusal probe.
//     Topology.RemoveRefusal answers whether Remove would refuse, with nothing touched.
//     A refusal is returned unchanged, so a refused removal leaves the session and its strands live.
//  3. Session end.
//     With the task worktree present it is Engine.Down, which also tears down the hub's tmux server after the last session.
//     With the task worktree gone it is reedengine.EndSessionByName, so no path under the gone worktree is recreated.
//
// RemovePair then calls Topology.Remove.
// It repeats the probe's checks, so a refusal raised only by Remove means the pair changed between probe and removal, and the session is already ended.
//
// # Quiet rule
//
// A pair is quiet when its run lock is free and its driver strand is absent, dead, retiring or parked.
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
