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
// so it says "pair" and "task worktree" and passes fabriclayout.RemoveResult through whole.
//
// # Sequence
//
// EndSession runs these steps in order and stops at the first failure:
//
//  1. Quiet wait, bounded by Request.QuietWait.
//     With the task worktree present, a pair with nothing left to do skips the wait and every driver check.
//     Nothing left to do is an ended or owned_elsewhere Request.Verdict,
//     a live pause entry covering the slug, a local board entry that reads done,
//     or a Finalize Done in the status history with no live registered batten
//     (batten still runs Land and deploy after Finalize Done).
//     A skipped pair still waits, bounded by runLockWait, for its run lock.
//     A holder that remains is refused with ErrDriverBusy and nothing ended,
//     unless it is waiting at the network gate or is a step waiting on its interrupted agent under a pause.
//  2. Refusal probe.
//     Topology.RemoveRefusal answers whether Remove would refuse, with nothing touched.
//     A refusal is returned unchanged and ends no session, strand or registered process.
//  3. Owned-process end.
//     runproc.End ends every process registered under the pair's slug and the processes below them,
//     for a present or a gone task worktree.
//     It spares the calling process and its ancestors, the orch's provider process and a teardown child.
//     Survivors, or a driver lock still held, are an ErrDriverBusy refusal with Request.RefuseWhenBusy
//     and no loop end or session end after it.
//     Without RefuseWhenBusy they are set on SessionResult.Survivors and SessionResult.DriverWasLive,
//     and the sequence goes on.
//     The pair's drift capture job is then terminated by name.
//  4. Loop end.
//     With the task worktree present, the pair's step loop and its in-flight step tree are killed.
//     The loop lock is then held through the session end,
//     and its teardown mark bars every later loop up to the removal.
//  5. Run-lock wait, bounded by runLockWait, so the session ends only after the in-flight step is dead.
//  6. Session end.
//     With the task worktree gone it goes by name, so no path under the gone worktree is recreated.
//     A failure before the session end restores the loop's pid file.
//
// RemovePair then archives the pair's logs and calls Topology.Remove.
// A refusal raised only by Remove means the pair changed between probe and removal; the session is already ended.
// Request.ClearStray makes RemovePair delete a directory that is no worktree at the task worktree's path.
// The delete happens once the removal left the pair gone.
// A failed delete returns an error wrapping fabricengine.ErrStrayPathKept;
// a delete that removed a directory sets StrayPathDeleted on the result.
//
// # Pull request
//
// A pair torn down unlanded leaves no open pull request that looks like pending work.
// RemovePair closes the run's recorded pull request before the archive and the removal; EndForCancel does too.
// A merged or closed pull request is left as it is.
// A task worktree already gone has no record to read, so a Warn says a pull request may stay open.
// A failure logs a Warn and never stops the teardown.
//
// # Run mail
//
// After a successful removal, RemovePair drops the hub's unread creel mail tagged with the removed pair's slug.
// It drops through the dropper `InstallRunMailDropper` sets, because creel's client tests import this package.
// With no dropper installed nothing is dropped, and a failed drop never fails the teardown.
//
// # Log archive
//
// RemovePair copies the pair's trace logs and `verify-<n>.log` files into `fabriclayout.LogArchiveDir`.
// It copies them before the removal.
// It writes one timestamped entry per teardown under the pair's slug.
// The archive copies and never moves, and it never fails or delays the teardown.
// Each teardown first prunes entries older than 30 days.
//
// # Sandbox clear
//
// A hardener run builds sandbox hubs under the worktree's `lymdirs.HardenSandboxDirName` folder.
// Their processes and tmux servers would outlive a removed worktree.
// RemovePair runs `lym harden sandbox clear` in a worktree that holds that folder, before the removal.
// A failed clear keeps the worktree and returns an error naming the way forward.
//
// # Board claim
//
// After a successful removal, RemovePair settles the board entry of the removed pair's slug,
// so no route leaves a run's claim behind.
// A landed run's entry is marked done.
// Landed is the request's Landed flag or a Finalize Done in the status history, read before the removal.
// A task worktree already gone is not landed.
// Otherwise the entry is settled from the batten run's state:
// a run never seeded, or one paused, blocked, failed or awaiting, clears it; a done run marks it done.
// A running run holds the claim while it has a live lifecycle owner, and the settle only warns.
// With no owner alive the claim is still kept, and the warning names `lym batten cancel <slug>`.
// A board or status read failure warns and writes nothing, and a failed removal settles nothing.
// A removal with Request.LeaveClaim set reads no landed answer and settles no claim; the caller clears it.
// A run whose Finalize merged but failed its done mark holds no Finalize Done,
// so its entry ends cleared, for the operator to mark done with `lym board set-status`.
//
// # Batten follow-up
//
// A removal leaves the batten run record of the pair's slug standing.
// BattenFollowUp words the step that settles it:
// the batten cancel before Land, the batten run at Land, and nothing with no record or past Land.
// Teardown.Run sets Result.Warning to it after a successful removal of a request without LeaveClaim.
//
// # Run FSM inputs
//
// ReadRun reads a slug's tracked records and the act rule's machine-local inputs once, for every verb that asks the rule.
// It reads the pair's child record from the task worktree when the pair stands here;
// otherwise it reads origin's fetched records.
// It never reads a cache a fresh machine lacks.
// ReadRunPulled is ReadRun behind a pull of the hub's Bolt, so a hold another machine wrote is read.
// An unreachable origin reads the local Bolt; any other failed pull is an error and reads nothing.
//
// # Local mode
//
// Request.Local removes the pair through Topology.RemoveLocal:
// no origin step, and no board read or write, so the claim settlement is skipped whole.
// A busy teardown lock or records write lock surfaces from RemovePair as fabricengine.ErrPairBusy.
// A local-mode caller passes RefuseWhenBusy with a zero QuietWait,
// so a busy driver is ErrDriverBusy at once with nothing ended, unless its Verdict skips the driver checks.
//
// Request.Snapshot is the force case, in local and origin mode alike.
// It keeps the uncommitted paths of the pair's task and records worktrees under a local snapshot tag before the removal.
// A snapshot refused busy, or an AfterSnapshot error, returns with nothing removed so a re-run still qualifies.
// The force case passes Force and no RefuseWhenBusy.
//
// # Cancel phase
//
// EndForCancel serves `lym batten cancel`, from the prime and after its own refusals.
// A cancel ends the holder instead of waiting for it,
// so it skips the quiet wait, the removal refusal probe and the pre-end run-lock wait.
// It ends the run's owned processes under the prime's and the pair's records write locks.
// It then runs the driver-lock probe and CancelRequest.AfterEnd;
// after those it ends the drift capture job, the strands, the loop, the run lock and the session.
// Locks and waits are bounded by CancelLockWait.
// Survivors, a held lock or a held driver lock refuse with ErrDriverBusy.
// With CancelRequest.Keep the teardown's loop-lock mark is withdrawn; without it the mark stays for the removal.
// Every refusal names `lym batten cancel <slug>` as the way to run the phase again.
// The removal after a cancel without Keep is RemovePair with Request.LeaveClaim.
//
// # Origin seams
//
// `Teardown.OriginSeams` builds the seams the origin reconcile takes by injection,
// so every call site reads a live pair and removes an ended entry's pair the same way.
// `LivePair` is the negation of the quiet rule's probe.
// A slug a live hub pause entry covers reads not live, since its driver is interrupted.
// `Teardown` runs the local mode with RefuseWhenBusy under the ended verdict.
// A driver lock or run lock still held after the end comes back as the report's busy flag, never an error;
// nothing is removed.
// The force case is a live wake-written pair pause entry or an earlier snapshot tag.
// It passes Force and Snapshot and no RefuseWhenBusy, so a busy driver is ended.
// `OwnerOf` reports whether another machine owns a slug.
// It returns every identity or decode error, so no owner is compared with an empty name.
// A nil `SeamOptions.Machine` builds no `OwnerOf`, so every entry stays owned here.
// `HubSeamOptions` fills the hub's seam fields from live reads and leaves the wake-hold fields nil.
//
// # Ending a subject owned elsewhere
//
// EndSubjectProcesses ends every process this machine runs for a subject another machine now owns.
// The subject is a beam, or the hub's root for runproc.HubOwner.
// The registered runs whose adopt subject it is end first, then the subject's own processes;
// the subject's dobby ends last, only when endDobby is set.
// A run whose subject cannot be resolved is left alone and logged.
//
// # Handing a run over
//
// PushRun is the push the background pusher, the teardown and the catch-up run.
// Every such push outside fabric goes through it.
// A rejected records push that meets another machine's takeover entry for the run gets a hand-over, not a replay.
// A rejected push of the prime's records hands over every batten run taken over on origin, then pushes the rest.
// HandOver ends the processes this machine runs for the run and archives the local tips.
// It then moves the owed code tip and writes the hand-over mark.
// Unless the caller runs inside the pair, it removes the local pair; inside the pair the parent's dobby removes it.
// Its error wraps fabricengine.ErrHandedOver and names the new owner, so the caller stops.
// A failed step returns its own error before the mark is written, and the next rejected push hands the run over again.
//
// HandOverOwnedElsewhere hands over a run another machine owns, from this machine's side.
// With no takeover entry on origin's record it first pushes the prime's records and the run's pair.
// A failed push returns its error and archives nothing.
//
// # Leftover pairs
//
// RemoveHandedOverPair serves the dobby for a run this machine handed over from inside its pair.
// It ends the run's processes and the pair's reed session, then removes the pair in local mode with force.
// RemoveOrphanPair serves the dobby for a pair no board entry or batten run names.
// It rechecks that the pair is an orphan with no live process.
// It then archives the code and records tips under owner archive tags and tears the pair down.
// Neither settles a board claim or touches a batten record, and a failed archive removes nothing.
//
// RemoveBenchPairs serves a run's teardown for the bench pairs its darn writer made, slugs `<slug>-b<N>`.
// It archives and removes each the orphan way, but it ends a live process instead of refusing it.
// It leaves standing a pair whose slug a board entry or batten run names.
//
// # Quiet rule
//
// A pair is quiet when its run lock and its loop lock are free,
// and its driver strand is absent, dead, retiring or parked.
// A busy driver's refusal names the slug's live owned processes by role and pid.
// It also names the way forward:
// retry once the driver has written its report, or `lym tender pause <slug>` and remove again.
// A pair whose task worktree is gone, meaning not a registered linked worktree, is quiet by exception:
// the session is ended by exact name with no wait.
//
// # Ending before a bare session end
//
// EndBeforeSessionDown serves `lym reed down`, which ends a session with no teardown.
// It reads only local state and makes no network call.
// A run with a live pause entry, a done board entry, a Finalize Done with no live batten, or not running passes.
// It then ends the slug's registered processes, sparing the dobby, the orch and teardown children.
// The dobby is spared because a beam's dobby serves its member runs.
// A running, unpaused run is refused with an error wrapping ErrRunRunning and nothing ended.
//
// # Caller policies
//
// `fabric remove` passes RemoveQuietWait with Request.RefuseWhenBusy:
// a driver still busy when the bound is spent is never ended, and EndSession returns ErrDriverBusy.
// `--force` answers dirtiness only, never the wait.
// Batten passes a zero wait without RefuseWhenBusy, since it already waited its own driver exit grace:
// a busy driver is ended anyway, and SessionResult.DriverWasLive reports it.
//
// ErrPairNotFound is the done post-condition of both phases:
// a teardown re-entered after a completed removal reports done;
// one re-entered after an interruption relies on Remove finishing a half-removed pair.
package pairteardown
