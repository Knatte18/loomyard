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
// RemovePair then archives the pair's logs and calls Topology.Remove.
// It repeats the probe's checks, so a refusal raised only by Remove means the pair changed between probe and removal, and the session is already ended.
//
// # Log archive
//
// Before the removal, and so after the session end and the run-lock wait, RemovePair copies the pair's trace logs and its `verify-<n>.log` files into the hub's log archive, `fabricengine.LogArchiveDir`.
// Each teardown writes one timestamped entry under the pair's slug, with `trace` and `verify` subdirectories, and writes nothing when the pair holds neither.
// The archive copies and never moves, and it never fails or delays the teardown: a failure logs a Warn and the removal goes on.
// Each teardown first prunes the archive, removing every entry older than 30 days and each slug directory that leaves empty; a path that does not parse as an archive entry is left alone.
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
// # Local mode
//
// Request.Local removes the pair through Topology.RemoveLocal: no origin step, and no board read or write, so the claim settlement is skipped whole.
// A busy teardown lock or records write lock surfaces from RemovePair as fabricengine.ErrPairBusy.
// A local-mode caller passes RefuseWhenBusy with a zero QuietWait, so a busy driver is ErrDriverBusy at once with nothing ended.
//
// Request.Snapshot, the force case, keeps the pair's task and records worktrees' uncommitted paths under a local snapshot tag before the removal.
// RemovePair takes it first, after EndSession ended the driver and before the teardown lock is tried, logs its tag and paths, sets them on the result's SnapshotTag and SnapshotPaths, and puts its record ahead of the removal's.
// Request.AfterSnapshot then runs, and the removal follows.
// A snapshot refused busy returns that error with nothing removed and AfterSnapshot not run, so a re-run still qualifies;
// an AfterSnapshot error returns with nothing removed and the snapshot tag kept, so a re-run takes the earlier-snapshot path.
// The force case passes Force and no RefuseWhenBusy, as batten's own teardown does.
//
// Every refusal and both EndSession waits stay in local mode: the loop lock's seizure and the run lock's release are each bounded by runLockWait.
//
// # Origin seams
//
// `Teardown.OriginSeams` builds the seams the origin reconcile takes by injection, so every call site's CLI package reads a live pair and removes an ended entry's pair the same way.
// `LivePair` is the negation of the quiet rule's probe, except that a slug a live hub pause entry covers reads not live, its driver being interrupted.
// `SeamOptions.HubPaused` is asked at each `LivePair` call, so a pause lifted while the reconcile waited at the network gate reads lifted and the resumed driver reads live.
// `Teardown` runs the local mode with a zero quiet wait and RefuseWhenBusy, and a busy driver or a held lock comes back as the report's busy flag, never as an error, with nothing removed.
// The force case, a live wake-written pair pause entry (`SeamOptions.WakeHeld`) or an earlier snapshot tag, passes Force and Snapshot and no RefuseWhenBusy, so a busy driver is ended;
// `SeamOptions.LiftWakeHold` runs as AfterSnapshot, lifting the hold only once the snapshot is kept.
// `OwnerReader` builds the owner seam, `OwnerOf`, beside the live-pair, parent-lock and teardown seams: it reads the machine's name, parses a `board.json` with `boardengine.ParseOwners`, takes the slug's effective owner with a parent resolver and reports whether another machine owns it.
// A name error, an empty hostname included, a parent resolver error and undecodable bytes are returned, so no owner is compared with an empty name.
// `SeamOptions.Machine` and `SeamOptions.ParentOf` feed it, and a nil `Machine` builds no `OwnerOf`, so a call site that sets none keeps every entry owned here.
// `HubSeamOptions` fills `HubPaused` from a read of the pause record at each call, `PrimeParentLockFree` from one listing of the parent lock store for the prime's branch, `Machine` with `machinename.Name` and `ParentOf` with `fabricengine.ParentSlug` over the prime's location, and leaves the two wake-hold fields nil, which read as no wake hold.
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
