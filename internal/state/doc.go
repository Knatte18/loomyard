// Package state provides generic locked typed JSON I/O for persistent state.
// A locked-JSON read-modify-write needs one lock that spans both the read and the write.
// Reading with ReadJSON, mutating in the caller's own frame and writing with WriteJSON releases the lock in between.
// A concurrent writer can land its own value in that gap,
// and the later write then composes its payload from a base that writer already superseded, silently clobbering it.
// UpdateJSON closes that window:
// it acquires lockPath exactly once and holds it across the read, the caller's mutate and the write.
//
// UpdateJSON cannot be composed from ReadJSON followed by WriteJSON.
// Both acquire lockPath internally, so nesting one inside a lock the caller already holds on the same path hangs.
// ReadJSON and WriteJSON therefore sit on lock-free cores, readJSONUnlocked and writeJSONUnlocked.
// UpdateJSON uses those cores directly under its own single lock.
//
// Not every locked-JSON read-modify-write goes through UpdateJSON, and no PATTERN entry asserts that one must.
//
// UpdateJSON's guarantee runs in one direction only.
// It serialises its caller against every other write to the file,
// so that caller can never compose its payload from a base another writer has already superseded.
// It cannot serialise a different writer that computes its own payload outside any lock and then writes under one;
// that writer still loses whatever landed on the file while it was computing.
// Closing that second direction requires the other writer to route through UpdateJSON as well.
//
// # Unknown keys
//
// No read refuses a field its type does not declare.
// ReadJSON and the read inside UpdateJSON decode with a plain unmarshal.
// Every write keeps each key of the file it replaces that the record's Go type does not declare,
// so a record a newer binary wrote loses nothing when an older one rewrites it.
// WriteJSON and UpdateJSON do this under the write lock they already hold.
// They read the file they replace and merge it into the new encoding.
// A kept key lands after the declared ones, in the old file's order, with its raw value, so its exact digits survive.
// A struct merges key by key, matching declared names as encoding/json does, case-insensitively;
// promoted fields of embedded structs count as declared.
// A pointer merges through to its element.
// A map merges the values under keys both files hold and adds no key the new map lacks.
// A slice or array element merges with the old element at its index only when both decode to the same value,
// so an element updated in place, removed or moved loses its unknown keys.
// An untouched or appended neighbour keeps its own.
// A type that encodes itself is opaque and is written whole:
// a json.Marshaler or json.Unmarshaler, json.RawMessage, an interface, or a scalar.
// An absent file, or one that does not parse, adds nothing.
//
// Only keys already in the file being replaced pass through.
// Each write costs one extra read of that file under the held lock.
//
// # Reads create nothing
//
// Only writes create directories.
// WriteJSON, WriteFile and UpdateJSON make the record's directory before they lock, and ReadJSON makes none.
// A reader polling a tree another process is still building never leaves a directory that process then refuses.
package state
