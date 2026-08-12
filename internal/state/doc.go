// Package state provides generic locked typed JSON I/O for persistent state, and states the rule
// that governs a locked-JSON read-modify-write: it must hold one lock across both the read and the
// write.
// Reading with ReadJSON, mutating in the caller's own frame, and writing with WriteJSON releases
// the lock between the two — a concurrent writer can land its own value in that gap, and the later
// write then composes its payload from a base that writer already superseded, silently clobbering
// it.
// UpdateJSON is the primitive that closes that window: it acquires lockPath exactly once and holds
// it across the read, the caller's mutate, and the write.
//
// UpdateJSON is not, and cannot be, composed from ReadJSON followed by WriteJSON.
// Both of those exported functions acquire lockPath internally, so nesting one inside a lock the
// caller already holds on the same path hangs rather than failing.
// This is why ReadJSON and WriteJSON are themselves re-expressed on top of lock-free cores
// (readJSONUnlocked, writeJSONUnlocked): UpdateJSON shares those cores directly, taking its own
// single lock around them, rather than calling the locking wrappers at all.
//
// Adoption of UpdateJSON is deliberately at one consumer today — internal/fabricengine's
// correspondence index.
// No CONSTRAINTS.md invariant asserts universal use of this primitive, because an invariant
// claiming every locked-JSON read-modify-write in the codebase goes through UpdateJSON would be
// false on the day it landed.
//
// UpdateJSON's guarantee runs in one direction only, and it is worth stating precisely rather than
// letting a reader assume more.
// It serialises its caller against every other write to the file, so that caller can never compose
// its payload from a base another writer has already superseded.
// It cannot serialise a different writer that computes its own payload outside any lock and then
// writes under one — that writer still loses whatever landed on the file while it was computing,
// exactly as before.
// Closing that second direction requires the other writer to route through UpdateJSON as well;
// nothing here makes a file race-free on its own.
package state
