// Package hibernateengine implements the hibernate request file and the submit, cancel and status sequences behind `lyx hibernate`, over seams.
//
// A hibernate request asks lyx's own Go code to hibernate the machine once every session in the hub is safely paused, so no LLM session runs the hibernate and none is cut mid-call.
// A detached daemon carries the request out; the verbs here record the request, end it and read it.
//
// Every path and every probe is told through Deps; the engine resolves no working directory, spawns nothing itself and reaches tender only through the Pause, TenderStatus and Lift seams.
//
// # The request file
//
// The hub holds one request, a JSON file at `Deps.RequestPath` with its lock at the same path plus `.lock`.
// It records the trigger (`now`, `after` with a slug, or `after-all`), the requester, the daemon's pid and start time, the pause entry the daemon owns once it has added one, the phase and the last reason.
// The phases are waiting, pausing, confirming, hibernating, resuming, done, failed and cancelled; the last three are terminal.
// Every write holds the request lock across its read and write and keeps keys a newer binary added; a read takes no lock and a missing file reads as no request.
// A daemon is live when its pid is running with the recorded start time, so a recycled pid is not taken for it.
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
// Submit checks the caller, the platform, `systemctl` on PATH, that `--after` and `--after-all` are not both given, and that an `--after` slug has a batten status; each refusal writes nothing and spawns nothing.
// In one hold of the request lock it then refuses a pending request whose daemon is live, lifts the pause entry a pending request with a dead daemon owns, spawns the daemon and writes a waiting request with its pid and start time.
// A terminal request and no request are replaced the same way.
// Because the check, the spawn and the write share one hold, two racing submits spawn one daemon.
// Submit returns once the daemon is spawned; the daemon decides when the trigger holds.
//
// # Cancel
//
// Cancel ends the pending request: no request and a terminal one succeed with nothing cancelled.
// A live daemon in the hibernating phase is refused, since the machine is going down and the daemon lifts its own pause on wake.
// A dead daemon's request is cancelled in any phase, which is the way out after a crash or a cold boot left the hub paused.
// Otherwise the request is written cancelled with the requester as the reason, a live daemon is killed, and the pause entry the request owns is lifted by identity, resume lines included.
// The request is read again after the kill, since the daemon may have recorded its entry between the hold and the kill.
// A request that owns no entry lifts nothing, so a hub pause that stood before the request stays.
// Bound: a daemon killed between its pause's record write and the write of its owned entry leaves that entry live and unowned, which `lyx resume` lifts; the window is one file write.
//
// # Status
//
// Status reads the request and whether its daemon is live and writes nothing, so a halted `--after` run's reason and a dead daemon show up in it.
package hibernateengine
