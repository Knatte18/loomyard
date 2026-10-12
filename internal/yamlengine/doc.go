// Package yamlengine reconciles, fills, checks and sets YAML configuration against a template while keeping the template's comments and key order.
//
// [Reconcile] merges a template with a user's existing file: the file's values win,
// template keys the file lacks are reported added,
// file keys the template lacks are reported removed,
// and the result is idempotent.
// [FillMissing] builds on the existing file instead: it appends only the mapping keys the template holds and the file lacks, never replaces a file value, and refuses a shape mismatch.
// [MissingKeys] reports the template leaf paths the file lacks, treating a template list as a default and not a minimum length.
// [SetValues] applies dotted key=value pairs to the template-shaped tree and refuses a key the template does not know.
// [RaiseIntFloor] raises one top-level integer key to a floor, carrying every other key and comment through unchanged, for a module's migration hook.
//
// # Layer operations
//
// Six operations work on raw unexpanded YAML bytes for the layer stack, each taking openMaps and, where it reads units, a wholeEntries flag:
//
//   - [Merge] lays an upper layer over a lower one key by key; no layer deletes a key from the one below.
//   - [Strip] removes from an upper layer every unit equal to the lower layer's, and reports the removed paths.
//   - [SetSparse] writes only the set keys into a sparse layer, never scaffolding from the template.
//   - [Prune] removes from a sparse layer every unit the template does not know, and adds nothing.
//   - [LeafPaths] lists the sorted dotted paths of every unit of a document.
//   - [AnnotateLeaves] writes a note as the line comment of the unit at a path.
//
// A unit is a mapping key that is descended into, except a key at a declared open-map path and, when wholeEntries is true, every top-level key, each carried or compared whole.
// A scalar or a sequence is always one unit.
//
// # Open maps
//
// Reconcile, MissingKeys, FillMissing and SetValues each take a trailing variadic openMaps: dotted key paths whose value is an open map, a mapping whose keys the user owns.
// An absent argument keeps exact-key behaviour on every mapping.
// For a declared path P:
//
//   - Reconcile carries the file's value at P whole, whatever its kind,
//     so a legacy list survives,
//     and reports nothing at or under P as added or removed.
//     When the file lacks P, the template's value stays and added reports P itself.
//   - FillMissing skips a key at P that the file holds before any shape check, and appends a missing P whole.
//   - MissingKeys counts a template leaf at or under P as satisfied by the presence of key P.
//   - SetValues knows P.<name> for any name, sets it to the scalar value in the mapping at P, and appends the key when absent.
//     A list at P is an error naming P;
//     the list is never converted.
//     Known lists each open map as P.<name>.
//
// Only declared paths change behaviour.
// The shape check on P belongs to the module that declares it, in its own decode.
package yamlengine
