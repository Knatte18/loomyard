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
// budget exhaustion persists a reason that starts with ReasonBounceBudgetExhausted, regardless of the producer's Reason.
// An Awaiting outcome is the planned human hand-off and routes nowhere: the run halts in state
// "awaiting" with the producer's Reason as the error, no bounce budget is consulted or spent, and a
// resume re-calls the same producer, exactly as it does after a blocked halt.
// The persisted state vocabulary is running, paused, done, blocked, failed and awaiting.
// A Done outcome routes via OnDone the same way: "" finishes the whole run from any list position
// (state: "done"), and a non-empty value jumps to the Name it names with no positional fallback of
// any kind -- an omitted OnDone is indistinguishable from an intended terminal one and ends the run
// quietly, so a caller assembling a producer list is responsible for asserting its own routing table
// exhaustively rather than relying on Shed to catch a missing entry.
// The bounce budget backing OnStuck is per-producer and episode-scoped: it is counted from the
// persisted history[] rather than held in memory, as the number of Stuck entries a producer has
// authored since its own most recent Done entry, a Done by any producer sharing its non-empty
// Segment, or a goto into its segment (all of them, if none exists), so the count spans
// invocations, crashes, and human resumes rather than resetting on every new Run call.
// See this package's own routing and bounce-budget documentation for the full design and its
// rationale; this package documentation states the contract, not the argument for it.
//
// # goto: a history-only outcome that moves a halted run
//
// Goto moves a halted run onto a named row and appends an entry whose outcome is OutcomeGoto.
// That outcome is history-only: no producer returns it, and it is never a Call verdict.
// The entry leaves the run paused, never running, so the next Run reads as an ordinary resume.
// It also sets an episode boundary for the target's segment:
// episodeStuckCount stops counting Stuck entries at a goto whose target shares the producer's Segment (or is the producer itself when it has no Segment),
// so the moved-onto segment starts with a fresh bounce budget.
// A goto whose target is missing from the producer list ends no episode.
//
// Goto only moves a halted run back.
// It refuses a running run, because the run lock is free between a step-driven driver's steps and nothing else tells a live driver from a crashed one.
// The reference row is the row current_producer names, or, when that names no row, the producer of the latest history entry that does, whose routed row (OnDone after done, OnStuck after stuck) is admitted too.
// A target is admitted when it sits at or before the reference row in the producer list;
// an awaiting run admits only rows strictly before it, since moving onto or past a hand-off would bypass it.
// Every refusal leaves the status file unchanged.
//
// Two stops name goto as their way forward in a trailing `way forward:` clause:
// the missing-producer refusal, which lists every valid producer name, and the budget-exhausted reason, whose ReasonBounceBudgetExhausted stays its exact prefix.
// Every refusal's way forward is tabulated in contracts/specs/refusal-spec.md, which this documentation links rather than restates.
//
// # Told, never derived
//
// Shed is told StatusPath, LockPath, and StatusLockPath and derives none of them; it resolves no cwd
// and names no durable/ephemeral directory convention.
// The caller is responsible for supplying paths that already obey the Durable-vs-Ephemeral State
// Invariant in CONSTRAINTS.md -- the status file durable, both locks never-tracked transients --
// because Shed cannot and does not choose either location.
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
// # loom's status.json is one instance of this shape
//
// internal/loomengine's own status type carries only loom's three fields -- slug, parent,
// start_sha -- inside the opaque Product passthrough field; every other field
// (current_producer/state/error/pause_requested/activity/history) is this package's own shape,
// documented above.
// See contracts/specs/loom-status-spec.md for loom's own half of the schema and the additional
// coherence rules loom's check 4 layers over this package's shell.
package shedengine
