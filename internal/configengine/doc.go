// Package configengine loads, resolves and writes a module's YAML configuration under a base directory's `_lyx/config/`.
//
// [Load] is strict: an absent base layer is an error.
// [LoadOrTemplate] degrades to the caller's embedded template on proven absence of the base layer's `_lyx/` or file, and is otherwise identical.
// `PATTERN-config-strictness` decides which of the two a caller adopts.
//
// Both resolve a layer stack, fill, in memory, the template keys the merged stack lacks and log each fill with the layer `template`;
// neither writes a file.
// A key missing inside a list element is the one gap fill cannot close, and is an error.
// The result is resolved through envsource's markers once, after the merge.
// A default- or worktree-layer file holding units the template lacks, open-map entries and retired keys aside, still loads, and is logged at Warn naming the file, the keys and the `lyx config reconcile` that removes them;
// the warning is logged once per file and key list in a process.
//
// A present file whose content fails to parse, lacks a key inside a list element or fails to resolve returns an error marked [ErrInvalid];
// an absent base file or `_lyx/`, an unreadable file and a base directory failure stay unmarked.
// A caller marks its own unmarshal and validation failures with [MarkInvalid], which leaves the error text unchanged.
//
// [Set] writes dotted key=value pairs into a module's file, scaffolding it from the template when absent, with no editor and no validation loop.
// [Edit] is the interactive counterpart.
// An editor that leaves the file empty or whitespace-only aborts the edit with [ErrAborted] before the YAML parse:
// a scaffolded file is removed and a pre-existing one is written back to the bytes it held on entry, so a layer cannot be emptied through the editor.
// A comment-only result is not empty and saves.
// Its default editor is `$VISUAL`, then `$EDITOR`, then `code --wait` when `code` is on PATH, then `notepad` on Windows, and elsewhere `nano` when it is on PATH, else `vi`.
//
// # Layers
//
// [Dirs] tells the geometry: `Anchor`, the reading worktree's anchor, and `Board`, the hub's board dir, empty standalone.
// Both are told, never derived.
// A [Spec] describes one module: its name, embedded template, open maps, whether it is hub-wide, whether each top-level key is one unit, and its per-key [Category] classification.
//
// A per-worktree module stacks, lowest first:
//
//   - the embedded template, [LayerTemplate], which has no file;
//   - the repo default, `<Board>/_lyx/config/<module>.yaml`, [LayerDefault];
//   - the worktree's own tracked file, `<Anchor>/_lyx/config/<module>.yaml`, [LayerWorktree];
//   - the untracked local override, `<Anchor>/.lyx/config/<module>.local.yaml`, [LayerLocal].
//
// A hub-wide module stacks the template, the hub file `<Board>/_lyx/config/<module>.yaml` and the local file.
// With `Board` empty every module stacks the template, the worktree file and the local file; with `Anchor` empty there is no worktree or local layer.
// [LayerFile] returns a layer's file and whether it applies, and [LocalFile] the local one, which is never the editor staging copy's path.
//
// The base layer ([BaseLayer]) is the lowest file layer, the repo default or hub file when `Board` is set, else the worktree file, and stays full.
// Every layer above it is sparse.
// Strictness applies to the base layer: [Load] errors on an absent base file with the way forward for its kind, [LoadOrTemplate] takes the template as the base and still stacks the upper layers present.
// An absent upper file adds nothing, and so does an empty or comment-only local file.
// A worktree with no `_lyx/` beside a told `Board` has no worktree or local layer; the board dir's base is not uninitialised for it.
//
// [Layers] is the one reader of layer files, and [StackLayers] the one place layers are merged:
// the present files merge unexpanded in stack order under the spec's open maps, a list, scalar or open map from a higher layer replacing the lower one whole.
// A [Stack] also names, in `Origins`, the highest layer holding each top-level key.
// The merged document is expanded once, against `Board` for a hub-wide module and `Anchor` otherwise.
//
// [Provenance] returns the resolved document of [Load], with strictness and refusals unchanged, each unit's line comment naming the highest layer whose file holds it, or `template` when none does.
// It is what `lyx config <module> --resolved` prints.
//
// # The local rule
//
// The local layer holds only keys classified [CategoryNone].
// [ValidateLocal] checks a local document: every unit path must exist in the template, or be a top-level key when the spec holds whole entries;
// its shape must match the template's;
// and the longest `Categories` entry covering it must be [CategoryNone], a hub-wide module also refusing a path no entry covers.
// [LocalKeys] and [LocalDenied] list the classification's two sides.
// A local file failing the rule is an [ErrInvalid] error naming the file and the key.
// A [Spec] may also declare `RetiredKeys`: dotted keys the module no longer reads, each mapped to the category it held as a template key.
// A unit a retired key covers is judged by that category alone, the longest covering key deciding, and its value shape is never checked, so [CategoryNone] admits it and any other category refuses it.
// Every load drops the retired keys from the stacked document before it fills the template, so a layer not yet reconciled still loads and no loader reads one.
// No cleanup deletes a local override file.
// Seed-Child removes a local file when it rewrites the pair's local layer to equal the task's run settings.
// Reconcile removes a local file whose retired keys it pruned to nothing.
// Those are the only removals.
//
// # Open maps
//
// [Set] takes a trailing variadic openMaps, and a [Spec] carries `OpenMaps`: the dotted paths of the module's open maps, mappings whose keys the user owns.
// They are passed unchanged to yamlengine, which defines what a declared path changes;
// with none declared, every caller behaves as before.
package configengine
