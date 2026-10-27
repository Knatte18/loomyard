// Package shedengine is a generic outer phase-FSM: it walks one flat, ordered list of producers,
// with no predefined slots, honoring resume, crash-recovery, and pause uniformly at producer
// granularity.
// What makes a product a product -- loom, or the eventual Hardener -- is purely which producers are
// in its list; Shed itself has no opinion on that list's contents.
// The list's order is display and enumeration order only -- see "Routing: OnDone and OnStuck, no
// positional fallback" below for why order carries no routing meaning of its own.
//
// # What Shed is
//
// Shed has no predefined slots at all -- no Preflight-slot, no Producer-slot, no shared Finalize.
// It is a generic engine that walks one ordered, flat list of producers, honoring resume,
// crash-recovery, and pause uniformly across every entry regardless of which producer occupies it.
// Everything that used to look "special" -- Preflight, Finalize, review gates -- is just a producer
// like any other in the list.
// A product built on Shed is nothing more than Shed plus that product's own producer list: what
// makes loom "loom" versus Hardener "Hardener" is purely which producers are in the list,
// configuration rather than architecture; list order is cosmetic, carrying zero routing meaning of
// its own.
//
// # Routing: OnDone and OnStuck, no positional fallback
//
// Every routing decision is a per-producer field read off the ProducerDef that just ran, never the
// entry's position in Producers.
// A Stuck outcome routes via OnStuck: "" escalates to a human (state: "blocked"), and a non-empty
// value bounces back to the Name it names, forward or backward, budget permitting.
// The OnStuck "" escalation persists the producer's own OutputPointer.Reason as the blocked halt's
// error (one line, falling back to ReasonNoOnStuckTarget when empty);
// budget exhaustion persists a reason that starts with ReasonBounceBudgetExhausted, regardless of the producer's Reason, and whose trailing way forward is the told Shed.BudgetWayForward.
// A producer that finds a precondition the run cannot settle on its own unmet halts with a reason that starts with ReasonPreconditionUnmet and ends in its own way forward; Shed itself never writes that prefix.
// The told Shed.LockedCheck runs under the run lock in Run and Step, before the status file is read, and a refusal returns an error wrapping ErrLockedCheckRefused with the status file untouched.
// An Awaiting outcome is the planned human hand-off and routes nowhere: the run halts in state
// "awaiting" with the producer's Reason as the error, no bounce budget is consulted or spent, and a
// resume re-calls the same producer, exactly as it does after a blocked halt.
// An Awaiting outcome may also carry OutputPointer.ParentNotice, a one-line notice for the run's parent:
// Shed persists it as the status file's optional parent_notice and returns it on StepResult, writes it on no other outcome, and clears it on the next write.
// A Yield outcome is a producer at a wait its own next call re-checks: Shed persists nothing for it (no history entry, no status write, no activity, no CommitStatus call), reads no budget or route, and leaves the run at the same row, running.
// Step reports it with State running, and Run returns RunYielded instead of continuing.
// Only a producer that waits that way returns it, and batten's InnerRun is the only one.
// The persisted state vocabulary is running, paused, done, blocked, failed and awaiting.
// A Done outcome routes via OnDone the same way: "" finishes the whole run from any list position
// (state: "done"), and a non-empty value jumps to the Name it names with no positional fallback of
// any kind -- an omitted OnDone is indistinguishable from an intended terminal one and ends the run
// quietly, so a caller assembling a producer list is responsible for asserting its own routing table
// exhaustively rather than relying on Shed to catch a missing entry.
// The bounce budget backing OnStuck is per-producer and episode-scoped: it is counted from the
// persisted history[] rather than held in memory, as the number of Stuck entries a producer has
// authored since its own most recent Done or skipped entry, a Done by any producer sharing its non-empty
// Segment, or a reroute into its segment (all of them, if none exists), so the count spans
// invocations, crashes, and human resumes rather than resetting on every new Run call.
// A goto does not end an episode: it keeps the spent budget.
// The halt that exhausts the budget marks its Stuck entry budget_capped, and the scan stops at a capped entry without counting it,
// so a resume of the halt (which writes no entry) grants exactly one more budget.
// A capped entry directly followed by a goto entry is counted and the scan goes on, so a goto onto the halted run grants none, and nor does a resume after that goto.
// A Stuck verdict whose OutputPointer.GenerationStart is set is recorded as counted with generation_start, never halts on the budget, and ends the scan right after itself,
// so the count reads 1 once the producer's generation has started, whatever the previous generation spent.
// A history entry records its call's gate count in gate_attempts and its gate spans in gate_spans, both taken from the OutputPointer and omitted when the call recorded none.
// A GateSpan is a plain struct of the gate entry's name, a kind (gate, held or rework) and RFC 3339 start and end times with fractional seconds, so this package imports nothing for it.
// A told Shed.Machine is stamped as machine on each entry the shed appends; batten tells it and loom and darn do not.
// A fold of consecutive identical exempt Stucks appends the folded call's spans to the surviving entry's own, and the spans take no part in deciding whether two entries fold.
// See this package's own routing and bounce-budget documentation for the full design and its
// rationale; this package documentation states the contract, not the argument for it.
//
// # goto: a history-only outcome that moves a halted run
//
// Goto moves a halted run onto a named row and appends an entry whose outcome is OutcomeGoto.
// That outcome is history-only: no producer returns it, and it is never a Call verdict.
// The entry leaves the run paused, never running, so the next Run reads as an ordinary resume.
// It sets no episode boundary: the bounce count of every row reads as it did before the goto,
// so a goto never gives a budget back, and a budget halt is lifted by a resume alone.
//
// Goto only moves a halted run back.
// It refuses a running run, because the run lock is free between a step-driven driver's steps and nothing else tells a live driver from a crashed one.
// The reference row is the row current_producer names, or, when that names no row, the producer of the latest history entry that does, whose routed row (OnDone after done, OnStuck after stuck) is admitted too.
// A target is admitted when it sits at or before the reference row in the producer list;
// an awaiting run admits only rows strictly before it, since moving onto or past a hand-off would bypass it.
// Every refusal leaves the status file unchanged.
// Shed knows no review state, so the recipe owner tells a GotoGuard on GotoRequest.Guard and RelaunchRequest.Guard;
// it runs after those checks and before the write, and a refusal it returns, or an error that stops it deciding, refuses the goto.
// A nil guard admits every target, and Shed never consults it for a target its own checks already refused.
//
// Relaunch is the status write a driver's re-launch performs, beside Goto.
// Under the run lock and one hold of the status lock it refuses a missing file, a done run and a running run, checks a non-empty Target with CheckGotoTarget and then the told guard, then calls the caller's Admit, and writes only if every one passed.
// Its goto differs from Goto's in keeping pause_requested, pause_before and pause_after, so a human's pause survives the re-launch; without a Target, or when Admit reports the target already in effect, it writes the status unchanged.
// The checks run before Admit so a refused target never lets Admit write its side effect.
//
// A busy refusal, and goto's refusal of a running run, name the command that halts the live driver at its next producer boundary: the told Shed.PauseWayForward, or `lym shed pause <run-id> --after <current_producer>` when none is told.
//
// The missing-producer refusal names goto as its way forward in a trailing `way forward:` clause, listing every valid producer name.
// Every refusal names its way forward, as contracts/specs/refusal-spec.md defines one.
//
// # The reroute edge and the halting Stuck
//
// A row may name one OnReroute row, forward or backward and across segments.
// A Stuck verdict whose OutputPointer.Reroute is set takes that edge instead of OnStuck: the run moves to the target and a history entry with outcome OutcomeReroute and the target in routed_to records it.
// A Stuck verdict that takes its row's OnStuck edge records the target in routed_to on its own Stuck entry, so a reader can tell a bounce from a halt; the outcome stays stuck and the bounce count ignores the field.
// A reroute spends no bounce budget, and it ends the episode of every row sharing its target's Segment.
// A row reroutes at most rerouteCap (three) times since its latest Done or its latest reroute-cap halt; the next reroute halts blocked at the row with a budget-exempt Stuck entry marked reroute_capped and a reason starting with `ReasonRerouteCapReached`, whose trailing way forward is the told Shed.RerouteCapWayForward.
// Resuming that halt grants three more reroutes, because the capped entry resets the count.
// A Reroute verdict from a row with no OnReroute is a wiring fault: the run is written failed naming the row.
//
// A Stuck verdict whose OutputPointer.Halt is set blocks the run at its own row with the producer's Reason whatever the row's OnStuck, and is budget-exempt.
// It can only stop a run, never advance or route it, so a row with an OnStuck route can still halt for a human.
// A Stuck that blocks at a row with no OnStuck is budget-exempt as well, since no budget applies there,
// so a re-step that halts the same way again folds into the halt's entry instead of appending one.
// OutcomeGoto stays the goto verb's alone.
//
// # Entry gates
//
// A ProducerDef may carry an EntryGate, a function that answers whether the row runs, the check's text with the values it read, and an evaluation error; nil is an ungated row.
// Shed evaluates a gated row when the run enters it, after the step's pause checks and before the resume write.
// A holding gate records the row in the status file's entered_row and the row is called.
// A gate that does not hold appends a history entry with outcome OutcomeSkipped and the gate's text in reason, and routes by the row's OnDone as a Done does; it spends no bounce budget and ends the row's episode as a Done does.
// The step then enters the row the skip routes to, so consecutive skips chain within one step.
// A pause_before naming a row the skip chain reaches pauses the run before that row's gate is evaluated, or before an ungated row is called, so a setting changed during the pause decides the gate.
// A pause_after naming a skipped row pauses the run at the row the skip routes to, with the reason "paused after <row>, as requested", and clears both conditions as a fired condition does.
// A skipped row with an empty OnDone finishes the run done.
// A chain longer than the producer list is a wiring fault no shipped recipe reaches: the status is written failed naming the rows of the chain, with the way forward of fixing the settings the gates read or moving the run with `lym shed goto`, then resuming.
// An evaluation error leaves the row not entered and writes the status failed before the step returns, so a resume evaluates the gate again.
// A resume or a halt re-calls the entered row without evaluating its gate.
// A gate skips only its own row, and the shed goto verb is the one bypass: Goto names its target as entered, so the target runs without evaluation.
//
// # Row renames
//
// Row names are durable resume identities, so a product that renames a row carries in-flight runs across with RenameProducers.
// It rewrites each retired name in current_producer, entered_row, pause_before, pause_after and every history entry's producer, under the run lock, and leaves a file that names no retired row untouched.
//
// # Field ownership and the stop conditions
//
// current_producer, state, pause_reason, error, activity and history are Shed-owned and rewritten on every persist.
// pause_reason is the why of a paused run: every write that records paused sets it, and every other write clears it.
// A pause request reads "pause requested", a cancelled context "cancelled", a fired condition its condition reason, Goto "goto to <row>" and a Relaunch onto a row "re-launched onto <row>";
// a Relaunch that moves nothing leaves the reason as it stands.
// entered_row is Shed-owned and sits outside that list, as the entry gate defines it: the entry evaluation sets it, every routed write and skip clears it, Goto sets it to its target, a Relaunch onto a target clears it, and the resume and halt writes keep it.
// A status without the field, or naming another row, reads as not entered.
// pause_requested is shared write-to-clear: an outside actor sets it and Shed clears it, exactly once, in the persist that records paused.
// pause_before and pause_after are outside-set, both-cleared: an outside actor records a producer name in either, and Shed clears both together only in the persist that records paused for a fired condition and in Goto's persist; every other write carries them through.
// pause_requested_by, pause_before_by and pause_after_by are outside-set beside the request they accompany, and Shed clears each in the write that clears its request.
// A reason names the label when its request or condition carries one, as "pause requested by <label>", and Goto's names its GotoRequest.By.
// pause_before fires when current_producer equals its name, before that producer is called.
// pause_after fires when the named producer returns a routed running outcome (done to OnDone, a bounced stuck to OnStuck), or its row is skipped by its entry gate, pausing at the routed row.
// A halting outcome wins and leaves pause_after recorded, and so does a producer error.
// The fired step's Reason reads "paused before <target>, as requested" or "paused after <target>, as requested".
//
// # Caller labels
//
// A history entry's by is the caller label of the goto or resume that wrote it, and the status file's pending_by is the label of a step that resumed a halted run, waiting for the next appended entry to carry it.
// Goto writes its GotoRequest.By on the goto entry and clears pending_by; Relaunch clears pending_by and writes no by.
// A step resuming a blocked, failed or awaiting run sets pending_by to Shed.ResumedBy, and RecordResumer does the same for a loop's waiter before its children step; a resume from paused records nothing.
// The first label stands: nothing sets pending_by while one is pending, so the waiter's label survives its loop children, each with its own.
// The next write that appends a history entry hands pending_by to that entry and clears it; a write that appends none carries it through.
// A skip write appends an entry and carries the label through, so the label of a resumed run lands on the next verdict and never on a skipped entry.
// CallerShed is the label of lym's own automatic steps.
// The labels are for reports only: no Go decision reads by, pending_by or ResumedBy, and the refusals of goto and step do not change.
// Shed takes the label as a plain string and imports nothing to classify it.
//
// # Told, never derived
//
// Shed is told StatusPath, LockPath, and StatusLockPath and derives none of them; it resolves no cwd
// and names no durable/ephemeral directory convention.
// The caller is responsible for supplying paths that already obey
// PATTERN-durable-vs-ephemeral-state -- the status file durable, both locks never-tracked transients --
// because Shed cannot and does not choose either location.
// The package imports only the standard library, internal/state and internal/lock, so every path and every recipe behaviour reaches it told.
//
// # The transient mark
//
// The transient mark is declared only here (transient.go), and its class set is closed at that declaration (PATTERN-transient-stop).
// A lower-level package exposes its own classification, and internal/shedtransient alone translates it into the mark.
// The mark is set only at a producer boundary, the Shed.Transient classifier shedbuild.NewShed tells, or at a step bootstrap boundary, a PreStep hook.
// A producer returns a failure shedtransient classifies as a hard error rather than a verdict, so it reaches that classifier,
// and a gate verdict (blocked, awaiting, paused) never carries the mark.
// A mark added at a new site changes what an automated driver re-steps without a human deciding.
//
// # The ShedProducer contract's two caller-side obligations
//
// A ShedProducer implementation binds itself to two obligations Shed cannot enforce mechanically.
// First, Call must return exactly Done, Stuck or Awaiting and nothing else; a fourth value is an
// engine-level failure, not a producer verdict.
// Second, Call must surface context cancellation as a non-nil error, never as Stuck.
// The second obligation cannot be enforced mechanically: a Stuck return with a cancelled context is
// indistinguishable to Shed from a genuine producer verdict, so a producer that reports cancellation
// as Stuck would silently consume bounce budget or escalate to blocked for what was actually an
// operator stop.
//
// # The external-writer lock contract
//
// Shed is not the status file's only writer; any other actor that writes it -- a product's pause
// verb, its spawn-time seeder, anything touching product -- must go through internal/state using the
// same StatusLockPath Shed was told.
// internal/state's lock is advisory and keyed on the caller-supplied lock path, so the read-modify-
// write merge is safe against a concurrent external writer that takes the same lock, and against no
// other -- this merge-safety property is never stated unconditionally.
//
// WriteFailedStop is a Shed write under the run lock, not an external write: it takes the run lock without waiting and writes failed, with the error and the transient class, only into a file that still reads running.
// That is how a failure stop leaves a run non-running while State and Error stay written by Shed alone.
//
// # loom's status.json is one instance of this shape
//
// internal/loomengine's own status type carries only loom's three fields -- slug, parent,
// start_sha -- inside the opaque Product passthrough field; every other field
// (current_producer/state/error/pause_requested/activity/history) is this package's own shape,
// documented above.
// See contracts/specs/loom-status-spec.md for loom's own half of the schema and the additional
// coherence rules loom's check 4 layers over this package's shell.
package shedengine
