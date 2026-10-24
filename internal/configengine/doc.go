// Package configengine loads, resolves and writes a module's YAML configuration under a base directory's `_lyx/config/`.
//
// [Load] is strict: an absent base layer is an error.
// [LoadOrTemplate] degrades to the caller's embedded template on proven absence of the base layer's `_lyx/` or file, and is otherwise identical.
// `PATTERN-config-layers` decides which of the two a caller adopts.
//
// Both resolve a layer stack, fill, in memory, the template keys the merged stack lacks and log each fill with the layer `template`, except under a hub's sparse base, whose fill is not logged;
// neither writes a file.
// A key missing inside a list element is the one gap fill cannot close, and is an error.
// The result is resolved through envsource's markers once, after the merge.
// A default- or worktree-layer file holding units the template lacks, open-map entries and retired keys aside, still loads, and is logged at Warn naming the file, the keys and the `lyx config reconcile` that removes them;
// the warning is logged once per file and key list in a process.
// A loader whose module lists entries that may match nothing, such as `lenses`' `remove`, reports the unmatched ones through [WarnSkippedEntries]:
// one Warn naming the highest layer file holding the key, once per file, key and entry list in a process.
//
// A present file whose content fails to parse, lacks a key inside a list element or fails to resolve returns an error marked [ErrInvalid];
// an absent base file or `_lyx/`, an unreadable file and a base directory failure stay unmarked.
// A caller marks its own unmarshal and validation failures with [MarkInvalid], which leaves the error text unchanged.
//
// [Set] writes dotted key=value pairs into a module's file with no editor and no validation loop, scaffolding a standalone or seed-only base from the template when absent and writing a hub's base sparse.
// For a spec with `JSONEntries` it writes each pair as one whole top-level entry from a JSON object instead, with no template key check.
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
// A hub-wide module with a [CategoryMachine] key also stacks, between the hub file and the local file, the untracked machine override `<Board>/.lyx/config/<module>.machine.yaml`, [LayerMachine]:
// the stack is then template, hub file, machine, local, so a local value wins over a machine one.
// The machine layer applies only when `Dirs` carries both `Anchor` and `Board`, so a load that tells `Board` alone, as a hub-file check does, stacks none, and a standalone load never reads one.
// It applies whether or not the anchor has a `_lyx/`.
// No reconcile reads, rewrites or removes the machine file.
// With `Board` empty every module stacks the template, the worktree file and the local file; with `Anchor` empty there is no worktree or local layer.
// [LayerFile] returns a layer's file and whether it applies, [LocalFile] the local one and [MachineFile] the machine one, neither ever the editor staging copy's path.
//
// The base layer ([BaseLayer]) is the lowest file layer, the repo default or hub file when `Board` is set, else the worktree file.
// In a hub it is sparse over the template: a key-less base holds [SparseBaseHeader] and loads as a present base, its fill is not logged, and [Set] and [Edit] write it sparse, scaffolding an absent one through [BaseScaffold].
// Only a [Spec] with `SeedOnly`, the operator's registry seeded whole and never rewritten, and a standalone worktree base stay full.
// Every layer above the base is sparse.
// Strictness applies to the base layer: [Load] errors on an absent base file with the way forward for its kind, [LoadOrTemplate] takes the template as the base and still stacks the upper layers present.
// An absent upper file adds nothing, and so does an empty or comment-only local file.
// A worktree with no `_lyx/` beside a told `Board` has no worktree or local layer; the board dir's base is not uninitialised for it.
//
// [Layers] is the one reader of layer files, checking a machine file with [ValidateMachine] and a local one with [ValidateLocal], and [StackLayers] the one place layers are merged:
// the present files merge unexpanded in stack order under the spec's open maps, a list, scalar or open map from a higher layer replacing the lower one whole.
// A [Stack] also names, in `Origins`, the highest layer holding each top-level key.
// The merged document is expanded once, against `Board` for a hub-wide module and `Anchor` otherwise.
//
// [Provenance] returns the resolved document of [Load], with strictness and refusals unchanged, each unit's line comment naming the highest layer whose file holds it, or `template` when none does;
// a unit no file holds that the picked overlay document holds is labelled `template:<value>`.
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
// # The machine rule
//
// The machine layer holds only keys classified [CategoryMachine], a key whose value is a property of the machine, bounding a resource every worktree of the hub on that machine shares.
// [MachineKeys] lists them, and the local layer keeps refusing them, since [CategoryMachine] is not [CategoryNone].
// [ValidateMachine] checks a machine document with the units, template-existence and shape checks of [ValidateLocal], and admits a unit only when the longest covering `Categories` entry, or the covering retired key, is [CategoryMachine].
// Every refusal is an [ErrInvalid] error naming the key and a way forward, and [Layers] wraps it with the machine file's path.
// An empty or comment-only machine file is dropped as an empty local file is.
// [Set] and [Edit] check a machine write with [ValidateMachine] as they check a local one, so a refused key writes nothing.
// A retired key recorded [CategoryMachine] is admitted and dropped from the stacked document without a warning.
//
// # Open maps
//
// [Set] takes a trailing variadic openMaps, and a [Spec] carries `OpenMaps`: the dotted paths of the module's open maps, mappings whose keys the user owns.
// They are passed unchanged to yamlengine, which defines what a declared path changes;
// with none declared, every caller behaves as before.
//
// # Template overlay
//
// A [Spec] may carry an [Overlay]: a `Selector`, the name of a top-level template key, and `Documents`, each selector value mapped to an overlay document.
// The selector's scalar value in the base layer's file picks one document, and the effective template is the spec's template with that document merged over it under the spec's open maps, so an overlay name under a per-name map replaces or adds one entry whole.
// An absent selector or an empty value picks none, and a spec with no overlay is unchanged.
// [EffectiveTemplate] composes it from a base document and [TemplateUnder] reads the base layer's file and does the same, so a caller judging a file above the base composes nothing itself.
// Every judging path uses the effective template: the load's unknown-key warning, template fill and missing-key check, [BelowLayer], and [Provenance].
// A value no document names, or a non-scalar value, is an [ErrInvalid] error naming the shipped values and the `lyx config` command that sets the selector in the base.
// Only the base layer picks: in a hub a worktree file holding the selector with a value other than the default file's is refused when read, naming the line to delete and the `--layer default` write from the prime worktree.
// A worktree value equal to the base's changes no selection and loads.
//
// The write side follows the same rule.
// [BaseScaffold] returns the template with every per-name map emptied for a spec with a selector, so no scaffolded entry pins itself over the picked layer.
// [Set] judges a base-layer write against the template its own document picks.
// The selector's value is the last pair setting it, else the one the file holds.
// A write to a layer above the base is judged against [TemplateUnder], and an unknown value fails before anything is written.
// [Set] and [Edit] refuse a hub worktree write or save that leaves the selector holding a value other than the default file's, writing nothing or putting the file back.
// The refusal names the line to delete when the file held the selector before.
//
// # Per-name maps
//
// A [Spec] also carries `PerNameMaps`: the dotted paths of the module's per-name maps, mappings of names the operator owns.
// The unknown-keys warning and [Set], on the base and the sparse layers, pass them unchanged to yamlengine, which defines what a declared path changes;
// so a name under a per-name map that the template lacks is never warned as a key no loader reads, and `--set` knows `P.<name>` for any name.
// It also knows `P.<name>.<key>` where the map's entries are mappings and some template entry holds the key.
// The layer merge and the template fill need no declaration: a layer's names add to or override the template's by name, and a mapping entry merges key by key.
// The load's log of filled keys leaves out a name under a per-name map, since a base file holds only the names it tuned.
// The local and machine layers judge a path under a per-name map against the template entry holding the rest of it, whatever the name.
package configengine
