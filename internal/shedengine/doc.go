// Package shedengine is a generic outer phase-FSM: it walks one flat, ordered list of producers,
// with no predefined slots, honoring resume, crash-recovery, and pause uniformly at producer
// granularity.
// What makes a product a product -- loom, or the eventual Hardener -- is purely which producers are
// in its list and in what order; Shed itself has no opinion on that list's contents.
//
// # What Shed is
//
// Shed has no predefined slots at all -- no Preflight-slot, no Producer-slot, no shared Finalize.
// It is a generic engine that walks one ordered, flat list of producers, honoring resume,
// crash-recovery, and pause uniformly across every entry regardless of which producer occupies it.
// Everything that used to look "special" -- Preflight, Finalize, review gates -- is just a producer
// like any other in the list.
// A product built on Shed is nothing more than Shed plus that product's own producer list: what
// makes loom "loom" versus Hardener "Hardener" is purely which producers are in the list and in what
// order, configuration rather than architecture.
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
// # Divergence from loom's status schema
//
// internal/loomengine's status type and contracts/specs/loom-status-spec.md pin a different shape
// (phase/stage/history entries of {phase, outcome, bounced_to, ts}) from this package's
// (current_producer/state/activity/history entries of {producer, outcome, output, at}), and this
// package deliberately defines its own rather than reconciling them.
// The opaque product passthrough field carries no compatibility claim for loom's schema -- a
// Shed-written file would still fail loom's coherence check, since phase, stage, and narration are
// top-level fields there.
// Reconciling the two shapes is loom's own later rewiring work.
package shedengine
