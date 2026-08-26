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
// A Done outcome routes via OnDone the same way: "" finishes the whole run from any list position
// (state: "done"), and a non-empty value jumps to the Name it names with no positional fallback of
// any kind -- an omitted OnDone is indistinguishable from an intended terminal one and ends the run
// quietly, so a caller assembling a producer list is responsible for asserting its own routing table
// exhaustively rather than relying on Shed to catch a missing entry.
// The bounce budget backing OnStuck is per-producer and episode-scoped: it is counted from the
// persisted history[] rather than held in memory, as the number of Stuck entries a producer has
// authored since its own most recent Done entry (all of them, if it has never returned Done), so the
// count spans invocations, crashes, and human resumes rather than resetting on every new Run call.
// See manifest/designs/shed.md's own routing and bounce-budget sections for the full design and its
// rationale; this package documentation states the contract, not the argument for it.
//
// # Told, never derived
//
// Shed is told StatusPath, LockPath, and StatusLockPath and derives none of them; it resolves no cwd
// and names no durable/ephemeral directory convention.
// The caller is responsible for supplying paths that already obey the Durable-vs-Ephemeral State
// Invariant in CONSTRAINTS.md, because Shed cannot and does not choose either location.
//
// # The ShedProducer contract's two caller-side obligations
//
// A ShedProducer implementation binds itself to two obligations Shed cannot enforce mechanically.
// First, Call must return exactly Done or Stuck and nothing else; a third value is an engine-level
// failure, not a producer verdict.
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
