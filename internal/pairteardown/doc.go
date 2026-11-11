// Package pairteardown is the one sequence that ends a pair.
// It waits for the pair's loom driver to go quiet and probes for removal refusals.
// It then kills the pair's step loop, ends the pair's reed session, and removes the pair.
// `lym fabric remove` calls Teardown.Run.
// Batten's Worktree-Teardown row calls its phases, Teardown.EndSession and Teardown.RemovePair.
// Every route shares one sequence and cannot drift.
//
// The sequence lives in its own package because reedengine imports fabricengine:
// fabric cannot end a session itself without an import cycle.
// The package sits outside the Fabric Vocabulary owner set,
// so it says "pair" and "task worktree" and passes fabricengine.RemoveResult through whole.
//
// # Sequence
//
// EndSession runs these steps in order and stops at the first failure:
//
//  1. Quiet wait, or the skip.
//     It polls until the pair's driver is quiet, bounded by Request.QuietWait.
//     With the task worktree present, a pair with nothing left to do skips the wait and every driver check.
//     Nothing left to do is read in this order, stopping at the first reading that holds:
//     an ended or owned_elsewhere Request.Verdict;
//     a live pause entry covering the slug;
//     a local board entry that reads done;
//     a Finalize Done in the task worktree's status history with no live registered batten.
//     Batten still runs Land and deploy after Finalize Done, hence the live-batten condition.
//     A read error is returned with nothing ended.
//     A skipped pair waits for its run lock first, before the refusal probe and the owned-process end.
//     The wait is bounded by runLockWait, so a step doing local work under a pause finishes before its loop is ended.
//     A holder still there at the bound is the newest unfinished step's pid.
//     Failing that it is the slug's registered driver, or unknown.
//     The holder passes, and the owned-process end and the loop end reach it, in two cases:
//     a live waiting marker of the slug names the holder's own pid, so it waits at the network gate;
//     or a pause covers the slug and no parent lock record names it, so it is a step waiting on its interrupted agent.
//     The paused pass applies to a known and an unknown holder alike.
//     Any other holder is an ErrDriverBusy refusal with nothing ended.
//     Under a pause with a parent lock record naming the slug, the refusal names resuming the run.
//     Resuming lets the step finish its merge:
//     `lym tender resume` with no slug, which lifts every live entry of the hub, when a hub entry covers the slug;
//     `lym tender resume <slug>` when slug entries alone do.
//     Unpaused, the refusal names `lym tender pause <slug>` and then running the removal again.
//     It omits the pause step when a parent lock record names the slug;
//     a pause would turn the next attempt into the paused refusal.
//  2. Refusal probe.
//     Topology.RemoveRefusal answers whether Remove would refuse, with nothing touched.
//     A refusal is returned unchanged;
//     a refused removal leaves the session and its strands live and ends no registered process.
//  3. Owned-process end.
//     runproc.End ends every process registered under the pair's slug and the processes below them.
//     Those include batten, the driver, the step loop and the pair's pushes.
//     It spares the calling process and its ancestors.
//     It always spares a teardown child.
//     That child holds the slug's teardown lock to run the remote steps of an earlier teardown.
//     It runs for a present or a gone task worktree,
//     so neither batten's strand revival nor the driver's re-step races what follows.
//     A *runproc.SurvivorsError with Request.RefuseWhenBusy returns an ErrDriverBusy refusal;
//     the refusal names each survivor's pid and role, and no loop end or session end follows.
//     Without RefuseWhenBusy the survivors are logged at Warn, set on SessionResult.Survivors, and the sequence goes on.
//     Any other error from the end is returned for every caller.
//     With the task worktree present the driver lock is then probed:
//     a lock still held means a process the owner record does not reach, such as one an older binary started.
//     With RefuseWhenBusy that is an ErrDriverBusy refusal.
//     On Linux its way forward is `lym tender status` and `ps`.
//     On Windows, where the end signals nothing, it is the driver's recorded pid with `taskkill /PID <pid> /T /F`;
//     with no driver record it is the lock path and Task Manager.
//     Without RefuseWhenBusy SessionResult.DriverWasLive is set and the sequence goes on.
//     The pair's drift capture job is then terminated by name, for a present or a gone task worktree.
//     On Windows the owner-record end signals nothing and only the detached capture child holds that job;
//     on Linux the capture is a registered process the owner-record end already ended, so the call ends nothing.
//     An error from the termination is returned with no loop lock taken.
//  4. Loop end.
//     With the task worktree present, the pair's detached step loop and its in-flight step tree are killed.
//     The loop lock is then taken and held through the session end.
//     Taking the lock replaces the loop's pid file with the teardown's mark.
//     The mark bars every later loop up to the removal, so no step starts once the session end begins.
//     A loop spawned between the kill and the take is killed again and the take retried, for at most runLockWait.
//  5. Run-lock wait.
//     It waits for at most runLockWait for the run lock to be released.
//     The session thus ends only after the in-flight step is dead.
//     A spent bound names the pid of the run's newest unfinished step as the holder, or an unknown holder.
//     A skipped pair has already waited once before the owned-process end, and this wait repeats after the loop end.
//  6. Session end.
//     With the task worktree present it is Engine.Down.
//     Engine.Down also tears down the hub's tmux server after the last session.
//     With the task worktree gone it is reedengine.EndSessionByName, so no path under the gone worktree is recreated.
//     The loop lock is released after it;
//     a failure before the session end puts the pid file back first, or removes the mark when there was no record.
//
// RemovePair then archives the pair's logs and calls Topology.Remove.
// It repeats the probe's checks.
// A refusal raised only by Remove therefore means the pair changed between probe and removal;
// the session is already ended.
// Request.ClearStray makes RemovePair delete a directory that is no worktree at the task worktree's path.
// It applies once the removal left the pair gone, through Topology.RemoveStrayTaskPath.
// Such a directory is one a build or language server refilled after the removal.
// A delete that fails returns an error wrapping fabricengine.ErrStrayPathKept;
// one that removed a directory sets StrayPathDeleted on the returned result.
//
// # Pull request
//
// A pair torn down unlanded leaves no open pull request that looks like pending work.
// RemovePair closes the run's pull request first, after the snapshot and before the archive and the removal,
// so the record under the run's scratch directory is still there.
// Every removing route goes through RemovePair, so each closes it.
// EndForCancel closes it too, because a cancel ends the run for good.
// It does so after the strand stop and before the session end with the task worktree present.
// A recorded pull request GitHub reports open is closed with one comment saying the pair was torn down before it landed;
// a merged or closed one is left as it is, so a landed run's merged pull request stays.
// A task worktree already gone has no record to read.
// It calls nothing and logs a Warn naming the slug and the task branch.
// The Warn says a pull request from that branch may stay open, if the run opened one.
// Every failure logs a Warn naming the pull request's URL where it is known, and never stops the teardown.
// Failures include an unreadable record, the client, the status read and the close.
//
// # Run mail
//
// After a successful removal, RemovePair drops the hub's unread creel mail tagged with the removed pair's slug.
// The slug is the mail's run tag.
// So a notice about the gone run never reaches an orch as live.
// It drops through the process's run mail dropper, which `InstallRunMailDropper` sets;
// the package cannot reach creel's client itself, since that client's tests import this package.
// With no dropper installed nothing is dropped.
// A failed drop logs a Warn and never fails the teardown.
//
// # Log archive
//
// RemovePair copies the pair's trace logs and its `verify-<n>.log` files into the hub's log archive.
// The archive is `fabricengine.LogArchiveDir`;
// the copy runs before the removal, after the session end and the run-lock wait.
// Each teardown writes one timestamped entry under the pair's slug, with `trace` and `verify` subdirectories.
// It writes nothing when the pair holds neither.
// The archive copies and never moves, and it never fails or delays the teardown:
// a failure logs a Warn and the removal goes on.
// Each teardown first prunes the archive.
// It removes every entry older than 30 days and each slug directory that leaves empty;
// a path that does not parse as an archive entry is left alone.
//
// # Board claim
//
// After a successful removal, RemovePair settles the board entry of the removed pair's slug,
// so no route leaves a run's claim behind.
// Whether the work landed is read before the removal, since the task worktree's status file goes with it.
// The answer is the request's Landed flag, or a Finalize with outcome Done in that status file's history.
// A task worktree already gone is not landed, and a failed read of the answer leaves the board alone.
// An entry with no status, a done status or a status without the run form is kept.
// A landed run's entry is marked done.
// Otherwise the entry is settled from the batten run's state:
// a run batten never seeded, or one that is paused, blocked, failed or awaiting, clears it;
// a done run marks it done;
// a running one holds it, and the settle only warns, naming `lym batten status <slug>` and `lym batten pause <slug>`.
// A running record holds the claim that way only while the run has a live lifecycle owner.
// That is a registered batten process of the slug.
// Between two passes it is a live dobby registered under the run's parent:
// the parent beam's slug, or the hub owner for the prime.
// That gap counts only while batten's status reads running and no cancel marker of the slug stands.
// It is the gap dobby drives.
// Every reader of that liveness reads the gap between passes as alive, and a halted status as settled.
// With no owner alive the claim is still kept, and the warning names `lym batten cancel <slug>` instead.
// That ends the run and releases the claim, since batten's own pause has no process to reach.
// A board or status read failure warns and writes nothing, and a failed removal settles nothing.
// A removal with Request.LeaveClaim set, in origin mode, reads no landed answer and settles no claim.
// The claim is left to the caller, which clears it under its own locks.
// A run whose Finalize merged but failed its done mark and stopped on its push holds no Finalize Done.
// Its entry ends cleared, for the operator to mark done with `lym board set-status`.
//
// # Batten follow-up
//
// A removal leaves the batten run record of the pair's slug standing,
// and the next batten run of the slug then meets a pair that is gone.
// BattenFollowUp words the step that settles that record.
// It reads the record and whether an owner pause covers the slug.
// A record before Land names the batten cancel that clears it.
// A record at Land names the batten run, never a cancel, since the run's work already reached its parent.
// The failed stop of that run words the restore of each branch the removal took.
// A slug with no record, a record past its Land row and a slug an owner pause covers give no follow-up.
// Teardown.Run sets Result.Warning to it, behind the slug, after a successful removal of a request without LeaveClaim.
// A cancel's own removal settles the record itself.
// A failed read of the record or of the owner pause is logged and adds no warning.
// A refusal or stopped reason elsewhere that quotes `lym fabric remove` ends in the same text.
// It reads the follow-up through ReadBattenRecord, OwnerPauseHeld and FollowUpFor.
//
// # Local mode
//
// Request.Local removes the pair through Topology.RemoveLocal:
// no origin step, and no board read or write, so the claim settlement is skipped whole.
// A busy teardown lock or records write lock surfaces from RemovePair as fabricengine.ErrPairBusy.
// A local-mode caller passes RefuseWhenBusy with a zero QuietWait,
// so a busy driver is ErrDriverBusy at once with nothing ended, unless its Verdict skips the driver checks.
//
// Request.Snapshot is the force case.
// It keeps the uncommitted paths of the pair's task and records worktrees under a local snapshot tag before the removal.
// It applies in local and origin mode alike.
// RemovePair takes it first, after EndSession ended the driver and before the teardown lock is tried.
// It logs the tag and paths and sets them on the result's SnapshotTag and SnapshotPaths.
// The snapshot's record goes ahead of the removal's.
// Request.AfterSnapshot then runs, and the removal follows.
// A snapshot refused busy returns that error with nothing removed and AfterSnapshot not run,
// so a re-run still qualifies.
// An AfterSnapshot error returns with nothing removed and the snapshot tag kept,
// so a re-run takes the earlier-snapshot path.
// The force case passes Force and no RefuseWhenBusy, as batten's own teardown does.
//
// Every refusal and each EndSession wait stay in local mode:
// the loop lock's seizure and the run lock's release are each bounded by runLockWait.
//
// # Cancel phase
//
// EndForCancel serves `lym batten cancel`, the one caller, from the prime and after its own refusals.
// It ends the run's owned processes and strands first and waits for the run lock after.
// A cancel ends the holder instead of waiting for it,
// so it skips the quiet wait, the removal refusal probe and the pre-end run-lock wait.
// It reads whether the task worktree is gone once, then runs these steps in order and stops at the first error:
//
//  1. Owned-process end.
//     The prime's records write lock is held, and so is the pair's while the pair's records worktree stands.
//     Each is bounded by CancelLockWait, and the registered processes are ended under them;
//     the locks are released as soon as the end returns.
//     A lock still held at the bound refuses with ErrDriverBusy naming its path.
//     The prime's lock is shared by the hub, so its holder is often a healthy commit of another run or the orch.
//     Survivors refuse with ErrDriverBusy naming each pid and role.
//  2. Driver-lock probe, task worktree present only, and the caller's hook.
//     A held driver lock refuses.
//     CancelRequest.AfterEnd then takes the caller's own locks.
//     A hook that reports one busy gets steps 1 and 2 once more and then the hook again;
//     a lock still busy then refuses.
//  3. Drift capture job termination.
//     The pair's drift capture job is terminated by name, as the session end does,
//     so a detached capture child ends with the cancel.
//  4. Strand stop, task worktree present only.
//     Every live strand of the task worktree's reed directory is stopped through shuttle's stop verb.
//     With the task worktree gone there is no reed directory to read, and step 7's end by name ends the strands' panes.
//  5. Loop end, task worktree present only.
//     The loop and its step tree are killed and the loop lock is taken.
//  6. Run-lock wait, task worktree present only.
//     It is bounded by CancelLockWait; a spent bound puts the loop's pid file back.
//  7. Session end.
//     It is Engine.Down with the task worktree present and reedengine.EndSessionByName once it is gone.
//  8. Loop-lock release.
//     With CancelRequest.Keep the teardown's mark is withdrawn, so the kept pair holds no mark;
//     without it the mark stays for the removal that follows.
//
// Every refusal names `lym batten cancel <slug>` as the way to run the phase again.
// The removal that follows a cancel without Keep is RemovePair with Request.LeaveClaim.
// That leaves the board claim to the cancel.
//
// # Origin seams
//
// `Teardown.OriginSeams` builds the seams the origin reconcile takes by injection,
// so every call site's CLI package reads a live pair and removes an ended entry's pair the same way.
// `LivePair` is the negation of the quiet rule's probe.
// A slug a live hub pause entry covers reads not live instead, since its driver is interrupted.
// `SeamOptions.HubPaused` is asked at each `LivePair` call,
// so a pause lifted while the reconcile waited at the network gate reads lifted and the resumed driver reads live.
// `Teardown` runs the local mode with RefuseWhenBusy under the ended verdict, which skips the driver checks.
// A driver lock or run lock still held after the owned processes are ended comes back as the report's busy flag.
// It is never an error, and nothing is removed.
// The force case is a live wake-written pair pause entry (`SeamOptions.WakeHeld`) or an earlier snapshot tag.
// It passes Force and Snapshot and no RefuseWhenBusy, so a busy driver is ended.
// `SeamOptions.LiftWakeHold` runs as AfterSnapshot, lifting the hold only once the snapshot is kept.
// `OwnerReader` builds the owner seam, `OwnerOf`, beside the live-pair, parent-lock and teardown seams.
// `OwnerOf` reads the machine's name and parses a `board.json` with `boardengine.ParseOwners`.
// It takes the slug's effective owner and its adopt subject with a parent resolver.
// It reports whether another machine owns it.
// A name error, an empty hostname included, a parent resolver error and undecodable bytes are returned,
// so no owner is compared with an empty name.
// `SeamOptions.Machine` and `SeamOptions.ParentOf` feed it.
// A nil `Machine` builds no `OwnerOf`, so a call site that sets none keeps every entry owned here.
// `HubSeamOptions` fills `HubPaused` from a read of the pause record at each call.
// It fills `PrimeParentLockFree` from one listing of the parent lock store for the prime's branch.
// It fills `Machine` with `machinename.Name` and `ParentOf` with `hubgeom.ParentOf` over the prime's location.
// It leaves the wake-hold fields nil, which read as no wake hold.
//
// # Quiet rule
//
// A pair is quiet when its run lock and its loop lock are free,
// and its driver strand is absent, dead, retiring or parked.
// A held loop lock reads as busy,
// and the refusal names the live loop and the way forward of pausing the run or retrying once it stops.
// The refusal for a busy driver names the slug's live owned processes by role and pid.
// Its way forward is retrying once the driver has written its report.
// The alternative is `lym tender pause <slug>` and then the removal again, which the skip lets through.
// A pair whose task worktree is gone is quiet by exception:
// neither the run lock nor reed's strand table can be probed there, and the session is ended by exact name with no wait.
// "Gone" means the task worktree path is not a registered linked worktree.
//
// # Ending before a bare session end
//
// EndBeforeSessionDown serves `lym reed down`, which ends a session with no teardown.
// It reads only local state and makes no network call, in this order and stopping at the first that holds:
// a live pause entry covers the slug;
// the local board entry reads done;
// the status history holds a Finalize Done with no registered batten alive;
// or the run is not running.
// Not running is no Go-driver lock held, the loop lock free, no live driver strand and no registered batten alive;
// there is no stopped reading, since that is a reconcile verdict.
// It then waits for the run lock as a skipped teardown does, with the retry step `lym reed down`.
// Then it ends the slug's registered processes with the dobby role spared:
// a beam's dobby runs from the hub root outside the session, and its member runs still need it, so it stays live;
// a teardown's own end ends a landed beam's dobby.
// A run that holds none of the readings is running and unpaused.
// It is refused with an error wrapping ErrRunRunning, with nothing ended.
// The error names `lym tender pause <slug>` and then `lym reed down` again.
// A refusal of the run-lock wait is returned as it is.
// Survivors of the end are returned wrapped, each named by pid.
//
// # Caller policies
//
// `fabric remove` passes RemoveQuietWait with Request.RefuseWhenBusy:
// a driver still busy when the bound is spent is never ended, and EndSession returns ErrDriverBusy.
// The bound keeps the operator path from killing a working driver.
// It also keeps an unbounded wait from hanging the hub's land step.
// `--force` answers dirtiness only, never the wait.
// Batten passes a zero wait without RefuseWhenBusy, since it already waited its own driver exit grace:
// a driver still busy is ended anyway, and SessionResult.DriverWasLive reports it.
// A parked driver is ended without refusal, since its park command already committed its records.
//
// ErrPairNotFound is the done post-condition of both phases:
// the refusal probe raises it once nothing of the pair remains, and so does RemovePair.
// A teardown re-entered after a completed removal therefore reports done instead of halting at session shutdown,
// and one re-entered after an interruption relies on Remove finishing a half-removed pair instead of refusing it.
package pairteardown
