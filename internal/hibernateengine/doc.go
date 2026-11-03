// Package hibernateengine implements the hibernate request file, the submit, cancel and status sequences behind `lym hibernate`, and the Carrier the prime's dobby carries the request out with, over seams.
//
// A hibernate request asks lym's own Go code to hibernate the machine once every session in the hub is safely paused, so no LLM session runs the hibernate and none is cut mid-call.
// The prime's dobby carries the request out; the verbs here record the request, end it and read it.
//
// Every path and every probe is told through Deps; the engine resolves no working directory, spawns nothing itself and reaches tender only through the Pause, TenderStatus and Lift seams and origin only through the Reconcile and ExemptNetwork seams.
//
// # The request file
//
// The hub holds one request, a JSON file at `Deps.RequestPath` with its lock at the same path plus `.lock`.
// It records the trigger (`now`, `after` with a slug, or `after-all`), whether the wake lifts the request's pause, the requester, the pid and start time of the daemon that carries it, the pause entry its sequence owns once it has added one, the phase and the last reason.
// The phases are waiting, pausing, confirming, hibernating, resuming, done, failed and cancelled; the last three are terminal.
// Every write holds the request lock across its read and write and keeps keys a newer binary added; a read takes no lock and a missing file reads as no request.
// A carrying daemon is live when its pid is running with the recorded start time, so a recycled pid is not taken for it.
//
// # Callers and platform
//
// An empty strand is an external caller, the orch's strand is the orch, and any other strand, a name that does not parse included, is an agent.
// Submit and Cancel refuse an agent, naming the parent session that runs the verb for it.
// Bound: the refusal guards against a mistaken call, as tender's does; the agents' settings deny any command naming the strand variable, so an agent cannot clear it to read as external.
// Submit and Cancel also refuse any platform but Linux, where `systemctl hibernate` is the machine's hibernate; Status reads on any platform.
//
// # Submit
//
// Submit checks the caller, the platform, `systemctl` on PATH, that `--after` and `--after-all` are not both given, and that an `--after` slug has a batten status; each refusal writes nothing.
// In one hold of the request lock it then refuses a pending request, whether or not a live daemon carries it, and writes a waiting request no daemon carries yet.
// A terminal request and no request are replaced the same way, except that a terminal request whose owned pause entry is still live is refused, naming "lym hibernate cancel" and "lym tender resume", so a pause its wake kept is not forgotten.
// Because the check and the write share one hold, of two racing submits one is refused.
// Submit returns at once, with whether the prime's dobby runs through `Deps.DobbyLive`, and names `lym beam start root` when it does not or its liveness does not read; the request waits on disk until that dobby takes it up.
// The request records the caller's `ResumeOnWake` as told, since the engine reads no config.
//
// # Cancel
//
// Cancel ends the pending request: no request and a terminal one succeed with nothing cancelled.
// A request a live daemon carries in the hibernating phase is refused, since the machine is going down; once the wake has ended the request, a cancel lifts a pause the wake kept.
// A request no live daemon carries is cancelled in any phase, which is the way out after a crash or a cold boot left the hub paused.
// Otherwise the request is written cancelled with the requester as the reason, and the pause entry the request owns is lifted by identity, resume lines included; Cancel stops no process.
// The carrying dobby's sequence sees the cancel at its next read of the request and ends quietly, lifting by identity the entry its pause added, which covers an entry recorded after Cancel read the request.
// A request that owns no entry lifts nothing, so a hub pause that stood before the request stays.
// A terminal request is not cancelled, but the pause entry it owns is still lifted by identity, which is the way out when its sequence left that entry live or its wake kept it as the request asked.
// Bound: a dobby that ends between its pause's record write and the write of its owned entry leaves that entry live and unowned, which `lym tender resume` lifts; the window is one file write.
//
// # Status
//
// Status reads the request and whether the daemon that carries it is live and writes nothing, so a halted `--after` run's reason and a carrier that ended show up in it.
//
// # The carrier
//
// The Carrier is what the prime's dobby runs the request with; no beam's dobby carries it.
// Each `Carrier.Tick` with no sequence of its own in flight takes up the pending request under the request lock: one no live daemon carries, and one that names the running process itself, which a re-exec of the process leaves behind with no sequence running.
// No request, a terminal one and one another live process carries are left alone, so the request is watched and carried in one place.
// Taking it up records the running process as its carrier, keeps a waiting request as it is, and sends a request in the pausing or confirming phase back to waiting with the pause entry it owns lifted, so its sequence starts again from the trigger.
// A request taken up in the hibernating or resuming phase ends failed at once with one notice, its owned entry lifted only when it resumes on wake, since the wake its sequence waited for can no longer be seen; the reason of one taken up in the resuming phase adds that its wake reconcile may not have run and that `lym cleanup --apply` runs it.
// Since the request stays on disk, a dobby that was not running or died takes it up again when it next ticks.
// A sequence's end rewrites the request file as it stands, so the watch on the file runs the next tick, which takes up a request written while a cancelled sequence was still in flight.
// The sequence then runs in a goroutine of its own, so the dobby's ticks never wait on it and a dobby tick that fails stops no sequence in flight.
// `Carrier.HoldsReexec` reports a sequence in flight past its trigger wait, which the dobby's wiring reads to skip its binary re-exec, so a re-exec never cuts a pause, a confirmation, a hibernate or a wake in the middle; the hold lasts until the sequence has lifted its entry and sent its notice, after its terminal phase is written; one still waiting for its trigger holds nothing, since the re-exec takes it up again at the same wait.
// Bound: a re-exec that passed the check in the instant the sequence moved past waiting cuts the sequence, and the takeover sends it back to waiting as above.
//
// # The sequence
//
// The sequence acts only while the request file holds a non-terminal request carrying the running process's own pid and start time.
// Every phase change is written under the request lock and stops the sequence quietly when the request is no longer carried here or has gone terminal.
// The sequence sends no key and no text of its own: every interrupt and resume line is tender's, under its guards.
// The timings are fixed constants with no config key.
//
// The trigger wait reads the trigger and writes the reason it still waits into the request, so status shows it.
// `now` fires at once.
// `after` fires once the run is past Land as `battenrecipe.PastLand` reads it, done or at the row after Land in any state; a run halted before Land keeps the wait going with the halt reason, and a status that is gone fails the request.
// `after-all` fires once no run reads running, at once with none, and skips a listed slug whose status is missing.
// The poll starts at one second, doubles to thirty seconds while the reason is unchanged and returns to one second when it changes.
// Bound: the wait has no deadline, since the operator cancels a request whose run halted.
//
// Firing pauses the hub through tender with a ten second grace.
// The pause's `Added` callback records the entry as the request's owned entry before the grace, whatever the phase, and the sequence keeps it in memory even when that write fails.
// A pause that found a live hub entry owns none, so neither the sequence nor Cancel lifts that pause.
// The pause holds sessions and the network calls of other processes; the dobby that carries the request is no session, and its sequence never waits on a pause, so the pause it adds never stops the wait.
// A pause lifted during its grace cancels the request.
// A pause error fails the request, lifts the owned entry and queues one notice; the lift and the notice run even when the failed write fails.
//
// Confirmation reads the hub right after the pause and then re-reads it five seconds apart, up to 120 times, ten minutes in all.
// A reading confirms when a hub entry is live, every session reads idle, asking, dead or busy only on background shells, no network operation is in flight and the land lock is free.
// A suspend freezes a shell and the wait loop that holds it and resumes both on wake, so a shell withholds nothing.
// A session with an outstanding background fork withholds, since a fork is never interrupted and its next batch is already held by the pause; while forks are the only reason for the latest reading, confirmation keeps re-reading up to 720 times, sixty minutes in all.
// A network operation in flight, a held land lock or any other busy session keeps the ten minute bound.
// A session whose strand is the request's requester is left out of the reading when the requester is the orch, since the orch is not interrupted by a hub pause and its writes that matter are withheld through the land lock and the network operations.
// The first reading also withholds while the pause left a strand busy or refused a session.
// The parent locks a paused run keeps by design are not read.
// A reading with no live hub entry cancels the request, and a reading error counts as unconfirmed.
// Without a confirmed reading the request fails with the last withholding reason and `Hibernate` is never called.
//
// Right before the hibernate the sequence reads the hub once more, needs a live hub entry, and moves the request to hibernating only if it is still carried here.
// The wake is a sleep the clock gap watch records ending after the hibernate call, seen within five minutes of awake time; a hibernate error or no wake fails the request with the owned entry lifted and one notice.
// What the wake does with the owned entry is the request's `ResumeOnWake`.
// When it is set the sequence lifts its owned entry on every path after the wake: a failed write of the resuming or done phase, a reconcile that stops and a notice that is never delivered all leave the entry lifted, and only a lift error leaves it, failing the request.
// When it is not set the sequence keeps its owned entry live on every path after the wake, a failure included, and the request ends with the entry still recorded as owned, written back when the request had lost its record, so the hub stays paused until `lym tender resume` or `lym hibernate cancel` lifts it.
// The reconcile, the pair pause entries and the notice run either way.
// After the move to resuming the sequence reconciles the hub with origin, through the `Reconcile` seam, before it lifts anything.
// The engine imports no fabric package: `WakeReconcile` is its own view of the report, with whether origin was unreachable, the stop on a branch every pair shares, one `WakeRun` per stopped, ended or owned-elsewhere entry and one rendered line per mutation.
// A hub entry the request does not own that is live skips the reconcile and the exemption, only an owned entry is lifted or kept, and the notice names the standing entry, `lym tender resume`, and that `lym beam start` or `lym cleanup --apply` runs the reconcile after that lift.
// Otherwise the owned entry, when live, is exempted from the network gate through `ExemptNetwork` until the reconcile returns, so only the reconcile's own network calls pass and every other process and entry stays held; with no hub entry live the reconcile runs with nothing to exempt.
// The sequence has no envelope, so each mutation line is logged at Info and never enters the notice.
// Unreachable through the retry bound, or a reconcile error, moves nothing further, and the notice names the cause and `lym cleanup --apply`.
// Otherwise each stopped run, and each ended run still busy, gets a plain pair pause entry through `Pause`, not recorded as owned so it outlives the request, before the owned entry is lifted or kept.
// A stop on a shared branch is no exception: its stopped runs are held the same way, and the notice names the stop before the runs.
// A pair whose pause fails is left unheld and named with `lym tender pause <slug>`.
// A run whose `WakeRun.Owner` is set is owned by another machine: its pair pause carries the owner caller, so a slug entry already standing adds none, and the notice names the owner and `lym adopt <subject>` from this machine's prime, the subject being `WakeRun.AdoptSubject`, the beam or `root`, and the slug when it is empty.
// Once an owner pause is added, `Deps.EndOwned` ends the run's owned processes on this machine; the hub's wiring fills it with `tendercli.EndOwnedElsewhere`, which spares a Finalize past its parent-side merge and the calling process and its ancestors.
// An end error is logged and leaves the notice as it was, and a nil `EndOwned` skips the end.
// A failed owner pause is named with "the next wake or `lym beam start` pauses it again", never with `lym tender pause`, whose entry would carry no owner label.
// The notice names per stopped run its way forward then `lym tender resume <slug>`, only `lym tender resume <slug>` for a pair busy on a parked push, and `lym cleanup --apply` in place of the resume for a pair stopped as gone; a busy ended run is named with its landing or removal and `lym cleanup --apply`, and the other ended runs are listed.
// A `TenderStatus` error before the reconcile fails the request.
// Otherwise the owned entry of a request that resumes on wake is lifted by identity, so an entry already resumed, with another hub entry added meanwhile, lifts nothing, and the request ends done with one notice saying what became of the pause.
//
// # Notices
//
// Every notice is one line, queued through `Notify`, the hub's orch being its receiver.
// A line over one kilobyte, or one spanning lines, is written whole to `Deps.NoticePath`, and a short line saying what became of the pause and naming that file is queued in its place, since creel refuses a line over its hook delivery limit.
// A refused notice is queued again after a wait that starts at one second and doubles to thirty seconds, until the waits add up to ten minutes, since right after a wake the orch's session may not be listed yet; the last refusal is returned.
// A hub that records no orch strand has no receiver, so `Notify` answers nil without queueing and the notice is not retried; the wiring logs the dropped notice at Warn.
package hibernateengine
