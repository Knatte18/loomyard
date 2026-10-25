// Package pairteardown is the one sequence that ends a pair: it waits for the pair's loom driver to go quiet, probes for removal refusals, kills the pair's step loop, ends the pair's reed session, and only then removes the pair.
// `lyx fabric remove` calls Teardown.Run, and batten's Worktree-Teardown row calls the two phases, Teardown.EndSession and Teardown.RemovePair, so the two callers cannot drift.
//
// The sequence lives in its own package because reedengine imports fabricengine: fabric cannot end a session itself without an import cycle.
// The package sits outside the Fabric Vocabulary owner set, so it says "pair" and "task worktree" and passes fabricengine.RemoveResult through whole.
//
// # Sequence
//
// EndSession runs six steps in order and stops at the first failure:
//
//  1. Quiet wait, or the skip.
//     It polls until the pair's driver is quiet, bounded by Request.QuietWait.
//     With the task worktree present the wait is skipped, and no driver check is made, for a pair with nothing left to do.
//     That is read, in this order and stopping at the first that holds, as an ended or owned_elsewhere Request.Verdict, a live pause entry covering the slug, a local board entry that reads done, and a Finalize Done in the task worktree's status history with no live registered batten, since batten still runs Land and deploy after Finalize Done.
//     A read error is returned with nothing ended.
//     A skipped pair instead waits for its run lock first, before the refusal probe and the owned-process end, bounded by runLockWait, so a step doing local work under a pause finishes before its loop is ended.
//     A holder still there at the bound is the newest unfinished step's pid, or else the slug's registered driver, or unknown.
//     It passes, and the owned-process end and the loop end reach it, when a live waiting marker of the slug names the holder's own pid, a holder waiting at the network gate,
//     or when a pause covers the slug and no parent lock record names it, a step waiting on its interrupted agent; the paused pass applies to a known and an unknown holder alike.
//     Any other holder is an ErrDriverBusy refusal with nothing ended.
//     Under a pause with a parent lock record naming the slug, the refusal names resuming the run so the step finishes its merge: `lyx tender resume` with no slug, lifting every live entry of the hub, when a hub entry covers the slug, and `lyx tender resume <slug>` when slug entries alone do.
//     Unpaused it names `lyx tender pause <slug>` and then running the removal again, with no pause step when a parent lock record names the slug, since a pause would turn the next attempt into the paused refusal.
//  2. Refusal probe.
//     Topology.RemoveRefusal answers whether Remove would refuse, with nothing touched.
//     A refusal is returned unchanged, so a refused removal leaves the session and its strands live and ends no registered process.
//  3. Owned-process end.
//     runproc.End ends every process registered under the pair's slug, batten, the driver, the step loop and the pair's pushes among them, and the processes below them, sparing the calling process and its ancestors.
//     It runs for a present or a gone task worktree, so neither batten's strand revival nor the driver's re-step races what follows.
//     A *runproc.SurvivorsError with Request.RefuseWhenBusy returns an ErrDriverBusy refusal naming each survivor's pid and role, with no loop end and no session end;
//     without RefuseWhenBusy the survivors are logged at Warn, set on SessionResult.Survivors, and the sequence goes on.
//     Any other error from the end is returned for every caller.
//     With the task worktree present the driver lock is then probed: a lock still held means a process the owner record does not reach, such as one an older binary started.
//     With RefuseWhenBusy that is an ErrDriverBusy refusal whose way forward is `lyx tender status` and `ps` on Linux, and on Windows, where the end signals nothing, the driver's recorded pid with `taskkill /PID <pid> /T /F` or, with no driver record, the lock path and Task Manager;
//     without it SessionResult.DriverWasLive is set and the sequence goes on.
//  4. Loop end.
//     With the task worktree present, the pair's detached step loop and its in-flight step tree are killed, and the loop lock is taken and held through the session end.
//     Taking the lock replaces the loop's pid file with the teardown's mark, which bars every later loop up to the removal, so no step starts once the session end begins.
//     A loop spawned between the kill and the take is killed again and the take retried, for at most runLockWait.
//  5. Run-lock wait.
//     It waits, for at most runLockWait, for the run lock to be released, so the session ends only after the in-flight step is dead.
//     A spent bound names the pid of the run's newest unfinished step as the holder, or an unknown holder.
//     A skipped pair has already waited once before the owned-process end, and this wait repeats after the loop end.
//  6. Session end.
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
// A running record holds the claim that way only while the run has a live lifecycle owner:
// a registered batten process of the slug, or, between two passes, a live dobby registered under the run's parent (the parent beam's slug, or the hub owner for the prime) with batten's status reading running and no cancel marker of the slug standing, which is the gap dobby drives.
// Every reader of that liveness, the claim settlement, the session end and the driver checks, reads the gap between passes as alive, and a halted status as settled.
// With no owner alive the claim is kept all the same and the warning names `lyx batten cancel <slug>` instead, to end the run and release the claim, since batten's own pause has no process to reach.
// A board or status read failure warns and writes nothing, and a failed removal settles nothing.
// A removal with Request.LeaveClaim set, in origin mode, reads no landed answer and settles no claim, leaving the claim to the caller, which clears it under its own locks.
// A run whose Finalize merged but failed its done mark and stopped on its push holds no Finalize Done, so its entry ends cleared, for the operator to mark done with `lyx board set-status`.
//
// # Local mode
//
// Request.Local removes the pair through Topology.RemoveLocal: no origin step, and no board read or write, so the claim settlement is skipped whole.
// A busy teardown lock or records write lock surfaces from RemovePair as fabricengine.ErrPairBusy.
// A local-mode caller passes RefuseWhenBusy with a zero QuietWait, so a busy driver is ErrDriverBusy at once with nothing ended, unless its Verdict skips the driver checks.
//
// Request.Snapshot, the force case, keeps the pair's task and records worktrees' uncommitted paths under a local snapshot tag before the removal, in local and origin mode alike.
// RemovePair takes it first, after EndSession ended the driver and before the teardown lock is tried, logs its tag and paths, sets them on the result's SnapshotTag and SnapshotPaths, and puts its record ahead of the removal's.
// Request.AfterSnapshot then runs, and the removal follows.
// A snapshot refused busy returns that error with nothing removed and AfterSnapshot not run, so a re-run still qualifies;
// an AfterSnapshot error returns with nothing removed and the snapshot tag kept, so a re-run takes the earlier-snapshot path.
// The force case passes Force and no RefuseWhenBusy, as batten's own teardown does.
//
// Every refusal and both EndSession waits stay in local mode: the loop lock's seizure and the run lock's release are each bounded by runLockWait.
//
// # Cancel phase
//
// EndForCancel serves `lyx batten cancel`, the one caller, from the prime and after its own refusals.
// It ends the run's owned processes and strands first and waits for the run lock after, since a cancel ends the holder instead of waiting for it,
// and it skips the quiet wait, the removal refusal probe and the pre-end run-lock wait.
// It reads whether the task worktree is gone once, then runs these steps in order and stops at the first error:
//
//  1. Owned-process end.
//     The prime's records write lock and, while the pair's records worktree stands, the pair's are held, each bounded by CancelLockWait, and the registered processes are ended under them;
//     the locks are released as soon as the end returns.
//     A lock still held at the bound refuses with ErrDriverBusy naming its path, since the prime's lock is shared by the hub and its holder is often a healthy commit of another run or the orch.
//     Survivors refuse with ErrDriverBusy naming each pid and role.
//  2. Driver-lock probe, task worktree present only, and the caller's hook.
//     A held driver lock refuses.
//     CancelRequest.AfterEnd then takes the caller's own locks; a hook that reports one busy gets steps 1 and 2 once more and then the hook again, and a lock still busy then refuses.
//  3. Strand stop, task worktree present only.
//     Every live strand of the task worktree's reed directory is stopped through shuttle's stop verb.
//     With the task worktree gone there is no reed directory to read, and step 6's end by name ends the strands' panes.
//  4. Loop end, task worktree present only.
//     The loop and its step tree are killed and the loop lock is taken.
//  5. Run-lock wait, task worktree present only.
//     It is bounded by CancelLockWait; a spent bound puts the loop's pid file back.
//  6. Session end.
//     It is Engine.Down with the task worktree present and reedengine.EndSessionByName once it is gone.
//  7. Loop-lock release.
//     With CancelRequest.Keep the teardown's mark is withdrawn, so the kept pair holds no mark; without it the mark stays for the removal that follows.
//
// Every refusal names `lyx batten cancel <slug>` as the way to run the phase again.
// The removal that follows a cancel without Keep is RemovePair with Request.LeaveClaim, which leaves the board claim to the cancel.
//
// # Origin seams
//
// `Teardown.OriginSeams` builds the seams the origin reconcile takes by injection, so every call site's CLI package reads a live pair and removes an ended entry's pair the same way.
// `LivePair` is the negation of the quiet rule's probe, except that a slug a live hub pause entry covers reads not live, its driver being interrupted.
// `SeamOptions.HubPaused` is asked at each `LivePair` call, so a pause lifted while the reconcile waited at the network gate reads lifted and the resumed driver reads live.
// `Teardown` runs the local mode with RefuseWhenBusy under the ended verdict, which skips the driver checks, and a driver lock or run lock still held after the owned processes are ended comes back as the report's busy flag, never as an error, with nothing removed.
// The force case, a live wake-written pair pause entry (`SeamOptions.WakeHeld`) or an earlier snapshot tag, passes Force and Snapshot and no RefuseWhenBusy, so a busy driver is ended;
// `SeamOptions.LiftWakeHold` runs as AfterSnapshot, lifting the hold only once the snapshot is kept.
// `OwnerReader` builds the owner seam, `OwnerOf`, beside the live-pair, parent-lock and teardown seams: it reads the machine's name, parses a `board.json` with `boardengine.ParseOwners`, takes the slug's effective owner with a parent resolver and reports whether another machine owns it.
// A name error, an empty hostname included, a parent resolver error and undecodable bytes are returned, so no owner is compared with an empty name.
// `SeamOptions.Machine` and `SeamOptions.ParentOf` feed it, and a nil `Machine` builds no `OwnerOf`, so a call site that sets none keeps every entry owned here.
// `HubSeamOptions` fills `HubPaused` from a read of the pause record at each call, `PrimeParentLockFree` from one listing of the parent lock store for the prime's branch, `Machine` with `machinename.Name` and `ParentOf` with `hubgeom.ParentOf` over the prime's location, and leaves the two wake-hold fields nil, which read as no wake hold.
//
// # Quiet rule
//
// A pair is quiet when its run lock and its loop lock are free and its driver strand is absent, dead, retiring or parked.
// A held loop lock reads as busy, and the refusal names the live loop and the way forward of pausing the run or retrying once it stops.
// The refusal for a busy driver names the slug's live owned processes by role and pid, and its way forward is retrying once the driver has written its report, or `lyx tender pause <slug>` and then the removal again, which the skip lets through.
// A pair whose task worktree is gone is quiet by exception: neither the run lock nor reed's strand table can be probed there, and the session is ended by exact name with no wait.
// "Gone" means the task worktree path is not a registered linked worktree.
//
// # Ending before a bare session end
//
// EndBeforeSessionDown serves `lyx reed down`, which ends a session with no teardown.
// It reads only local state and makes no network call, in this order and stopping at the first that holds:
// a live pause entry covers the slug, the local board entry reads done, the status history holds a Finalize Done with no registered batten alive, or the run is not running.
// Not running is no Go-driver lock held, the loop lock free, no live driver strand and no registered batten alive; there is no stopped reading, since that is a reconcile verdict.
// It then waits for the run lock as a skipped teardown does, with the retry step `lyx reed down`, and ends the slug's registered processes with the dobby role spared:
// a beam's dobby runs from the hub root outside the session, and its member runs still need it, so it stays live, while a teardown's own end ends a landed beam's dobby.
// A run that holds none of the readings is running and unpaused, and is refused with an error wrapping ErrRunRunning that names `lyx tender pause <slug>` and then `lyx reed down` again, with nothing ended.
// A refusal of the run-lock wait is returned as it is, and survivors of the end are returned wrapped, each named by pid.
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
