// Package hibernateengine implements the hibernate request file and the sequences behind `lym hibernate`:
// submit, cancel and status, and the Carrier the prime's dobby carries the request out with.
// Every effect on the outside goes through a seam.
//
// A hibernate request asks lym's own Go code to hibernate the machine once every session in the hub is safely paused.
// No LLM session runs the hibernate, so none is cut mid-call.
// Deps supplies every path and every probe; the engine resolves no working directory and spawns nothing itself.
// It reaches tender only through the Pause, TenderStatus and Lift seams.
// It reaches origin only through the Reconcile and ExemptNetwork seams.
//
// # The request file
//
// The hub holds one request, a JSON file at `Deps.RequestPath` with its lock at the same path plus `.lock`.
// The phases are waiting, pausing, confirming, hibernating, resuming, done, failed and cancelled;
// the last three are terminal.
// Every write holds the request lock across its read and write and keeps keys a newer binary added;
// a read takes no lock and a missing file reads as no request.
// A carrying daemon is live when its pid is running with the recorded start time, so a recycled pid is not taken for it.
//
// # Callers and platform
//
// An empty strand is an external caller, and the orch's strand is the orch.
// Any other strand, a name that does not parse included, is an agent.
// Submit and Cancel refuse an agent, naming the parent session that runs the verb for it.
// Bound: the refusal guards against a mistaken call; the agents' settings deny any command naming the strand variable.
// Submit and Cancel also refuse any platform but Linux; Status reads on any platform.
//
// # Submit
//
// Submit returns at once; the request waits on disk until the prime's dobby takes it up.
// Each refusal writes nothing.
// A pending request is refused, whether or not a live daemon carries it.
// A terminal request and no request are replaced.
// The exception is a terminal request whose owned pause entry is still live:
// it is refused, so a pause its wake kept is not forgotten.
// Of two racing submits one is refused.
// The request records the caller's `ResumeOnWake` as told, since the engine reads no config.
//
// # Cancel
//
// Cancel ends the pending request: no request and a terminal one succeed with nothing cancelled.
// A request a live daemon carries in the hibernating phase is refused, since the machine is going down.
// A request no live daemon carries is cancelled in any phase, the way out after a crash or a cold boot.
// Cancel stops no process; it lifts the pause entry the request owns, by identity.
// A request that owns no entry lifts nothing, so a hub pause that stood before the request stays.
// The carrying dobby's sequence sees the cancel at its next read of the request and ends quietly.
// Bound: a dobby that ends between its pause's record write and the write of its owned entry leaves that entry live;
// `lym tender resume` lifts it.
//
// # The carrier
//
// Each `Carrier.Tick` with no sequence of its own in flight takes up the pending request under the request lock.
// A request no live daemon carries, or one that names the running process itself, is pending.
// A request in the pausing or confirming phase goes back to waiting with its owned entry lifted.
// A request taken up in the hibernating or resuming phase ends failed at once with one notice.
// The sequence runs in a goroutine of its own, so the dobby's ticks never wait on it.
// `Carrier.HoldsReexec` reports a sequence in flight past its trigger wait,
// so a binary re-exec never cuts a pause, a confirmation, a hibernate or a wake in the middle.
// Bound: a re-exec that passed the check as the sequence moved past waiting cuts it,
// and the takeover sends it back to waiting.
//
// # The sequence
//
// The sequence acts only while the request file holds a non-terminal request carrying the running process's identity.
// It stops quietly when the request is no longer carried here or has gone terminal.
// It sends no key and no text of its own: every interrupt and resume line is tender's, under its guards.
//
// The trigger wait writes the reason it still waits into the request, so status shows it.
// Bound: the wait has no deadline, since the operator cancels a request whose run halted.
//
// Firing pauses the hub and confirms the pause through `PauseAndConfirm`, which the hub stop also uses.
// The pause's owned entry is recorded in the request before the grace.
// A pause that found a live hub entry owns none, so neither the sequence nor Cancel lifts that pause.
// A pause lifted during its grace cancels the request; a pause error fails it.
//
// Confirmation needs a live hub entry, no network operation in flight and the land lock free.
// Every session must read idle, asking, dead or busy only on background shells.
// A session with an outstanding background fork withholds.
// The orch's own session is left out of the reading when the orch is the requester.
// Without a confirmed reading the request fails and `Hibernate` is never called.
//
// What the wake does with the owned entry is the request's `ResumeOnWake`.
// When it is set the sequence lifts the owned entry on every path after the wake, except a lift error.
// When it is not set the owned entry stays live on every path after the wake, a failure included,
// and the hub stays paused until `lym tender resume` or `lym hibernate cancel` lifts it.
// After the wake the sequence reconciles the hub with origin through the `Reconcile` seam before it lifts anything.
// The engine imports no fabric package: `WakeReconcile` is its own view of the report.
// Each stopped run, and each ended run still busy, gets a plain pair pause entry that outlives the request.
// A run whose `WakeRun.Owner` is set is owned by another machine and gets none.
// The request ends done with one notice saying what became of the pause.
//
// # Notices
//
// Every notice is one line, queued through `Notify`, the hub's orch being its receiver.
// A line over one kilobyte, or one spanning lines, is written whole to `Deps.NoticePath`,
// and a short line naming that file is queued in its place.
// A refused notice is retried with a growing wait, since the orch's session may not be listed right after a wake.
// A hub that records no orch strand has no receiver, so `Notify` answers nil without queueing.
package hibernateengine
