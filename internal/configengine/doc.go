// Package configengine loads, resolves and writes a module's YAML configuration under a base directory's `_lym/config/`.
//
// [Load] is strict: an absent base layer is an error.
// [LoadOrTemplate] degrades to the caller's embedded template on proven absence of the base layer's `_lym/` or file.
// `PATTERN-config-layers` decides which of the two a caller adopts.
//
// Both resolve a layer stack and fill the template keys the merged stack lacks, in memory; neither writes a file.
// A key missing inside a list element is the one gap fill cannot close, and is an error.
// A default- or worktree-layer file holding keys the template lacks still loads, with one Warn naming the file and keys.
// A module's loader reports list entries that match nothing through [WarnSkippedEntries].
//
// A present file that fails to parse or resolve, or lacks a key inside a list element, yields an [ErrInvalid] error.
// An absent base file or `_lym/`, an unreadable file and a base directory failure stay unmarked.
// A caller marks its own unmarshal and validation failures with [MarkInvalid].
//
// [Set] writes dotted key=value pairs into a module's file with no editor and no validation loop.
// It scaffolds an absent standalone or seed-only base from the template and writes a hub's base sparse.
// [Unset] removes dotted keys from a module's file the same way, so their values resolve from the layers below.
// [Edit] is the interactive counterpart.
// An editor that leaves the file empty or whitespace-only aborts the edit with [ErrAborted],
// so a layer cannot be emptied through the editor.
//
// # Layers
//
// [Dirs] tells the geometry: `Anchor`, the reading worktree's anchor, and `Board`, the hub's board dir.
// Both are told, never derived.
// A [Spec] describes one module: its name, embedded template, open maps, per-name maps and overlay;
// whether the module is hub-wide, whether each top-level key is one unit and each key's [Category].
//
// A per-worktree module stacks, lowest first:
//
//   - the embedded template, [LayerTemplate], which has no file;
//   - the repo default, `<Board>/_lym/config/<module>.yaml`, [LayerDefault];
//   - the worktree's own tracked file, `<Anchor>/_lym/config/<module>.yaml`, [LayerWorktree];
//   - the untracked machine override, `<Board>/.lym/config/<module>.machine.yaml`, [LayerMachine];
//   - the untracked local override, `<Anchor>/.lym/config/<module>.local.yaml`, [LayerLocal].
//
// A hub-wide module stacks the template and the hub file `<Board>/_lym/config/<module>.yaml`.
// It stacks the machine file and the local file above them.
// The machine layer applies only to a module with a [CategoryMachine] key.
// It applies only when `Dirs` carries both `Anchor` and `Board`.
// A higher layer's list, scalar or open map replaces the lower one whole.
// With `Board` empty every module stacks the template, the worktree file and the local file;
// with `Anchor` empty there is no worktree or local layer.
// No reconcile reads, rewrites or removes the machine file.
//
// The base layer ([BaseLayer]) is the lowest file layer:
// the repo default or hub file when `Board` is set, else the worktree file.
// In a hub it is sparse over the template, and [Set] and [Edit] scaffold an absent one through [BaseScaffold].
// Only a standalone worktree base and a [Spec] with `SeedOnly`, the operator's registry, stay full.
// Strictness applies to the base layer: [Load] errors on an absent one;
// [LoadOrTemplate] takes the template as the base and still stacks the upper layers present.
//
// [Layers] is the one reader of layer files, and [StackLayers] the one place layers are merged.
// [TopLevelValue] answers the one top-level key the highest present layer under the told dirs sets.
// Told one anchor alone, it reads what that worktree's own files set and no layer below them.
// The merged document is expanded through envsource's markers once.
//
// [Provenance] returns the resolved document of [Load], with strictness and refusals unchanged.
//
// # The local rule
//
// The local layer holds only keys classified [CategoryNone], and the machine layer only [CategoryMachine].
// [ValidateLocal] and [ValidateMachine] check a document:
// every unit path must exist in the template, its shape must match,
// and the longest `Categories` entry covering it must have the layer's category.
// A hub-wide module also refuses a path no entry covers.
// A failing file is an [ErrInvalid] error naming the file, the key and a way forward.
// [Set] and [Edit] apply the same check to a write, and a refused key writes nothing.
// A [Spec]'s `RetiredKeys` map keys the module no longer reads to the category they held;
// such a unit is judged by that category alone, with its value shape unchecked.
// `RetiredWayForward` and `HubOnlyWayForward` texts end the matching refusal.
// In a default or worktree layer a retired key is an unknown key:
// only `lym config reconcile --apply` or [Unset] removes it.
//
// # Open maps
//
// A [Spec] carries `OpenMaps`: the dotted paths of mappings whose keys the user owns.
// They are passed unchanged to yamlengine, which defines what a declared path changes.
// `OpenMapShapes` maps an open-map path to the shape the module decodes there.
// [Set] therefore writes a key inside a shaped open map alone and refuses a path the shape does not reach.
// A key naming an open map that holds a mapping replaces it whole from a YAML flow mapping.
//
// # Template overlay
//
// A spec's [Overlay] names a top-level template `Selector` and maps each selector value to an overlay document.
// The selector's scalar value in the base layer's file picks one document, merged over the template.
// An absent or empty selector picks none.
// [EffectiveTemplate] and [TemplateUnder] compose the result, and every judging path uses it.
// A value no document names, or a non-scalar value, is an [ErrInvalid] error.
// Only the base layer picks:
// in a hub, a worktree file holding a selector value other than the default file's is refused when read or written.
// [BaseScaffold] returns the template with every per-name map emptied,
// so no scaffolded entry pins itself over the picked layer.
//
// # Per-name maps
//
// A [Spec]'s `PerNameMaps` are the dotted paths of mappings of names the operator owns.
// The unknown-keys warning and [Set] pass them unchanged to yamlengine, which defines what a declared path changes.
// The layer merge and the template fill need no declaration.
// The local and machine layers judge a path under a per-name map against the template entry holding the rest of it.
// This holds whatever the name.
package configengine
