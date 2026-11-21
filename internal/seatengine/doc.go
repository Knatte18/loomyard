// Package seatengine runs one step as a table of seats: a chair and its advisors, each a shuttle run in its own strand.
// The chair's turn end, outputs and gate decide the step as a single agent's would;
// the advisors' outputs are the chair's inputs, and the advisors talk to the chair through creel mail.
// The seat-running logic lives here, below internal/shedadapters.
// Its MultiLLM producer maps the chair's result onto the Shed contract.
//
// # The seat model
//
// The first seat is the lead, also called the chair, and it carries no name.
// Every later seat carries a name valid under ValidateSeatName, and its strand role is SeatRole(prefix, name),
// so a spec never spells a role as a literal.
// A table's later seats are all advisors, all writers or all assistants.
// Table.Validate refuses a violated table rule as an error naming the seat;
// Table.CheckMarkers refuses a stencil marker no value fills.
// Validate leaves that out because a resumed seat keeps the prompt it started with.
// ForkSubagents authorizes a seat's session to spawn fork subagents; the engine audits no fork, so the caller owns it.
// A Required advisor has an Accept hook that judges its terminal result, and a step depends on that result.
//
// # Told geometry
//
// The package is told the absolute paths it works on through Geometry and derives none of its own,
// so it imports neither internal/lymcwd nor a geometry constructor.
// It runs in a directory that is not a git repository (PATTERN-cwd-resolution).
//
// # The shuttle seam
//
// Shuttle is the seam the engine starts and probes its seats through.
// It is handle-shaped because the engine holds several live runs at once and sends into the chair's while it waits.
//
// # Running a table fresh
//
// Engine.Run validates the table, then starts the chair first and the advisors in table order.
// The chair holds the gate, the operator's channel and the step's outcome;
// an advisor keeps its pane and holds a turn end quietly.
// A table's Amendment is appended verbatim to the lead's prompt on this fresh path only.
// The chair's held-turn notice to the parent is deferred while advisor work is outstanding,
// so its planned wait on its advisors does not reach the parent as a stall notice.
// The settle interval and the maximum deferral are shuttle's.
// A chair that fails to start starts no advisor and returns the error.
// An advisor that fails to start is recorded on its SeatResult and the step goes on without it.
// A required advisor that fails to start ends the step.
// A seat whose strand took a different name than expected is stopped and reported as a start error.
// The chair is told in one line when each advisor is up or did not start.
//
// # The notice sender
//
// The sender queues one line for the chair when an advisor dies, times out or whose wait errors.
// An advisor that reaches done queues nothing, since it stays reachable and the chair reads its file.
// A required advisor queues no ending, since its end stops the chair.
// A line that never lands is logged rather than failing the step.
//
// # The stop rule
//
// Run waits on the chair, then stops every started advisor, a kept done one included, before it returns.
// A stop that fails is an error wrapping ErrSeatNotStopped, and it is never retried.
// Only a required advisor or a writer can end the step before the chair, through its own end or its own Accept.
// The engine then sets EndedStep on its SeatResult.
// Accept is called once, with the terminal result, for a run that ended done with its gate passed.
// The result carries the chair's outcome, gate and NotStarted as shuttle reported them:
// the engine judges nothing about finishing itself (PATTERN-shuttle-provider-seam).
//
// # Writers
//
// A Seat marked Writer writes a product of its own, which the chair joins.
// An advisor's output is advice the chair weighs.
// A writer is required by kind: every end of its run that its gate and Accept hook do not pass ends the step.
// When a writer ended done with its gate and Accept passed, the chair is told its part stands and it keeps answering.
// A Seat's own Timeout, when non-zero, bounds its run in place of the table's, so a chair can outlast its writers.
//
// # Assistants
//
// A Seat marked Assistant writes its outputs and nothing else:
// it gets no up line and no mail, sends none, and so never needs creel.
// It is never required and never ends the step.
// Each assistant carries an EndMarker, which the engine writes once the assistant's run ends or when it does not start.
// The marker's text names the ending.
// The chair reads an assistant's report only when its marker reads done.
// An assistant that runs subagents lists their report files as SubagentReports; they are no outputs.
// A fresh Run removes every assistant's stale marker, outputs and subagent reports before the chair starts.
// An assistant whose SkipReason is set, whose start fails or whose engine lacks an included engine block does not start;
// its marker reads not started with the reason and the step goes on.
// The engine stops every assistant when the chair ends.
//
// # Probe before archive
//
// Engine.Probe reports which seats have a live run and archives and starts nothing.
// With the chair live, Probe returns its handle and every advisor found.
// With the chair not live, Probe stops every live advisor and returns an empty LiveTable,
// so a caller archives stale outputs and starts fresh only once no seat is live.
// The engine never archives: the caller owns that step, between Probe and Run.
//
// # Resume
//
// Engine.Resume attaches to a live chair and the advisors the probe found, and joins them as a fresh run does;
// an advisor the probe did not find is never restarted.
// A required advisor the probe did not find ends the step at once with EndedStep set and no terminal result.
// A writer the probe did not find counts as finished when all its outputs exist and its Accept hook passes;
// otherwise it ends the step at once.
// A LiveTable built without Probe carries no advisor work.
package seatengine
