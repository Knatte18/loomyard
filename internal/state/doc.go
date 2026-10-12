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
// No PATTERN entry asserts universal use of this primitive, because an invariant
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
//
// # Unknown keys
//
// No read refuses a field its type does not declare: ReadJSON, ReadJSONStrict and UpdateJSON's own read all decode with a plain unmarshal.
// Every write keeps, in the file it replaces, each key the record's Go type does not declare, so a record a newer binary wrote loses nothing when an older one rewrites it.
// WriteJSON and UpdateJSON do this under the write lock they already hold, by reading the file they replace and merging it into the new encoding.
// A kept key lands after the declared ones, in the old file's order, with its raw value, so its exact digits survive.
// A struct merges key by key, matching declared names as encoding/json does, case-insensitively, and promoted fields of embedded structs count as declared.
// A pointer merges through to its element, and a map merges the values under keys both files hold, adding no key the new map lacks.
// A slice or array element merges with the old element at its index only when both decode to the same value, so an element updated in place, removed or moved loses its unknown keys, and an untouched or appended neighbour keeps its own.
// A type that encodes itself (a json.Marshaler or json.Unmarshaler, json.RawMessage, an interface, or a scalar) is opaque and is written whole.
// An absent file, or one that does not parse, adds nothing.
//
// The bound: only keys already in the file being replaced pass through, and each write costs one extra read of that file under the held lock.
package state
