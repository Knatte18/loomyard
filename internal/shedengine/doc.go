// Package shedengine is a generic outer phase-FSM that walks one flat, ordered list of producers.
// It honors resume, crash-recovery, and pause uniformly at producer granularity.
// Shed has no predefined slots: Preflight, Finalize, and review gates are producers like any other.
// A product, such as loom, is defined purely by which producers are in its list.
// The list's order is display and enumeration order only; it carries no routing meaning.
//
// # Routing: OnDone and OnStuck, no positional fallback
//
// Every routing decision reads a field of the ProducerDef that just ran, never the entry's position in Producers.
// A Done outcome routes via OnDone: "" finishes the whole run from any position,
// and a non-empty value jumps to the Name it names.
// An omitted OnDone is indistinguishable from an intended terminal one,
// so a caller assembling a producer list asserts its own routing table exhaustively.
// A Stuck outcome routes via OnStuck: "" escalates to a human (state "blocked"),
// and a non-empty value bounces to the Name it names, forward or backward, budget permitting.
// The bounce budget is per-producer and counted from the persisted history[],
// so it spans invocations, crashes, and human resumes.
// A resume of a budget halt grants exactly one more budget; a goto never gives budget back.
// An Awaiting outcome is the planned human hand-off: the run halts in state "awaiting" and spends no budget.
// A Yield outcome is a producer at a wait its own next call re-checks:
// Shed persists nothing, and Run returns RunYielded.
// The persisted states are running, paused, done, blocked, failed and awaiting.
//
// Halt reasons start with ReasonBounceBudgetExhausted, ReasonPreconditionUnmet or ReasonRerouteCapReached.
// Each ends in a way forward.
// Every refusal names its way forward, as contracts/specs/refusal-spec.md defines one.
// The told Shed.LockedCheck runs under the run lock before the status file is read;
// a refusal wraps ErrLockedCheckRefused and leaves the status file untouched.
//
// # goto: a history-only outcome that moves a halted run
//
// Goto moves a halted run onto a named row and appends an entry whose outcome is OutcomeGoto.
// No producer returns that outcome.
// The entry leaves the run paused, so the next Run reads as an ordinary resume,
// and it sets no episode boundary.
// Goto refuses a running run.
// It does so because the run lock is free between a step-driven driver's steps;
// and nothing else tells a live driver from a crashed one.
// A target is admitted when it sits at or before the reference row in the producer list;
// an awaiting run admits only rows strictly before it, since moving onto or past a hand-off would bypass it.
// Every refusal leaves the status file unchanged.
// Shed knows no review state, so the recipe owner tells a GotoGuard on GotoRequest.Guard and RelaunchRequest.Guard;
// a nil guard admits every target.
//
// Relaunch is the status write a driver's re-launch performs.
// Under the run lock it refuses a missing file, a done run and a running run.
// It then checks a non-empty Target with CheckGotoTarget and the told guard, and calls the caller's Admit.
// It writes only if every one passed.
// It keeps a human's pause requests across the re-launch.
//
// # The reroute edge and the halting Stuck
//
// A row may name one OnReroute row, forward or backward and across segments.
// A Stuck verdict whose OutputPointer.Reroute is set takes that edge instead of OnStuck and spends no bounce budget.
// A row reroutes at most `rerouteCap` times between Dones.
// The next reroute halts blocked with a reason starting `ReasonRerouteCapReached`,
// and a resume grants a fresh allowance.
// A Reroute verdict from a row with no OnReroute is a wiring fault: the run is written failed naming the row.
//
// A Stuck verdict whose OutputPointer.Halt is set blocks the run at its own row with the producer's Reason.
// It ignores OnStuck, is budget-exempt, and can only stop a run, never advance or route it.
//
// # Entry gates
//
// A ProducerDef may carry an EntryGate; nil is an ungated row.
// Shed evaluates it when the run enters the row, after the step's pause checks.
// A gate that does not hold appends a history entry with outcome OutcomeSkipped.
// It routes by SkipTo when set, else OnDone, and spends no bounce budget.
// Consecutive skips chain within one step.
// A chain longer than the producer list is a wiring fault: the status is written failed naming the rows.
// An evaluation error leaves the row not entered and writes the status failed, so a resume evaluates the gate again.
// A resume or a halt re-calls the entered row without evaluating its gate.
// A gate skips only its own row, and goto is the one bypass: it names its target as entered.
//
// # Field ownership and the stop conditions
//
// current_producer, state, pause_reason, error, activity and history are Shed-owned and rewritten on every persist.
// An empty current_producer is a run at its start, the first row of the producer list.
// cancel, pause_intent and takeover are outside-owned: Shed never writes them, and a rewrite carries them through.
// pause_requested is set by an outside actor and cleared by Shed, once, in the persist that records paused.
// pause_before and pause_after are outside-set.
// They are cleared together, only in the persist that records paused for a fired condition and in Goto's persist.
// pause_before fires before the named producer is called.
// pause_after fires when the named producer returns a routed running outcome, or its entry gate skips its row.
// A halting outcome or a producer error wins over pause_after and leaves it recorded.
//
// # Caller labels
//
// A history entry's by and the status file's pending_by are caller labels, for reports only.
// No Go decision reads by, pending_by or ResumedBy.
// Shed takes the label as a plain string and imports nothing to classify it.
//
// # Told, never derived
//
// Shed is told StatusPath, LockPath, and StatusLockPath and derives none of them.
// It resolves no cwd and names no durable/ephemeral directory convention.
// The caller supplies paths that already obey PATTERN-lym-files:
// the status file durable, both locks never-tracked transients.
// The package imports only the standard library, internal/state and internal/lock.
//
// # The transient mark
//
// The transient mark is declared only in transient.go, and its class set is closed there (PATTERN-shed-driver).
// It is set only at a producer boundary, the Shed.Transient classifier, or at a PreStep hook.
// A gate verdict (blocked, awaiting, paused) never carries it.
// A mark added at a new site changes what an automated driver re-steps without a human deciding.
//
// # The ShedProducer contract
//
// Two obligations Shed cannot enforce mechanically.
// Call must return exactly Done, Stuck or Awaiting; a fourth value is an engine-level failure.
// Call must surface context cancellation as a non-nil error, never as Stuck.
// This is because Shed cannot tell it from a genuine verdict and would spend bounce budget or block on an operator stop.
//
// # The external-writer lock contract
//
// Shed is not the status file's only writer.
// Any other actor that writes it must go through internal/state with the same StatusLockPath Shed was told.
// Such actors include a product's pause verb or spawn-time seeder.
// The lock is advisory, so the read-modify-write merge is safe only against a writer that takes it.
package shedengine
