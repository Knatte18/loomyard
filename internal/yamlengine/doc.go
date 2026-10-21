// Package yamlengine reconciles, fills, checks and sets YAML configuration against a template while keeping the template's comments and key order.
//
// [Reconcile] merges a template with a user's existing file: the file's values win,
// template keys the file lacks are reported added,
// file keys the template lacks are reported removed,
// and the result is idempotent.
// [FillMissing] builds on the existing file instead: it appends only the mapping keys the template holds and the file lacks, never replaces a file value, and refuses a shape mismatch.
// [MissingKeys] reports the template leaf paths the file lacks, treating a template list as a default and not a minimum length.
// [SetValues] applies dotted key=value pairs to the template-shaped tree and refuses a key the template does not know.
// [SetEntries] sets whole top-level entries from JSON objects, for a module whose top-level keys are names the operator owns.
// [RaiseIntFloor] raises one top-level integer key to a floor, carrying every other key and comment through unchanged, for a module's migration hook.
// [RenameKeys] renames retired top-level keys to their new names with their values and the template's line comment for each new name, for a module's migration hook.
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
//   - SetValues and SetSparse know P.<name> for any name, set it in the mapping at P, and append the key when absent.
//     The entry is a string scalar, except that a value whose first non-blank character is { is a YAML flow mapping and one whose first non-blank character is [ is a YAML flow list, each replacing the entry whole and keeping its own scalar tags.
//     A braced or bracketed value that is not one node of its kind, or carries text after its closing bracket, is refused naming the key, and a string holding a brace, comma or bracket is double-quoted inside it.
//     A list at P is an error naming P;
//     the list is never converted.
//     Known lists each open map as P.<name>.
//   - SetValues and SetSparse set P itself when P holds a scalar or a list, not a mapping, replacing it whole with the shape the value spells:
//     a value starting with [ is a YAML flow list of scalars or mappings of scalars, and any other value a string.
//     A bracketed value that is not such a list is refused, never written as a string.
//     A module declares a key that takes one value or a list, such as a per-round model list, this way.
//
// Only declared paths change behaviour.
// The shape check on P belongs to the module that declares it, in its own decode.
//
// # Per-name maps
//
// Reconcile, Prune, SetValues and SetSparse each take a perNameMaps argument before the open maps: dotted key paths whose template value is a mapping of names, each name one unit carried whole, such as a lens's prose or a fan's list.
// An empty argument keeps every other behaviour byte for byte.
// For a declared path P:
//
//   - Reconcile writes at P only what the file changed, since the load's fill resolves every other template name from the template:
//     each template name the file holds with a value other than the template's, whole and in template order, then every name only the file holds in file order.
//     It leaves out a template name the file holds with the template's value and reports it P.<name> dropped, equal meaning the two values decode to the same data;
//     the loaded value is unchanged, so a later template edit to that name reaches the file.
//     A template name the file lacks stays out and is not reported.
//     It writes an absent or null P as an empty mapping and reports P added,
//     and reports P.<name> kept for each name only the file holds.
//     Nothing at or under P is reported removed, and a list under a name is never merged element by element.
//     A file value at P that is neither a mapping nor null is refused with the way forward `rewrite P as a map of name to value`.
//   - Prune keeps every name under P whole and reports a name the template lacks as kept, never removed.
//   - SetValues and SetSparse know P.<name> for any one dot-free name and refuse a deeper key under P and P itself.
//     The pair sets the name to the shape the template's entries at P decide:
//     a string scalar under a mapping of scalars, whatever its first character, and a YAML flow list under a mapping of lists.
//     SetValues also carries every name the file holds under P and the template lacks,
//     and leaves out every name holding the template's value once the pairs apply, as Reconcile does.
//     A pair under a P that holds neither a mapping nor null is refused with the same way forward.
package yamlengine
