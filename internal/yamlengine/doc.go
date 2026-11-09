// Package yamlengine reconciles, fills, checks and sets YAML configuration against a template.
// It keeps the template's comments and key order.
//
// [Reconcile] merges a template with a user's existing file: the file's values win,
// template keys the file lacks are reported added,
// file keys the template lacks are reported removed,
// and the result is idempotent.
// [FillMissing] appends only the mapping keys the template holds and the file lacks;
// it never replaces a file value, and it refuses a shape mismatch.
// [MissingKeys] reports the template leaf paths the file lacks.
// It treats a template list as a default, not a minimum length.
// [SetValues] applies dotted key=value pairs to the template-shaped tree and refuses a key the template does not know.
// [SetEntries] sets whole top-level entries from JSON objects, or one value below an entry by a dotted key.
// It serves a module whose top-level keys are names the operator owns.
// [RaiseIntFloor] raises one top-level integer key to a floor, for a module's migration hook.
// It carries every other key and comment through unchanged.
// [RenameKeys] renames retired top-level keys to their new names, for a module's migration hook.
// Each keeps its value and takes the template's line comment for the new name.
// [DropKeys] removes retired top-level keys with their values and comments, for a module's migration hook.
// It reports one line per removed key.
// [FollowTemplate] carries a template change into an operator-owned file unit by unit.
// It works against the template the file last followed:
// a unit left at that template's value takes the new one,
// and a unit the operator set is kept and reported when the template holds it otherwise.
//
// # Layer operations
//
// These operations work on raw unexpanded YAML bytes for the layer stack.
// Each takes openMaps and, where it reads units, a wholeEntries flag;
// [DropTemplateEqualNames] takes the per-name maps alone:
//
//   - [Merge] lays an upper layer over a lower one key by key; no layer deletes a key from the one below.
//   - [Strip] removes from an upper layer every unit equal to the lower layer's, and reports the removed paths.
//   - [SetSparse] writes only the set keys into a sparse layer, never scaffolding from the template.
//   - [Prune] removes from a sparse layer every unit the template does not know, and adds nothing.
//   - [DropTemplateEqualNames] removes from a sparse layer every per-name entry whose value equals the template's.
//     It reports the removed P.<name> paths, adds nothing, and leaves an untouched layer byte for byte.
//   - [LeafPaths] lists the sorted dotted paths of every unit of a document.
//   - [AnnotateLeaves] writes a note as the line comment of the unit at a path.
//
// A unit is a mapping key that is descended into.
// The exceptions are a key at a declared open-map path and, when wholeEntries is true, every top-level key:
// each is carried or compared whole.
// A scalar or a sequence is always one unit.
//
// # Open maps
//
// Reconcile, MissingKeys, FillMissing and SetValues each take a trailing variadic openMaps:
// dotted key paths whose value is an open map, a mapping whose keys the user owns.
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
//     The entry is a string scalar, except that a value whose first non-blank character is { is a YAML flow mapping,
//     and one whose first non-blank character is [ is a YAML flow list;
//     each replaces the entry whole and keeps its own scalar tags.
//     A braced or bracketed value that is not one node of its kind is refused naming the key.
//     So is one that carries text after its closing bracket.
//     A string holding a brace, comma or bracket is double-quoted inside it.
//     A list at P is an error naming P;
//     the list is never converted.
//     Known lists each open map as P and as P.<name>.
//   - SetValues and SetSparse set P itself when P holds a scalar or a list, not a mapping.
//     They replace it whole with the shape the value spells:
//     a value starting with [ is a YAML flow list of scalars or mappings of scalars, and any other value a string.
//     A bracketed value that is not such a list is refused, never written as a string.
//     A module declares a key that takes one value or a list, such as a per-round model list, this way.
//   - SetValues and SetSparse set P itself when P holds a mapping.
//     They replace it whole with the YAML flow mapping the value spells, {} included.
//     A value that is not one flow mapping is refused naming P and the per-key form P.<key>.
//     A shaped P refuses a key inside the mapping its shape does not reach,
//     and it refuses a mapping or list where its shape takes a string, naming it.
//   - SetValues and SetSparse also take a map from an open-map path to its [Shape].
//     [ShapeOf] derives a Shape from the module's decode type.
//     A shaped P takes a dotted key at any path its shape reaches.
//     An example is P.<name>.env.<var> under a mapping of names whose entries hold a map.
//     It sets that one value and creates every mapping on the way:
//     a text value is a string, and any other scalar takes the tag its own text resolves to.
//     A path the shape does not reach is refused naming the key, so a misspelled field never lands as a new entry;
//     an unshaped P keeps the dotted rest as one entry name.
//
// Only declared paths change behaviour.
// The shape check on P belongs to the module that declares it, in its own decode.
//
// # Per-name maps
//
// Reconcile, Prune, DropTemplateEqualNames, SetValues and SetSparse each take a perNameMaps argument.
// It comes before the open maps, and DropTemplateEqualNames takes no open maps.
// A per-name map is a dotted key path whose template value is a mapping of names, each name one unit carried whole.
// Examples are a lens's prose and a fan's list.
// An empty argument keeps every other behaviour byte for byte.
// Merge, Strip, LeafPaths, AnnotateLeaves and FillMissing take no per-name maps.
// They step into P name by name and into each mapping entry key by key.
// For a declared path P:
//
//   - Reconcile writes at P only what the file changed.
//     The load's fill resolves every other template name from the template.
//     The written names are:
//     each template name the file holds with a value other than the template's, whole and in template order;
//     then every name only the file holds, in file order.
//     It leaves out a template name the file holds with the template's value and reports it P.<name> dropped.
//     Equal means the two values decode to the same data.
//     The loaded value is unchanged, so a later template edit to that name reaches the file.
//     A template name the file lacks stays out and is not reported.
//     It writes an absent or null P as an empty mapping and reports P added,
//     and reports P.<name> kept for each name only the file holds.
//     Nothing at or under P is reported removed, and a list under a name is never merged element by element.
//     A file value at P that is neither a mapping nor null is refused.
//     The refusal's way forward is `rewrite P as a map of name to value`.
//   - Prune keeps every name under P whole and reports a name the template lacks as kept, never removed.
//   - SetValues and SetSparse know P.<name> for any one dot-free name.
//     They refuse P itself and a deeper key under P, bar one case below.
//     The pair sets the name to the shape the template's entries at P decide:
//     a string scalar under a mapping of scalars, whatever its first character;
//     a YAML flow list under a mapping of lists; and a YAML flow mapping under a mapping of mappings.
//     Under a mapping of mappings they also know P.<name>.<rest> for any name.
//     Here rest is a scalar or list path some template entry holds.
//     They set that one key under that entry's tag, creating the name's mapping.
//     SetValues also carries every name the file holds under P and the template lacks,
//     and leaves out every name holding the template's value once the pairs apply, as Reconcile does.
//     A pair under a P that holds neither a mapping nor null is refused with the same way forward.
package yamlengine
